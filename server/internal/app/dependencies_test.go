package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/config"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/pve"
)

func TestOpenPVEAdapterLoadsSecretFile(t *testing.T) {
	t.Parallel()
	secretFile := filepath.Join(t.TempDir(), "pve-token")
	if err := os.WriteFile(secretFile, []byte("secret-from-file\n"), 0o600); err != nil {
		t.Fatalf("write secret fixture: %v", err)
	}
	adapter, err := OpenPVEAdapter(config.Config{
		PVEAdapter:         "http",
		PVEClusterID:       "70000000-0000-4000-8000-000000000001",
		PVEManagedPool:     "cloud-classroom-managed",
		PVEBaseURL:         "https://pve.example.invalid:8006",
		PVETokenID:         "cloudclass@pve!controller",
		PVETokenSecretFile: secretFile,
	})
	if err != nil {
		t.Fatalf("open HTTP PVE adapter: %v", err)
	}
	if _, ok := adapter.(*pve.HTTPAdapter); !ok {
		t.Fatalf("unexpected adapter type %T", adapter)
	}
}

func TestReadSecretValueRejectsMissingOrEmptyFile(t *testing.T) {
	t.Parallel()
	if _, err := readSecretValue("", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing secret file must fail")
	}
	emptyFile := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(emptyFile, []byte("\n"), 0o600); err != nil {
		t.Fatalf("write empty secret fixture: %v", err)
	}
	if _, err := readSecretValue("", emptyFile); err == nil {
		t.Fatal("empty secret file must fail")
	}
	if _, err := readSecretValue("inline-secret", emptyFile); err == nil {
		t.Fatal("ambiguous inline and file secret sources must fail")
	}
}
