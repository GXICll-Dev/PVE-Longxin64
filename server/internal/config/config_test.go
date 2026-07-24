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

func TestProductionAcceptsHTTPAdapter(t *testing.T) {
	t.Setenv("PVE_ENVIRONMENT", "production")
	t.Setenv("PVE_STORE_DRIVER", "postgres")
	t.Setenv("PVE_DATABASE_URL", "postgres://example.invalid/classroom")
	t.Setenv("PVE_ADAPTER", "http")
	t.Setenv("PVE_CLUSTER_ID", "70000000-0000-4000-8000-000000000001")
	t.Setenv("PVE_MANAGED_POOL", "cloud-classroom-managed")
	t.Setenv("PVE_BASE_URL", "https://pve.example.invalid:8006")
	t.Setenv("PVE_TOKEN_ID", "cloudclass@pve!controller")
	t.Setenv("PVE_TOKEN_SECRET", "test-secret")
	t.Setenv("PVE_TOKEN_SECRET_FILE", "")
	t.Setenv("PVE_EMBEDDED_WORKER", "false")

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load production HTTP adapter config: %v", err)
	}
	if loaded.PVEAdapter != "http" || loaded.PVEClusterID == "" || loaded.PVEManagedPool == "" || loaded.PVEBaseURL == "" || loaded.PVETokenID == "" {
		t.Fatalf("unexpected HTTP adapter config: %+v", loaded)
	}
}

func TestHTTPAdapterSettingsCanBeDeferredForStandaloneAPI(t *testing.T) {
	t.Setenv("PVE_ENVIRONMENT", "development")
	t.Setenv("PVE_ADAPTER", "http")
	t.Setenv("PVE_CLUSTER_ID", "")
	t.Setenv("PVE_MANAGED_POOL", "")
	t.Setenv("PVE_BASE_URL", "")
	t.Setenv("PVE_TOKEN_ID", "")
	t.Setenv("PVE_TOKEN_SECRET", "")
	t.Setenv("PVE_TOKEN_SECRET_FILE", "")
	t.Setenv("PVE_EMBEDDED_WORKER", "false")
	loaded, err := Load()
	if err != nil {
		t.Fatalf("standalone API must load without PVE credentials: %v", err)
	}
	if loaded.PVEAdapter != "http" || loaded.EmbeddedWorker {
		t.Fatalf("unexpected standalone API config: %+v", loaded)
	}
}

func TestProductionRejectsFakeAdapter(t *testing.T) {
	t.Setenv("PVE_ENVIRONMENT", "production")
	t.Setenv("PVE_STORE_DRIVER", "postgres")
	t.Setenv("PVE_DATABASE_URL", "postgres://example.invalid/classroom")
	t.Setenv("PVE_ADAPTER", "fake")
	t.Setenv("PVE_EMBEDDED_WORKER", "false")
	if _, err := Load(); err == nil {
		t.Fatal("production must reject the Fake PVE adapter")
	}
}
