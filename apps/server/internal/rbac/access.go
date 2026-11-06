package rbac

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

var (
	// ErrNotFound 表示用户不存在。
	ErrNotFound = errors.New("rbac: user not found")
	// ErrInactive 表示用户存在但状态不是 active。
	ErrInactive = errors.New("rbac: user is not active")
)

// RoleSummary 是 Access 与 /users/me 返回的角色摘要。
type RoleSummary struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Access 是一次实时解析得到的用户角色与权限集合。
type Access struct {
	UserID      int64
	IsSuper     bool
	Roles       []RoleSummary
	Permissions map[string]struct{}
}

// Service 提供 RBAC 查询；每次调用都直接读取数据库，不缓存。
type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Access 查询用户状态、角色与权限并集。用户必须存在且状态为 active，
// 否则返回 ErrNotFound / ErrInactive。
func (s *Service) Access(ctx context.Context, userID int64) (*Access, error) {
	db := s.db.WithContext(ctx)

	var u user.User
	result := db.Select("id", "status").Where("id = ?", userID).Limit(1).Find(&u)
	if result.Error != nil {
		return nil, fmt.Errorf("rbac: load user %d: %w", userID, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	if u.Status != user.StatusActive {
		return nil, ErrInactive
	}

	roles := make([]RoleSummary, 0)
	if err := db.Model(&Role{}).
		Select("roles.id", "roles.code", "roles.name").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Order("roles.code").
		Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("rbac: load roles for user %d: %w", userID, err)
	}

	access := &Access{
		UserID:      userID,
		Roles:       roles,
		Permissions: make(map[string]struct{}),
	}
	for _, role := range roles {
		if role.Code == RoleCodeSuperAdmin {
			access.IsSuper = true
			break
		}
	}

	var codes []string
	if err := db.Table("permissions").
		Select("DISTINCT permissions.code").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN roles ON roles.id = role_permissions.role_id").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Order("permissions.code").
		Scan(&codes).Error; err != nil {
		return nil, fmt.Errorf("rbac: load permissions for user %d: %w", userID, err)
	}
	for _, code := range codes {
		access.Permissions[code] = struct{}{}
	}
	return access, nil
}
