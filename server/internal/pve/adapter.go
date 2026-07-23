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
	PVEVMID       int
	Type          domain.OperationType
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
	ErrorUnavailable ErrorCode = "PVE_UNAVAILABLE"
	ErrorPermission  ErrorCode = "PVE_PERMISSION_DENIED"
	ErrorInvalid     ErrorCode = "PVE_INVALID_REQUEST"
	ErrorTimeout     ErrorCode = "PVE_TIMEOUT"
	ErrorUnknownTask ErrorCode = "PVE_UNKNOWN_TASK"
)

type Error struct {
	Code      ErrorCode
	Retryable bool
	Message   string
	Cause     error
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
	var adapterError *Error
	if !errors.As(err, &adapterError) {
		return true
	}
	switch adapterError.Code {
	case ErrorUnavailable, ErrorTimeout, ErrorUnknownTask:
		return true
	default:
		return false
	}
}
