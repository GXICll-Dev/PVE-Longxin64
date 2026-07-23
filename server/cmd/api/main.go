package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/app"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/config"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/httpapi"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/operations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("API process stopped", "error", err)
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

	signalContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(signalContext)
	defer cancel()

	repository, err := app.OpenRepository(ctx, cfg)
	if err != nil {
		return err
	}
	defer repository.Close()
	adapter, err := app.OpenPVEAdapter(cfg)
	if err != nil {
		return err
	}
	runner := operations.NewRunner(repository, adapter, logger, operations.RunnerConfig{
		WaveSize:      cfg.OperationWaveSize,
		PollInterval:  cfg.WorkerPollInterval,
		Lease:         cfg.WorkerLease,
		SubmitTimeout: cfg.PVERequestTimeout,
		TaskTimeout:   cfg.PVETaskTimeout,
	})
	manager := operations.NewManager(repository, runner)
	handler := httpapi.NewHandler(httpapi.Options{
		Repository:     repository,
		Operations:     manager,
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
	})
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	workerDone := make(chan error, 1)
	if cfg.EmbeddedWorker {
		go func() { workerDone <- runner.Run(ctx) }()
	}
	serverDone := make(chan error, 1)
	go func() {
		logger.Info("API listening",
			"address", cfg.HTTPAddress,
			"environment", cfg.Environment,
			"store_driver", cfg.StoreDriver,
			"embedded_worker", cfg.EmbeddedWorker,
		)
		serverDone <- httpServer.ListenAndServe()
	}()

	var processErr error
	workerExited := false
	select {
	case <-signalContext.Done():
		logger.Info("shutdown signal received")
	case processErr = <-serverDone:
		if processErr != nil && !errors.Is(processErr, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", processErr)
		}
	case processErr = <-workerDone:
		workerExited = true
		if processErr == nil {
			processErr = errors.New("embedded worker stopped unexpectedly")
		}
		logger.Error("embedded worker failed", "error", processErr)
	}
	cancel()

	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("gracefully shut down API: %w", err)
	}
	if cfg.EmbeddedWorker && !workerExited {
		select {
		case workerErr := <-workerDone:
			if workerErr != nil && processErr == nil {
				processErr = workerErr
			}
		case <-shutdownContext.Done():
			return fmt.Errorf("wait for embedded worker shutdown: %w", shutdownContext.Err())
		}
	}
	logger.Info("API shutdown complete")
	if processErr != nil && !errors.Is(processErr, http.ErrServerClosed) {
		return processErr
	}
	return nil
}
