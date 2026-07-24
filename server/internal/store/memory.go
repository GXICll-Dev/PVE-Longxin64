package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

type idempotencyRecord struct {
	RequestHash string
	OperationID string
}

type lease struct {
	Owner     string
	ExpiresAt time.Time
}

type MemoryRepository struct {
	mu             sync.RWMutex
	classrooms     map[string]domain.Classroom
	operations     map[string]domain.Operation
	idempotency    map[string]idempotencyRecord
	operationLease map[string]lease
	closed         bool
}

func NewMemoryRepository(classrooms []domain.Classroom) *MemoryRepository {
	repository := &MemoryRepository{
		classrooms:     make(map[string]domain.Classroom, len(classrooms)),
		operations:     make(map[string]domain.Operation),
		idempotency:    make(map[string]idempotencyRecord),
		operationLease: make(map[string]lease),
	}
	for _, classroom := range classrooms {
		repository.classrooms[classroom.ID] = cloneClassroom(classroom)
	}
	return repository
}

func NewDevelopmentRepository(now time.Time) *MemoryRepository {
	return NewMemoryRepository(developmentClassrooms(now.UTC()))
}

func (repository *MemoryRepository) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	if repository.closed {
		return fmt.Errorf("memory repository is closed")
	}
	return nil
}

func (repository *MemoryRepository) Close() {
	repository.mu.Lock()
	repository.closed = true
	repository.mu.Unlock()
}

func (repository *MemoryRepository) ListClassrooms(ctx context.Context) ([]domain.ClassroomSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]domain.ClassroomSummary, 0, len(repository.classrooms))
	for _, classroom := range repository.classrooms {
		result = append(result, domain.SummarizeClassroom(classroom))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Site == result[j].Site {
			return result[i].Name < result[j].Name
		}
		return result[i].Site < result[j].Site
	})
	return result, nil
}

func (repository *MemoryRepository) GetClassroom(ctx context.Context, id string) (domain.Classroom, error) {
	if err := ctx.Err(); err != nil {
		return domain.Classroom{}, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	classroom, ok := repository.classrooms[id]
	if !ok {
		return domain.Classroom{}, ErrNotFound
	}
	return cloneClassroom(classroom), nil
}

func (repository *MemoryRepository) ListOperations(ctx context.Context, limit int) ([]domain.Operation, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]domain.Operation, 0, len(repository.operations))
	for _, operation := range repository.operations {
		result = append(result, cloneOperation(operation))
	}
	total := len(result)
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, total, nil
}

