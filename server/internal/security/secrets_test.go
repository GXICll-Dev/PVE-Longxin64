package security

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestKeyringRoundTripAndRotation(t *testing.T) {
	t.Parallel()
	oldKey := bytes.Repeat([]byte{0x11}, 32)
	newKey := bytes.Repeat([]byte{0x22}, 32)
	oldRing, err := NewKeyring("old", map[string][]byte{"old": oldKey})
	if err != nil {
		t.Fatalf("create old keyring: %v", err)
	}
	associatedData := []byte("pve-token:cluster-1")
	secret := []byte("PVEAPIToken=teacher@pve!classroom=never-log-this")
	oldEnvelope, err := oldRing.Seal(secret, associatedData)
	if err != nil {
		t.Fatalf("seal with old key: %v", err)
	}
	if strings.Contains(oldEnvelope, string(secret)) {
		t.Fatal("encrypted envelope contains plaintext secret")
	}

	rotatedRing, err := NewKeyring("new", map[string][]byte{"old": oldKey, "new": newKey})
	if err != nil {
		t.Fatalf("create rotated keyring: %v", err)
	}
	plaintext, err := rotatedRing.Open(oldEnvelope, associatedData)
	if err != nil {
		t.Fatalf("open old envelope during rotation: %v", err)
	}
	if !bytes.Equal(plaintext, secret) {
		t.Fatalf("unexpected plaintext %q", plaintext)
	}
	if !rotatedRing.NeedsRotation(oldEnvelope) {
		t.Fatal("old envelope should require rotation")
	}

	newEnvelope, err := rotatedRing.Seal(secret, associatedData)
	if err != nil {
		t.Fatalf("seal with primary key: %v", err)
	}
	if rotatedRing.NeedsRotation(newEnvelope) {
		t.Fatal("new envelope should use the primary key")
	}
}

func TestKeyringRejectsTamperingAndWrongContext(t *testing.T) {
	t.Parallel()
	keyring, err := NewKeyring("primary", map[string][]byte{"primary": bytes.Repeat([]byte{0x44}, 32)})
	if err != nil {
		t.Fatalf("create keyring: %v", err)
	}
	envelope, err := keyring.Seal([]byte("secret"), []byte("pve-token:cluster-1"))
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	if _, err := keyring.Open(envelope, []byte("pve-token:cluster-2")); !errors.Is(err, ErrDecryptSecret) {
		t.Fatalf("wrong associated data must fail authentication, got %v", err)
	}

	parts := strings.Split(envelope, ":")
	payload, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode envelope payload: %v", err)
	}
	payload[len(payload)-1] ^= 0x01
	parts[2] = base64.RawURLEncoding.EncodeToString(payload)
	tampered := strings.Join(parts, ":")
	if _, err := keyring.Open(tampered, []byte("pve-token:cluster-1")); err == nil {
		t.Fatal("tampered envelope must not decrypt")
	}
}

func TestKeyringValidatesConfigurationAndEnvelope(t *testing.T) {
	t.Parallel()
	if _, err := NewKeyring("missing", map[string][]byte{"other": bytes.Repeat([]byte{1}, 32)}); err == nil {
		t.Fatal("primary key must exist")
	}
	if _, err := NewKeyring("primary", map[string][]byte{"primary": []byte("too-short")}); err == nil {
		t.Fatal("non-AES-256 key must be rejected")
	}
	if _, err := NewKeyring("bad:key", map[string][]byte{"bad:key": bytes.Repeat([]byte{1}, 32)}); err == nil {
		t.Fatal("unsafe key id must be rejected")
	}

	keyring, err := NewKeyring("primary", map[string][]byte{"primary": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatalf("create keyring: %v", err)
	}
	if _, err := keyring.Open("not-an-envelope", []byte("context")); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("malformed envelope returned %v", err)
	}
	if _, err := keyring.Seal([]byte("secret"), nil); err == nil {
		t.Fatal("associated data must be required")
	}
}
