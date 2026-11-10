package ai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// KeyView 是 AI 加密密钥的状态视图，绝不包含完整密钥。
type KeyView struct {
	Configured  bool   `json:"configured"`
	Source      string `json:"source"`
	Masked      string `json:"masked"`
	Fingerprint string `json:"fingerprint"`
}

// KeyGenerateResult 在 KeyView 基础上额外返回一次完整密钥，仅供生成后备份。
type KeyGenerateResult struct {
	KeyView
	Key string `json:"key"`
}

// KeyInput 是手动填写密钥的请求体。
type KeyInput struct {
	Key string `json:"key"`
}

// KeyView 返回当前密钥状态。
func (s *SettingsService) KeyView() KeyView {
	return KeyView{
		Configured:  s.keys.Configured(),
		Source:      s.keys.Source(),
		Masked:      s.keys.Masked(),
		Fingerprint: s.keys.Fingerprint(),
	}
}

// GenerateKey 生成 32 字节随机密钥并轮换保存，仅在结果中返回一次完整密钥。
func (s *SettingsService) GenerateKey(ctx context.Context, actorID int64, audit func(*gorm.DB) error) (*KeyGenerateResult, error) {
	raw, err := generateKeyBase64()
	if err != nil {
		return nil, err
	}
	view, err := s.rotate(ctx, actorID, raw, audit, keepUserConfigs)
	if err != nil {
		return nil, err
	}
	return &KeyGenerateResult{KeyView: *view, Key: raw}, nil
}

// SetKey 校验并保存手动填写的密钥；已有密钥时执行轮换重加密。
func (s *SettingsService) SetKey(ctx context.Context, actorID int64, raw string, audit func(*gorm.DB) error) (*KeyView, error) {
	return s.rotate(ctx, actorID, raw, audit, keepUserConfigs)
}

// ResetKey 删除所有用户 AI 配置并用新的随机密钥替换平台密钥。
// 用于旧密钥丢失后的恢复；不保留旧密文。
func (s *SettingsService) ResetKey(ctx context.Context, actorID int64, audit func(*gorm.DB) error) (*KeyGenerateResult, error) {
	raw, err := generateKeyBase64()
	if err != nil {
		return nil, err
	}
	view, err := s.rotate(ctx, actorID, raw, audit, purgeUserConfigs)
	if err != nil {
		return nil, err
	}
	return &KeyGenerateResult{KeyView: *view, Key: raw}, nil
}

// keyRotationMode 区分轮换时是否保留用户配置：生成/手动保存保留并重加密，
// 重置则全部清除。
type keyRotationMode bool

const (
	keepUserConfigs  keyRotationMode = true
	purgeUserConfigs keyRotationMode = false
)

