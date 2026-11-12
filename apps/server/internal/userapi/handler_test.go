package userapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type fakeIdentityLookup struct {
	identities map[int64]gamification.Identity
	err        error
	calls      [][]int64
}

func (f *fakeIdentityLookup) Identities(_ context.Context, userIDs []int64) (map[int64]gamification.Identity, error) {
	f.calls = append(f.calls, append([]int64{}, userIDs...))
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[int64]gamification.Identity, len(userIDs))
	for _, id := range userIDs {
		if identity, ok := f.identities[id]; ok {
			out[id] = identity
		}
	}
	return out, nil
}

type fakeGuestPermissions struct {
	codes []string
	err   error
	calls int
}

func (f *fakeGuestPermissions) GuestPermissions(_ context.Context) ([]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.codes, nil
}

type fakeActivityLookup struct {
	activity *submission.UserActivity
	err      error
	calls    []int64
}

func (f *fakeActivityLookup) UserActivity(_ context.Context, userID int64) (*submission.UserActivity, error) {
	f.calls = append(f.calls, userID)
	return f.activity, f.err
}

type fixture struct {
	engine *gin.Engine
	repo   user.Repository
	svc    *user.Service
	auth   *auth.Service
	issuer *auth.TokenIssuer
	db     *gorm.DB
}

func setup(t *testing.T) *fixture {
	t.Helper()
	return setupWithOptions(t)
}

func setupWithIdentity(t *testing.T, lookup IdentityLookup) *fixture {
	t.Helper()
	return setupWithOptions(t, Options{IdentityLookup: lookup})
}

func setupWithOptions(t *testing.T, opts ...Options) *fixture {
	t.Helper()
	return setupWithRepository(t, user.NewMemoryRepository(), opts...)
}

func setupWithDatabaseRBAC(t *testing.T) *fixture {
	t.Helper()
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))
	f := setupWithRepository(t, user.NewGormRepository(g), Options{RBAC: rbac.NewService(g)})
	f.db = g
	return f
}

func setupWithRepository(t *testing.T, repo user.Repository, opts ...Options) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	authSvc := auth.NewService(repo, issuer, auth.NewRefreshStore(rdb, time.Hour), auth.NewLoginLimiter(rdb))
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := user.NewService(repo, user.WithStorage(store))

	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	RegisterRoutes(r.Group("/api"), svc, opts...)
	RegisterAvatarRoutes(r.Group("/api"), r.Group("/api"), svc)
	return &fixture{engine: r, repo: repo, svc: svc, auth: authSvc, issuer: issuer}
}

func (f *fixture) register(t *testing.T, name string) (*user.User, string) {
	t.Helper()
	s, err := f.auth.Register(context.Background(), auth.RegisterInput{
		Username: name, Email: name + "@example.com", Password: "correct-horse",
	})
	require.NoError(t, err)
	return s.User, s.AccessToken
}

