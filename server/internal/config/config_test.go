package config

import "testing"

func TestProductionRejectsMemoryStore(t *testing.T) {
	t.Setenv("PVE_ENVIRONMENT", "production")
	t.Setenv("PVE_STORE_DRIVER", "memory")
	t.Setenv("PVE_ADAPTER", "fake")
	if _, err := Load(); err == nil {
		t.Fatal("production must reject the in-memory store")
	}
}

func TestDevelopmentDefaultsAreRunnable(t *testing.T) {
	t.Setenv("PVE_ENVIRONMENT", "development")
	t.Setenv("PVE_STORE_DRIVER", "")
	t.Setenv("PVE_ADAPTER", "")
	config, err := Load()
	if err != nil {
		t.Fatalf("load development configuration: %v", err)
	}
	if config.StoreDriver != "memory" || config.PVEAdapter != "fake" || !config.EmbeddedWorker {
		t.Fatalf("unexpected development defaults: %+v", config)
	}
}
