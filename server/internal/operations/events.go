package operations

import (
	"sync"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

const (
	defaultEventHistoryLimit   = 256
	defaultEventOperationLimit = 512

	// MaxEventSequence keeps sequence values exactly representable by
	// JavaScript numbers while leaving room to advance after Last-Event-ID.
	MaxEventSequence int64 = 9_007_199_254_740_000
	// MaxLastEventID leaves ample headroom for a reset snapshot and future
	// events even when a client reconnects with an ID from another API process.
	MaxLastEventID int64 = MaxEventSequence / 2
)

type EventType string

const (
	EventOperationSnapshot EventType = "operation.snapshot"
	EventOperationUpdated  EventType = "operation.updated"
	EventItemUpdated       EventType = "operation.item.updated"
)

type OperationProgress struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Unknown   int `json:"unknown"`
}

type OperationItemEventState struct {
	ItemID     string            `json:"item_id"`
	SeatID     string            `json:"seat_id"`
	SeatLabel  string            `json:"seat_label"`
	TargetName string            `json:"target_name"`
	Status     domain.ItemStatus `json:"status"`
	ErrorCode  string            `json:"error_code,omitempty"`
	Message    string            `json:"message,omitempty"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// OperationEvent is the JSON payload carried by an SSE frame. Snapshot
// events contain Items, while item updates expose the changed item through
// the top-level item fields so clients can apply a small object-level patch.
type OperationEvent struct {
	EventType       EventType                 `json:"event_type"`
	OperationID     string                    `json:"operation_id"`
	ItemID          string                    `json:"item_id,omitempty"`
	Sequence        int64                     `json:"sequence"`
	Timestamp       time.Time                 `json:"timestamp"`
	OperationStatus domain.OperationStatus    `json:"operation_status"`
	ItemStatus      domain.ItemStatus         `json:"item_status,omitempty"`
	SeatID          string                    `json:"seat_id,omitempty"`
	SeatLabel       string                    `json:"seat_label,omitempty"`
	TargetName      string                    `json:"target_name,omitempty"`
	ErrorCode       string                    `json:"error_code,omitempty"`
	Message         string                    `json:"message,omitempty"`
	ItemUpdatedAt   *time.Time                `json:"item_updated_at,omitempty"`
	Progress        OperationProgress         `json:"progress"`
	ResourceVersion int64                     `json:"resource_version"`
	Reset           bool                      `json:"reset,omitempty"`
	Items           []OperationItemEventState `json:"items,omitempty"`
}

type operationEventView struct {
	status          domain.OperationStatus
	progress        OperationProgress
	resourceVersion int64
	updatedAt       time.Time
	items           map[string]OperationItemEventState
	itemOrder       []string
}

type eventStream struct {
	nextSequence int64
	history      []OperationEvent
	last         operationEventView
	initialized  bool
	lastAccess   time.Time
}

// EventLog retains a bounded, process-local history for Last-Event-ID replay.
// It intentionally owns no goroutines. The HTTP stream observes durable
// Operation snapshots from the repository, so a standalone Worker remains
// visible without an in-process pub/sub dependency.
type EventLog struct {
	mu             sync.Mutex
	historyLimit   int
	operationLimit int
	streams        map[string]*eventStream
	now            func() time.Time
}

func NewEventLog(historyLimit, operationLimit int) *EventLog {
	if historyLimit <= 0 {
		historyLimit = defaultEventHistoryLimit
	}
	if operationLimit <= 0 {
		operationLimit = defaultEventOperationLimit
	}
	return &EventLog{
		historyLimit:   historyLimit,
		operationLimit: operationLimit,
		streams:        make(map[string]*eventStream),
		now:            time.Now,
	}
}

// EventsSince observes the current durable snapshot and returns events newer
// than lastEventID. A nil ID starts a fresh stream with a complete snapshot.
// If the requested ID is no longer replayable (or came from another API
// process), a reset snapshot is emitted with a sequence greater than the
// supplied ID.
func (log *EventLog) EventsSince(operation domain.Operation, lastEventID *int64) []OperationEvent {
	log.mu.Lock()
	defer log.mu.Unlock()

	now := log.now().UTC()
	stream, created := log.streamLocked(operation.ID, now)
	log.observeLocked(stream, operation)
	stream.lastAccess = now

	if lastEventID == nil {
		if !created {
			log.appendSnapshotLocked(stream, operation.ID, false, 0)
		}
		return []OperationEvent{cloneEvent(stream.history[len(stream.history)-1])}
	}

	after := *lastEventID
	if created {
		log.appendSnapshotLocked(stream, operation.ID, true, after+1)
		return []OperationEvent{cloneEvent(stream.history[len(stream.history)-1])}
	}
	oldest := stream.history[0].Sequence
	latest := stream.history[len(stream.history)-1].Sequence
	if after < oldest-1 || after > latest {
		minimum := int64(0)
		if after >= latest {
			minimum = after + 1
		}
		log.appendSnapshotLocked(stream, operation.ID, true, minimum)
		return []OperationEvent{cloneEvent(stream.history[len(stream.history)-1])}
	}

	events := make([]OperationEvent, 0, len(stream.history))
	for _, event := range stream.history {
		if event.Sequence > after {
			events = append(events, cloneEvent(event))
		}
	}
	return events
}

func (log *EventLog) streamLocked(operationID string, now time.Time) (*eventStream, bool) {
	if stream, ok := log.streams[operationID]; ok {
		return stream, false
	}
	if len(log.streams) >= log.operationLimit {
		var oldestID string
		var oldestAccess time.Time
		for id, candidate := range log.streams {
			if oldestID == "" || candidate.lastAccess.Before(oldestAccess) {
				oldestID = id
				oldestAccess = candidate.lastAccess
			}
		}
		delete(log.streams, oldestID)
	}
	stream := &eventStream{history: make([]OperationEvent, 0, log.historyLimit), lastAccess: now}
	log.streams[operationID] = stream
	return stream, true
}

func (log *EventLog) observeLocked(stream *eventStream, operation domain.Operation) {
	next := makeOperationEventView(operation)
	if !stream.initialized {
		stream.last = next
		stream.initialized = true
		log.appendSnapshotLocked(stream, operation.ID, false, 0)
		return
	}

	itemsRemoved := false
	for itemID := range stream.last.items {
		if _, exists := next.items[itemID]; !exists {
			itemsRemoved = true
			break
		}
	}
	if itemsRemoved {
		stream.last = next
		log.appendSnapshotLocked(stream, operation.ID, true, 0)
		return
	}

	changedItems := make([]OperationItemEventState, 0)
	for _, itemID := range next.itemOrder {
		item := next.items[itemID]
		previous, exists := stream.last.items[itemID]
		if !exists || !itemEventStateEqual(previous, item) {
			changedItems = append(changedItems, item)
		}
	}
	parentChanged := stream.last.status != next.status ||
		stream.last.progress != next.progress ||
		stream.last.resourceVersion != next.resourceVersion ||
		!stream.last.updatedAt.Equal(next.updatedAt)
	if !parentChanged && len(changedItems) == 0 {
		return
	}

	stream.last = next
	log.appendEventLocked(stream, OperationEvent{
		EventType:       EventOperationUpdated,
		OperationID:     operation.ID,
		OperationStatus: next.status,
		Progress:        next.progress,
		ResourceVersion: next.resourceVersion,
	}, 0)
	for _, item := range changedItems {
		updatedAt := item.UpdatedAt
		log.appendEventLocked(stream, OperationEvent{
			EventType:       EventItemUpdated,
			OperationID:     operation.ID,
			ItemID:          item.ItemID,
			OperationStatus: next.status,
			ItemStatus:      item.Status,
			SeatID:          item.SeatID,
			SeatLabel:       item.SeatLabel,
			TargetName:      item.TargetName,
			ErrorCode:       item.ErrorCode,
			Message:         item.Message,
			ItemUpdatedAt:   &updatedAt,
			Progress:        next.progress,
			ResourceVersion: next.resourceVersion,
		}, 0)
	}
}

func (log *EventLog) appendSnapshotLocked(stream *eventStream, operationID string, reset bool, minimumSequence int64) {
	items := make([]OperationItemEventState, 0, len(stream.last.itemOrder))
	for _, itemID := range stream.last.itemOrder {
		items = append(items, stream.last.items[itemID])
	}
	log.appendEventLocked(stream, OperationEvent{
		EventType:       EventOperationSnapshot,
		OperationID:     operationID,
		OperationStatus: stream.last.status,
		Progress:        stream.last.progress,
		ResourceVersion: stream.last.resourceVersion,
		Reset:           reset,
		Items:           items,
	}, minimumSequence)
}

func (log *EventLog) appendEventLocked(stream *eventStream, event OperationEvent, minimumSequence int64) {
	next := stream.nextSequence + 1
	if minimumSequence > next {
		next = minimumSequence
	}
	if next > MaxEventSequence {
		// The HTTP layer rejects IDs at this boundary. Reaching it through local
		// increments is practically impossible; resetting preserves availability.
		next = 1
		stream.history = stream.history[:0]
	}
	stream.nextSequence = next
	event.Sequence = next
	event.Timestamp = log.now().UTC()
	stream.history = append(stream.history, event)
	if len(stream.history) > log.historyLimit {
		overflow := len(stream.history) - log.historyLimit
		copy(stream.history, stream.history[overflow:])
		stream.history = stream.history[:log.historyLimit]
	}
}

func makeOperationEventView(operation domain.Operation) operationEventView {
	view := operationEventView{
		status:          operation.Status,
		progress:        progressFor(operation.Counts),
		resourceVersion: operation.ResourceVersion,
		updatedAt:       operation.UpdatedAt,
		items:           make(map[string]OperationItemEventState, len(operation.Items)),
		itemOrder:       make([]string, 0, len(operation.Items)),
	}
	for _, item := range operation.Items {
		state := OperationItemEventState{
			ItemID:     item.ID,
			SeatID:     item.SeatID,
			SeatLabel:  item.SeatLabel,
			TargetName: item.TargetName,
			Status:     item.Status,
			ErrorCode:  item.ErrorCode,
			Message:    item.Message,
			UpdatedAt:  item.UpdatedAt,
		}
		view.items[item.ID] = state
		view.itemOrder = append(view.itemOrder, item.ID)
	}
	return view
}

func progressFor(counts domain.OperationCounts) OperationProgress {
	return OperationProgress{
		Total:     counts.Total,
		Completed: counts.Succeeded + counts.Failed + counts.Skipped + counts.Unknown,
		Queued:    counts.Queued,
		Running:   counts.Running,
		Succeeded: counts.Succeeded,
		Failed:    counts.Failed,
		Skipped:   counts.Skipped,
		Unknown:   counts.Unknown,
	}
}

func itemEventStateEqual(left, right OperationItemEventState) bool {
	return left.ItemID == right.ItemID &&
		left.SeatID == right.SeatID &&
		left.SeatLabel == right.SeatLabel &&
		left.TargetName == right.TargetName &&
		left.Status == right.Status &&
		left.ErrorCode == right.ErrorCode &&
		left.Message == right.Message &&
		left.UpdatedAt.Equal(right.UpdatedAt)
}

func cloneEvent(event OperationEvent) OperationEvent {
	event.Items = append([]OperationItemEventState(nil), event.Items...)
	if event.ItemUpdatedAt != nil {
		updatedAt := *event.ItemUpdatedAt
		event.ItemUpdatedAt = &updatedAt
	}
	return event
}
