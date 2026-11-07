package adminapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/adminapi"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// newRatingFixture 构造挂载了 rating 设置接口的 adminapi 测试引擎；固定 now
// 让赛季计算可断言。
func newRatingFixture(t *testing.T) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	svc := rbac.NewService(g)
	ratingSvc := rating.NewService(g, nil, nil, rating.Options{
		Now: func() time.Time { return time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC) },
	})
	adminapi.RegisterRoutes(r.Group("/api"), svc, adminapi.Options{Rating: ratingSvc})
	return &fixture{engine: r, db: g, issuer: issuer, svc: svc}
}

func TestAdminRatingSettingsPermission(t *testing.T) {
	f := newRatingFixture(t)

	// 未登录 401。
	w, resp := f.do(http.MethodGet, "/api/admin/rating/settings", "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)

	// 无 rating.settings.manage（content_admin）403。
	_, contentTok := f.user(t, "rating_content", rbac.RoleCodeContentAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/rating/settings", contentTok, "")
	requireStatus(t, w, resp, http.StatusForbidden)

	// super_admin 绕过 200。
	_, superTok := f.user(t, "rating_super", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/rating/settings", superTok, "")
	requireStatus(t, w, resp, http.StatusOK)
	dataMap(t, resp)

	// 拥有 rating.settings.manage 的自定义角色 200。
	role, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "rating_manager", Name: "Rating 管理员",
		PermissionCodes: []string{"rating.settings.manage"},
	})
	require.NoError(t, err)
	_, mgrTok := f.user(t, "rating_manager_user", role.Code)
	w, resp = f.do(http.MethodGet, "/api/admin/rating/settings", mgrTok, "")
	requireStatus(t, w, resp, http.StatusOK)
}

func TestAdminRatingSettingsUpdateAndAudit(t *testing.T) {
	f := newRatingFixture(t)

	role, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "rating_manager_2", Name: "Rating 管理员2",
		PermissionCodes: []string{"rating.settings.manage"},
	})
	require.NoError(t, err)
	actorID, tok := f.user(t, "rating_manager_user_2", role.Code)

	// 非法输入 -> 40001。
	w, resp := f.do(http.MethodPut, "/api/admin/rating/settings", tok,
		`{"decayEnabled":true,"decayThreshold":999,"decayInactiveDays":60,"decayPercent":3,
		  "decayMax":100,"seasonEnabled":true,"seasonMonths":6,"seasonResetPercent":30,
		  "seasonAnchor":"2026-11-01"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	// 合法 PUT 保存并返回设置与当前赛季。
	w, resp = f.do(http.MethodPut, "/api/admin/rating/settings", tok,
		`{"decayEnabled":false,"decayThreshold":2600,"decayInactiveDays":90,"decayPercent":5,
		  "decayMax":200,"seasonEnabled":true,"seasonMonths":6,"seasonResetPercent":40,
		  "seasonAnchor":"2026-11-01"}`)
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.Equal(t, false, data["decayEnabled"])
	assert.EqualValues(t, 2600, data["decayThreshold"])
	assert.EqualValues(t, 90, data["decayInactiveDays"])
	assert.EqualValues(t, 5, data["decayPercent"])
	assert.EqualValues(t, 200, data["decayMax"])
	assert.EqualValues(t, 40, data["seasonResetPercent"])
	assert.Equal(t, "2026-11-01", data["seasonAnchor"])
	currentSeason, ok := data["currentSeason"].(map[string]any)
	require.True(t, ok, "响应应包含 currentSeason")
	assert.Equal(t, "2026-11-01", currentSeason["key"])
	assert.NotEmpty(t, currentSeason["nextResetAt"])

	// 审计 action=rating.settings.manage target=rating_settings id=1。
	var logs []rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ?", rbac.AuditActionRatingSettingsManage).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, rbac.AuditTargetRatingSettings, logs[0].TargetType)
	require.NotNil(t, logs[0].TargetID)
	assert.EqualValues(t, 1, *logs[0].TargetID)
	assert.Equal(t, actorID, logs[0].ActorID)
}
