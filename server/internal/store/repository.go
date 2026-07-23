package store

import (
	"context"
	"errors"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

var (
	ErrNotFound            = errors.New("resource not found")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with a different request")
	ErrVersionConflict     = errors.New("resource version conflict")
)

type CreateOperationResult struct {
	Operation domain.Operation
	Created   bool
}

// Repository is the persistence boundary shared by the API and worker. Both
// the in-memory development store and PostgreSQL implementation must preserve
// the same idempotency and optimistic-concurrency semantics.
type Repository interface {
	Ping(context.Context) error
	Close()

	ListClassrooms(context.Context) ([]domain.ClassroomSummary, error)
	GetClassroom(context.Context, string) (domain.Classroom, error)

	ListOperations(context.Context, int) ([]domain.Operation, int, error)
	GetOperation(context.Context, string) (domain.Operation, error)
	CreateOperation(context.Context, string, string, domain.Operation) (CreateOperationResult, error)
	SaveOperation(context.Context, *domain.Operation) error
	ClaimNextOperation(context.Context, string, time.Duration) (*domain.Operation, error)
	RenewOperationLease(context.Context, string, string, time.Duration) error
	ApplyOperationItemResult(context.Context, string, string, domain.OperationType, domain.ItemStatus) error
}
