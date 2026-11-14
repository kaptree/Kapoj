package rbac

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const (
	// SeedVersion 是当前权限目录与内置角色的数据版本。提升版本只会补齐缺失的
	// 权限目录与内置角色，不会新增或恢复任何角色权限授权。
	SeedVersion = "4"

	metaSeedVersionKey = "seed_version"
)

// Seed 幂等地初始化权限目录与内置角色，并在首次运行时写入内置角色权限、
// 迁移旧 users.role 并记录 seed_version。后续启动或版本升级只补齐缺失的
// 权限与角色，不会重新添加管理员已删除的角色权限。
func Seed(ctx context.Context, g *gorm.DB) error {
	return g.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		storedVersion, hasVersion, err := readSeedVersion(tx)
		if err != nil {
			return err
		}

		if err := ensurePermissions(tx); err != nil {
			return err
		}
		if err := ensureRoles(tx); err != nil {
			return err
		}

		if !hasVersion {
			if err := seedRolePermissions(tx); err != nil {
				return err
			}
			if err := mapLegacyRoles(tx); err != nil {
				return err
			}
			return writeSeedVersion(tx)
		}

		stored, err := parseSeedVersion(storedVersion)
		if err != nil {
			return fmt.Errorf("parse rbac seed version %q: %w", storedVersion, err)
		}
		current, err := parseSeedVersion(SeedVersion)
		if err != nil {
			return fmt.Errorf("parse current rbac seed version %q: %w", SeedVersion, err)
		}
		if stored >= current {
			return nil
		}

		// 新版本只确保上面的权限目录与内置角色；角色授权变更必须走显式数据迁移，
		// 避免重启时恢复管理员已删除的 role_permissions。
		if stored < 2 {
			if err := migrateSeedV2(tx); err != nil {
				return err
			}
		}
		if stored < 3 {
			if err := migrateSeedV3(tx); err != nil {
				return err
			}
		}
		if stored < 4 {
			if err := migrateSeedV4(tx); err != nil {
				return err
			}
		}
		return writeSeedVersion(tx)
	})
}

// migrateSeedV4 intentionally grants the new backup permission to no built-in
// role; only super administrators (who bypass checks) have it until an
// administrator grants it to a custom role.
func migrateSeedV4(*gorm.DB) error { return nil }

func migrateSeedV3(tx *gorm.DB) error {
	for _, role := range []string{RoleCodeContentAdmin, RoleCodeContestAdmin} {
		if err := grantRolePermissions(tx, role, []PermissionCode{PermSubmissionRejudge}); err != nil {
			return err
		}
	}
	return nil
}

// migrateSeedV2 grants the permissions introduced with the problem-author
// ownership model. It only touches the new permission and the new built-in
// role; deleted grants on existing permissions are left alone.
func migrateSeedV2(tx *gorm.DB) error {
	if err := grantRolePermissions(tx, RoleCodeContentAdmin, []PermissionCode{PermProblemManageAll}); err != nil {
		return err
	}
	var authorPerms []PermissionCode
	for _, role := range BuiltinRoles() {
		if role.Code == RoleCodeProblemAuthor {
			authorPerms = role.Permissions
			break
		}
	}
	if len(authorPerms) == 0 {
		return fmt.Errorf("rbac: builtin role %q missing permissions", RoleCodeProblemAuthor)
	}
	return grantRolePermissions(tx, RoleCodeProblemAuthor, authorPerms)
}

func grantRolePermissions(tx *gorm.DB, roleCode string, codes []PermissionCode) error {
	roleIDs, err := roleIDsByCode(tx)
	if err != nil {
		return err
	}
	permissionIDs, err := permissionIDsByCode(tx)
	if err != nil {
		return err
	}
	roleID, ok := roleIDs[roleCode]
	if !ok {
		return fmt.Errorf("rbac: builtin role %q missing after seed", roleCode)
	}
	rows := make([]RolePermission, 0, len(codes))
	for _, code := range codes {
		permissionID, ok := permissionIDs[string(code)]
		if !ok {
			return fmt.Errorf("rbac: permission %q missing after seed", code)
		}
		rows = append(rows, RolePermission{RoleID: roleID, PermissionID: permissionID})
	}
	if len(rows) == 0 {
		return nil
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return fmt.Errorf("grant problem author permissions: %w", err)
	}
	return nil
}

