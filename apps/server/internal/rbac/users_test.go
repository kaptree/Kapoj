package rbac_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func roleIDForCode(t *testing.T, g *gorm.DB, code string) int64 {
	t.Helper()
	var role rbac.Role
	require.NoError(t, g.Where("code = ?", code).First(&role).Error)
	return role.ID
}

func TestAdminListUsersFilters(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	alice := createSeedUser(t, g, "filter_alice", user.RoleUser)
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", alice).Update("nickname", "100%_done").Error)
	bob := createSeedUser(t, g, "filter_bob", user.RoleUser)
	assignAccessTestRole(t, g, bob, rbac.RoleCodeContentAdmin)
	charlie := createSeedUser(t, g, "filter_charlie", user.RoleUser)
	assignAccessTestRole(t, g, charlie, rbac.RoleCodeContestAdmin)
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", charlie).Update("status", user.StatusBanned).Error)

	contentRoleID := roleIDForCode(t, g, rbac.RoleCodeContentAdmin)

	// 无过滤。
	page, err := svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), page.Total)
	require.Len(t, page.Items, 3)

	// keyword 命中 username。
	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Keyword: "bob"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "filter_bob", page.Items[0].Username)

	// keyword 命中 email。
	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Keyword: "charlie@example.com"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "filter_charlie", page.Items[0].Username)

	// keyword 命中 nickname，且特殊字符被转义。
	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Keyword: "100%"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "filter_alice", page.Items[0].Username)

	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Keyword: "%"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "字面 % 只应命中含 % 的昵称")
	assert.Equal(t, "filter_alice", page.Items[0].Username)

	// status 过滤。
	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Status: user.StatusBanned})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "filter_charlie", page.Items[0].Username)
	assert.Equal(t, user.StatusBanned, page.Items[0].Status)

	// roleId 过滤，并包含角色摘要。
	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{RoleID: contentRoleID})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "filter_bob", page.Items[0].Username)
	require.Len(t, page.Items[0].Roles, 1)
	assert.Equal(t, rbac.RoleCodeContentAdmin, page.Items[0].Roles[0].Code)
	assert.Equal(t, contentRoleID, page.Items[0].Roles[0].ID)
	assert.NotEmpty(t, page.Items[0].Roles[0].Name)

	// 分页与 total。
	page, err = svc.AdminListUsers(ctx, 2, 2, rbac.AdminUserFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "filter_charlie", page.Items[0].Username)

	// 封禁用户仍出现在列表与详情中。
	detail, err := svc.AdminUser(ctx, charlie)
	require.NoError(t, err)
	assert.Equal(t, user.StatusBanned, detail.Status)
}

func TestAdminUpdateUser(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	actorID := createSeedUser(t, g, "update_user_actor", user.RoleUser)
	assignAccessTestRole(t, g, actorID, rbac.RoleCodeSuperAdmin)
	target := createSeedUser(t, g, "update_user_target", user.RoleUser)
	actor := rbac.AuditMeta{ActorID: actorID, IP: "203.0.113.6", UserAgent: "update-user-test"}

	nickname := "新昵称"
	bio := "新的简介"
	avatar := "https://example.com/a.png"
	require.NoError(t, svc.AdminUpdateUser(ctx, actor, target, rbac.AdminUserUpdateInput{
		Nickname: &nickname, Bio: &bio, AvatarURL: &avatar,
	}))

	var u user.User
	require.NoError(t, g.Where("id = ?", target).First(&u).Error)
	assert.Equal(t, "新昵称", u.Nickname)
	assert.Equal(t, "新的简介", u.Bio)
	assert.Equal(t, "https://example.com/a.png", u.AvatarURL)

	var audit rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_type = ? AND target_id = ?",
		"user.update", "user", target).First(&audit).Error)
	assert.Equal(t, actorID, audit.ActorID)
	assert.Equal(t, "203.0.113.6", audit.IP)
	assert.Equal(t, "update-user-test", audit.UserAgent)
	assert.Contains(t, audit.Summary, "update_user_target")

	// 非法输入。
	badNick := "   "
	requireAppErrorCode(t, svc.AdminUpdateUser(ctx, actor, target, rbac.AdminUserUpdateInput{Nickname: &badNick}), 40001)
	badBio := strings.Repeat("字", 201)
	requireAppErrorCode(t, svc.AdminUpdateUser(ctx, actor, target, rbac.AdminUserUpdateInput{Bio: &badBio}), 40001)
	badAvatar := "javascript:alert(1)"
	requireAppErrorCode(t, svc.AdminUpdateUser(ctx, actor, target, rbac.AdminUserUpdateInput{AvatarURL: &badAvatar}), 40001)

	// 非法输入不改变数据。
	require.NoError(t, g.Where("id = ?", target).First(&u).Error)
	assert.Equal(t, "新昵称", u.Nickname)

	// 用户不存在。
	requireAppErrorCode(t, svc.AdminUpdateUser(ctx, actor, 999999, rbac.AdminUserUpdateInput{Nickname: &nickname}), 40400)
}

