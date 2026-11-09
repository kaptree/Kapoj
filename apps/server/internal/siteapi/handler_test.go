package siteapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t      *testing.T
	h      http.Handler
	db     *gorm.DB
	rbac   *rbac.Service
	tokens map[string]string
	nextN  int
}

func newSiteFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)

	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	h := router.New(router.Deps{
		DB:      g,
		Redis:   redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer: noopQueue{},
		RBAC:     rbacSvc,
	})
	return &fx{t: t, h: h, db: g, rbac: rbacSvc, tokens: map[string]string{}}
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

func (f *fx) register(name string) (int64, string) {
	f.t.Helper()
	w, _ := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.com", "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	w, resp := f.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"account": name, "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	tok := data["accessToken"].(string)
	userID := int64(data["user"].(map[string]any)["id"].(float64))
	return userID, tok
}

// tokenForRole 创建（或复用）指定 code 的角色并把新建用户分配到该角色。
// perms 为该角色的权限码；nil 表示无权限的普通用户。
func (f *fx) tokenForRole(t *testing.T, roleCode string, perms []string) string {
	t.Helper()
	key := roleCode + "|" + strings.Join(perms, ",")
	if tok, ok := f.tokens[key]; ok {
		return tok
	}

	var role rbac.Role
	err := f.db.Where("code = ?", roleCode).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_, createErr := f.rbac.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
			Code: roleCode, Name: roleCode, PermissionCodes: perms,
		})
		require.NoError(t, createErr)
	} else {
		require.NoError(t, err)
	}

	f.nextN++
	userID, tok := f.register(fmt.Sprintf("%s%d", strings.ReplaceAll(roleCode, "_", ""), f.nextN))
	testutil.AssignRole(t, f.db, userID, roleCode)
	f.tokens[key] = tok
	return tok
}

// multipart 以字段名 file 上传一张图片。
func (f *fx) multipart(path, token string, data []byte) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "image.png")
	require.NoError(f.t, err)
	_, err = part.Write(data)
	require.NoError(f.t, err)
	require.NoError(f.t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestSitePublicAndAdminHTTP(t *testing.T) {
	f := newSiteFx(t)
	// 公开读取无需登录
	w, resp := f.do(http.MethodGet, "/api/site", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotEmpty(t, resp["data"].(map[string]any)["home"])

	// 未登录管理接口 401
	w, _ = f.do(http.MethodGet, "/api/admin/site", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 普通用户没有 site.settings.manage -> 403
	userToken := f.tokenForRole(t, "site_user", nil)
	w, _ = f.do(http.MethodGet, "/api/admin/site", userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 拥有权限的用户可读写
	managerToken := f.tokenForRole(t, "site_manager", []string{"site.settings.manage"})
	w, resp = f.do(http.MethodGet, "/api/admin/site", managerToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	home := resp["data"].(map[string]any)["home"].(map[string]any)
	home["eyebrow"] = "UPDATED"
	w, _ = f.do(http.MethodPut, "/api/admin/site/home", managerToken, home)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// 公开读取已更新
	_, resp = f.do(http.MethodGet, "/api/site", "", nil)
	assert.Equal(t, "UPDATED", resp["data"].(map[string]any)["home"].(map[string]any)["eyebrow"])
}

func TestSiteWriteAudit(t *testing.T) {
	f := newSiteFx(t)
	token := f.tokenForRole(t, "site_manager", []string{"site.settings.manage"})

	w, resp := f.do(http.MethodGet, "/api/admin/site", token, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	home := resp["data"].(map[string]any)["home"].(map[string]any)
	home["eyebrow"] = "AUDITED"
	w, _ = f.do(http.MethodPut, "/api/admin/site/home", token, home)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var settingsAudit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_type = ?",
		"site.settings.manage", "site_settings").Order("id DESC").First(&settingsAudit).Error)
	assert.NotZero(t, settingsAudit.ActorID)
	assert.Equal(t, "修改首页内容", settingsAudit.Summary)

	w, _ = f.multipart("/api/admin/site/logo", token, pngBytes(t))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var assetAudit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_type = ?",
		"site.settings.manage", "site_asset").Order("id DESC").First(&assetAudit).Error)
	assert.Equal(t, "更新站点 Logo", assetAudit.Summary)
}

func TestSiteUploadAndVisitHTTP(t *testing.T) {
	f := newSiteFx(t)
	token := f.tokenForRole(t, "site_manager", []string{"site.settings.manage"})
	png := pngBytes(t)
	w, resp := f.multipart("/api/admin/site/logo", token, png)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	logoURL := resp["data"].(map[string]any)["logoUrl"].(string)
	assert.Contains(t, logoURL, "/api/site/logo")
	w, _ = f.do(http.MethodGet, logoURL, "", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodPost, "/api/site/visit", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["visitCount"])
	_, resp = f.do(http.MethodPost, "/api/site/visit", "", nil)
	assert.EqualValues(t, 2, resp["data"].(map[string]any)["visitCount"])
}
