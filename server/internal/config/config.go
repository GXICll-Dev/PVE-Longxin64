package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentProduction  Environment = "production"
)

type Config struct {
	Environment        Environment
	HTTPAddress        string
	LogLevel           slog.Level
	StoreDriver        string
	DatabaseURL        string
	PVEAdapter         string
	EmbeddedWorker     bool
	OperationWaveSize  int
	WorkerPollInterval time.Duration
	WorkerLease        time.Duration
	FakePVEDelay       time.Duration
	PVERequestTimeout  time.Duration
	PVETaskTimeout     time.Duration
	ShutdownTimeout    time.Duration
	ReadHeaderTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	AllowedOrigins     []string
}

func Load() (Config, error) {
	environment := Environment(strings.ToLower(getenv("PVE_ENVIRONMENT", string(EnvironmentDevelopment))))
	if environment != EnvironmentDevelopment && environment != EnvironmentTest && environment != EnvironmentProduction {
		return Config{}, fmt.Errorf("PVE_ENVIRONMENT must be development, test, or production")
	}

	storeDriver := strings.ToLower(strings.TrimSpace(os.Getenv("PVE_STORE_DRIVER")))
	if storeDriver == "" && environment != EnvironmentProduction {
		storeDriver = "memory"
	}
	if storeDriver != "memory" && storeDriver != "postgres" {
		return Config{}, errors.New("PVE_STORE_DRIVER must be memory or postgres")
	}
	if environment == EnvironmentProduction && storeDriver == "memory" {
		return Config{}, errors.New("production cannot use the in-memory store; configure PVE_STORE_DRIVER=postgres")
	}
	databaseURL := strings.TrimSpace(os.Getenv("PVE_DATABASE_URL"))
	if storeDriver == "postgres" && databaseURL == "" {
		return Config{}, errors.New("PVE_DATABASE_URL is required for the postgres store")
	}

	pveAdapter := strings.ToLower(strings.TrimSpace(os.Getenv("PVE_ADAPTER")))
	if pveAdapter == "" && environment != EnvironmentProduction {
		pveAdapter = "fake"
	}
	if pveAdapter != "fake" {
		return Config{}, errors.New("this build supports PVE_ADAPTER=fake only; a production PVE adapter must be configured in a later integration phase")
	}
	if environment == EnvironmentProduction && pveAdapter == "fake" {
		return Config{}, errors.New("production cannot use the Fake PVE adapter")
	}

	logLevel, err := parseLogLevel(getenv("PVE_LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	operationWaveSize, err := positiveInt("PVE_OPERATION_WAVE_SIZE", 10)
	if err != nil {
		return Config{}, err
	}
	workerPollInterval, err := duration("PVE_WORKER_POLL_INTERVAL", time.Second)
	if err != nil {
		return Config{}, err
	}
	workerLease, err := duration("PVE_WORKER_LEASE", 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	fakePVEDelay, err := duration("PVE_FAKE_DELAY", 25*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	pveRequestTimeout, err := duration("PVE_REQUEST_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	pveTaskTimeout, err := duration("PVE_TASK_TIMEOUT", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := duration("PVE_SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	readHeaderTimeout, err := duration("PVE_HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := duration("PVE_HTTP_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := duration("PVE_HTTP_WRITE_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := duration("PVE_HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	embeddedWorker, err := boolean("PVE_EMBEDDED_WORKER", environment != EnvironmentProduction)
	if err != nil {
		return Config{}, err
	}
	if environment == EnvironmentProduction && embeddedWorker {
		return Config{}, errors.New("production API must not embed the worker; run cmd/worker separately")
	}

	allowedOriginsDefault := ""
	if environment != EnvironmentProduction {
		allowedOriginsDefault = "http://localhost:5173"
	}
	return Config{
		Environment:        environment,
		HTTPAddress:        getenv("PVE_HTTP_ADDRESS", ":8080"),
		LogLevel:           logLevel,
		StoreDriver:        storeDriver,
		DatabaseURL:        databaseURL,
		PVEAdapter:         pveAdapter,
		EmbeddedWorker:     embeddedWorker,
		OperationWaveSize:  operationWaveSize,
		WorkerPollInterval: workerPollInterval,
		WorkerLease:        workerLease,
		FakePVEDelay:       fakePVEDelay,
		PVERequestTimeout:  pveRequestTimeout,
		PVETaskTimeout:     pveTaskTimeout,
		ShutdownTimeout:    shutdownTimeout,
		ReadHeaderTimeout:  readHeaderTimeout,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
		IdleTimeout:        idleTimeout,
		AllowedOrigins:     commaSeparated(getenv("PVE_ALLOWED_ORIGINS", allowedOriginsDefault)),
	}, nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func positiveInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", key)
	}
	return parsed, nil
}

func boolean(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func commaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, errors.New("PVE_LOG_LEVEL must be debug, info, warn, or error")
	}
}
