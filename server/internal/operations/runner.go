package operations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/pve"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

type RunnerConfig struct {
	WorkerID      string
	WaveSize      int
	PollInterval  time.Duration
	Lease         time.Duration
	SubmitTimeout time.Duration
	TaskTimeout   time.Duration
}

type Runner struct {
	repository store.Repository
	adapter    pve.Adapter
	logger     *slog.Logger
	config     RunnerConfig
	wake       chan struct{}
	now        func() time.Time
}

func NewRunner(repository store.Repository, adapter pve.Adapter, logger *slog.Logger, config RunnerConfig) *Runner {
	if config.WorkerID == "" {
		config.WorkerID = "worker-" + domain.NewID()
	}
	if config.WaveSize <= 0 {
		config.WaveSize = 10
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.Lease <= 0 {
		config.Lease = 2 * time.Minute
	}
	if config.SubmitTimeout <= 0 {
		config.SubmitTimeout = 15 * time.Second
	}
	if config.TaskTimeout <= 0 {
		config.TaskTimeout = 30 * time.Minute
	}
	return &Runner{
		repository: repository,
		adapter:    adapter,
		logger:     logger,
		config:     config,
		wake:       make(chan struct{}, 1),
		now:        time.Now,
	}
}

func (runner *Runner) Notify() {
	select {
	case runner.wake <- struct{}{}:
	default:
	}
}

func (runner *Runner) Run(ctx context.Context) error {
	runner.logger.InfoContext(ctx, "operation worker started", "worker_id", runner.config.WorkerID, "wave_size", runner.config.WaveSize)
	ticker := time.NewTicker(runner.config.PollInterval)
	defer ticker.Stop()
	for {
		if err := runner.drain(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			runner.logger.ErrorContext(ctx, "operation worker iteration failed", "worker_id", runner.config.WorkerID, "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-runner.wake:
		case <-ticker.C:
		}
	}
}

func (runner *Runner) drain(ctx context.Context) error {
	for {
		operation, err := runner.repository.ClaimNextOperation(ctx, runner.config.WorkerID, runner.config.Lease)
		if err != nil {
			return err
		}
		if operation == nil {
			return nil
		}
		if err := runner.execute(ctx, operation); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			runner.logger.ErrorContext(ctx, "operation execution failed",
				"operation_id", operation.ID,
				"classroom_id", operation.ClassroomID,
				"request_id", operation.RequestID,
				"error", err,
			)
			if failErr := runner.fail(operation, err); failErr != nil {
				return fmt.Errorf("save failed operation after %v: %w", err, failErr)
			}
		}
	}
}

func (runner *Runner) execute(ctx context.Context, operation *domain.Operation) error {
	logger := runner.logger.With(
		"operation_id", operation.ID,
		"classroom_id", operation.ClassroomID,
		"request_id", operation.RequestID,
		"operation_type", operation.Type,
	)
	logger.InfoContext(ctx, "operation claimed", "status", operation.Status)
	if err := runner.repository.RenewOperationLease(ctx, operation.ID, runner.config.WorkerID, runner.config.Lease); err != nil {
		return err
	}

	if operation.Status == domain.OperationQueued {
		if err := operation.Transition(domain.OperationValidating, runner.now()); err != nil {
			return err
		}
		if err := runner.repository.SaveOperation(ctx, operation); err != nil {
			return err
		}
	}
	if operation.Status == domain.OperationValidating {
		for index := range operation.Items {
			item := &operation.Items[index]
			if operation.Type != domain.OperationPrecheck && item.DesktopID == "" {
				now := runner.now().UTC()
				item.Status = domain.ItemFailed
				item.ErrorCode = "DESKTOP_NOT_ASSIGNED"
				item.Message = "座位未分配虚拟桌面"
				item.CompletedAt = &now
				item.UpdatedAt = now
				if err := runner.repository.ApplyOperationItemResult(ctx, operation.ClassroomID, item.SeatID, operation.Type, domain.ItemFailed); err != nil {
					return err
				}
				continue
			}
			if operation.Type == domain.OperationRestore && item.SnapshotName == "" {
				now := runner.now().UTC()
				item.Status = domain.ItemFailed
				item.ErrorCode = "BASELINE_SNAPSHOT_NOT_CONFIGURED"
				item.Message = "虚拟桌面未配置可回滚的基线快照"
				item.CompletedAt = &now
				item.UpdatedAt = now
				if err := runner.repository.ApplyOperationItemResult(ctx, operation.ClassroomID, item.SeatID, operation.Type, domain.ItemFailed); err != nil {
					return err
				}
			}
		}
		if err := operation.Transition(domain.OperationRunning, runner.now()); err != nil {
			return err
		}
		if err := runner.repository.SaveOperation(ctx, operation); err != nil {
			return err
		}
	}
	if operation.Status == domain.OperationVerifying {
		return runner.finalize(ctx, operation)
	}

	pending := pendingItemIndexes(operation.Items)
	for offset := 0; offset < len(pending); offset += runner.config.WaveSize {
		if err := runner.repository.RenewOperationLease(ctx, operation.ID, runner.config.WorkerID, runner.config.Lease); err != nil {
			return err
		}
		end := min(offset+runner.config.WaveSize, len(pending))
		wave := pending[offset:end]
		if operation.Status != domain.OperationRunning {
			if err := operation.Transition(domain.OperationRunning, runner.now()); err != nil {
				return err
			}
		}
		now := runner.now().UTC()
		for _, itemIndex := range wave {
			item := &operation.Items[itemIndex]
			if item.Status == domain.ItemQueued || item.Status == domain.ItemRunning {
				item.Status = domain.ItemRunning
				item.StartedAt = &now
				item.UpdatedAt = now
				if err := runner.repository.ApplyOperationItemResult(ctx, operation.ClassroomID, item.SeatID, operation.Type, domain.ItemRunning); err != nil {
					return err
				}
			}
		}
		operation.RefreshCounts()
		if err := runner.repository.SaveOperation(ctx, operation); err != nil {
			return err
		}
		logger.InfoContext(ctx, "operation wave started", "wave_start", offset+1, "wave_size", len(wave), "item_total", len(operation.Items))

		for _, itemIndex := range wave {
			item := &operation.Items[itemIndex]
			if item.UPID != "" {
				item.Status = domain.ItemWaitingPVE
				continue
			}
			if err := runner.repository.RenewOperationLease(ctx, operation.ID, runner.config.WorkerID, runner.config.Lease); err != nil {
				return err
			}
			submitContext, cancelSubmit := context.WithTimeout(ctx, runner.config.SubmitTimeout)
			upid, err := runner.adapter.Submit(submitContext, pve.Request{
				CorrelationID: operation.RequestID,
				OperationID:   operation.ID,
				ItemID:        item.ID,
				ClassroomID:   operation.ClassroomID,
				SeatID:        item.SeatID,
				DesktopID:     item.DesktopID,
				ClusterID:     item.ClusterID,
				PVEVMID:       item.PVEVMID,
				SnapshotName:  item.SnapshotName,
				Type:          operation.Type,
			})
			cancelSubmit()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				itemStatus := domain.ItemFailed
				message := "PVE 未受理该操作"
				if errors.Is(err, context.DeadlineExceeded) || pve.OutcomeUncertain(err) {
					itemStatus = domain.ItemUnknown
					message = "无法确认 PVE 是否受理该操作"
				}
				runner.markItemTerminal(item, itemStatus, pve.CodeOf(err), message)
				if applyErr := runner.repository.ApplyOperationItemResult(ctx, operation.ClassroomID, item.SeatID, operation.Type, itemStatus); applyErr != nil {
					return applyErr
				}
				if saveErr := runner.repository.SaveOperation(ctx, operation); saveErr != nil {
					return saveErr
				}
				continue
			}
			item.UPID = upid
			item.Status = domain.ItemWaitingPVE
			item.UpdatedAt = runner.now().UTC()
			// Persist the UPID immediately. A worker restart must track this task
			// instead of blindly submitting the destructive action again.
			if err := runner.repository.SaveOperation(ctx, operation); err != nil {
				return err
			}
		}
		if operation.Status == domain.OperationRunning {
			if err := operation.Transition(domain.OperationWaitingPVE, runner.now()); err != nil {
				return err
			}
			if err := runner.repository.SaveOperation(ctx, operation); err != nil {
				return err
			}
		}

		for _, itemIndex := range wave {
			item := &operation.Items[itemIndex]
			switch item.Status {
			case domain.ItemFailed, domain.ItemUnknown, domain.ItemSkipped, domain.ItemSucceeded:
				continue
			}
			taskContext, cancelTask := context.WithTimeout(ctx, runner.config.TaskTimeout)
			result, err := runner.waitForPVE(taskContext, operation.ID, item.UPID)
			cancelTask()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				runner.markItemTerminal(item, domain.ItemUnknown, pve.CodeOf(err), "无法确认 PVE 任务结果")
				if applyErr := runner.repository.ApplyOperationItemResult(ctx, operation.ClassroomID, item.SeatID, operation.Type, domain.ItemUnknown); applyErr != nil {
					return applyErr
				}
			} else {
				item.Status = domain.ItemVerifying
				item.UpdatedAt = runner.now().UTC()
				if err := runner.repository.SaveOperation(ctx, operation); err != nil {
					return err
				}
				completedAt := runner.now().UTC()
				item.CompletedAt = &completedAt
				item.UpdatedAt = completedAt
				item.Message = result.Message
				if result.Succeeded {
					item.Status = domain.ItemSucceeded
				} else {
					item.Status = domain.ItemFailed
					item.ErrorCode = result.Code
				}
				if applyErr := runner.repository.ApplyOperationItemResult(ctx, operation.ClassroomID, item.SeatID, operation.Type, item.Status); applyErr != nil {
					return applyErr
				}
			}
			if err := runner.repository.SaveOperation(ctx, operation); err != nil {
				return err
			}
		}
		logger.InfoContext(ctx, "operation wave completed", "wave_start", offset+1, "wave_size", len(wave))
	}

	if operation.Status != domain.OperationVerifying {
		if err := operation.Transition(domain.OperationVerifying, runner.now()); err != nil {
			return err
		}
		if err := runner.repository.SaveOperation(ctx, operation); err != nil {
			return err
		}
	}
	return runner.finalize(ctx, operation)
}