func TestReplaceUserRoles(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	actorID := createSeedUser(t, g, "replace_roles_actor", user.RoleUser)
	assignAccessTestRole(t, g, actorID, rbac.RoleCodeSuperAdmin)
	target := createSeedUser(t, g, "replace_roles_target", user.RoleUser)
	actor := rbac.AuditMeta{ActorID: actorID, IP: "203.0.113.5", UserAgent: "roles-assign-test"}

	contentRoleID := roleIDForCode(t, g, rbac.RoleCodeContentAdmin)
	contestRoleID := roleIDForCode(t, g, rbac.RoleCodeContestAdmin)

	// 多角色分配并去重。
	require.NoError(t, svc.ReplaceUserRoles(ctx, actor, target,
		[]int64{contentRoleID, contestRoleID, contentRoleID}))
	assert.Equal(t, []string{rbac.RoleCodeContentAdmin, rbac.RoleCodeContestAdmin}, userRoleCodes(t, g, target))

	// assigned_by 来自 actor。
	var rows []rbac.UserRole
	require.NoError(t, g.Where("user_id = ?", target).Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.NotNil(t, row.AssignedBy)
		assert.Equal(t, actorID, *row.AssignedBy)
	}

	// 审计。
	var audit rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_type = ? AND target_id = ?",
		"user.roles.assign", "user", target).First(&audit).Error)
	assert.Equal(t, actorID, audit.ActorID)
	assert.Contains(t, audit.Summary, "replace_roles_target")

	// 未知角色 -> 40001，且不改变现有角色。
	requireAppErrorCode(t, svc.ReplaceUserRoles(ctx, actor, target,
		[]int64{contentRoleID, 999999}), 40001)
	assert.Equal(t, []string{rbac.RoleCodeContentAdmin, rbac.RoleCodeContestAdmin}, userRoleCodes(t, g, target))

	// 空列表移除所有角色。
	require.NoError(t, svc.ReplaceUserRoles(ctx, actor, target, nil))
	assert.Empty(t, userRoleCodes(t, g, target))

	// 用户不存在。
	requireAppErrorCode(t, svc.ReplaceUserRoles(ctx, actor, 999999, []int64{contentRoleID}), 40400)
}

