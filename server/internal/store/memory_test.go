package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

func TestCreateOperationIsIdempotent(t *testing.T) {
	t.Parallel()
	repository := NewDevelopmentRepository(time.Now())
	operation := domain.Operation{
		ID:          domain.NewID(),
		ClassroomID: "00000000-0000-4000-8000-000000000001",
		Status:      domain.OperationQueued,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	first, err := repository.CreateOperation(context.Background(), "same-key", "hash-a", operation)
	if err != nil || !first.Created {
		t.Fatalf("create first operation: created=%v err=%v", first.Created, err)
	}
	second, err := repository.CreateOperation(context.Background(), "same-key", "hash-a", operation)
	if err != nil || second.Created || second.Operation.ID != first.Operation.ID {
		t.Fatalf("repeat operation: %+v err=%v", second, err)
	}
	_, err = repository.CreateOperation(context.Background(), "same-key", "hash-b", operation)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}
