package operations

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/pve"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

type waitErrorAdapter struct{}

func (waitErrorAdapter) Submit(context.Context, pve.Request) (string, error) {
	return "UPID:test:unknown", nil
}

func (waitErrorAdapter) Wait(context.Context, string) (pve.Result, error) {
	return pve.Result{}, &pve.Error{Code: pve.ErrorUnknownTask, Retryable: true, Message: "injected unknown task"}
}

type uncertainSubmitAdapter struct {
	waitCalls int
}

func (*uncertainSubmitAdapter) Submit(context.Context, pve.Request) (string, error) {
	return "", &pve.Error{Code: pve.ErrorUnavailable, Retryable: true, Message: "injected unavailable PVE"}
}

func (adapter *uncertainSubmitAdapter) Wait(context.Context, string) (pve.Result, error) {
	adapter.waitCalls++
	return pve.Result{}, nil
}

type submitFailureAdapter struct {
	code        pve.ErrorCode
	submitCalls int
	waitCalls   int
}

type captureAdapter struct {
	requests []pve.Request
}

func (adapter *captureAdapter) Submit(_ context.Context, request pve.Request) (string, error) {
	adapter.requests = append(adapter.requests, request)
	return "UPID:fake:captured", nil
}

func (*captureAdapter) Wait(context.Context, string) (pve.Result, error) {
	return pve.Result{Succeeded: true, Code: "OK", Message: "完成"}, nil
}

func (adapter *submitFailureAdapter) Submit(context.Context, pve.Request) (string, error) {
	adapter.submitCalls++
	return "", &pve.Error{Code: adapter.code, Retryable: adapter.code == pve.ErrorUnavailable, Message: "injected submit failure"}
}

func (adapter *submitFailureAdapter) Wait(context.Context, string) (pve.Result, error) {
	adapter.waitCalls++
	return pve.Result{}, nil
}

var errSimulatedWorkerCrash = errors.New("simulated worker crash after durable item save")

type crashAfterTerminalSaveRepository struct {
	*store.MemoryRepository
	crashed bool
}

func (repository *crashAfterTerminalSaveRepository) SaveOperation(ctx context.Context, operation *domain.Operation) error {
	if err := repository.MemoryRepository.SaveOperation(ctx, operation); err != nil {
		return err
	}
	if repository.crashed {
		return nil
	}
	for _, item := range operation.Items {
		if item.UPID == "" && (item.Status == domain.ItemFailed || item.Status == domain.ItemUnknown) {
			repository.crashed = true
			return errSimulatedWorkerCrash
		}
	}
	return nil
}

