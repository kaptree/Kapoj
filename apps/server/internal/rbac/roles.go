package rbac

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// AdminAdvisoryLockKey is a fixed cluster-wide transaction advisory lock shared
// by RBAC admin mutations (roles, permissions, user role assignment and bans).
// Serializing these mutations keeps system-role and last-super invariants
// correct under concurrency.
const AdminAdvisoryLockKey int64 = 0x4B41504F4A524231 // "KAPOJRB1"

var roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

var knownPermissionCodes = func() map[string]struct{} {
	defs := AllPermissions()
	codes := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		codes[string(def.Code)] = struct{}{}
	}
	return codes
}()

// PermissionItem 是权限矩阵展示的一项。
type PermissionItem struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	GroupName   string `json:"groupName"`
	Description string `json:"description"`
	SortOrder   int    `json:"sortOrder"`
}

// RoleItem 是角色列表/详情返回项，Permissions 为权限码并按字典序排列。
type RoleItem struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"isSystem"`
	Permissions []string  `json:"permissions"`
	UserCount   int64     `json:"userCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// RoleInput 是创建自定义角色的输入。
type RoleInput struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PermissionCodes []string `json:"permissionCodes"`
}

// RoleUpdateInput 是编辑角色名称/描述的输入。名称为空表示保持原名；
// 角色 code 不属于可更新字段，创建后不可修改。
type RoleUpdateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AuditMeta 是写审计所需的操作者信息。
type AuditMeta struct {
	ActorID   int64
	IP        string
	UserAgent string
}

// ListPermissions 返回权限目录，按目录分组顺序与 SortOrder 排列。
func (s *Service) ListPermissions(context.Context) []PermissionItem {
	defs := AllPermissions()
	groupOrder := make(map[string]int, 8)
	items := make([]PermissionItem, 0, len(defs))
	for _, def := range defs {
		if _, ok := groupOrder[def.GroupName]; !ok {
			groupOrder[def.GroupName] = len(groupOrder)
		}
		items = append(items, PermissionItem{
			Code:        string(def.Code),
			Name:        def.Name,
			GroupName:   def.GroupName,
			Description: def.Description,
			SortOrder:   def.SortOrder,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		gi, gj := groupOrder[items[i].GroupName], groupOrder[items[j].GroupName]
		if gi != gj {
			return gi < gj
		}
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].Code < items[j].Code
	})
	return items
}

// ListRoles 返回全部内置与自定义角色，内置角色在前。
func (s *Service) ListRoles(ctx context.Context) ([]RoleItem, error) {
	roles := make([]Role, 0)
	if err := s.db.WithContext(ctx).
		Order("is_system DESC, id ASC").
		Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("rbac: list roles: %w", err)
	}
	return roleItemsForRoles(s.db.WithContext(ctx), roles)
}

