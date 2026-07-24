package operations

import (
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

func TestEventLogEmitsSnapshotAndIncrementalItemUpdate(t *testing.T) {
	t.Parallel()
	log := NewEventLog(16, 4)
	operation := eventTestOperation("operation-1")

	initial := log.EventsSince(operation, nil)
	if len(initial) != 1 || initial[0].EventType != EventOperationSnapshot {
		t.Fatalf("unexpected initial events: %+v", initial)
	}
	if initial[0].Sequence != 1 || initial[0].OperationID != operation.ID || len(initial[0].Items) != 2 {
		t.Fatalf("incomplete initial snapshot: %+v", initial[0])
	}
	if initial[0].Progress.Total != 2 || initial[0].Progress.Completed != 0 {
		t.Fatalf("unexpected initial progress: %+v", initial[0].Progress)
	}

	after := initial[0].Sequence
	operation.ResourceVersion++
	operation.Status = domain.OperationRunning
	operation.UpdatedAt = operation.UpdatedAt.Add(time.Second)
	operation.Items[0].Status = domain.ItemRunning
	operation.Items[0].UpdatedAt = operation.UpdatedAt
	operation.RefreshCounts()
	updates := log.EventsSince(operation, &after)
	if len(updates) != 2 {
		t.Fatalf("expected parent and item update, got %+v", updates)
	}
	if updates[0].EventType != EventOperationUpdated || updates[1].EventType != EventItemUpdated {
		t.Fatalf("unexpected event types: %s, %s", updates[0].EventType, updates[1].EventType)
	}
	if updates[0].Sequence <= after || updates[1].Sequence <= updates[0].Sequence {
		t.Fatalf("sequences are not monotonic: %+v", updates)
	}
	if updates[1].ItemID != operation.Items[0].ID || updates[1].ItemStatus != domain.ItemRunning {
		t.Fatalf("unexpected item patch: %+v", updates[1])
	}
	if updates[1].Progress.Running != 1 || updates[1].Progress.Queued != 1 {
		t.Fatalf("unexpected update progress: %+v", updates[1].Progress)
	}

	replayAfter := updates[0].Sequence
	replayed := log.EventsSince(operation, &replayAfter)
	if len(replayed) != 1 || replayed[0].Sequence != updates[1].Sequence {
		t.Fatalf("unexpected replay: %+v", replayed)
	}
	latest := updates[1].Sequence
	if events := log.EventsSince(operation, &latest); len(events) != 0 {
		t.Fatalf("expected no duplicate events, got %+v", events)
	}
}

func TestEventLogResetsWhenHistoryIsMissingOrStreamWasEvicted(t *testing.T) {
	t.Parallel()
	log := NewEventLog(2, 1)
	operation := eventTestOperation("operation-1")
	initial := log.EventsSince(operation, nil)[0]

	for version := int64(2); version <= 3; version++ {
		operation.ResourceVersion = version
		operation.UpdatedAt = operation.UpdatedAt.Add(time.Second)
		operation.Items[0].UpdatedAt = operation.UpdatedAt
		if version == 2 {
			operation.Items[0].Status = domain.ItemRunning
		} else {
			operation.Items[0].Status = domain.ItemSucceeded
		}
		operation.RefreshCounts()
		cursor := initial.Sequence
		_ = log.EventsSince(operation, &cursor)
	}

	expired := initial.Sequence
	reset := log.EventsSince(operation, &expired)
	if len(reset) != 1 || !reset[0].Reset || reset[0].EventType != EventOperationSnapshot {
		t.Fatalf("expected reset snapshot for expired history, got %+v", reset)
	}
	if reset[0].Sequence <= expired || len(reset[0].Items) != len(operation.Items) {
		t.Fatalf("reset snapshot cannot rebuild state: %+v", reset[0])
	}

	other := eventTestOperation("operation-2")
	_ = log.EventsSince(other, nil)
	previous := reset[0].Sequence
	afterEviction := log.EventsSince(operation, &previous)
	if len(afterEviction) != 1 || !afterEviction[0].Reset || afterEviction[0].Sequence <= previous {
		t.Fatalf("expected reset after stream eviction, got %+v", afterEviction)
	}
}

func TestEventLogResetsAboveFutureLastEventID(t *testing.T) {
	t.Parallel()
	log := NewEventLog(8, 2)
	operation := eventTestOperation("operation-1")
	future := int64(1000)

	events := log.EventsSince(operation, &future)
	if len(events) != 1 || !events[0].Reset || events[0].Sequence != future+1 {
		t.Fatalf("unexpected reset for future Last-Event-ID: %+v", events)
	}
}

func eventTestOperation(id string) domain.Operation {
	now := time.Date(2026, time.July, 24, 8, 0, 0, 0, time.UTC)
	operation := domain.Operation{
		ID:              id,
		Status:          domain.OperationQueued,
		ResourceVersion: 1,
		CreatedAt:       now,
		UpdatedAt:       now,
		Items: []domain.OperationItem{
			{ID: id + "-item-1", SeatID: "seat-1", SeatLabel: "01", TargetName: "desktop-1", Status: domain.ItemQueued, UpdatedAt: now},
			{ID: id + "-item-2", SeatID: "seat-2", SeatLabel: "02", TargetName: "desktop-2", Status: domain.ItemQueued, UpdatedAt: now},
		},
	}
	operation.RefreshCounts()
	return operation
}