type waitResult struct {
	result pve.Result
	err    error
}

func (runner *Runner) waitForPVE(ctx context.Context, operationID, upid string) (pve.Result, error) {
	waitContext, cancel := context.WithCancel(ctx)
	defer cancel()
	resultChannel := make(chan waitResult, 1)
	go func() {
		result, err := runner.adapter.Wait(waitContext, upid)
		resultChannel <- waitResult{result: result, err: err}
	}()
	heartbeatInterval := runner.config.Lease / 3
	if heartbeatInterval > 30*time.Second {
		heartbeatInterval = 30 * time.Second
	}
	if heartbeatInterval < time.Second {
		heartbeatInterval = time.Second
	}
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return pve.Result{}, ctx.Err()
		case completed := <-resultChannel:
			return completed.result, completed.err
		case <-ticker.C:
			if err := runner.repository.RenewOperationLease(ctx, operationID, runner.config.WorkerID, runner.config.Lease); err != nil {
				cancel()
				return pve.Result{}, err
			}
		}
	}
}

func (runner *Runner) finalize(ctx context.Context, operation *domain.Operation) error {
	operation.RefreshCounts()
	var terminal domain.OperationStatus
	switch {
	case operation.Counts.Failed > 0 || operation.Counts.Unknown > 0:
		if operation.Counts.Succeeded > 0 || operation.Counts.Skipped > 0 {
			terminal = domain.OperationPartiallySucceeded
		} else {
			terminal = domain.OperationFailed
		}
	default:
		terminal = domain.OperationSucceeded
	}
	if err := operation.Transition(terminal, runner.now()); err != nil {
		return err
	}
	if err := runner.repository.SaveOperation(ctx, operation); err != nil {
		return err
	}
	runner.logger.InfoContext(ctx, "operation completed",
		"operation_id", operation.ID,
		"status", operation.Status,
		"succeeded", operation.Counts.Succeeded,
		"failed", operation.Counts.Failed,
		"unknown", operation.Counts.Unknown,
	)
	return nil
}