// GetRole 返回单个角色及其权限与用户数。
func (s *Service) GetRole(ctx context.Context, id int64) (*RoleItem, error) {
	db := s.db.WithContext(ctx)
	var role Role
	err := db.First(&role, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("rbac: load role %d: %w", id, err)
	}
	items, err := roleItemsForRoles(db, []Role{role})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// CreateRole 创建自定义角色并在同一事务内写审计。
func (s *Service) CreateRole(ctx context.Context, actor AuditMeta, in RoleInput) (*RoleItem, error) {
	code := strings.TrimSpace(in.Code)
	if !roleCodePattern.MatchString(code) {
		return nil, invalidRole("角色 code 需为 3–64 位小写字母、数字或下划线，且以字母开头")
	}
	name, err := normalizeRoleName(in.Name)
	if err != nil {
		return nil, err
	}
	description, err := normalizeRoleDescription(in.Description)
	if err != nil {
		return nil, err
	}
	codes, err := normalizePermissionCodes(in.PermissionCodes)
	if err != nil {
		return nil, err
	}

	var roleID int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := acquireAdminAdvisoryLock(tx); err != nil {
			return err
		}
		role := &Role{
			Code:        code,
			Name:        name,
			Description: description,
			IsSystem:    false,
		}
		if err := tx.Create(role).Error; err != nil {
			if isRoleCodeConflict(err) {
				return invalidRole("角色 code 已存在")
			}
			return fmt.Errorf("rbac: create role %q: %w", code, err)
		}
		if err := insertRolePermissions(tx, role.ID, codes); err != nil {
			return err
		}
		if err := writeAudit(tx, actor, AuditActionRoleCreate, AuditTargetRole, role.ID,
			fmt.Sprintf("创建角色「%s」（%s）", role.Name, role.Code)); err != nil {
			return err
		}
		roleID = role.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetRole(ctx, roleID)
}

// UpdateRole 更新角色名称/描述。系统角色名称不可改，描述可改；自定义角色
// 名称/描述均可改。名称为空表示保持原值。
func (s *Service) UpdateRole(ctx context.Context, actor AuditMeta, id int64, in RoleUpdateInput) (*RoleItem, error) {
	rawName := strings.TrimSpace(in.Name)
	if rawName != "" {
		if _, err := normalizeRoleName(rawName); err != nil {
			return nil, err
		}
	}
	description, err := normalizeRoleDescription(in.Description)
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := acquireAdminAdvisoryLock(tx); err != nil {
			return err
		}
		var role Role
		if err := tx.First(&role, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return fmt.Errorf("rbac: load role %d: %w", id, err)
		}

		name := role.Name
		if rawName != "" {
			if role.IsSystem && rawName != role.Name {
				return invalidRole("系统角色名称不可修改")
			}
			name = rawName
		}

		if err := tx.Model(&Role{}).Where("id = ?", role.ID).
			Updates(map[string]any{"name": name, "description": description}).Error; err != nil {
			return fmt.Errorf("rbac: update role %d: %w", id, err)
		}
		if err := writeAudit(tx, actor, AuditActionRoleUpdate, AuditTargetRole, role.ID,
			fmt.Sprintf("更新角色「%s」（%s）", name, role.Code)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetRole(ctx, id)
}

// DeleteRole 删除自定义角色并清理 role_permissions/user_roles 关联；系统角色
// 不可删除。删除与审计在同一事务内完成。
func (s *Service) DeleteRole(ctx context.Context, actor AuditMeta, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := acquireAdminAdvisoryLock(tx); err != nil {
			return err
		}
		var role Role
		if err := tx.First(&role, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return fmt.Errorf("rbac: load role %d: %w", id, err)
		}
		if role.IsSystem {
			return invalidRole("系统角色不可删除")
		}

		if err := tx.Where("role_id = ?", role.ID).Delete(&UserRole{}).Error; err != nil {
			return fmt.Errorf("rbac: delete user roles for role %d: %w", id, err)
		}
		if err := tx.Where("role_id = ?", role.ID).Delete(&RolePermission{}).Error; err != nil {
			return fmt.Errorf("rbac: delete role permissions for role %d: %w", id, err)
		}
		if err := tx.Delete(&role).Error; err != nil {
			return fmt.Errorf("rbac: delete role %d: %w", id, err)
		}
		if err := writeAudit(tx, actor, AuditActionRoleDelete, AuditTargetRole, role.ID,
			fmt.Sprintf("删除角色「%s」（%s）", role.Name, role.Code)); err != nil {
			return err
		}
		return nil
	})
}

// ReplaceRolePermissions 整体替换角色权限；空集合表示移除全部权限。
// super_admin 权限不可修改，权限码必须存在于目录且会去重。
func (s *Service) ReplaceRolePermissions(ctx context.Context, actor AuditMeta, id int64, codes []string) (*RoleItem, error) {
	normalized, err := normalizePermissionCodes(codes)
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := acquireAdminAdvisoryLock(tx); err != nil {
			return err
		}
		var role Role
		if err := tx.First(&role, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return fmt.Errorf("rbac: load role %d: %w", id, err)
		}
		if role.Code == RoleCodeSuperAdmin {
			return invalidRole("超级管理员权限不可修改")
		}

		if err := tx.Where("role_id = ?", role.ID).Delete(&RolePermission{}).Error; err != nil {
			return fmt.Errorf("rbac: clear role permissions for role %d: %w", id, err)
		}
		if err := insertRolePermissions(tx, role.ID, normalized); err != nil {
			return err
		}
		if err := writeAudit(tx, actor, AuditActionRolePermissionsUpdate, AuditTargetRole, role.ID,
			fmt.Sprintf("更新角色「%s」的权限（%d 项）", role.Name, len(normalized))); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetRole(ctx, id)
}