func readSeedVersion(tx *gorm.DB) (string, bool, error) {
	var meta Meta
	result := tx.Where("key = ?", metaSeedVersionKey).Limit(1).Find(&meta)
	if result.Error != nil {
		return "", false, fmt.Errorf("read rbac seed version: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return "", false, nil
	}
	return meta.Value, true, nil
}

func parseSeedVersion(value string) (int, error) {
	version, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return version, nil
}

func ensurePermissions(tx *gorm.DB) error {
	defs := AllPermissions()
	permissions := make([]Permission, 0, len(defs))
	for _, def := range defs {
		permissions = append(permissions, Permission{
			Code:        string(def.Code),
			Name:        def.Name,
			GroupName:   def.GroupName,
			Description: def.Description,
			SortOrder:   def.SortOrder,
		})
	}
	if len(permissions) == 0 {
		return nil
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&permissions).Error; err != nil {
		return fmt.Errorf("insert rbac permissions: %w", err)
	}
	return nil
}

func ensureRoles(tx *gorm.DB) error {
	defs := BuiltinRoles()
	roles := make([]Role, 0, len(defs))
	for _, def := range defs {
		roles = append(roles, Role{
			Code:        def.Code,
			Name:        def.Name,
			Description: def.Description,
			IsSystem:    true,
		})
	}
	if len(roles) == 0 {
		return nil
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&roles).Error; err != nil {
		return fmt.Errorf("insert rbac roles: %w", err)
	}
	return nil
}

func seedRolePermissions(tx *gorm.DB) error {
	roleIDs, err := roleIDsByCode(tx)
	if err != nil {
		return err
	}
	permissionIDs, err := permissionIDsByCode(tx)
	if err != nil {
		return err
	}

	var rows []RolePermission
	for _, role := range BuiltinRoles() {
		if role.Code == RoleCodeSuperAdmin {
			continue
		}
		roleID, ok := roleIDs[role.Code]
		if !ok {
			return fmt.Errorf("rbac: builtin role %q missing after seed", role.Code)
		}
		for _, code := range role.Permissions {
			permissionID, ok := permissionIDs[string(code)]
			if !ok {
				return fmt.Errorf("rbac: permission %q missing after seed", code)
			}
			rows = append(rows, RolePermission{RoleID: roleID, PermissionID: permissionID})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return fmt.Errorf("insert rbac role permissions: %w", err)
	}
	return nil
}

func mapLegacyRoles(tx *gorm.DB) error {
	roleIDs, err := roleIDsByCode(tx)
	if err != nil {
		return err
	}
	mappings := []struct {
		legacyRole user.Role
		roleCode   string
	}{
		{legacyRole: user.RoleSuperAdmin, roleCode: RoleCodeSuperAdmin},
		{legacyRole: user.RoleProblemAdmin, roleCode: RoleCodeContentAdmin},
		{legacyRole: user.RoleContestAdmin, roleCode: RoleCodeContestAdmin},
	}
	for _, mapping := range mappings {
		roleID, ok := roleIDs[mapping.roleCode]
		if !ok {
			return fmt.Errorf("rbac: builtin role %q missing after seed", mapping.roleCode)
		}
		if err := tx.Exec(`
			INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at)
			SELECT id, ?, NULL, NOW() FROM users WHERE role = ?
			ON CONFLICT (user_id, role_id) DO NOTHING
		`, roleID, string(mapping.legacyRole)).Error; err != nil {
			return fmt.Errorf("map legacy role %q: %w", mapping.legacyRole, err)
		}
	}
	return nil
}

func writeSeedVersion(tx *gorm.DB) error {
	meta := Meta{Key: metaSeedVersionKey, Value: SeedVersion}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&meta).Error; err != nil {
		return fmt.Errorf("write rbac seed version: %w", err)
	}
	return nil
}

func roleIDsByCode(tx *gorm.DB) (map[string]int64, error) {
	var roles []Role
	if err := tx.Select("id", "code").Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("load rbac roles: %w", err)
	}
	ids := make(map[string]int64, len(roles))
	for _, role := range roles {
		ids[role.Code] = role.ID
	}
	return ids, nil
}

func permissionIDsByCode(tx *gorm.DB) (map[string]int64, error) {
	var permissions []Permission
	if err := tx.Select("id", "code").Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("load rbac permissions: %w", err)
	}
	ids := make(map[string]int64, len(permissions))
	for _, permission := range permissions {
		ids[permission.Code] = permission.ID
	}
	return ids, nil
}
