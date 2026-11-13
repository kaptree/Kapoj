package secret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// KeyManager owns the current encryption key and tracks where it came from
// (env, file, or none). EnsureKey generates a file-backed key on first use.
type KeyManager struct {
	mu       sync.RWMutex
	enc      *Encryptor
	source   string
	filePath string
}

// NewKeyManager loads the key from the environment (preferred) or a local key
// file. An invalid env key is a startup error; an invalid file is treated as
// unconfigured so an operator can re-enter the secret in the admin UI.
func NewKeyManager(envKey, filePath string) (*KeyManager, error) {
	km := &KeyManager{source: "none", filePath: filePath}

	envKey = strings.TrimSpace(envKey)
	if envKey != "" {
		enc, err := NewEncryptor(envKey)
		if err != nil {
			return nil, fmt.Errorf("secret: invalid encryption key from env: %w", err)
		}
		km.enc = enc
		km.source = "env"
		return km, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("secret: cannot read key file", "path", filePath, "err", err)
		}
		return km, nil
	}

	raw := strings.TrimSpace(string(data))
	enc, err := NewEncryptor(raw)
	if err != nil {
		slog.Warn("secret: invalid key file", "path", filePath, "err", err)
		return km, nil
	}
	km.enc = enc
	km.source = "file"
	return km, nil
}

// Configured reports whether a usable key is loaded.
func (km *KeyManager) Configured() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.enc != nil
}

// Source returns "env", "file", or "none".
func (km *KeyManager) Source() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.source
}

// Encrypt encrypts plaintext with the current key.
func (km *KeyManager) Encrypt(plaintext string) (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.enc == nil {
		return "", ErrNotConfigured
	}
	return km.enc.Encrypt(plaintext)
}

// Decrypt decrypts ciphertext with the current key.
func (km *KeyManager) Decrypt(ciphertext string) (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.enc == nil {
		return "", ErrNotConfigured
	}
	return km.enc.Decrypt(ciphertext)
}

// EnsureKey is a no-op when a key is already configured. Otherwise it creates a
// random 32-byte key, writes it atomically with 0600 permissions, and switches
// the manager to file-backed mode.
func (km *KeyManager) EnsureKey() error {
	km.mu.Lock()
	defer km.mu.Unlock()
	if km.enc != nil {
		return nil
	}
	if km.filePath == "" {
		return errors.New("secret: key file path is not configured")
	}

	raw, err := generateKeyBase64()
	if err != nil {
		return err
	}
	if err := writeKeyFile(km.filePath, raw+"\n"); err != nil {
		return err
	}
	enc, err := NewEncryptor(raw)
	if err != nil {
		return err
	}
	km.enc = enc
	km.source = "file"
	return nil
}

func generateKeyBase64() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("secret: generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func writeKeyFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("secret: create key directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("secret: write key file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("secret: install key file: %w", err)
	}
	return nil
}
