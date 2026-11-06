package rbac

import (
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
	AuditActionUserRolesAssign = "user.roles.assign"
	AuditActionUserBan         = "user.ban"
	AuditActionUserUnban       = "user.unban"

	AuditTargetRole = "role"
	AuditTargetUser = "user"
)

// writeAudit 在给定事务内写入管理员操作审计；失败会使调用方事务回滚。
func writeAudit(tx *gorm.DB, actor AuditMeta, action, targetType string, targetID int64, summary string) error {
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

func truncateText(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	return string([]rune(value)[:max])
}
