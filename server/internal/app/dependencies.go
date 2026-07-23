package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/config"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/pve"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

func NewLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func OpenRepository(ctx context.Context, cfg config.Config) (store.Repository, error) {
	switch cfg.StoreDriver {
	case "memory":
		return store.NewDevelopmentRepository(time.Now()), nil
	case "postgres":
		return store.NewPostgresRepository(ctx, cfg.DatabaseURL)
	default:
		return nil, fmt.Errorf("unsupported store driver %q", cfg.StoreDriver)
	}
}

func OpenPVEAdapter(cfg config.Config) (pve.Adapter, error) {
	switch cfg.PVEAdapter {
	case "fake":
		return pve.NewFakeAdapter(cfg.FakePVEDelay), nil
	default:
		return nil, fmt.Errorf("unsupported PVE adapter %q", cfg.PVEAdapter)
	}
}
