package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// KeyManager 持有当前 AI 加密密钥并追踪其来源（env|file|none）。
// 运行时轮换通过 Replace 原子切换，所有 Encrypt/Decrypt 走当前密钥。
type KeyManager struct {
	mu     sync.RWMutex
	enc    *Encryptor
	source string
}

// errNoKey 是未配置 AI 加密密钥时返回的统一 API 错误。
func errNoKey() *httpx.AppError {
	return httpx.ErrValidation("AI 加密密钥未配置")
}

// NewKeyManager 从环境变量（优先）或本地密钥文件加载加密密钥。
//
// env 非空时校验密钥；非法则返回错误（启动失败），并忽略文件。
// env 为空时读取文件（trim 末尾空白）：文件不存在 → 未配置；读取或解码失败 →
// 记录警告并视为未配置；合法 → 来源 file。
func NewKeyManager(envKey, filePath string) (*KeyManager, error) {
	km := &KeyManager{source: "none"}

	envKey = strings.TrimSpace(envKey)
	if envKey != "" {
		enc, err := NewEncryptor(envKey)
		if err != nil {
			return nil, fmt.Errorf("ai: invalid encryption key from env: %w", err)
		}
		km.enc = enc
		km.source = "env"
		return km, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("ai: cannot read key file", "path", filePath, "err", err)
		}
		return km, nil
	}

	raw := strings.TrimSpace(string(data))
	enc, err := NewEncryptor(raw)
	if err != nil {
		slog.Warn("ai: invalid key file", "path", filePath, "err", err)
		return km, nil
	}
	km.enc = enc
	km.source = "file"
	return km, nil
}

// Configured 报告是否已配置可用密钥。
func (km *KeyManager) Configured() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.enc != nil
}

// Source 返回密钥来源：env、file 或 none。
func (km *KeyManager) Source() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.source
}

// Masked 返回密钥的脱敏值（前 3…后 4，长度≤8 为 "****"）；未配置返回空串。
func (km *KeyManager) Masked() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.enc == nil {
		return ""
	}
	return maskKey(km.enc.key)
}

// Fingerprint 返回密钥的 SHA-256 hex 指纹（对 base64 密钥串哈希）；未配置返回空串。
func (km *KeyManager) Fingerprint() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.enc == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(km.enc.key))
	return hex.EncodeToString(sum[:])
}

// Encrypt 使用当前密钥加密明文；未配置时返回 40001。
func (km *KeyManager) Encrypt(plaintext string) (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.enc == nil {
		return "", errNoKey()
	}
	return km.enc.Encrypt(plaintext)
}

// Decrypt 使用当前密钥解密密文；未配置时返回 40001。
func (km *KeyManager) Decrypt(ciphertext string) (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.enc == nil {
		return "", errNoKey()
	}
	return km.enc.Decrypt(ciphertext)
}

// Replace 原子地切换内存中的密钥与来源（轮换成功后调用）。
func (km *KeyManager) Replace(enc *Encryptor, source string) {
	km.mu.Lock()
	defer km.mu.Unlock()
	km.enc = enc
	km.source = source
}
