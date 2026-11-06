package gamificationapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// customRoleToken 创建一个只拥有给定权限码的自定义角色并分配给新用户。
func (f *fx) customRoleToken(name string, codes ...string) string {
	f.t.Helper()
	key := "custom:" + name
	if tok, ok := f.tokens[key]; ok {
		return tok
	}
	svc := rbac.NewService(f.db)
	role, err := svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: name, Name: name, PermissionCodes: codes,
	})
	require.NoError(f.t, err)

	f.nextN++
	uname := fmt.Sprintf("cu%s%d", name, f.nextN)
	userID, tok := f.register(uname)
	testutil.AssignRole(f.t, f.db, userID, role.Code)
	f.tokens[key] = tok
	f.ids[key] = userID
	return tok
}

func TestLegacyRoleWithoutRBACIsForbidden(t *testing.T) {
	f := newFx(t)
	legacy := f.legacyOnlyToken("super_admin")

	w, _ := f.do(http.MethodPost, "/api/achievements", legacy,
		achievementBody("legacy-ac", "旧角色成就", "ac_count", map[string]any{"threshold": 1}, 10))
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, _ = f.do(http.MethodPost,
		fmt.Sprintf("/api/admin/users/%d/coins", f.ids["legacy:super_admin"]),
		legacy, map[string]any{"amount": 10, "note": "测试"})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGamificationAdminDomainsButNotCoins(t *testing.T) {
	f := newFx(t)
	tok := f.rbacToken(rbac.RoleCodeGamificationAdmin)
	f.token("user")
	targetID := f.ids["user"]

	w, _ := f.do(http.MethodPost, "/api/achievements", tok,
		achievementBody("ga-ac", "成就", "ac_count", map[string]any{"threshold": 1}, 10))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w, _ = f.do(http.MethodPost, "/api/tasks", tok,
		taskBody("任务", []int64{f.addPublicProblem(t, "ga-task").ID}, 10,
			time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w, _ = f.do(http.MethodPost, "/api/badges", tok, badgeBody("徽章", 10))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w, _ = f.do(http.MethodPost, "/api/titles", tok,
		titleBody("称号", "#7C3AED", "rating", map[string]any{"rating": 1600}, 10))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// 游戏化管理员不能调整金币。
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/coins", targetID),
		tok, map[string]any{"amount": 10, "note": "测试"})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUserAdminCoinsButNotGamification(t *testing.T) {
	f := newFx(t)
	tok := f.rbacToken(rbac.RoleCodeUserAdmin)
	f.token("user")
	targetID := f.ids["user"]

	w, resp := f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/coins", targetID),
		tok, map[string]any{"amount": 100, "note": "活动奖励"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 100, resp["data"].(map[string]any)["balance"])

	// 用户管理员不能管理游戏化内容。
	w, _ = f.do(http.MethodPost, "/api/achievements", tok,
		achievementBody("ua-ac", "成就", "ac_count", map[string]any{"threshold": 1}, 10))
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodPost, "/api/badges", tok, badgeBody("徽章", 10))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestIncludeInactiveDomainSpecific(t *testing.T) {
	f := newFx(t)
	achOnly := f.customRoleToken("ach_only_role", string(rbac.PermGamificationAchievementManage))

	// 拥有 achievement.manage：成就 includeInactive 可读。
	w, _ := f.do(http.MethodGet, "/api/achievements?includeInactive=true", achOnly, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 但任务/徽章/称号的 includeInactive 各自按域拒绝。
	for _, path := range []string{"/api/tasks", "/api/badges", "/api/titles"} {
		w, _ = f.do(http.MethodGet, path+"?includeInactive=true", achOnly, nil)
		assert.Equal(t, http.StatusForbidden, w.Code, path)
	}
}

func TestGamificationContentAudit(t *testing.T) {
	f := newFx(t)
	tok := f.rbacToken(rbac.RoleCodeGamificationAdmin)

	w, _ := f.do(http.MethodPost, "/api/achievements", tok,
		achievementBody("audit-ac", "审计成就", "ac_count", map[string]any{"threshold": 1}, 10))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var logs []rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ?", string(rbac.PermGamificationAchievementManage)).
		Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "achievement", logs[0].TargetType)
	assert.NotEmpty(t, logs[0].Summary)
}

func TestCoinAdjustAuditRow(t *testing.T) {
	f := newFx(t)
	tok := f.rbacToken(rbac.RoleCodeUserAdmin)
	f.token("user")
	targetID := f.ids["user"]

	w, resp := f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/coins", targetID),
		tok, map[string]any{"amount": 100, "note": "活动奖励"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 100, resp["data"].(map[string]any)["balance"])

	var logs []rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ?", rbac.AuditActionUserCoinsAdjust).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, rbac.AuditTargetUser, logs[0].TargetType)
	require.NotNil(t, logs[0].TargetID)
	assert.EqualValues(t, targetID, *logs[0].TargetID)
	assert.Equal(t, "调整金币 100", logs[0].Summary)
	assert.NotContains(t, logs[0].Summary, "活动奖励")
}
