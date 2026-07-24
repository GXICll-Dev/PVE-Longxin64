package pve

import (
	"context"
	"errors"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

type Request struct {
	CorrelationID string
	OperationID   string
	ItemID        string
	ClassroomID   string
	SeatID        string
	DesktopID     string
	ClusterID     string
	// PVENode is optional. The HTTP adapter resolves the node from the cluster
	// inventory when it is empty. Supplying it avoids the extra read request.
	PVENode string
	PVEVMID int
	// SnapshotName is required for snapshot rollback operations. It is kept in
	// the adapter request rather than inferred from a naming convention.
	SnapshotName string
	Type         domain.OperationType
}

type Result struct {
	Succeeded bool
	Code      string
	Message   string
}

type Adapter interface {
	Submit(context.Context, Request) (string, error)
	Wait(context.Context, string) (Result, error)
}

type ErrorCode string

const (
	ErrorUnavailable      ErrorCode = "PVE_UNAVAILABLE"
	ErrorAuthentication   ErrorCode = "PVE_AUTHENTICATION_FAILED"
	ErrorPermission       ErrorCode = "PVE_PERMISSION_DENIED"
	ErrorOutOfScope       ErrorCode = "PVE_RESOURCE_OUT_OF_SCOPE"
	ErrorInvalid          ErrorCode = "PVE_INVALID_REQUEST"
	ErrorNotFound         ErrorCode = "PVE_RESOURCE_NOT_FOUND"
	ErrorTLS              ErrorCode = "PVE_TLS_VALIDATION_FAILED"
	ErrorTimeout          ErrorCode = "PVE_TIMEOUT"
	ErrorCancelled        ErrorCode = "PVE_CANCELLED"
	ErrorUnknownTask      ErrorCode = "PVE_UNKNOWN_TASK"
	ErrorOutcomeUncertain ErrorCode = "PVE_OUTCOME_UNCERTAIN"
	ErrorTaskFailed       ErrorCode = "PVE_TASK_FAILED"
)

type Error struct {
	Code       ErrorCode
	Retryable  bool
	Message    string
	Cause      error
	HTTPStatus int
	// OutcomeCertain means the mutating PVE request is known not to have been
	// accepted. It lets callers safely distinguish a failed preflight or an
	// explicit rejection from a lost response after submission.
	OutcomeCertain bool
}

func (err *Error) Error() string {
	if err.Message != "" {
		return err.Message
	}
	return string(err.Code)
}

func (err *Error) Unwrap() error { return err.Cause }

func CodeOf(err error) string {
	var adapterError *Error
	if errors.As(err, &adapterError) {
		return string(adapterError.Code)
	}
	return string(ErrorUnavailable)
}

// OutcomeUncertain reports whether a failed submission may still have reached
// PVE. Callers must not blindly replay such a request without reconciliation.
func OutcomeUncertain(err error) bool {
	if err == nil {
		return false
	}
	var adapterError *Error
	if !errors.As(err, &adapterError) {
		return true
	}
	if adapterError.OutcomeCertain {
		return false
	}
	switch adapterError.Code {
	case ErrorUnavailable, ErrorTimeout, ErrorCancelled, ErrorUnknownTask, ErrorOutcomeUncertain:
		return true
	default:
		return false
	}
}
