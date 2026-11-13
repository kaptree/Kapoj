// Package secret provides AES-256-GCM encryption for secrets stored at rest
// (for example SMTP passwords) and a small key manager for their key material.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidKey indicates that the supplied key is not a valid 32-byte AES key.
var ErrInvalidKey = errors.New("invalid encryption key")

// ErrNotConfigured is returned when encryption is attempted without a key.
var ErrNotConfigured = errors.New("secret: key not configured")

const ciphertextVersion = "v1"

// Encryptor encrypts values with AES-256-GCM. It keeps the original base64 key
// so callers can display a masked value or compute a fingerprint.
type Encryptor struct {
	aead cipher.AEAD
	key  string
}

// NewEncryptor parses a base64-encoded key. The key must decode to 32 bytes.
func NewEncryptor(base64Key string) (*Encryptor, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: must decode to 32 bytes, got %d", ErrInvalidKey, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Encryptor{aead: aead, key: base64Key}, nil
}

// Key returns the original base64 key. It must only be used for masking and
// fingerprinting; never log or return it to clients.
func (e *Encryptor) Key() string { return e.key }

// Encrypt returns "v1:" + base64(nonce+ciphertext); the nonce is 12 bytes.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := e.aead.Seal(nil, nonce, []byte(plaintext), nil)
	blob := append(nonce, sealed...)
	return ciphertextVersion + ":" + base64.StdEncoding.EncodeToString(blob), nil
}

// Decrypt opens a value produced by Encrypt. A version mismatch or failed GCM
// authentication returns an error.
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	payload, ok := strings.CutPrefix(ciphertext, ciphertextVersion+":")
	if !ok {
		return "", errors.New("unsupported ciphertext version")
	}
	blob, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", err
	}
	nonceSize := e.aead.NonceSize()
	if len(blob) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, sealed := blob[:nonceSize], blob[nonceSize:]
	plain, err := e.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
