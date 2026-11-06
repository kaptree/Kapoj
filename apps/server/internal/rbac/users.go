package rbac

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// AdminUserItem 是管理后台用户列表的一项；Role 为 legacy 展示字段，
// Roles 为实际的管理角色集合。
type AdminUserItem struct {
	ID        int64         `json:"id"`
	Username  string        `json:"username"`
	Nickname  string        `json:"nickname"`
	Email     string        `json:"email"`
	Role      user.Role     `json:"role"`
	Rating    int           `json:"rating"`
	Coins     int64         `json:"coins"`
	Status    user.Status   `json:"status"`
	Roles     []RoleSummary `json:"roles"`
	CreatedAt time.Time     `json:"createdAt"`
}

// AdminUserDetail 在列表项基础上补充 RBAC 摘要。
type AdminUserDetail struct {
	AdminUserItem
	IsSuperAdmin bool `json:"isSuperAdmin"`
}

// AdminUserFilter 是管理后台用户列表的筛选条件。
type AdminUserFilter struct {
	Keyword string
	Status  user.Status
	RoleID  int64
}

// AdminUserUpdateInput 是管理员可编辑的用户资料字段；nil 表示不修改。
type AdminUserUpdateInput struct {
	Nickname  *string `json:"nickname"`
	Bio       *string `json:"bio"`
	AvatarURL *string `json:"avatarUrl"`
}

// AdminListUsers 按 keyword/status/roleId 过滤并分页返回用户，包含角色摘要。
// 封禁用户同样出现在列表与详情中，仅 rbac.Access 将其视为 inactive。
func (s *Service) AdminListUsers(ctx context.Context, page, size int, filter AdminUserFilter) (httpx.Page[AdminUserItem], error) {
	db := s.db.WithContext(ctx)
	q := db.Model(&user.User{})

	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		pat := "%" + escapeLike(kw) + "%"
		q = q.Where(`(username ILIKE ? ESCAPE '\' OR nickname ILIKE ? ESCAPE '\' OR email ILIKE ? ESCAPE '\')`,
			pat, pat, pat)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.RoleID > 0 {
		q = q.Where("EXISTS (SELECT 1 FROM user_roles WHERE user_roles.user_id = users.id AND user_roles.role_id = ?)",
			filter.RoleID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return httpx.Page[AdminUserItem]{}, fmt.Errorf("rbac: count users: %w", err)
	}
	var users []user.User
	if err := q.Order("id").Offset((page - 1) * size).Limit(size).Find(&users).Error; err != nil {
		return httpx.Page[AdminUserItem]{}, fmt.Errorf("rbac: list users: %w", err)
	}
	items, err := adminUserItems(db, users)
	if err != nil {
		return httpx.Page[AdminUserItem]{}, err
	}
	return httpx.Page[AdminUserItem]{Total: total, Items: items}, nil
}

// AdminUser 返回单个用户及其角色；用户不存在返回 404。
func (s *Service) AdminUser(ctx context.Context, id int64) (*AdminUserDetail, error) {
	db := s.db.WithContext(ctx)
	var u user.User
	if err := db.Where("id = ?", id).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, fmt.Errorf("rbac: load user %d: %w", id, err)
	}
	items, err := adminUserItems(db, []user.User{u})
	if err != nil {
		return nil, err
	}
	detail := &AdminUserDetail{AdminUserItem: items[0]}
	for _, role := range detail.Roles {
		if role.Code == RoleCodeSuperAdmin {
			detail.IsSuperAdmin = true
			break
		}
	}
	return detail, nil
}

