package pve

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

type FakeAdapter struct {
	mu       sync.RWMutex
	delay    time.Duration
	tasks    map[string]Result
	failures map[string]Result
}

func NewFakeAdapter(delay time.Duration) *FakeAdapter {
	return &FakeAdapter{
		delay:    delay,
		tasks:    make(map[string]Result),
		failures: make(map[string]Result),
	}
}

// FailSeat configures a deterministic per-seat failure for integration tests
// and development demonstrations. No failures are enabled by default.
func (adapter *FakeAdapter) FailSeat(seatID, code, message string) {
	adapter.mu.Lock()
	adapter.failures[seatID] = Result{Succeeded: false, Code: code, Message: message}
	adapter.mu.Unlock()
}

func (adapter *FakeAdapter) Submit(ctx context.Context, request Request) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !request.Type.Valid() {
		return "", &Error{Code: ErrorInvalid, Message: "不支持的 PVE 操作类型"}
	}
	upid := fmt.Sprintf("UPID:fake:%s:%s", request.OperationID, request.ItemID)
	result := Result{Succeeded: true, Code: "OK", Message: fakeSuccessMessage(request.Type)}
	adapter.mu.Lock()
	if failure, ok := adapter.failures[request.SeatID]; ok {
		result = failure
	}
	adapter.tasks[upid] = result
	adapter.mu.Unlock()
	return upid, nil
}

func (adapter *FakeAdapter) Wait(ctx context.Context, upid string) (Result, error) {
	timer := time.NewTimer(adapter.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-timer.C:
	}
	adapter.mu.RLock()
	result, ok := adapter.tasks[upid]
	adapter.mu.RUnlock()
	if !ok {
		return Result{}, &Error{Code: ErrorUnknownTask, Retryable: true, Message: "Fake PVE 任务不存在或已过期"}
	}
	return result, nil
}

func fakeSuccessMessage(operationType domain.OperationType) string {
	switch operationType {
	case domain.OperationPrecheck:
		return "课前检查通过"
	case domain.OperationStart:
		return "虚拟桌面已启动"
	case domain.OperationShutdown:
		return "虚拟桌面已优雅关机"
	case domain.OperationRestore:
		return "虚拟桌面已回滚到基线快照"
	default:
		return "操作已完成"
	}
}