func TestOnlySuperAssignsSuper(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	superActor := createSeedUser(t, g, "assign_actor_super", user.RoleUser)
	assignAccessTestRole(t, g, superActor, rbac.RoleCodeSuperAdmin)
	secActor := createSeedUser(t, g, "assign_actor_sec", user.RoleUser)
	assignAccessTestRole(t, g, secActor, rbac.RoleCodeSecurityAdmin)

	target := createSeedUser(t, g, "assign_target", user.RoleUser)
	superRoleID := roleIDForCode(t, g, rbac.RoleCodeSuperAdmin)
	contentRoleID := roleIDForCode(t, g, rbac.RoleCodeContentAdmin)

	superMeta := rbac.AuditMeta{ActorID: superActor}
	secMeta := rbac.AuditMeta{ActorID: secActor}

	// 非 super 授予 super -> 403。
	requireAppErrorCode(t, svc.ReplaceUserRoles(ctx, secMeta, target, []int64{superRoleID}), 40300)
	assert.Empty(t, userRoleCodes(t, g, target))

	// 非 super 移除 super 目标的 super -> 403。
	superTarget := createSeedUser(t, g, "assign_super_target", user.RoleUser)
	assignAccessTestRole(t, g, superTarget, rbac.RoleCodeSuperAdmin)
	requireAppErrorCode(t, svc.ReplaceUserRoles(ctx, secMeta, superTarget, []int64{contentRoleID}), 40300)
	assert.Equal(t, []string{rbac.RoleCodeSuperAdmin}, userRoleCodes(t, g, superTarget))

	// super 可以授予 super。
	require.NoError(t, svc.ReplaceUserRoles(ctx, superMeta, target, []int64{superRoleID, contentRoleID}))
	assert.Equal(t, []string{rbac.RoleCodeContentAdmin, rbac.RoleCodeSuperAdmin}, userRoleCodes(t, g, target))
}

func TestLastSuperProtection(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	super1 := createSeedUser(t, g, "last_protect_s1", user.RoleUser)
	assignAccessTestRole(t, g, super1, rbac.RoleCodeSuperAdmin)
	super2 := createSeedUser(t, g, "last_protect_s2", user.RoleUser)
	assignAccessTestRole(t, g, super2, rbac.RoleCodeSuperAdmin)

	actor1 := rbac.AuditMeta{ActorID: super1}

	// 有两个 super：移除其中一个允许。
	require.NoError(t, svc.ReplaceUserRoles(ctx, actor1, super2, nil))
	assert.Empty(t, userRoleCodes(t, g, super2))

	// 只剩 super1：移除最后一个活跃 super 被拒（40001）。
	requireAppErrorCode(t, svc.ReplaceUserRoles(ctx, actor1, super1, nil), 40001)
	assert.Equal(t, []string{rbac.RoleCodeSuperAdmin}, userRoleCodes(t, g, super1))

	// 有两个 super 时允许封禁其中一个。
	assignAccessTestRole(t, g, super2, rbac.RoleCodeSuperAdmin)
	require.NoError(t, svc.SetUserBanned(ctx, actor1, super2, true, "违规"))
	assert.Equal(t, user.StatusBanned, userStatus(t, g, super2))

	// 封禁最后一个活跃 super 被拒：用"持有 super 角色但已封禁"的 actor，
	// 使 super1 成为唯一活跃 super（防御纵深分支）。
	bannedSuper := createSeedUser(t, g, "last_protect_banned", user.RoleUser)
	assignAccessTestRole(t, g, bannedSuper, rbac.RoleCodeSuperAdmin)
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", bannedSuper).Update("status", user.StatusBanned).Error)
	requireAppErrorCode(t, svc.SetUserBanned(ctx, rbac.AuditMeta{ActorID: bannedSuper}, super1, true, "违规"), 40001)
	assert.Equal(t, user.StatusActive, userStatus(t, g, super1))
}

