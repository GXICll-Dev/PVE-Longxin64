package operations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

func TestManagerCreatesItemsAndPreservesIdempotency(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	manager := NewManager(repository, nil)
	classrooms, err := repository.ListClassrooms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request := CreateRequest{Type: domain.OperationStart}
	first, err := manager.Create(context.Background(), classrooms[0].ID, "test-key", "request-1", request)
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if len(first.Operation.Items) != classrooms[0].SeatsTotal {
		t.Fatalf("expected %d items, got %d", classrooms[0].SeatsTotal, len(first.Operation.Items))
	}
	for _, item := range first.Operation.Items {
		if item.SnapshotName != "" {
			t.Fatalf("start item unexpectedly contains a snapshot name: %+v", item)
		}
	}
	second, err := manager.Create(context.Background(), classrooms[0].ID, "test-key", "request-2", request)
	if err != nil {
		t.Fatalf("repeat operation: %v", err)
	}
	if second.Created || second.Operation.ID != first.Operation.ID {
		t.Fatalf("expected same operation, got %+v", second)
	}
}

func TestRestoreRequiresReasonAndConfirmation(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	manager := NewManager(repository, nil)
	classrooms, _ := repository.ListClassrooms(context.Background())
	_, err := manager.Create(context.Background(), classrooms[0].ID, "restore-key", "request-1", CreateRequest{Type: domain.OperationRestore})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestManagerCopiesDesktopBaselineIntoRestoreItem(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	manager := NewManager(repository, nil)
	classrooms, _ := repository.ListClassrooms(context.Background())
	classroom, _ := repository.GetClassroom(context.Background(), classrooms[0].ID)
	result, err := manager.Create(context.Background(), classroom.ID, "restore-with-baseline", "request-restore", CreateRequest{
		Type:      domain.OperationRestore,
		SeatIDs:   []string{classroom.Seats[0].ID},
		Reason:    "下课后恢复教学基线",
		Confirmed: true,
	})
	if err != nil {
		t.Fatalf("create restore operation: %v", err)
	}
	if len(result.Operation.Items) != 1 || result.Operation.Items[0].SnapshotName != "classroom-baseline" {
		t.Fatalf("restore item did not preserve the baseline snapshot: %+v", result.Operation.Items)
	}
}