type rolePermissionRow struct {
	RoleID int64
	Code   string
}

type roleUserCountRow struct {
	RoleID    int64
	UserCount int64
}

func roleItemsForRoles(db *gorm.DB, roles []Role) ([]RoleItem, error) {
	items := make([]RoleItem, 0, len(roles))
	if len(roles) == 0 {
		return items, nil
	}
	ids := make([]int64, len(roles))
	for i, role := range roles {
		ids[i] = role.ID
	}

	var permissionRows []rolePermissionRow
	if err := db.Table("role_permissions").
		Select("role_permissions.role_id, permissions.code").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id IN ?", ids).
		Order("role_permissions.role_id ASC, permissions.code ASC").
		Scan(&permissionRows).Error; err != nil {
		return nil, fmt.Errorf("rbac: list role permissions: %w", err)
	}
	permissionsByRole := make(map[int64][]string, len(roles))
	for _, row := range permissionRows {
		permissionsByRole[row.RoleID] = append(permissionsByRole[row.RoleID], row.Code)
	}

	var countRows []roleUserCountRow
	if err := db.Table("user_roles").
		Select("role_id, COUNT(*) AS user_count").
		Where("role_id IN ?", ids).
		Group("role_id").
		Scan(&countRows).Error; err != nil {
		return nil, fmt.Errorf("rbac: count role users: %w", err)
	}
	countByRole := make(map[int64]int64, len(countRows))
	for _, row := range countRows {
		countByRole[row.RoleID] = row.UserCount
	}

	for _, role := range roles {
		permissions := permissionsByRole[role.ID]
		if permissions == nil {
			permissions = []string{}
		}
		items = append(items, RoleItem{
			ID:          role.ID,
			Code:        role.Code,
			Name:        role.Name,
			Description: role.Description,
			IsSystem:    role.IsSystem,
			Permissions: permissions,
			UserCount:   countByRole[role.ID],
			CreatedAt:   role.CreatedAt,
			UpdatedAt:   role.UpdatedAt,
		})
	}
	return items, nil
}

func insertRolePermissions(tx *gorm.DB, roleID int64, codes []string) error {
	if len(codes) == 0 {
		return nil
	}
	permissionIDs, err := permissionIDsForCodes(tx, codes)
	if err != nil {
		return err
	}
	rows := make([]RolePermission, len(permissionIDs))
	for i, permissionID := range permissionIDs {
		rows[i] = RolePermission{RoleID: roleID, PermissionID: permissionID}
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("rbac: insert role permissions: %w", err)
	}
	return nil
}

func permissionIDsForCodes(tx *gorm.DB, codes []string) ([]int64, error) {
	var permissions []Permission
	if err := tx.Where("code IN ?", codes).Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("rbac: load permissions: %w", err)
	}
	idsByCode := make(map[string]int64, len(permissions))
	for _, permission := range permissions {
		idsByCode[permission.Code] = permission.ID
	}
	ids := make([]int64, 0, len(codes))
	for _, code := range codes {
		id, ok := idsByCode[code]
		if !ok {
			return nil, fmt.Errorf("rbac: permission %q missing from database; seed may be out of sync", code)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func normalizeRoleName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		return "", invalidRole("角色名称长度需为 1–100 个字符")
	}
	return name, nil
}

func normalizeRoleDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) > 500 {
		return "", invalidRole("角色描述不能超过 500 个字符")
	}
	return description, nil
}

func normalizePermissionCodes(codes []string) ([]string, error) {
	out := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if _, ok := knownPermissionCodes[code]; !ok {
			return nil, invalidRole("权限码不存在：%s", code)
		}
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out, nil
}

func invalidRole(format string, args ...any) error {
	return httpx.ErrValidation(fmt.Sprintf(format, args...))
}

func acquireAdminAdvisoryLock(tx *gorm.DB) error {
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", AdminAdvisoryLockKey).Error; err != nil {
		return fmt.Errorf("rbac: acquire admin advisory lock: %w", err)
	}
	return nil
}

func isRoleCodeConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_roles_code"
}
