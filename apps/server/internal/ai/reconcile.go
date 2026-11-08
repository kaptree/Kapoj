package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gorm.io/gorm"
)

// ReconcileKeyFile 在启动时（DB 迁移后、路由挂载前）核对密钥文件与
// ai_settings.key_fingerprint，恢复“写文件已成功但重加密事务尚未提交”这一
// 崩溃窗口：此时密钥文件是新密钥、数据库密文与指纹仍是旧密钥。
//
// 规则：
//   - 密钥来源为 env（环境变量优先）→ 跳过（env 密钥只读，不参与文件核对）；
//   - 数据库无设置行或指纹为空 → 跳过（从未通过管理接口持久化过密钥）；
//   - 当前密钥指纹与数据库指纹一致 → 通过；
//   - 不一致且存在与数据库指纹一致的 .bak 备份 → 用备份原子覆盖密钥文件并
//     切换内存密钥，记录醒目警告后继续启动；
//   - 其余情况 → 返回错误（拒绝静默启动，避免用户配置永久无法解密）。
func ReconcileKeyFile(ctx context.Context, db *gorm.DB, manager *KeyManager, filePath string) error {
	if manager.Source() == "env" {
		return nil
	}

	var settings Settings
	err := db.WithContext(ctx).First(&settings, settingsID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ai: load settings for key reconciliation: %w", err)
	}
	dbFingerprint := settings.KeyFingerprint
	if dbFingerprint == "" {
		return nil
	}

	if manager.Fingerprint() == dbFingerprint {
		return nil
	}

	// 指纹不一致：尝试用备份文件（旧密钥）恢复。备份指纹必须与数据库一致，
	// 证明它就是写入新密钥前被替换下来的旧密钥。
	backupPath := filePath + ".bak"
	data, readErr := os.ReadFile(backupPath)
	if readErr == nil {
		oldRaw := strings.TrimSpace(string(data))
		if fingerprintOf(oldRaw) == dbFingerprint {
			oldEnc, encErr := NewEncryptor(oldRaw)
			if encErr != nil {
				return fmt.Errorf("ai: key backup invalid: %w", encErr)
			}
			if renameErr := os.Rename(backupPath, filePath); renameErr != nil {
				return fmt.Errorf("ai: restore key backup over key file: %w", renameErr)
			}
			manager.Replace(oldEnc, "file")
			slog.Warn("ai: key file diverged from database, restored from backup",
				"path", filePath, "db_fingerprint", dbFingerprint)
			return nil
		}
	}

	return fmt.Errorf(
		"ai: key file fingerprint %q does not match database %q and no matching backup exists; refusing to start (user AI configs may be undecryptable)",
		manager.Fingerprint(), dbFingerprint,
	)
}