func TestRunnerExecutesItemsInFakePVE(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	adapter := pve.NewFakeAdapter(time.Millisecond)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(repository, adapter, logger, RunnerConfig{WaveSize: 3, Lease: time.Minute})
	manager := NewManager(repository, runner)
	classrooms, _ := repository.ListClassrooms(context.Background())
	detail, _ := repository.GetClassroom(context.Background(), classrooms[0].ID)
	request := CreateRequest{Type: domain.OperationStart, SeatIDs: []string{detail.Seats[0].ID, detail.Seats[1].ID, detail.Seats[2].ID, detail.Seats[3].ID}}
	created, err := manager.Create(context.Background(), detail.ID, "run-key", "request-1", request)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := repository.ClaimNextOperation(context.Background(), runner.config.WorkerID, time.Minute)
	if err != nil || operation == nil {
		t.Fatalf("claim operation: operation=%v err=%v", operation, err)
	}
	if err := runner.execute(context.Background(), operation); err != nil {
		t.Fatalf("execute operation: %v", err)
	}
	stored, err := repository.GetOperation(context.Background(), created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.OperationSucceeded || stored.Counts.Succeeded != 4 {
		t.Fatalf("unexpected operation result: status=%s counts=%+v", stored.Status, stored.Counts)
	}
	for _, item := range stored.Items {
		if item.UPID == "" {
			t.Fatalf("item %s did not persist a UPID", item.ID)
		}
	}
}

func TestRunnerFailureConvergesEveryNonterminalItemToUnknown(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(repository, pve.NewFakeAdapter(time.Millisecond), logger, RunnerConfig{})
	now := time.Now().UTC()
	operation := domain.Operation{
		ID:          domain.NewID(),
		ClassroomID: "00000000-0000-4000-8000-000000000001",
		Type:        domain.OperationStart,
		Status:      domain.OperationQueued,
		CreatedAt:   now,
		UpdatedAt:   now,
		Items: []domain.OperationItem{
			{ID: domain.NewID(), Status: domain.ItemQueued, UpdatedAt: now},
			{ID: domain.NewID(), Status: domain.ItemRunning, UpdatedAt: now},
			{ID: domain.NewID(), Status: domain.ItemWaitingPVE, UPID: "UPID:fake:waiting", UpdatedAt: now},
			{ID: domain.NewID(), Status: domain.ItemVerifying, UPID: "UPID:fake:verifying", UpdatedAt: now},
		},
	}
	created, err := repository.CreateOperation(context.Background(), "failure-key", "failure-hash", operation)
	if err != nil {
		t.Fatal(err)
	}
	stored := created.Operation
	for _, status := range []domain.OperationStatus{domain.OperationValidating, domain.OperationRunning, domain.OperationWaitingPVE} {
		if err := stored.Transition(status, now); err != nil {
			t.Fatal(err)
		}
		if err := repository.SaveOperation(context.Background(), &stored); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.fail(&stored, errors.New("injected persistence failure")); err != nil {
		t.Fatal(err)
	}
	final, err := repository.GetOperation(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.OperationFailed || final.Counts.Queued != 0 || final.Counts.Running != 0 || final.Counts.Unknown != 4 {
		t.Fatalf("failure did not converge: status=%s counts=%+v", final.Status, final.Counts)
	}
}

func TestRunnerWaitErrorMarksItemAndSeatUnknown(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(repository, waitErrorAdapter{}, logger, RunnerConfig{WaveSize: 1})
	manager := NewManager(repository, runner)
	classrooms, _ := repository.ListClassrooms(context.Background())
	classroom, _ := repository.GetClassroom(context.Background(), classrooms[0].ID)
	created, err := manager.Create(context.Background(), classroom.ID, "wait-error-key", "request-wait-error", CreateRequest{
		Type:    domain.OperationStart,
		SeatIDs: []string{classroom.Seats[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := repository.ClaimNextOperation(context.Background(), runner.config.WorkerID, time.Minute)
	if err != nil || operation == nil {
		t.Fatalf("claim operation: operation=%v err=%v", operation, err)
	}
	if err := runner.execute(context.Background(), operation); err != nil {
		t.Fatalf("execute operation: %v", err)
	}
	final, err := repository.GetOperation(context.Background(), created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.OperationFailed || final.Counts.Unknown != 1 || final.Counts.Failed != 0 {
		t.Fatalf("unexpected operation result: status=%s counts=%+v", final.Status, final.Counts)
	}
	if final.Items[0].Status != domain.ItemUnknown || final.Items[0].CompletedAt == nil || final.Items[0].ErrorCode != string(pve.ErrorUnknownTask) {
		t.Fatalf("wait error did not preserve unknown semantics: %+v", final.Items[0])
	}
	updatedClassroom, err := repository.GetClassroom(context.Background(), classroom.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedClassroom.Seats[0].OperationState != domain.ItemUnknown {
		t.Fatalf("seat state diverged from operation item: %s", updatedClassroom.Seats[0].OperationState)
	}
}

func TestRunnerUncertainSubmitDoesNotWaitWithoutUPID(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adapter := &uncertainSubmitAdapter{}
	runner := NewRunner(repository, adapter, logger, RunnerConfig{WaveSize: 1})
	manager := NewManager(repository, runner)
	classrooms, _ := repository.ListClassrooms(context.Background())
	classroom, _ := repository.GetClassroom(context.Background(), classrooms[0].ID)
	created, err := manager.Create(context.Background(), classroom.ID, "submit-unknown-key", "request-submit-unknown", CreateRequest{
		Type:    domain.OperationStart,
		SeatIDs: []string{classroom.Seats[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := repository.ClaimNextOperation(context.Background(), runner.config.WorkerID, time.Minute)
	if err != nil || operation == nil {
		t.Fatalf("claim operation: operation=%v err=%v", operation, err)
	}
	if err := runner.execute(context.Background(), operation); err != nil {
		t.Fatalf("execute operation: %v", err)
	}
	final, err := repository.GetOperation(context.Background(), created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.waitCalls != 0 {
		t.Fatalf("Wait was called %d times without a UPID", adapter.waitCalls)
	}
	item := final.Items[0]
	if item.Status != domain.ItemUnknown || item.UPID != "" || item.ErrorCode != string(pve.ErrorUnavailable) || item.Message != "无法确认 PVE 是否受理该操作" {
		t.Fatalf("submission uncertainty was not preserved: %+v", item)
	}
}

func TestSubmitFailureIsDurableBeforeWorkerCrash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		code         pve.ErrorCode
		expectedItem domain.ItemStatus
	}{
		{name: "explicit rejection", code: pve.ErrorPermission, expectedItem: domain.ItemFailed},
		{name: "uncertain outcome", code: pve.ErrorUnavailable, expectedItem: domain.ItemUnknown},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			baseRepository := store.NewDevelopmentRepository(time.Now())
			repository := &crashAfterTerminalSaveRepository{MemoryRepository: baseRepository}
			adapter := &submitFailureAdapter{code: test.code}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			workerID := "worker-crash-boundary"
			runner := NewRunner(repository, adapter, logger, RunnerConfig{WorkerID: workerID, WaveSize: 1})
			manager := NewManager(repository, runner)
			classrooms, _ := repository.ListClassrooms(context.Background())
			classroom, _ := repository.GetClassroom(context.Background(), classrooms[0].ID)
			created, err := manager.Create(context.Background(), classroom.ID, "crash-"+string(test.code), "request-crash", CreateRequest{
				Type:    domain.OperationStart,
				SeatIDs: []string{classroom.Seats[0].ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			operation, err := repository.ClaimNextOperation(context.Background(), workerID, time.Minute)
			if err != nil || operation == nil {
				t.Fatalf("claim operation: operation=%v err=%v", operation, err)
			}
			if err := runner.execute(context.Background(), operation); !errors.Is(err, errSimulatedWorkerCrash) {
				t.Fatalf("expected crash after durable save, got %v", err)
			}
			durable, err := baseRepository.GetOperation(context.Background(), created.Operation.ID)
			if err != nil {
				t.Fatal(err)
			}
			if durable.Items[0].Status != test.expectedItem || durable.Items[0].UPID != "" {
				t.Fatalf("terminal submit result was not durable: %+v", durable.Items[0])
			}

			restarted := NewRunner(baseRepository, adapter, logger, RunnerConfig{WorkerID: workerID, WaveSize: 1})
			recovered, err := baseRepository.ClaimNextOperation(context.Background(), workerID, time.Minute)
			if err != nil || recovered == nil {
				t.Fatalf("reclaim operation: operation=%v err=%v", recovered, err)
			}
			if err := restarted.execute(context.Background(), recovered); err != nil {
				t.Fatalf("resume operation: %v", err)
			}
			if adapter.submitCalls != 1 || adapter.waitCalls != 0 {
				t.Fatalf("terminal item was replayed: submit=%d wait=%d", adapter.submitCalls, adapter.waitCalls)
			}
		})
	}
}

func TestMissingDesktopFailsBothItemAndSeat(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	classroomID := domain.NewID()
	seatID := domain.NewID()
	repository := store.NewMemoryRepository([]domain.Classroom{{
		ID:              classroomID,
		OrganizationID:  domain.NewID(),
		Name:            "无桌面测试教室",
		Site:            "测试校区",
		Status:          domain.ClassroomDegraded,
		Timezone:        "Asia/Shanghai",
		TemplateName:    "测试模板",
		TemplateVersion: "1.0.0",
		ResourceVersion: 1,
		UpdatedAt:       now,
		Seats: []domain.Seat{{
			ID:             seatID,
			Label:          "01",
			OperationState: domain.ItemSucceeded,
		}},
	}})
	adapter := &submitFailureAdapter{code: pve.ErrorPermission}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(repository, adapter, logger, RunnerConfig{WaveSize: 1})
	manager := NewManager(repository, runner)
	created, err := manager.Create(context.Background(), classroomID, "missing-desktop", "request-missing-desktop", CreateRequest{
		Type:    domain.OperationStart,
		SeatIDs: []string{seatID},
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := repository.ClaimNextOperation(context.Background(), runner.config.WorkerID, time.Minute)
	if err != nil || operation == nil {
		t.Fatalf("claim operation: operation=%v err=%v", operation, err)
	}
	if err := runner.execute(context.Background(), operation); err != nil {
		t.Fatal(err)
	}
	final, _ := repository.GetOperation(context.Background(), created.Operation.ID)
	detail, _ := repository.GetClassroom(context.Background(), classroomID)
	if final.Items[0].Status != domain.ItemFailed || detail.Seats[0].OperationState != domain.ItemFailed {
		t.Fatalf("item/seat state diverged: item=%s seat=%s", final.Items[0].Status, detail.Seats[0].OperationState)
	}
	if adapter.submitCalls != 0 {
		t.Fatalf("missing desktop was submitted to PVE %d times", adapter.submitCalls)
	}
}

func TestRunnerPassesBaselineSnapshotToPVE(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	adapter := &captureAdapter{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(repository, adapter, logger, RunnerConfig{WaveSize: 1})
	manager := NewManager(repository, runner)
	classrooms, _ := repository.ListClassrooms(context.Background())
	classroom, _ := repository.GetClassroom(context.Background(), classrooms[0].ID)
	created, err := manager.Create(context.Background(), classroom.ID, "restore-submit", "request-restore-submit", CreateRequest{
		Type:      domain.OperationRestore,
		SeatIDs:   []string{classroom.Seats[0].ID},
		Reason:    "恢复课程基线",
		Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := repository.ClaimNextOperation(context.Background(), runner.config.WorkerID, time.Minute)
	if err != nil || operation == nil {
		t.Fatalf("claim operation: operation=%v err=%v", operation, err)
	}
	if err := runner.execute(context.Background(), operation); err != nil {
		t.Fatalf("execute restore: %v", err)
	}
	if len(adapter.requests) != 1 || adapter.requests[0].SnapshotName != "classroom-baseline" {
		t.Fatalf("PVE request did not contain the baseline snapshot: %+v", adapter.requests)
	}
	stored, err := repository.GetOperation(context.Background(), created.Operation.ID)
	if err != nil || stored.Status != domain.OperationSucceeded {
		t.Fatalf("restore operation did not succeed: operation=%+v err=%v", stored, err)
	}
}

func TestRestoreWithoutBaselineFailsBeforePVE(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	classroomID := domain.NewID()
	seatID := domain.NewID()
	repository := store.NewMemoryRepository([]domain.Classroom{{
		ID:              classroomID,
		OrganizationID:  domain.NewID(),
		Name:            "缺少基线测试教室",
		Site:            "测试校区",
		Status:          domain.ClassroomReady,
		Timezone:        "Asia/Shanghai",
		TemplateName:    "测试模板",
		TemplateVersion: "1.0.0",
		ResourceVersion: 1,
		UpdatedAt:       now,
		Seats: []domain.Seat{{
			ID:             seatID,
			Label:          "01",
			OperationState: domain.ItemSucceeded,
			Desktop: &domain.VirtualDesktop{
				ID:              domain.NewID(),
				Name:            "student-01",
				ClusterID:       domain.NewID(),
				PVEVMID:         101,
				DesiredState:    domain.PowerStopped,
				ObservedState:   domain.PowerStopped,
				TemplateVersion: "1.0.0",
			},
		}},
	}})
	adapter := &submitFailureAdapter{code: pve.ErrorPermission}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(repository, adapter, logger, RunnerConfig{WaveSize: 1})
	manager := NewManager(repository, runner)
	created, err := manager.Create(context.Background(), classroomID, "restore-no-baseline", "request-no-baseline", CreateRequest{
		Type:      domain.OperationRestore,
		SeatIDs:   []string{seatID},
		Reason:    "验证缺少基线",
		Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := repository.ClaimNextOperation(context.Background(), runner.config.WorkerID, time.Minute)
	if err != nil || operation == nil {
		t.Fatalf("claim operation: operation=%v err=%v", operation, err)
	}
	if err := runner.execute(context.Background(), operation); err != nil {
		t.Fatal(err)
	}
	stored, _ := repository.GetOperation(context.Background(), created.Operation.ID)
	if stored.Items[0].Status != domain.ItemFailed || stored.Items[0].ErrorCode != "BASELINE_SNAPSHOT_NOT_CONFIGURED" {
		t.Fatalf("missing baseline was not reported per item: %+v", stored.Items[0])
	}
	if adapter.submitCalls != 0 {
		t.Fatalf("restore without a baseline reached PVE %d times", adapter.submitCalls)
	}
}
