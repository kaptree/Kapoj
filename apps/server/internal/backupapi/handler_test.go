package backupapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t    *testing.T
	h    http.Handler
	db   *gorm.DB
	rbac *rbac.Service
	svc  *backup.Service
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	mr := miniredis.RunT(t)
	root := t.TempDir()
	store, err := storage.NewLocal(filepath.Join(root, "data"))
	require.NoError(t, err)
	svc, err := backup.NewService(g, os.Getenv("KAPOJ_TEST_DATABASE_URL"), backup.Options{
		StorageDir: filepath.Join(root, "data"),
		BackupDir:  filepath.Join(root, "data", "backups"),
	})
	require.NoError(t, err)
	require.NoError(t, svc.Ensure(context.Background()))
	h := router.New(router.Deps{
		DB:      g,
		Redis:   redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		Storage: store,
		Config: &config.Config{
			JWTSecret: strings.Repeat("s", 32), AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
		Enqueuer: noopQueue{}, RBAC: rbacSvc, Backup: svc,
	})
	return &fx{t: t, h: h, db: g, rbac: rbacSvc, svc: svc}
}

func (f *fx) do(method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(f.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func (f *fx) userWithBackupRole(t *testing.T) (int64, string) {
	t.Helper()
	u := &user.User{
		Username: "backup-admin", Nickname: "Backup Admin",
		Email: "backup-admin@example.com", PasswordHash: "x",
	}
	require.NoError(t, f.db.Create(u).Error)
	_, err := f.rbac.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "backup_admin", Name: "backup_admin",
		PermissionCodes: []string{string(rbac.PermSystemBackupManage)},
	})
	require.NoError(t, err)
	testutil.AssignRole(t, f.db, u.ID, "backup_admin")
	token, err := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute).
		IssueAccess(u)
	require.NoError(t, err)
	return u.ID, token
}

func TestBackupAPIRequiresPermission(t *testing.T) {
	f := newFx(t)
	w, _ := f.do(http.MethodGet, "/api/admin/backups", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	plain := &user.User{Username: "plain", Nickname: "Plain", Email: "plain@example.com", PasswordHash: "x"}
	require.NoError(t, f.db.Create(plain).Error)
	token, err := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute).IssueAccess(plain)
	require.NoError(t, err)
	w, _ = f.do(http.MethodGet, "/api/admin/backups", token, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestBackupAPICreateDownloadScheduleAndAudit(t *testing.T) {
	f := newFx(t)
	userID, token := f.userWithBackupRole(t)

	w, resp := f.do(http.MethodPost, "/api/admin/backups/platform", token, nil)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	pkg := resp["data"].(map[string]any)
	require.Equal(t, "ready", pkg["status"])
	id := int64(pkg["id"].(float64))
	require.NotEmpty(t, pkg["filename"])

	w, _ = f.do(http.MethodGet, "/api/admin/backups?x=1", token, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups/"+strconv.FormatInt(id, 10)+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	assert.Greater(t, w.Body.Len(), 100)

	w, resp = f.do(http.MethodPut, "/api/admin/backups/schedules/platform", token, map[string]any{
		"enabled": true, "frequency": "daily", "runHour": 3, "runMinute": 30,
		"weekday": 1, "dayOfMonth": 1,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, resp["data"].(map[string]any)["enabled"])

	w, _ = f.do(http.MethodPut, "/api/admin/backups/schedules/platform", token, map[string]any{
		"enabled": true, "frequency": "bogus", "runHour": 3, "runMinute": 30,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("actor_id = ? AND target_type = ?", userID, rbac.AuditTargetBackup).
		Order("id DESC").First(&audit).Error)
	assert.Equal(t, rbac.AuditActionSystemBackupManage, audit.Action)
}

func TestBackupAPIRestoreValidation(t *testing.T) {
	f := newFx(t)
	_, token := f.userWithBackupRole(t)
	w, resp := f.do(http.MethodPost, "/api/admin/backups/restore/platform", token, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.do(http.MethodPost, "/api/admin/backups/restore/platform", token, map[string]any{
		"packageId": 1,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.do(http.MethodPost, "/api/admin/backups/restore/account", token, map[string]any{
		"packageId": 1,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}
