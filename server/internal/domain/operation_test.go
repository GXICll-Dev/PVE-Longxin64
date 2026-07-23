package domain

import (
	"errors"
	"testing"
	"time"
)

func TestOperationStateMachine(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	operation := Operation{Status: OperationQueued, Items: []OperationItem{{Status: ItemQueued}}}

	for _, next := range []OperationStatus{
		OperationValidating,
		OperationRunning,
		OperationWaitingPVE,
		OperationVerifying,
		OperationSucceeded,
	} {
		if err := operation.Transition(next, now); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	if operation.CompletedAt == nil {
		t.Fatal("terminal operation must record completed_at")
	}
	if err := operation.Transition(OperationRunning, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid terminal transition, got %v", err)
	}
}

func TestOperationCounts(t *testing.T) {
	t.Parallel()
	operation := Operation{Items: []OperationItem{
		{Status: ItemQueued},
		{Status: ItemWaitingPVE},
		{Status: ItemVerifying},
		{Status: ItemSucceeded},
		{Status: ItemFailed},
		{Status: ItemSkipped},
		{Status: ItemUnknown},
	}}
	operation.RefreshCounts()
	if operation.Counts != (OperationCounts{Total: 7, Queued: 1, Running: 2, Succeeded: 1, Failed: 1, Skipped: 1, Unknown: 1}) {
		t.Fatalf("unexpected counts: %+v", operation.Counts)
	}
}