func (runner *Runner) fail(operation *domain.Operation, cause error) error {
	now := runner.now().UTC()
	for index := range operation.Items {
		item := &operation.Items[index]
		switch item.Status {
		case domain.ItemQueued, domain.ItemRunning, domain.ItemWaitingPVE, domain.ItemVerifying:
			item.Status = domain.ItemUnknown
			item.ErrorCode = "WORKER_EXECUTION_FAILED"
			item.Message = "Worker 无法确认操作结果"
			item.UpdatedAt = now
		}
	}
	if !operation.Status.Terminal() {
		if err := operation.Transition(domain.OperationFailed, now); err != nil {
			return err
		}
	}
	return runner.repository.SaveOperation(context.Background(), operation)
}

func (runner *Runner) markItemTerminal(item *domain.OperationItem, status domain.ItemStatus, code, message string) {
	now := runner.now().UTC()
	item.Status = status
	item.ErrorCode = code
	item.Message = message
	item.CompletedAt = &now
	item.UpdatedAt = now
}

func pendingItemIndexes(items []domain.OperationItem) []int {
	result := make([]int, 0, len(items))
	for index := range items {
		if items[index].Status == domain.ItemUnknown && items[index].UPID == "" {
			continue
		}
		if items[index].Status != domain.ItemSucceeded && items[index].Status != domain.ItemFailed && items[index].Status != domain.ItemSkipped {
			result = append(result, index)
		}
	}
	return result
}
