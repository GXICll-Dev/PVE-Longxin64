package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/app"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/config"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/operations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker process stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := app.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	repository, err := app.OpenRepository(ctx, cfg)
	if err != nil {
		return err
	}
	defer repository.Close()
	adapter, err := app.OpenPVEAdapter(cfg)
	if err != nil {
		return err
	}
	if closer, ok := adapter.(interface{ CloseIdleConnections() }); ok {
		defer closer.CloseIdleConnections()
	}
	if cfg.StoreDriver == "memory" {
		logger.Warn("standalone worker is using an isolated in-memory store; use the API embedded worker for development or PostgreSQL for multi-process execution")
	}
	runner := operations.NewRunner(repository, adapter, logger, operations.RunnerConfig{
		WaveSize:      cfg.OperationWaveSize,
		PollInterval:  cfg.WorkerPollInterval,
		Lease:         cfg.WorkerLease,
		SubmitTimeout: cfg.PVERequestTimeout,
		TaskTimeout:   cfg.PVETaskTimeout,
	})
	if err := runner.Run(ctx); err != nil {
		return fmt.Errorf("run worker: %w", err)
	}
	logger.Info("worker shutdown complete")
	return nil
}