func TestBanRules(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	superActor := createSeedUser(t, g, "ban_actor_super", user.RoleUser)
	assignAccessTestRole(t, g, superActor, rbac.RoleCodeSuperAdmin)
	userAdminActor := createSeedUser(t, g, "ban_actor_useradmin", user.RoleUser)
	assignAccessTestRole(t, g, userAdminActor, rbac.RoleCodeUserAdmin)

	normal := createSeedUser(t, g, "ban_normal", user.RoleUser)
	superTarget := createSeedUser(t, g, "ban_super_target", user.RoleUser)
	assignAccessTestRole(t, g, superTarget, rbac.RoleCodeSuperAdmin)
	anotherSuper := createSeedUser(t, g, "ban_another_super", user.RoleUser)
	assignAccessTestRole(t, g, anotherSuper, rbac.RoleCodeSuperAdmin)

	superMeta := rbac.AuditMeta{ActorID: superActor, IP: "127.0.0.1", UserAgent: "ban-test"}
	userAdminMeta := rbac.AuditMeta{ActorID: userAdminActor, IP: "127.0.0.1", UserAgent: "ban-test"}

	// 不能封禁自己。
	requireAppErrorCode(t, svc.SetUserBanned(ctx, superMeta, superActor, true, "自封"), 40001)

	// 只有 super 能封/解封 super。
	requireAppErrorCode(t, svc.SetUserBanned(ctx, userAdminMeta, superTarget, true, "越权"), 40300)
	requireAppErrorCode(t, svc.SetUserBanned(ctx, userAdminMeta, superTarget, false, ""), 40300)

	// super 封禁普通用户成功并审计。
	require.NoError(t, svc.SetUserBanned(ctx, superMeta, normal, true, "恶意行为"))
	assert.Equal(t, user.StatusBanned, userStatus(t, g, normal))

	var audit rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_type = ? AND target_id = ?",
		"user.ban", "user", normal).First(&audit).Error)
	assert.Equal(t, superActor, audit.ActorID)
	assert.Contains(t, audit.Summary, "恶意行为")

	// 解封成功并审计。
	require.NoError(t, svc.SetUserBanned(ctx, superMeta, normal, false, ""))
	assert.Equal(t, user.StatusActive, userStatus(t, g, normal))
	audit = rbac.AdminAuditLog{}
	require.NoError(t, g.Where("action = ? AND target_id = ?", "user.unban", normal).First(&audit).Error)
	assert.Equal(t, superActor, audit.ActorID)
}

func TestConcurrentLastSuperRemoval(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	super1 := createSeedUser(t, g, "conc_super1", user.RoleUser)
	assignAccessTestRole(t, g, super1, rbac.RoleCodeSuperAdmin)
	super2 := createSeedUser(t, g, "conc_super2", user.RoleUser)
	assignAccessTestRole(t, g, super2, rbac.RoleCodeSuperAdmin)

	actor1 := rbac.AuditMeta{ActorID: super1}
	actor2 := rbac.AuditMeta{ActorID: super2}

	// 两个 super 各自并发移除自己的 super 角色；advisory lock 串行化后，
	// 后一个必然发现只剩一个活跃 super，被拒（40001）。
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = svc.ReplaceUserRoles(ctx, actor1, super1, nil) }()
	go func() { defer wg.Done(); errs[1] = svc.ReplaceUserRoles(ctx, actor2, super2, nil) }()
	wg.Wait()

	var success, rejected int
	for _, err := range errs {
		if err == nil {
			success++
			continue
		}
		var appErr *httpx.AppError
		require.True(t, errors.As(err, &appErr), "应为 AppError，实际为 %T: %v", err, err)
		assert.Equal(t, 40001, appErr.Code, appErr.Message)
		rejected++
	}
	assert.Equal(t, 1, success, "恰好一个移除成功")
	assert.Equal(t, 1, rejected, "恰好一个移除被拒")

	// 最终恰好剩一个活跃 super。
	remaining := 0
	if slices.Contains(userRoleCodes(t, g, super1), rbac.RoleCodeSuperAdmin) {
		remaining++
	}
	if slices.Contains(userRoleCodes(t, g, super2), rbac.RoleCodeSuperAdmin) {
		remaining++
	}
	assert.Equal(t, 1, remaining, "并发移除后必须仍有一个超级管理员")
}

func TestNonSuperAssignsNonSuperRoles(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	secActor := createSeedUser(t, g, "assign_nonsuper_actor", user.RoleUser)
	assignAccessTestRole(t, g, secActor, rbac.RoleCodeSecurityAdmin)

	target := createSeedUser(t, g, "assign_nonsuper_target", user.RoleUser)
	contentRoleID := roleIDForCode(t, g, rbac.RoleCodeContentAdmin)
	superRoleID := roleIDForCode(t, g, rbac.RoleCodeSuperAdmin)

	secMeta := rbac.AuditMeta{ActorID: secActor}

	// 非 super 且有 user.roles.assign 可分配非 super 角色。
	require.NoError(t, svc.ReplaceUserRoles(ctx, secMeta, target, []int64{contentRoleID}))
	assert.Equal(t, []string{rbac.RoleCodeContentAdmin}, userRoleCodes(t, g, target))

	// 非 super 可移除非 super 角色（清空）。
	require.NoError(t, svc.ReplaceUserRoles(ctx, secMeta, target, nil))
	assert.Empty(t, userRoleCodes(t, g, target))

	// 非 super 分配 super 仍 403。
	requireAppErrorCode(t, svc.ReplaceUserRoles(ctx, secMeta, target, []int64{superRoleID}), 40300)
	assert.Empty(t, userRoleCodes(t, g, target))
}

