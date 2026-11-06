package rbac

import "time"

// Role 是管理角色；Code 稳定不可改。
type Role struct {
	ID          int64     `gorm:"primaryKey"`
	Code        string    `gorm:"size:64;not null;uniqueIndex:uniq_roles_code"`
	Name        string    `gorm:"size:100;not null"`
	Description string    `gorm:"size:500;not null;default:''"`
	IsSystem    bool      `gorm:"not null;default:false"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

// Permission 是权限目录中的一项；Code 稳定不可改名。
type Permission struct {
	ID          int64     `gorm:"primaryKey"`
	Code        string    `gorm:"size:64;not null;uniqueIndex:uniq_permissions_code"`
	Name        string    `gorm:"size:100;not null"`
	GroupName   string    `gorm:"column:group_name;size:64;not null"`
	Description string    `gorm:"size:500;not null;default:''"`
	SortOrder   int       `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

// RolePermission 关联角色与权限。
type RolePermission struct {
	ID           int64 `gorm:"primaryKey"`
	RoleID       int64 `gorm:"not null;uniqueIndex:uniq_role_permissions,priority:1"`
	PermissionID int64 `gorm:"not null;uniqueIndex:uniq_role_permissions,priority:2"`
}

// UserRole 关联用户与角色；AssignedBy 为空表示 Seed 迁移。
type UserRole struct {
	ID         int64     `gorm:"primaryKey"`
	UserID     int64     `gorm:"not null;uniqueIndex:uniq_user_roles,priority:1"`
	RoleID     int64     `gorm:"not null;uniqueIndex:uniq_user_roles,priority:2;index:idx_user_roles_role_id"`
	AssignedBy *int64    `gorm:"column:assigned_by"`
	AssignedAt time.Time `gorm:"type:timestamptz;not null"`
}

// AdminAuditLog 记录管理员敏感操作；Summary 不得包含敏感信息。
type AdminAuditLog struct {
	ID         int64     `gorm:"primaryKey"`
	ActorID    int64     `gorm:"not null;index:idx_admin_audit_logs_actor"`
	Action     string    `gorm:"size:64;not null;index:idx_admin_audit_logs_action"`
	TargetType string    `gorm:"size:32;not null;default:''"`
	TargetID   *int64    `gorm:"column:target_id"`
	Summary    string    `gorm:"size:500;not null;default:''"`
	IP         string    `gorm:"size:45;not null;default:''"`
	UserAgent  string    `gorm:"size:512;not null;default:''"`
	CreatedAt  time.Time `gorm:"type:timestamptz;not null;index:idx_admin_audit_logs_created"`
}

// Meta 保存 RBAC 初始化标记等键值数据。
type Meta struct {
	Key   string `gorm:"column:key;size:64;primaryKey"`
	Value string `gorm:"size:255;not null;default:''"`
}

func (Meta) TableName() string { return "rbac_meta" }