// AdminUpdateUser 编辑用户昵称/简介/头像并在同一事务内写审计。
func (s *Service) AdminUpdateUser(ctx context.Context, actor AuditMeta, id int64, in AdminUserUpdateInput) error {
	cols := make(map[string]any, 3)
	if in.Nickname != nil {
		nick := strings.TrimSpace(*in.Nickname)
		if err := user.ValidateNickname(nick); err != nil {
			return err
		}
		cols["nickname"] = nick
	}
	if in.Bio != nil {
		if err := user.ValidateBio(*in.Bio); err != nil {
			return err
		}
		cols["bio"] = *in.Bio
	}
	if in.AvatarURL != nil {
		avatar := strings.TrimSpace(*in.AvatarURL)
		if err := user.ValidateAvatarURL(avatar); err != nil {
			return err
		}
		cols["avatar_url"] = avatar
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u user.User
		if err := tx.Select("id", "username").Where("id = ?", id).First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return fmt.Errorf("rbac: load user %d: %w", id, err)
		}
		if len(cols) > 0 {
			if err := tx.Model(&user.User{}).Where("id = ?", id).Updates(cols).Error; err != nil {
				return fmt.Errorf("rbac: update user %d profile: %w", id, err)
			}
		}
		return writeAudit(tx, actor, AuditActionUserUpdate, AuditTargetUser, id,
			fmt.Sprintf("更新用户「%s」的资料", u.Username))
	})
}

// ReplaceUserRoles 整体替换用户角色（空集合移除所有角色），在事务内校验并写审计。
// 只有 super 能授予/移除 super_admin 角色；不能移除最后一个活跃 super。
func (s *Service) ReplaceUserRoles(ctx context.Context, actor AuditMeta, userID int64, roleIDs []int64) error {
	ids := dedupeIDs(roleIDs)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := acquireAdminAdvisoryLock(tx); err != nil {
			return err
		}
		var u user.User
		if err := tx.Select("id", "username", "status").Where("id = ?", userID).First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return fmt.Errorf("rbac: load user %d: %w", userID, err)
		}

		currentCodes, err := userRoleCodesInTx(tx, userID)
		if err != nil {
			return err
		}
		currentHasSuper := containsString(currentCodes, RoleCodeSuperAdmin)

		newCodes, err := roleCodesForIDs(tx, ids)
		if err != nil {
			return err
		}
		newHasSuper := containsString(newCodes, RoleCodeSuperAdmin)

		actorSuper, err := userHasRoleCode(tx, actor.ActorID, RoleCodeSuperAdmin)
		if err != nil {
			return err
		}
		if (currentHasSuper || newHasSuper) && !actorSuper {
			return httpx.ErrForbidden
		}

		if currentHasSuper && !newHasSuper && u.Status == user.StatusActive {
			activeSupers, err := countActiveSuperAdmins(tx)
			if err != nil {
				return err
			}
			if activeSupers <= 1 {
				return invalidUser("不能移除最后一个活跃超级管理员的角色")
			}
		}

		if err := tx.Where("user_id = ?", userID).Delete(&UserRole{}).Error; err != nil {
			return fmt.Errorf("rbac: clear user roles: %w", err)
		}
		if len(ids) > 0 {
			assignedBy := actor.ActorID
			rows := make([]UserRole, len(ids))
			for i, roleID := range ids {
				rows[i] = UserRole{UserID: userID, RoleID: roleID, AssignedBy: &assignedBy, AssignedAt: time.Now()}
			}
			if err := tx.Create(&rows).Error; err != nil {
				return fmt.Errorf("rbac: insert user roles: %w", err)
			}
		}
		return writeAudit(tx, actor, AuditActionUserRolesAssign, AuditTargetUser, userID,
			fmt.Sprintf("更新用户「%s」的角色（%d 项）", u.Username, len(ids)))
	})
}

