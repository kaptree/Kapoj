package rbac

import (
	"context"
	"fmt"
	"unicode/utf8"

	"gorm.io/gorm"
)

// 管理操作审计 action 码；稳定不可改名。
const (
	AuditActionRoleCreate            = "role.create"
	AuditActionRoleUpdate            = "role.update"
	AuditActionRoleDelete            = "role.delete"
	AuditActionRolePermissionsUpdate = "role.permissions.update"

	AuditActionUserUpdate      = "user.update"
	AuditActionUserImport      = "user.import"
	AuditActionUserRolesAssign = "user.roles.assign"
	AuditActionUserBan         = "user.ban"
	AuditActionUserUnban       = "user.unban"
	AuditActionUserCoinsAdjust = "user.coins.adjust"

	AuditActionRatingSettingsManage = "rating.settings.manage"
	AuditActionAISettingsManage     = "ai.settings.manage"

	AuditActionAIPlatformModelCreate = "ai.platform_model.create"
	AuditActionAIPlatformModelUpdate = "ai.platform_model.update"
	AuditActionAIPlatformModelDelete = "ai.platform_model.delete"
	AuditActionAIPlatformModelSelect = "ai.platform_model.select"

	AuditActionSiteSettingsManage    = "site.settings.manage"
	AuditActionMailSettingsManage    = "mail.settings.manage"
	AuditActionSystemBackupManage    = "system.backup.manage"
	AuditActionSystemJobsManage      = "system.jobs.manage"
	AuditActionAgentManage           = "agent.manage"
	AuditActionProblemExport         = "problem.export"
	AuditActionProblemBulk           = "problem.bulk"
	AuditActionProblemImport         = "problem.import"
	AuditActionProblemImportSettings = "problem.import.settings"

	AuditTargetRole                  = "role"
	AuditTargetUser                  = "user"
	AuditTargetRatingSettings        = "rating_settings"
	AuditTargetAISettings            = "ai_settings"
	AuditTargetAIKey                 = "ai_key"
	AuditTargetAIPlatformModel       = "ai_platform_model"
	AuditTargetCertificate           = "certificate"
	AuditTargetCertificateType       = "certificate_type"
	AuditTargetSiteSettings          = "site_settings"
	AuditTargetSiteAsset             = "site_asset"
	AuditTargetMailSettings          = "mail_settings"
	AuditTargetBackup                = "backup"
	AuditTargetScheduledJob          = "scheduled_job"
	AuditTargetAgent                 = "agent"
	AuditTargetDifficultySettings    = "difficulty_settings"
	AuditTargetJudgeSettings         = "judge_settings"
	AuditTargetProblemImportSettings = "problem_import_settings"
)

// WriteAuditTx 在给定事务内写入管理员操作审计；失败会使调用方事务回滚。
// 用于角色/用户等敏感操作，需要与业务变更同事务提交。
func WriteAuditTx(tx *gorm.DB, actor AuditMeta, action, targetType string, targetID int64, summary string) error {
	var target *int64
	if targetID > 0 {
		id := targetID
		target = &id
	}
	row := AdminAuditLog{
		ActorID:    actor.ActorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   target,
		Summary:    truncateText(summary, 500),
		IP:         truncateText(actor.IP, 45),
		UserAgent:  truncateText(actor.UserAgent, 512),
	}
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("rbac: write admin audit log: %w", err)
	}
	return nil
}

// RecordAudit 以独立事务写入一条 best-effort 审计；调用方（如内容管理中间件）
// 应忽略失败并仅记录日志，不阻断业务响应。
func (s *Service) RecordAudit(ctx context.Context, actor AuditMeta, action, targetType string, targetID int64, summary string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return WriteAuditTx(tx, actor, action, targetType, targetID, summary)
	})
}

func truncateText(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	return string([]rune(value)[:max])
}
