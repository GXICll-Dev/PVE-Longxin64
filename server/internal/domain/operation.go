package domain

import (
	"errors"
	"fmt"
	"time"
)

type OperationType string

const (
	OperationPrecheck OperationType = "PRECHECK"
	OperationStart    OperationType = "START"
	OperationShutdown OperationType = "SHUTDOWN"
	OperationRestore  OperationType = "RESTORE"
)

func (operationType OperationType) Valid() bool {
	switch operationType {
	case OperationPrecheck, OperationStart, OperationShutdown, OperationRestore:
		return true
	default:
		return false
	}
}

type OperationStatus string

const (
	OperationQueued             OperationStatus = "QUEUED"
	OperationValidating         OperationStatus = "VALIDATING"
	OperationRunning            OperationStatus = "RUNNING"
	OperationWaitingPVE         OperationStatus = "WAITING_PVE"
	OperationVerifying          OperationStatus = "VERIFYING"
	OperationSucceeded          OperationStatus = "SUCCEEDED"
	OperationPartiallySucceeded OperationStatus = "PARTIALLY_SUCCEEDED"
	OperationFailed             OperationStatus = "FAILED"
	OperationCancelRequested    OperationStatus = "CANCEL_REQUESTED"
	OperationCancelled          OperationStatus = "CANCELLED"
)

func (status OperationStatus) Terminal() bool {
	switch status {
	case OperationSucceeded, OperationPartiallySucceeded, OperationFailed, OperationCancelled:
		return true
	default:
		return false
	}
}

var operationTransitions = map[OperationStatus]map[OperationStatus]struct{}{
	OperationQueued: {
		OperationValidating:      {},
		OperationCancelRequested: {},
		OperationFailed:          {},
	},
	OperationValidating: {
		OperationRunning:         {},
		OperationCancelRequested: {},
		OperationFailed:          {},
	},
	OperationRunning: {
		OperationWaitingPVE:         {},
		OperationVerifying:          {},
		OperationSucceeded:          {},
		OperationPartiallySucceeded: {},
		OperationCancelRequested:    {},
		OperationFailed:             {},
	},
	OperationWaitingPVE: {
		OperationRunning:         {},
		OperationVerifying:       {},
		OperationCancelRequested: {},
		OperationFailed:          {},
	},
	OperationVerifying: {
		OperationSucceeded:          {},
		OperationPartiallySucceeded: {},
		OperationFailed:             {},
		OperationCancelRequested:    {},
	},
	OperationCancelRequested: {
		OperationCancelled:          {},
		OperationSucceeded:          {},
		OperationPartiallySucceeded: {},
		OperationFailed:             {},
	},
}

var ErrInvalidTransition = errors.New("invalid operation state transition")

type Operation struct {
	ID              string          `json:"id"`
	ClassroomID     string          `json:"classroom_id"`
	ClassroomName   string          `json:"classroom_name"`
	Type            OperationType   `json:"type"`
	Status          OperationStatus `json:"status"`
	Reason          string          `json:"reason,omitempty"`
	RequestID       string          `json:"request_id"`
	IdempotencyKey  string          `json:"-"`
	Items           []OperationItem `json:"items"`
	Counts          OperationCounts `json:"counts"`
	ResourceVersion int64           `json:"resource_version"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	StartedAt       *time.Time      `json:"started_at"`
	CompletedAt     *time.Time      `json:"completed_at"`
}

type ItemStatus string

const (
	ItemQueued     ItemStatus = "QUEUED"
	ItemRunning    ItemStatus = "RUNNING"
	ItemWaitingPVE ItemStatus = "WAITING_PVE"
	ItemVerifying  ItemStatus = "VERIFYING"
	ItemSucceeded  ItemStatus = "SUCCEEDED"
	ItemFailed     ItemStatus = "FAILED"
	ItemSkipped    ItemStatus = "SKIPPED"
	ItemUnknown    ItemStatus = "UNKNOWN"
)

type OperationItem struct {
	ID          string     `json:"id"`
	OperationID string     `json:"operation_id"`
	SeatID      string     `json:"seat_id"`
	SeatLabel   string     `json:"seat_label"`
	DesktopID   string     `json:"desktop_id,omitempty"`
	ClusterID   string     `json:"cluster_id,omitempty"`
	PVEVMID     int        `json:"pve_vmid,omitempty"`
	TargetName  string     `json:"target_name"`
	Status      ItemStatus `json:"status"`
	UPID        string     `json:"upid,omitempty"`
	ErrorCode   string     `json:"error_code,omitempty"`
	Message     string     `json:"message,omitempty"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type OperationCounts struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Unknown   int `json:"unknown"`
}

func (operation *Operation) Transition(next OperationStatus, now time.Time) error {
	if operation.Status == next {
		return nil
	}
	allowed := operationTransitions[operation.Status]
	if _, ok := allowed[next]; !ok {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, operation.Status, next)
	}
	operation.Status = next
	operation.UpdatedAt = now.UTC()
	if operation.StartedAt == nil && next != OperationQueued {
		startedAt := now.UTC()
		operation.StartedAt = &startedAt
	}
	if next.Terminal() {
		completedAt := now.UTC()
		operation.CompletedAt = &completedAt
	}
	operation.RefreshCounts()
	return nil
}

func (operation *Operation) RefreshCounts() {
	counts := OperationCounts{Total: len(operation.Items)}
	for _, item := range operation.Items {
		switch item.Status {
		case ItemQueued:
			counts.Queued++
		case ItemRunning, ItemWaitingPVE, ItemVerifying:
			counts.Running++
		case ItemSucceeded:
			counts.Succeeded++
		case ItemFailed:
			counts.Failed++
		case ItemSkipped:
			counts.Skipped++
		case ItemUnknown:
			counts.Unknown++
		}
	}
	operation.Counts = counts
}