// SetUserBanned 封禁/解封用户，在事务内校验并写审计。禁止封禁自己；只有 super
// 能封/解封 super；禁止封禁最后一个活跃 super。
func (s *Service) SetUserBanned(ctx context.Context, actor AuditMeta, userID int64, banned bool, reason string) error {
	reason = strings.TrimSpace(reason)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := acquireAdminAdvisoryLock(tx); err != nil {
			return err
		}
		var u user.User
		if err := tx.Select("id", "username", "status").Where("id = ?", userID).First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return fmt.Errorf("rbac: load user %d: %w", userID, err)
		}

		targetSuper, err := userHasRoleCode(tx, userID, RoleCodeSuperAdmin)
		if err != nil {
			return err
		}
		actorSuper, err := userHasRoleCode(tx, actor.ActorID, RoleCodeSuperAdmin)
		if err != nil {
			return err
		}

		action := AuditActionUserUnban
		newStatus := user.StatusActive
		summary := fmt.Sprintf("解封用户「%s」", u.Username)
		if banned {
			action = AuditActionUserBan
			newStatus = user.StatusBanned
			if actor.ActorID == userID {
				return invalidUser("不能封禁自己")
			}
			if targetSuper && !actorSuper {
				return httpx.ErrForbidden
			}
			if targetSuper && u.Status == user.StatusActive {
				activeSupers, err := countActiveSuperAdmins(tx)
				if err != nil {
					return err
				}
				if activeSupers <= 1 {
					return invalidUser("不能封禁最后一个活跃超级管理员")
				}
			}
			summary = fmt.Sprintf("封禁用户「%s」", u.Username)
			if reason != "" {
				summary = fmt.Sprintf("封禁用户「%s」（原因：%s）", u.Username, reason)
			}
		} else if targetSuper && !actorSuper {
			return httpx.ErrForbidden
		}

		if u.Status != newStatus {
			if err := tx.Model(&user.User{}).Where("id = ?", userID).Update("status", newStatus).Error; err != nil {
				return fmt.Errorf("rbac: update user %d status: %w", userID, err)
			}
		}
		return writeAudit(tx, actor, action, AuditTargetUser, userID, summary)
	})
}

func adminUserItems(db *gorm.DB, users []user.User) ([]AdminUserItem, error) {
	items := make([]AdminUserItem, len(users))
	for i, u := range users {
		items[i] = AdminUserItem{
			ID: u.ID, Username: u.Username, Nickname: u.Nickname, Email: u.Email,
			Role: u.Role, Rating: u.Rating, Coins: u.Coins, Status: u.Status,
			Roles: []RoleSummary{}, CreatedAt: u.CreatedAt,
		}
	}
	if len(users) == 0 {
		return items, nil
	}
	ids := make([]int64, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}

	var rows []userRoleRow
	if err := db.Table("user_roles").
		Select("user_roles.user_id, roles.id, roles.code, roles.name").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id IN ?", ids).
		Order("roles.code").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("rbac: list user roles: %w", err)
	}
	byUser := make(map[int64][]RoleSummary, len(users))
	for _, row := range rows {
		byUser[row.UserID] = append(byUser[row.UserID], RoleSummary{ID: row.ID, Code: row.Code, Name: row.Name})
	}
	for i := range items {
		if roles := byUser[items[i].ID]; roles != nil {
			items[i].Roles = roles
		}
	}
	return items, nil
}

type userRoleRow struct {
	UserID int64
	ID     int64
	Code   string
	Name   string
}

func userRoleCodesInTx(tx *gorm.DB, userID int64) ([]string, error) {
	var codes []string
	if err := tx.Table("user_roles").
		Select("roles.code").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ?", userID).
		Order("roles.code").
		Scan(&codes).Error; err != nil {
		return nil, fmt.Errorf("rbac: load user role codes: %w", err)
	}
	return codes, nil
}

func roleCodesForIDs(tx *gorm.DB, ids []int64) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var roles []Role
	if err := tx.Select("id", "code").Where("id IN ?", ids).Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("rbac: load roles: %w", err)
	}
	if len(roles) != len(ids) {
		return nil, invalidUser("角色不存在")
	}
	codes := make([]string, len(roles))
	for i, role := range roles {
		codes[i] = role.Code
	}
	return codes, nil
}

func userHasRoleCode(tx *gorm.DB, userID int64, code string) (bool, error) {
	var count int64
	if err := tx.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ?", userID, code).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("rbac: check user role: %w", err)
	}
	return count > 0, nil
}

func countActiveSuperAdmins(tx *gorm.DB) (int64, error) {
	var count int64
	if err := tx.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Joins("JOIN users ON users.id = user_roles.user_id").
		Where("roles.code = ? AND users.status = ?", RoleCodeSuperAdmin, user.StatusActive).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("rbac: count active super admins: %w", err)
	}
	return count, nil
}

func dedupeIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func invalidUser(format string, args ...any) error {
	return httpx.ErrValidation(fmt.Sprintf(format, args...))
}
