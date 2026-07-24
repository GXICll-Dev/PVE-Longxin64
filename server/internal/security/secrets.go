// Package security contains security primitives shared by control-plane
// adapters and persistence code. It deliberately exposes no logging hooks so
// plaintext credentials cannot accidentally enter structured logs.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const envelopeVersion = "v1"

var (
	ErrInvalidEnvelope = errors.New("invalid encrypted secret envelope")
	ErrDecryptSecret   = errors.New("encrypted secret could not be decrypted")
)

// Keyring encrypts new values with one primary key while retaining previous
// keys for online rotation. Keys must be exactly 32 bytes (AES-256).
type Keyring struct {
	primaryID string
	keys      map[string][]byte
	random    io.Reader
}

func NewKeyring(primaryID string, keys map[string][]byte) (*Keyring, error) {
	if err := validateKeyID(primaryID); err != nil {
		return nil, fmt.Errorf("primary key id: %w", err)
	}
	if len(keys) == 0 {
		return nil, errors.New("at least one encryption key is required")
	}
	copied := make(map[string][]byte, len(keys))
	for id, key := range keys {
		if err := validateKeyID(id); err != nil {
			return nil, fmt.Errorf("encryption key id: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("encryption key %q must be exactly 32 bytes", id)
		}
		copied[id] = append([]byte(nil), key...)
	}
	if _, ok := copied[primaryID]; !ok {
		return nil, errors.New("primary encryption key is not present in the keyring")
	}
	return &Keyring{primaryID: primaryID, keys: copied, random: rand.Reader}, nil
}

// Seal encrypts a secret and binds it to associatedData, such as
// "pve-token:<cluster-id>". Associated data is authenticated but not stored in
// the envelope, so callers must supply the same value when decrypting.
func (keyring *Keyring) Seal(secret, associatedData []byte) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("secret must not be empty")
	}
	if len(associatedData) == 0 {
		return "", errors.New("associated data must not be empty")
	}
	aead, err := keyring.aead(keyring.primaryID)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(keyring.random, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, secret, associatedData)
	payload := append(nonce, sealed...)
	return strings.Join([]string{
		envelopeVersion,
		base64.RawURLEncoding.EncodeToString([]byte(keyring.primaryID)),
		base64.RawURLEncoding.EncodeToString(payload),
	}, ":"), nil
}

// Open authenticates and decrypts an envelope without exposing its plaintext
// or payload in returned errors.
func (keyring *Keyring) Open(envelope string, associatedData []byte) ([]byte, error) {
	if len(associatedData) == 0 {
		return nil, errors.New("associated data must not be empty")
	}
	keyID, payload, err := parseEnvelope(envelope)
	if err != nil {
		return nil, err
	}
	aead, err := keyring.aead(keyID)
	if err != nil {
		return nil, ErrDecryptSecret
	}
	if len(payload) <= aead.NonceSize() {
		return nil, ErrInvalidEnvelope
	}
	nonce, ciphertext := payload[:aead.NonceSize()], payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, ErrDecryptSecret
	}
	return plaintext, nil
}

// NeedsRotation reports whether an otherwise parseable envelope was encrypted
// with a non-primary key. Malformed or unknown-key envelopes return true.
func (keyring *Keyring) NeedsRotation(envelope string) bool {
	keyID, _, err := parseEnvelope(envelope)
	if err != nil {
		return true
	}
	_, known := keyring.keys[keyID]
	return !known || keyID != keyring.primaryID
}

func (keyring *Keyring) aead(keyID string) (cipher.AEAD, error) {
	key, ok := keyring.keys[keyID]
	if !ok {
		return nil, ErrDecryptSecret
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("initialize secret cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("initialize authenticated secret cipher")
	}
	return aead, nil
}

func parseEnvelope(envelope string) (string, []byte, error) {
	parts := strings.Split(envelope, ":")
	if len(parts) != 3 || parts[0] != envelopeVersion {
		return "", nil, ErrInvalidEnvelope
	}
	keyIDBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || validateKeyID(string(keyIDBytes)) != nil {
		return "", nil, ErrInvalidEnvelope
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(payload) == 0 {
		return "", nil, ErrInvalidEnvelope
	}
	return string(keyIDBytes), payload, nil
}

func validateKeyID(value string) error {
	if value == "" || len(value) > 64 {
		return errors.New("must contain between 1 and 64 characters")
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-", character) {
			continue
		}
		return errors.New("may contain only letters, digits, dot, underscore, or hyphen")
	}
	return nil
}