func (repository *MemoryRepository) GetOperation(ctx context.Context, id string) (domain.Operation, error) {
	if err := ctx.Err(); err != nil {
		return domain.Operation{}, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	operation, ok := repository.operations[id]
	if !ok {
		return domain.Operation{}, ErrNotFound
	}
	return cloneOperation(operation), nil
}

func (repository *MemoryRepository) CreateOperation(ctx context.Context, key, requestHash string, operation domain.Operation) (CreateOperationResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateOperationResult{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	scope := operation.ClassroomID + ":" + key
	if existing, ok := repository.idempotency[scope]; ok {
		if existing.RequestHash != requestHash {
			return CreateOperationResult{}, ErrIdempotencyConflict
		}
		stored := repository.operations[existing.OperationID]
		return CreateOperationResult{Operation: cloneOperation(stored), Created: false}, nil
	}
	if _, exists := repository.operations[operation.ID]; exists {
		return CreateOperationResult{}, ErrVersionConflict
	}
	operation.ResourceVersion = 1
	operation.RefreshCounts()
	repository.operations[operation.ID] = cloneOperation(operation)
	repository.idempotency[scope] = idempotencyRecord{RequestHash: requestHash, OperationID: operation.ID}
	return CreateOperationResult{Operation: cloneOperation(operation), Created: true}, nil
}

func (repository *MemoryRepository) SaveOperation(ctx context.Context, operation *domain.Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	stored, ok := repository.operations[operation.ID]
	if !ok {
		return ErrNotFound
	}
	if stored.ResourceVersion != operation.ResourceVersion {
		return ErrVersionConflict
	}
	operation.ResourceVersion++
	operation.RefreshCounts()
	repository.operations[operation.ID] = cloneOperation(*operation)
	if operation.Status.Terminal() {
		delete(repository.operationLease, operation.ID)
	}
	return nil
}

func (repository *MemoryRepository) ClaimNextOperation(ctx context.Context, owner string, leaseDuration time.Duration) (*domain.Operation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	now := time.Now().UTC()
	var selected *domain.Operation
	for id, candidate := range repository.operations {
		if candidate.Status.Terminal() || candidate.Status == domain.OperationCancelRequested {
			continue
		}
		if currentLease, ok := repository.operationLease[id]; ok && currentLease.ExpiresAt.After(now) && currentLease.Owner != owner {
			continue
		}
		candidateCopy := cloneOperation(candidate)
		if selected == nil || candidateCopy.CreatedAt.Before(selected.CreatedAt) {
			selected = &candidateCopy
		}
	}
	if selected == nil {
		return nil, nil
	}
	repository.operationLease[selected.ID] = lease{Owner: owner, ExpiresAt: now.Add(leaseDuration)}
	return selected, nil
}

func (repository *MemoryRepository) RenewOperationLease(ctx context.Context, operationID, owner string, leaseDuration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	operation, ok := repository.operations[operationID]
	if !ok {
		return ErrNotFound
	}
	if operation.Status.Terminal() {
		return nil
	}
	current, ok := repository.operationLease[operationID]
	if !ok || current.Owner != owner {
		return ErrVersionConflict
	}
	repository.operationLease[operationID] = lease{Owner: owner, ExpiresAt: time.Now().UTC().Add(leaseDuration)}
	return nil
}

func (repository *MemoryRepository) ApplyOperationItemResult(ctx context.Context, classroomID, seatID string, operationType domain.OperationType, status domain.ItemStatus) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	classroom, ok := repository.classrooms[classroomID]
	if !ok {
		return ErrNotFound
	}
	found := false
	for index := range classroom.Seats {
		seat := &classroom.Seats[index]
		if seat.ID != seatID {
			continue
		}
		found = true
		seat.OperationState = status
		if status == domain.ItemSucceeded && seat.Desktop != nil {
			switch operationType {
			case domain.OperationStart, domain.OperationRestore:
				seat.Desktop.DesiredState = domain.PowerRunning
				seat.Desktop.ObservedState = domain.PowerRunning
				seat.Desktop.GuestAgentReady = true
			case domain.OperationShutdown:
				seat.Desktop.DesiredState = domain.PowerStopped
				seat.Desktop.ObservedState = domain.PowerStopped
				seat.Desktop.GuestAgentReady = false
			}
			now := time.Now().UTC()
			seat.Desktop.LastReconciledAt = &now
		}
		break
	}
	if !found {
		return ErrNotFound
	}
	classroom.ResourceVersion++
	classroom.UpdatedAt = time.Now().UTC()
	repository.classrooms[classroomID] = classroom
	return nil
}

func cloneClassroom(classroom domain.Classroom) domain.Classroom {
	result := classroom
	result.Seats = make([]domain.Seat, len(classroom.Seats))
	copy(result.Seats, classroom.Seats)
	for index := range result.Seats {
		if classroom.Seats[index].Terminal != nil {
			terminal := *classroom.Seats[index].Terminal
			result.Seats[index].Terminal = &terminal
		}
		if classroom.Seats[index].Desktop != nil {
			desktop := *classroom.Seats[index].Desktop
			result.Seats[index].Desktop = &desktop
		}
	}
	return result
}

func cloneOperation(operation domain.Operation) domain.Operation {
	result := operation
	result.Items = append([]domain.OperationItem(nil), operation.Items...)
	return result
}

func developmentClassrooms(now time.Time) []domain.Classroom {
	return []domain.Classroom{
		developmentClassroom(1, "计算机教室 A101", "主校区", domain.ClassroomReady, 24, 22, 20, now),
		developmentClassroom(2, "云教室 B203", "实训楼", domain.ClassroomActive, 32, 30, 28, now),
	}
}

func developmentClassroom(sequence int, name, site string, status domain.ClassroomStatus, seatCount, onlineCount, runningCount int, now time.Time) domain.Classroom {
	classroomID := deterministicID(sequence)
	organizationID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	clusterID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	activeSession := (*string)(nil)
	if status == domain.ClassroomActive {
		value := "高一信息技术 · 第 3 节"
		activeSession = &value
	}
	classroom := domain.Classroom{
		ID:              classroomID,
		OrganizationID:  organizationID,
		Name:            name,
		Site:            site,
		Status:          status,
		Timezone:        "Asia/Shanghai",
		TemplateName:    "Windows 11 教学镜像",
		TemplateVersion: "2026.07.1",
		ActiveSession:   activeSession,
		ResourceVersion: 1,
		UpdatedAt:       now,
		Seats:           make([]domain.Seat, 0, seatCount),
	}
	for index := 1; index <= seatCount; index++ {
		lastSeen := now.Add(-time.Duration(index%5) * time.Minute)
		terminalOnline := index <= onlineCount
		desktopRunning := index <= runningCount
		desiredState := domain.PowerStopped
		observedState := domain.PowerStopped
		if desktopRunning {
			desiredState = domain.PowerRunning
			observedState = domain.PowerRunning
		}
		base := sequence*1000 + index
		classroom.Seats = append(classroom.Seats, domain.Seat{
			ID:             deterministicID(10000 + base),
			Label:          fmt.Sprintf("%02d", index),
			OperationState: domain.ItemSucceeded,
			Terminal: &domain.ThinClient{
				ID:           deterministicID(20000 + base),
				Name:         fmt.Sprintf("TC-%s-%02d", string(rune('A'+sequence-1)), index),
				Online:       terminalOnline,
				IPAddress:    fmt.Sprintf("10.%d.10.%d", sequence, 20+index),
				Architecture: "loong64",
				AgentVersion: "0.1.0-dev",
				LastSeenAt:   &lastSeen,
			},
			Desktop: &domain.VirtualDesktop{
				ID:               deterministicID(30000 + base),
				Name:             fmt.Sprintf("student-%d-%02d", sequence, index),
				ClusterID:        clusterID,
				PVEVMID:          sequence*100 + index,
				DesiredState:     desiredState,
				ObservedState:    observedState,
				TemplateVersion:  classroom.TemplateVersion,
				BaselineSnapshot: "classroom-baseline",
				GuestAgentReady:  index <= onlineCount,
				LastReconciledAt: &lastSeen,
				ConfigHash:       fmt.Sprintf("dev-%d-%02d", sequence, index),
			},
		})
	}
	return classroom
}

func deterministicID(value int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", value)
}
