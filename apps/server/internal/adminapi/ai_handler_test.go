package adminapi_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/adminapi"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// newAISettingsFixture 构造挂载了 AI 设置接口的 adminapi 测试引擎。
func newAISettingsFixture(t *testing.T, keyConfigured bool) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	svc := rbac.NewService(g)

	envKey := ""
	if keyConfigured {
		envKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	}
	keyMgr, err := ai.NewKeyManager(envKey, "")
	require.NoError(t, err)
	settings := ai.NewSettingsService(g, keyMgr, "")
	adminapi.RegisterRoutes(r.Group("/api"), svc, adminapi.Options{AI: settings})
	return &fixture{engine: r, db: g, issuer: issuer, svc: svc}
}

func TestAdminAISettingsPermission(t *testing.T) {
	f := newAISettingsFixture(t, true)

	// 未登录 401。
	w, resp := f.do(http.MethodGet, "/api/admin/ai/settings", "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)
	assert.EqualValues(t, 40100, resp["code"])

	// 无 ai.settings.manage（content_admin）403。
	_, contentTok := f.user(t, "ai_content", rbac.RoleCodeContentAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/ai/settings", contentTok, "")
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	// super_admin 绕过 200。
	_, superTok := f.user(t, "ai_super", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/ai/settings", superTok, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.Equal(t, true, data["enabled"])
	assert.Equal(t, true, data["keyConfigured"])

	// 拥有 ai.settings.manage 的自定义角色 200。
	role, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "ai_manager", Name: "AI 管理员",
		PermissionCodes: []string{"ai.settings.manage"},
	})
	require.NoError(t, err)
	_, mgrTok := f.user(t, "ai_manager_user", role.Code)
	w, resp = f.do(http.MethodGet, "/api/admin/ai/settings", mgrTok, "")
	requireStatus(t, w, resp, http.StatusOK)
}

func TestAdminAISettingsUpdateAndAudit(t *testing.T) {
	t.Run("启用关闭与审计", func(t *testing.T) {
		f := newAISettingsFixture(t, true)

		role, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
			Code: "ai_manager_2", Name: "AI 管理员2",
			PermissionCodes: []string{"ai.settings.manage"},
		})
		require.NoError(t, err)
		actorID, tok := f.user(t, "ai_manager_user_2", role.Code)

		// 关闭 AI：200，enabled=false。
		w, resp := f.do(http.MethodPut, "/api/admin/ai/settings", tok, `{"enabled":false}`)
		requireStatus(t, w, resp, http.StatusOK)
		assert.Equal(t, false, dataMap(t, resp)["enabled"])

		// 重新启用：200，enabled=true。
		w, resp = f.do(http.MethodPut, "/api/admin/ai/settings", tok, `{"enabled":true}`)
		requireStatus(t, w, resp, http.StatusOK)
		assert.Equal(t, true, dataMap(t, resp)["enabled"])

		// 审计 action/target/summary。
		var logs []rbac.AdminAuditLog
		require.NoError(t, f.db.Where("action = ?", rbac.AuditActionAISettingsManage).
			Order("id").Find(&logs).Error)
		require.Len(t, logs, 2)
		assert.Equal(t, rbac.AuditTargetAISettings, logs[0].TargetType)
		require.NotNil(t, logs[0].TargetID)
		assert.EqualValues(t, 1, *logs[0].TargetID)
		assert.Equal(t, actorID, logs[0].ActorID)
		assert.Contains(t, logs[0].Summary, "关闭 AI 功能")
		assert.Contains(t, logs[1].Summary, "启用 AI 功能")
	})

	t.Run("缺省 enabled 字段", func(t *testing.T) {
		f := newAISettingsFixture(t, true)
		_, tok := f.user(t, "ai_omit_super", rbac.RoleCodeSuperAdmin)

		// 空 body：40001，状态保持不变（不写审计、不改变 enabled）。
		w, resp := f.do(http.MethodPut, "/api/admin/ai/settings", tok, `{}`)
		requireStatus(t, w, resp, http.StatusBadRequest)
		assert.EqualValues(t, 40001, resp["code"])

		w, resp = f.do(http.MethodGet, "/api/admin/ai/settings", tok, "")
		requireStatus(t, w, resp, http.StatusOK)
		assert.Equal(t, true, dataMap(t, resp)["enabled"])

		var count int64
		require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).
			Where("action = ?", rbac.AuditActionAISettingsManage).Count(&count).Error)
		assert.Equal(t, int64(0), count, "缺省 enabled 不写审计")

		// 显式 true/false 仍可用。
		w, resp = f.do(http.MethodPut, "/api/admin/ai/settings", tok, `{"enabled":false}`)
		requireStatus(t, w, resp, http.StatusOK)
		assert.Equal(t, false, dataMap(t, resp)["enabled"])

		w, resp = f.do(http.MethodPut, "/api/admin/ai/settings", tok, `{"enabled":true}`)
		requireStatus(t, w, resp, http.StatusOK)
		assert.Equal(t, true, dataMap(t, resp)["enabled"])
	})

	t.Run("未配置密钥", func(t *testing.T) {
		f := newAISettingsFixture(t, false)
		_, tok := f.user(t, "ai_nokey_super", rbac.RoleCodeSuperAdmin)

		// 未配置密钥：启用 -> 40001；关闭允许。
		w, resp := f.do(http.MethodPut, "/api/admin/ai/settings", tok, `{"enabled":true}`)
		requireStatus(t, w, resp, http.StatusBadRequest)
		assert.EqualValues(t, 40001, resp["code"])

		w, resp = f.do(http.MethodPut, "/api/admin/ai/settings", tok, `{"enabled":false}`)
		requireStatus(t, w, resp, http.StatusOK)
		assert.Equal(t, false, dataMap(t, resp)["enabled"])

		// 失败（未配置密钥）不写成功审计。
		var count int64
		require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).
			Where("action = ?", rbac.AuditActionAISettingsManage).Count(&count).Error)
		assert.Equal(t, int64(1), count, "仅关闭操作写审计，启用失败不写")
	})
}
