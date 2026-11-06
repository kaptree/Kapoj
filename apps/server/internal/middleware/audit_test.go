package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func createAuditTestUser(t *testing.T, g *gorm.DB, username string) int64 {
	t.Helper()
	u := &user.User{
		Username:     username,
		Nickname:     username,
		Email:        username + "@example.com",
		PasswordHash: "not-a-real-hash",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	require.NoError(t, g.Create(u).Error)
	return u.ID
}

func postReq(r *gin.Engine, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("User-Agent", "audit-middleware-test")
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func auditRowCount(t *testing.T, g *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, g.Model(&rbac.AdminAuditLog{}).Count(&count).Error)
	return count
}

func TestAuditAdminActionWritesOnSuccess(t *testing.T) {
	g := testutil.PG(t)
	svc := testutil.SeedRBAC(t, g)
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)

	authorized := createAuditTestUser(t, g, "audit_authorized")
	testutil.AssignRole(t, g, authorized, rbac.RoleCodeContentAdmin)
	unauthorized := createAuditTestUser(t, g, "audit_unauthorized")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Authenticate(issuer))
	r.POST("/problems/:id", LoadAccess(svc), AuditAdminAction(svc, "problem.update", "problem"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.POST("/reject", LoadAccess(svc), AuditAdminAction(svc, "problem.update", "problem"), func(c *gin.Context) {
		c.Status(http.StatusBadRequest)
	})

	authorizedHeader := "Bearer " + token(t, issuer, authorized, user.RoleUser)
	unauthorizedHeader := "Bearer " + token(t, issuer, unauthorized, user.RoleUser)

	t.Run("2xx with permission writes audit", func(t *testing.T) {
		w := postReq(r, "/problems/123", authorizedHeader)
		require.Equal(t, http.StatusOK, w.Code)

		var row rbac.AdminAuditLog
		require.NoError(t, g.Where("action = ? AND target_type = ?", "problem.update", "problem").First(&row).Error)
		assert.Equal(t, authorized, row.ActorID)
		require.NotNil(t, row.TargetID)
		assert.Equal(t, int64(123), *row.TargetID)
		assert.Equal(t, "203.0.113.7", row.IP)
		assert.Equal(t, "audit-middleware-test", row.UserAgent)
		assert.Contains(t, row.Summary, "POST /problems/123")
		assert.Contains(t, row.Summary, "200")
	})

	t.Run("2xx without permission writes nothing", func(t *testing.T) {
		before := auditRowCount(t, g)
		w := postReq(r, "/problems/456", unauthorizedHeader)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, before, auditRowCount(t, g))
	})

	t.Run("4xx writes nothing", func(t *testing.T) {
		before := auditRowCount(t, g)
		w := postReq(r, "/reject", authorizedHeader)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, before, auditRowCount(t, g))
	})

	t.Run("audit failure keeps 2xx and logs warning", func(t *testing.T) {
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(previous) })

		require.NoError(t, g.Migrator().DropTable(&rbac.AdminAuditLog{}))

		w := postReq(r, "/problems/789", authorizedHeader)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, logs.String(), "level=WARN")
	})
}
