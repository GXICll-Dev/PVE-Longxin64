package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

var (
	ErrInvalidRequest         = errors.New("invalid operation request")
	ErrIdempotencyKeyRequired = errors.New("Idempotency-Key header is required")
	ErrConfirmationRequired   = errors.New("restore operations require explicit confirmation and a reason")
)

type CreateRequest struct {
	Type      domain.OperationType `json:"type"`
	SeatIDs   []string             `json:"seat_ids,omitempty"`
	Reason    string               `json:"reason,omitempty"`
	Confirmed bool                 `json:"confirmed,omitempty"`
}

type Notifier interface {
	Notify()
}

type Manager struct {
	repository store.Repository
	notifier   Notifier
	now        func() time.Time
}

func NewManager(repository store.Repository, notifier Notifier) *Manager {
	return &Manager{repository: repository, notifier: notifier, now: time.Now}
}

func (manager *Manager) Create(ctx context.Context, classroomID, idempotencyKey, requestID string, request CreateRequest) (store.CreateOperationResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return store.CreateOperationResult{}, ErrIdempotencyKeyRequired
	}
	if len(idempotencyKey) > 128 || containsControlCharacter(idempotencyKey) {
		return store.CreateOperationResult{}, fmt.Errorf("%w: Idempotency-Key must be at most 128 printable characters", ErrInvalidRequest)
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if !request.Type.Valid() {
		return store.CreateOperationResult{}, fmt.Errorf("%w: type must be PRECHECK, START, SHUTDOWN, or RESTORE", ErrInvalidRequest)
	}
	if len(request.Reason) > 500 {
		return store.CreateOperationResult{}, fmt.Errorf("%w: reason must be at most 500 characters", ErrInvalidRequest)
	}
	if request.Type == domain.OperationRestore && (!request.Confirmed || request.Reason == "") {
		return store.CreateOperationResult{}, ErrConfirmationRequired
	}

	classroom, err := manager.repository.GetClassroom(ctx, classroomID)
	if err != nil {
		return store.CreateOperationResult{}, err
	}
	seats, err := selectSeats(classroom.Seats, request.SeatIDs)
	if err != nil {
		return store.CreateOperationResult{}, err
	}
	if len(seats) == 0 {
		return store.CreateOperationResult{}, fmt.Errorf("%w: classroom does not contain any target seats", ErrInvalidRequest)
	}
	now := manager.now().UTC()
	operation := domain.Operation{
		ID:             domain.NewID(),
		ClassroomID:    classroom.ID,
		ClassroomName:  classroom.Name,
		Type:           request.Type,
		Status:         domain.OperationQueued,
		Reason:         request.Reason,
		RequestID:      requestID,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
		Items:          make([]domain.OperationItem, 0, len(seats)),
	}
	for _, seat := range seats {
		desktopID := ""
		clusterID := ""
		pveVMID := 0
		targetName := seat.Label
		if seat.Desktop != nil {
			desktopID = seat.Desktop.ID
			clusterID = seat.Desktop.ClusterID
			pveVMID = seat.Desktop.PVEVMID
			targetName = seat.Desktop.Name
		}
		operation.Items = append(operation.Items, domain.OperationItem{
			ID:          domain.NewID(),
			OperationID: operation.ID,
			SeatID:      seat.ID,
			SeatLabel:   seat.Label,
			DesktopID:   desktopID,
			ClusterID:   clusterID,
			PVEVMID:     pveVMID,
			TargetName:  targetName,
			Status:      domain.ItemQueued,
			UpdatedAt:   now,
		})
	}
	operation.RefreshCounts()
	canonicalRequest := request
	if len(request.SeatIDs) > 0 {
		canonicalRequest.SeatIDs = make([]string, 0, len(seats))
		for _, seat := range seats {
			canonicalRequest.SeatIDs = append(canonicalRequest.SeatIDs, seat.ID)
		}
	}
	requestHash, err := hashRequest(canonicalRequest)
	if err != nil {
		return store.CreateOperationResult{}, fmt.Errorf("hash operation request: %w", err)
	}
	result, err := manager.repository.CreateOperation(ctx, idempotencyKey, requestHash, operation)
	if err != nil {
		return store.CreateOperationResult{}, err
	}
	if result.Created && manager.notifier != nil {
		manager.notifier.Notify()
	}
	return result, nil
}

func selectSeats(available []domain.Seat, requested []string) ([]domain.Seat, error) {
	byID := make(map[string]domain.Seat, len(available))
	for _, seat := range available {
		byID[seat.ID] = seat
	}
	if len(requested) == 0 {
		result := append([]domain.Seat(nil), available...)
		sort.Slice(result, func(i, j int) bool { return result[i].Label < result[j].Label })
		return result, nil
	}
	seen := make(map[string]struct{}, len(requested))
	result := make([]domain.Seat, 0, len(requested))
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("%w: seat_ids cannot contain an empty value", ErrInvalidRequest)
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seat, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: seat %s does not belong to the classroom", ErrInvalidRequest, id)
		}
		seen[id] = struct{}{}
		result = append(result, seat)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Label < result[j].Label })
	return result, nil
}

func hashRequest(request CreateRequest) (string, error) {
	canonical := request
	canonical.SeatIDs = append([]string(nil), request.SeatIDs...)
	sort.Strings(canonical.SeatIDs)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}
