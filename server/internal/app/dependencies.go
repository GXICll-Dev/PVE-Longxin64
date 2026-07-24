package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
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
	case "http":
		tokenSecret, err := readSecretValue(cfg.PVETokenSecret, cfg.PVETokenSecretFile)
		if err != nil {
			return nil, err
		}
		var caCertificate []byte
		if cfg.PVECACertFile != "" {
			caCertificate, err = os.ReadFile(cfg.PVECACertFile)
			if err != nil {
				return nil, fmt.Errorf("read PVE CA certificate file: %w", err)
			}
		}
		return pve.NewHTTPAdapter(pve.HTTPConfig{
			BaseURL:          cfg.PVEBaseURL,
			ClusterID:        cfg.PVEClusterID,
			ManagedPool:      cfg.PVEManagedPool,
			TokenID:          cfg.PVETokenID,
			TokenSecret:      tokenSecret,
			CACertificatePEM: caCertificate,
			RequestTimeout:   cfg.PVERequestTimeout,
			TaskTimeout:      cfg.PVETaskTimeout,
			TaskPollInterval: cfg.PVETaskPollInterval,
		})
	default:
		return nil, fmt.Errorf("unsupported PVE adapter %q", cfg.PVEAdapter)
	}
}

func readSecretValue(inlineValue, filename string) (string, error) {
	value := strings.TrimSpace(inlineValue)
	filename = strings.TrimSpace(filename)
	if value != "" && filename != "" {
		return "", errors.New("configure only one of PVE_TOKEN_SECRET or PVE_TOKEN_SECRET_FILE")
	}
	if filename == "" {
		if value == "" {
			return "", errors.New("PVE token secret is empty")
		}
		return value, nil
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("read PVE token secret file: %w", err)
	}
	value = strings.TrimSpace(string(contents))
	if value == "" {
		return "", errors.New("PVE token secret file is empty")
	}
	return value, nil
}