func (f *fixture) do(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestGetMeRequiresAuth(t *testing.T) {
	f := setup(t)
	w, resp := f.do(http.MethodGet, "/api/users/me", "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, resp["code"])
}

func TestGetMeReturnsPrivateFields(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")
	w, resp := f.do(http.MethodGet, "/api/users/me", tok, "")
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "alice", data["username"])
	assert.Equal(t, "alice@example.com", data["email"])
	assert.Equal(t, "user", data["role"])
	assert.EqualValues(t, 1500, data["rating"])
	assert.NotContains(t, w.Body.String(), "asswordHash")
}

func TestUpdateMeOnlyChangesProvidedFields(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")
	f.do(http.MethodPut, "/api/users/me", tok, `{"nickname":"小艾"}`)

	w, resp := f.do(http.MethodPut, "/api/users/me", tok, `{"bio":"喜欢图论"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "喜欢图论", data["bio"])
	assert.Equal(t, "小艾", data["nickname"])
}

func TestUpdateMeValidates(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")
	for _, body := range []string{
		`{"nickname":"   "}`,
		`{"bio":"` + strings.Repeat("字", 201) + `"}`,
		`{"avatarUrl":"javascript:alert(1)"}`,
	} {
		w, resp := f.do(http.MethodPut, "/api/users/me", tok, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		assert.EqualValues(t, 40001, resp["code"], body)
	}
}

func TestProfileIsPublicAndHidesEmail(t *testing.T) {
	f := setup(t)
	u, _ := f.register(t, "alice")
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/profile", u.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "alice", data["username"])
	assert.NotContains(t, data, "email")
	assert.NotContains(t, data, "role")
	assert.Equal(t, []any{}, data["equippedBadges"])
	assert.Contains(t, data, "equippedTitle")
	assert.Nil(t, data["equippedTitle"])
}

func TestProfileIncludesEquippedBadgesAndTitle(t *testing.T) {
	lookup := &fakeIdentityLookup{identities: map[int64]gamification.Identity{}}
	f := setupWithIdentity(t, lookup)
	u, _ := f.register(t, "alice")
	lookup.identities[u.ID] = gamification.Identity{
		Badges: []gamification.BadgeSummary{
			{ID: 7, Name: "早鸟", ImageURL: "/api/badges/7/image"},
			{ID: 3, Name: "全勤", ImageURL: "/api/badges/3/image"},
		},
		Title: &gamification.TitleSummary{ID: 9, Name: "卷王", Color: "#7C3AED"},
	}

	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/profile", u.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)

	badges := data["equippedBadges"].([]any)
	require.Len(t, badges, 2)
	first := badges[0].(map[string]any)
	assert.EqualValues(t, 7, first["id"])
	assert.Equal(t, "早鸟", first["name"])
	assert.Equal(t, "/api/badges/7/image", first["imageUrl"])
	assert.Equal(t, "全勤", badges[1].(map[string]any)["name"])

	title := data["equippedTitle"].(map[string]any)
	assert.EqualValues(t, 9, title["id"])
	assert.Equal(t, "卷王", title["name"])
	assert.Equal(t, "#7C3AED", title["color"])

	require.Len(t, lookup.calls, 1)
	assert.Equal(t, []int64{u.ID}, lookup.calls[0])

	// 游客同样能看到公开身份。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/profile", u.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, resp["data"].(map[string]any)["equippedBadges"], 2)
}

func TestProfileNotFound(t *testing.T) {
	f := setup(t)
	for _, path := range []string{"/api/users/999/profile", "/api/users/abc/profile", "/api/users/-1/profile"} {
		w, resp := f.do(http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.EqualValues(t, 40400, resp["code"], path)
	}
}

func TestUserActivityIsPublicAndReturnsCalendar(t *testing.T) {
	lookup := &fakeActivityLookup{activity: &submission.UserActivity{
		SubmissionCount:      12,
		AcceptedProblemCount: 5,
		ContributionTotal:    9,
		StartDate:            "2025-11-13",
		EndDate:              "2026-11-12",
		CurrentStreak:        2,
		LongestStreak:        4,
		Days: []submission.ActivityDay{
			{Date: "2025-11-13", Count: 0},
			{Date: "2026-11-12", Count: 3},
		},
	}}
	f := setupWithOptions(t, Options{Activity: lookup})
	u, _ := f.register(t, "alice")

	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/activity", u.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 12, data["submissionCount"])
	assert.EqualValues(t, 5, data["acceptedProblemCount"])
	assert.EqualValues(t, 9, data["contributionTotal"])
	assert.Equal(t, "2025-11-13", data["startDate"])
	assert.Equal(t, "2026-11-12", data["endDate"])
	assert.EqualValues(t, 2, data["currentStreak"])
	assert.EqualValues(t, 4, data["longestStreak"])

	days := data["days"].([]any)
	require.Len(t, days, 2)
	assert.Equal(t, "2025-11-13", days[0].(map[string]any)["date"])
	assert.EqualValues(t, 3, days[1].(map[string]any)["count"])

	assert.Equal(t, []int64{u.ID}, lookup.calls)
}

func TestUserActivityNotFound(t *testing.T) {
	lookup := &fakeActivityLookup{}
	f := setupWithOptions(t, Options{Activity: lookup})
	for _, path := range []string{"/api/users/999/activity", "/api/users/abc/activity", "/api/users/-1/activity"} {
		w, resp := f.do(http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.EqualValues(t, 40400, resp["code"], path)
	}
	assert.Empty(t, lookup.calls)
}

func grantMeTestRole(t *testing.T, g *gorm.DB, userID int64, roleCode string) {
	t.Helper()
	var role rbac.Role
	require.NoError(t, g.Where("code = ?", roleCode).First(&role).Error)
	require.NoError(t, g.Create(&rbac.UserRole{UserID: userID, RoleID: role.ID, AssignedAt: time.Now()}).Error)
}

func meResponseStringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	require.True(t, ok, "应为 JSON 数组，实际为 %T", value)
	out := make([]string, len(raw))
	for i, item := range raw {
		out[i] = item.(string)
	}
	return out
}

func TestMeIncludesPermissions(t *testing.T) {
	f := setupWithDatabaseRBAC(t)

	u, tok := f.register(t, "alice")
	grantMeTestRole(t, f.db, u.ID, rbac.RoleCodeContentAdmin)

	w, resp := f.do(http.MethodGet, "/api/users/me", tok, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)

	roles := data["roles"].([]any)
	require.Len(t, roles, 1)
	role := roles[0].(map[string]any)
	assert.Equal(t, rbac.RoleCodeContentAdmin, role["code"])
	assert.Equal(t, "内容管理员", role["name"])
	assert.NotZero(t, role["id"])

	permissions := meResponseStringSlice(t, data["permissions"])
	assert.Contains(t, permissions, "problem.view_hidden")
	assert.Contains(t, permissions, "problem.publish")
	assert.NotContains(t, permissions, "user.read")
	assert.Equal(t, false, data["isSuperAdmin"])

	// 同一账号叠加超级管理员角色后立即生效。
	grantMeTestRole(t, f.db, u.ID, rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, "/api/users/me", tok, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["isSuperAdmin"])
	assert.Len(t, data["roles"], 2)
}

func TestMeWithoutRolesHasEmptyPermissionArrays(t *testing.T) {
	f := setupWithDatabaseRBAC(t)
	_, tok := f.register(t, "alice")

	w, resp := f.do(http.MethodGet, "/api/users/me", tok, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, []any{}, data["roles"])
	assert.Equal(t, []any{}, data["permissions"])
	assert.Equal(t, false, data["isSuperAdmin"])
}

func TestMeWithoutRBACReturnsEmptyPermissions(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")

	w, resp := f.do(http.MethodGet, "/api/users/me", tok, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, []any{}, data["roles"])
	assert.Equal(t, []any{}, data["permissions"])
	assert.Equal(t, false, data["isSuperAdmin"])
}

func TestGuestPermissionsWithoutRBACReturnsEmptyArray(t *testing.T) {
	f := setup(t)
	w, resp := f.do(http.MethodGet, "/api/guest/permissions", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, []any{}, data["permissions"])
	assert.NotContains(t, w.Body.String(), `"permissions":null`)
}

func TestGuestPermissionsReturnsGuestRolePermissions(t *testing.T) {
	guest := &fakeGuestPermissions{codes: []string{"guest.growth.view"}}
	f := setupWithOptions(t, Options{GuestPermissions: guest})

	// 游客 token 为空也可访问。
	w, resp := f.do(http.MethodGet, "/api/guest/permissions", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []any{"guest.growth.view"}, resp["data"].(map[string]any)["permissions"])
	assert.Equal(t, 1, guest.calls)
}

func TestGuestPermissionsErrorReturnsInternal(t *testing.T) {
	guest := &fakeGuestPermissions{err: errors.New("guest lookup failed")}
	f := setupWithOptions(t, Options{GuestPermissions: guest})

	w, resp := f.do(http.MethodGet, "/api/guest/permissions", "", "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.EqualValues(t, 50000, resp["code"])
	assert.Equal(t, 1, guest.calls)
}

func (f *fixture) doMultipart(t *testing.T, method, path, token string, file []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "avatar.png")
	require.NoError(t, err)
	_, err = fw.Write(file)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func (f *fixture) doMultipartNoFile(t *testing.T, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("other", "x"))
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func TestUploadAvatarHTTP(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")

	w := f.doMultipart(t, http.MethodPost, "/api/users/me/avatar", tok, pngBytes())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["avatarUploaded"])
	assert.Contains(t, data["avatarUrl"].(string), "/api/users/")

	// 未登录 401。
	w = f.doMultipart(t, http.MethodPost, "/api/users/me/avatar", "", pngBytes())
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 缺 file 字段 40001。
	w = f.doMultipartNoFile(t, http.MethodPost, "/api/users/me/avatar", tok)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 非法图片 40001。
	w = f.doMultipart(t, http.MethodPost, "/api/users/me/avatar", tok, []byte("plain text"))
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestDeleteAvatarHTTP(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")

	w := f.doMultipart(t, http.MethodPost, "/api/users/me/avatar", tok, pngBytes())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp := f.do(http.MethodDelete, "/api/users/me/avatar", tok, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, false, resp["data"].(map[string]any)["avatarUploaded"])
	assert.Equal(t, "", resp["data"].(map[string]any)["avatarUrl"])

	// 幂等。
	w, _ = f.do(http.MethodDelete, "/api/users/me/avatar", tok, "")
	assert.Equal(t, http.StatusOK, w.Code)

	// 未登录 401。
	w, resp = f.do(http.MethodDelete, "/api/users/me/avatar", "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, resp["code"])
}

func TestAvatarImageHTTP(t *testing.T) {
	f := setup(t)
	u, tok := f.register(t, "alice")

	// 无上传 -> 404，公开（无需登录）。
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/avatar", u.ID), "", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	// 上传后公开可读，返回原始字节与 contentType。
	w = f.doMultipart(t, http.MethodPost, "/api/users/me/avatar", tok, pngBytes())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/users/%d/avatar", u.ID), nil)
	w = httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, pngBytes(), w.Body.Bytes())
	assert.Contains(t, w.Header().Get("Cache-Control"), "immutable")

	// 缺失用户 404。
	w, _ = f.do(http.MethodGet, "/api/users/999999/avatar", "", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMeAndProfileAvatarUploaded(t *testing.T) {
	f := setup(t)
	u, tok := f.register(t, "alice")

	// 上传前为 false。
	_, resp := f.do(http.MethodGet, "/api/users/me", tok, "")
	assert.Equal(t, false, resp["data"].(map[string]any)["avatarUploaded"])
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/profile", u.ID), "", "")
	assert.Equal(t, false, resp["data"].(map[string]any)["avatarUploaded"])

	// 上传后为 true。
	w := f.doMultipart(t, http.MethodPost, "/api/users/me/avatar", tok, pngBytes())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, resp = f.do(http.MethodGet, "/api/users/me", tok, "")
	assert.Equal(t, true, resp["data"].(map[string]any)["avatarUploaded"])
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/profile", u.ID), "", "")
	assert.Equal(t, true, resp["data"].(map[string]any)["avatarUploaded"])
}

func pngBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
}