// rotate 是生成、手动保存与重置共享的实现：
//
//  1. env 来源只读；
//  2. 校验新密钥（base64/32 字节）；
//  3. 仅保留模式：无旧密钥且存在用户配置 → 拒绝破坏性覆盖；
//  4. 写密钥文件（备份旧文件）→ DB 事务（重加密或清除配置 + 指纹 + 审计）→
//     成功删除备份并切换内存密钥，失败恢复备份/回滚。
func (s *SettingsService) rotate(ctx context.Context, actorID int64, raw string, audit func(*gorm.DB) error, mode keyRotationMode) (*KeyView, error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()

	if s.keys.Source() == "env" {
		return nil, httpx.ErrValidation("AI 加密密钥由环境变量管理，只读")
	}
	if s.keyFile == "" {
		return nil, errors.New("ai: key file path is not configured")
	}

	raw = strings.TrimSpace(raw)
	newEnc, err := NewEncryptor(raw)
	if err != nil {
		return nil, httpx.ErrValidation("密钥必须为 base64 编码的 32 字节密钥")
	}

	var cfgCount int64
	if err := s.db.WithContext(ctx).Model(&UserConfig{}).Count(&cfgCount).Error; err != nil {
		return nil, err
	}
	hasOld := s.keys.Configured()
	if mode == keepUserConfigs && cfgCount > 0 && !hasOld {
		return nil, httpx.ErrValidation("存在无法解密的用户 AI 配置，请先恢复旧密钥")
	}

	fingerprint := fingerprintOf(raw)

	// 先写文件（备份旧文件），再执行 DB 事务；DB 失败恢复备份。
	hadBackup, err := writeKeyFileWithBackup(s.keyFile, raw+"\n")
	if err != nil {
		return nil, fmt.Errorf("ai: write key file: %w", err)
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSettings(tx); err != nil {
			return err
		}
		if mode == keepUserConfigs {
			if err := s.reencryptConfigs(tx, newEnc); err != nil {
				return err
			}
		} else if err := tx.Where("1 = 1").Delete(&UserConfig{}).Error; err != nil {
			return fmt.Errorf("ai: purge user configs: %w", err)
		}
		if err := tx.Model(&Settings{}).Where("id = ?", settingsID).Updates(map[string]any{
			"key_fingerprint": fingerprint,
			"updated_by":      actorID,
			"updated_at":      time.Now().UTC(),
		}).Error; err != nil {
			return fmt.Errorf("ai: update key fingerprint: %w", err)
		}
		if audit != nil {
			return audit(tx)
		}
		return nil
	})
	if err != nil {
		restoreKeyFile(s.keyFile, hadBackup)
		return nil, err
	}

	removeKeyBackup(s.keyFile)
	s.keys.Replace(newEnc, "file")
	return &KeyView{
		Configured:  true,
		Source:      "file",
		Masked:      s.keys.Masked(),
		Fingerprint: fingerprint,
	}, nil
}

// reencryptConfigs 在事务内以 SELECT FOR UPDATE 锁定全部用户配置，用旧密钥
// 解密、新密钥加密并更新密文；任一失败使事务回滚。
func (s *SettingsService) reencryptConfigs(tx *gorm.DB, newEnc *Encryptor) error {
	var cfgs []UserConfig
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Find(&cfgs).Error; err != nil {
		return fmt.Errorf("ai: load user configs for rotation: %w", err)
	}
	for i := range cfgs {
		plain, err := s.keys.Decrypt(cfgs[i].APIKeyEncrypted)
		if err != nil {
			return fmt.Errorf("ai: decrypt user config %d: %w", cfgs[i].UserID, err)
		}
		ciphertext, err := newEnc.Encrypt(plain)
		if err != nil {
			return fmt.Errorf("ai: re-encrypt user config %d: %w", cfgs[i].UserID, err)
		}
		if err := tx.Model(&UserConfig{}).Where("id = ?", cfgs[i].ID).
			Update("api_key_encrypted", ciphertext).Error; err != nil {
			return fmt.Errorf("ai: update user config %d: %w", cfgs[i].UserID, err)
		}
	}
	return nil
}

// generateKeyBase64 用 crypto/rand 生成 32 字节并 base64 编码。
func generateKeyBase64() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("ai: generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// fingerprintOf 计算 base64 密钥串的 SHA-256 hex 指纹，与 KeyManager 一致。
func fingerprintOf(b64 string) string {
	sum := sha256.Sum256([]byte(b64))
	return hex.EncodeToString(sum[:])
}

// writeKeyFileWithBackup 原子写入密钥文件（0600），父目录不存在时以 0700
// 创建；若已存在旧文件则先重命名为 .bak 备份，返回是否产生了备份。
func writeKeyFileWithBackup(path, content string) (hadBackup bool, err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}

	if _, statErr := os.Stat(path); statErr == nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return false, err
		}
		hadBackup = true
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		if hadBackup {
			_ = os.Rename(path+".bak", path)
		}
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		if hadBackup {
			_ = os.Rename(path+".bak", path)
		}
		return false, err
	}
	return hadBackup, nil
}

// restoreKeyFile 在 DB 事务失败后恢复密钥文件：有备份则回滚备份，否则删除
// 新写入的文件（回到未配置状态）。
func restoreKeyFile(path string, hadBackup bool) {
	if hadBackup {
		_ = os.Rename(path+".bak", path)
		return
	}
	_ = os.Remove(path)
}

// removeKeyBackup 删除轮换成功后的旧文件备份。
func removeKeyBackup(path string) {
	_ = os.Remove(path + ".bak")
}