func TestAdminListUsersKeywordEscaping(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	// 用户名含下划线；昵称/邮箱无其他特殊字符。
	_ = createSeedUser(t, g, "esc_under", user.RoleUser)
	// 昵称含百分号。
	percent := createSeedUser(t, g, "escpercent", user.RoleUser)
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", percent).Update("nickname", "50%off").Error)
	// 昵称含反斜杠。
	backslash := createSeedUser(t, g, "escbackslash", user.RoleUser)
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", backslash).Update("nickname", `C:\dir`).Error)
	// 无任何特殊字符。
	createSeedUser(t, g, "escplain", user.RoleUser)

	// 字面 `_` 只命中含下划线的记录（username/email），不充当单字符通配符。
	page, err := svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Keyword: "_"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "esc_under", page.Items[0].Username)

	// 字面 `%` 只命中含百分号的记录。
	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Keyword: "%"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "escpercent", page.Items[0].Username)

	// 字面 `\` 只命中含反斜杠的记录（nickname）。
	page, err = svc.AdminListUsers(ctx, 1, 20, rbac.AdminUserFilter{Keyword: `\`})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "escbackslash", page.Items[0].Username)
}

func userStatus(t *testing.T, g *gorm.DB, userID int64) user.Status {
	t.Helper()
	var u user.User
	require.NoError(t, g.Select("status").Where("id = ?", userID).First(&u).Error)
	return u.Status
}

func TestAdminUpdateAvatarClearsUpload(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	actorID := createSeedUser(t, g, "avatar_actor", user.RoleUser)
	assignAccessTestRole(t, g, actorID, rbac.RoleCodeSuperAdmin)
	target := createSeedUser(t, g, "avatar_target", user.RoleUser)
	actor := rbac.AuditMeta{ActorID: actorID}

	internalURL := "/api/users/" + strconv.FormatInt(target, 10) + "/avatar?v=1"

	// 直接给目标设置一个已上传头像。
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", target).Updates(map[string]any{
		"avatar_key":          "avatars/1/abc",
		"avatar_content_type": "image/png",
		"avatar_url":          internalURL,
	}).Error)

	// 非空外链 -> 清空 key/content_type。
	external := "https://example.com/a.png"
	require.NoError(t, svc.AdminUpdateUser(ctx, actor, target, rbac.AdminUserUpdateInput{AvatarURL: &external}))
	var u user.User
	require.NoError(t, g.Where("id = ?", target).First(&u).Error)
	assert.Equal(t, external, u.AvatarURL)
	assert.Empty(t, u.AvatarKey)
	assert.Empty(t, u.AvatarContentType)

	// 重新设置上传，空外链 + 已有上传 -> 保留 key 与内部 URL。
	internalURL2 := "/api/users/" + strconv.FormatInt(target, 10) + "/avatar?v=2"
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", target).Updates(map[string]any{
		"avatar_key":          "avatars/1/def",
		"avatar_content_type": "image/gif",
		"avatar_url":          internalURL2,
	}).Error)
	empty := ""
	require.NoError(t, svc.AdminUpdateUser(ctx, actor, target, rbac.AdminUserUpdateInput{AvatarURL: &empty}))
	require.NoError(t, g.Where("id = ?", target).First(&u).Error)
	assert.Equal(t, internalURL2, u.AvatarURL)
	assert.Equal(t, "avatars/1/def", u.AvatarKey)
	assert.Equal(t, "image/gif", u.AvatarContentType)
}
