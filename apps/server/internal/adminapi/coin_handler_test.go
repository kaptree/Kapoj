package adminapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/adminapi"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// newCoinFixture 构造仅挂载 adminapi 的测试引擎；withGame 控制是否注入
// 游戏化服务（决定金币路由是否挂载）。金币调整不需要 storage。
func newCoinFixture(t *testing.T, withGame bool) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	svc := rbac.NewService(g)
	opts := []adminapi.Options{}
	if withGame {
		opts = append(opts, adminapi.Options{Game: gamification.NewService(gamification.NewRepository(g))})
	}
	adminapi.RegisterRoutes(r.Group("/api"), svc, opts...)
	return &fixture{engine: r, db: g, issuer: issuer, svc: svc}
}

func TestCoinAdjustRequiresPermissionAndAudits(t *testing.T) {
	f := newCoinFixture(t, true)

	// 未登录 401。
	w, resp := f.do(http.MethodPost, "/api/admin/users/1/coins", "", `{"amount":10,"note":"测试"}`)
	requireStatus(t, w, resp, http.StatusUnauthorized)

	// 无 user.coins.adjust（gamification_admin）403。
	_, gmTok := f.user(t, "coin_gamification", rbac.RoleCodeGamificationAdmin)
	w, resp = f.do(http.MethodPost, "/api/admin/users/1/coins", gmTok, `{"amount":10,"note":"测试"}`)
	requireStatus(t, w, resp, http.StatusForbidden)

	// user_admin 可调币并写审计。
	targetID, _ := f.user(t, "coin_target")
	_, uaTok := f.user(t, "coin_user_admin", rbac.RoleCodeUserAdmin)
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/coins", targetID),
		uaTok, `{"amount":100,"note":"活动奖励"}`)
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 100, dataMap(t, resp)["balance"])

	var logs []rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ?", rbac.AuditActionUserCoinsAdjust).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, rbac.AuditTargetUser, logs[0].TargetType)
	require.NotNil(t, logs[0].TargetID)
	assert.EqualValues(t, targetID, *logs[0].TargetID)
	assert.Equal(t, "调整用户金币 +100", logs[0].Summary)
	assert.NotContains(t, logs[0].Summary, "活动奖励")
}

func TestCoinRouteNotMountedWithoutGame(t *testing.T) {
	f := newCoinFixture(t, false)

	_, tok := f.user(t, "coin_nogame_admin", rbac.RoleCodeUserAdmin)
	w, _ := f.do(http.MethodPost, "/api/admin/users/1/coins", tok, `{"amount":10,"note":"测试"}`)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}
