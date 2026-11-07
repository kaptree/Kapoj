package ai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidKey 表示加密密钥无法解析为合法的 32 字节 AES 密钥。
var ErrInvalidKey = errors.New("invalid encryption key")

const ciphertextVersion = "v1"

// Encryptor 使用 AES-256-GCM 加密用户 API key；密钥为 32 字节。
type Encryptor struct {
	aead cipher.AEAD
}

// NewEncryptor 解析 base64 编码的密钥，密钥必须解码为 32 字节。
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
	return &Encryptor{aead: aead}, nil
}

// Encrypt 加密明文并返回 "v1:" + base64(nonce+ciphertext)，nonce 为 12 字节。
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := e.aead.Seal(nil, nonce, []byte(plaintext), nil)
	blob := append(nonce, sealed...)
	return ciphertextVersion + ":" + base64.StdEncoding.EncodeToString(blob), nil
}

// Decrypt 解密 Encrypt 生成的密文；版本不匹配或认证失败返回错误。
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
