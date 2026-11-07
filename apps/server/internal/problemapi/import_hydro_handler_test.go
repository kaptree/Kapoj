package problemapi_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/problemapi"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func hydroZipBytes(t *testing.T, entries ...struct{ name, body string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		cw, err := w.Create(e.name)
		require.NoError(t, err)
		_, err = cw.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func validHydroZip(t *testing.T) []byte {
	t.Helper()
	return hydroZipBytes(t,
		struct{ name, body string }{"6760/problem.yaml", "title: 求和\ntag:\n  - 循环\n"},
		struct{ name, body string }{"6760/problem_zh.md", "题面"},
		struct{ name, body string }{"6760/testdata/1.in", "1"},
		struct{ name, body string }{"6760/testdata/1.out", "2"},
	)
}

func TestImportHydroHandlerPermissions(t *testing.T) {
	f := newFx(t)
	zip := validHydroZip(t)
	path := "/api/problems/import/hydro"

	w, resp := f.multipart(path, "", nil, filePart{"file", "p.zip", bytes.NewReader(zip)})
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])

	w, resp = f.multipart(path, f.token("user"), nil, filePart{"file", "p.zip", bytes.NewReader(zip)})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.EqualValues(t, 40300, resp["code"])

	w, resp = f.multipart(path, f.token("problem_admin"), nil, filePart{"file", "p.zip", bytes.NewReader(zip)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["code"])
}

func TestImportHydroHandlerResultAndAudit(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")

	// 两题：一好（6760）一坏（6761 缺题面）。
	zip := hydroZipBytes(t,
		struct{ name, body string }{"6760/problem.yaml", "title: 好题\n"},
		struct{ name, body string }{"6760/problem_zh.md", "好题面"},
		struct{ name, body string }{"6760/testdata/1.in", "1"},
		struct{ name, body string }{"6760/testdata/1.out", "2"},
		struct{ name, body string }{"6761/problem.yaml", "title: 坏题\n"},
		struct{ name, body string }{"6761/testdata/1.in", "1"},
		struct{ name, body string }{"6761/testdata/1.out", "2"},
	)

	w, resp := f.multipart("/api/problems/import/hydro", admin, nil, filePart{"file", "p.zip", bytes.NewReader(zip)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	data := resp["data"].(map[string]any)
	imported := data["imported"].([]any)
	failed := data["failed"].([]any)
	require.Len(t, imported, 1)
	require.Len(t, failed, 1)

	imp := imported[0].(map[string]any)
	assert.Equal(t, "好题", imp["title"])
	assert.Equal(t, "hydro-6760", imp["slug"])
	assert.EqualValues(t, 1, imp["testcaseCount"])
	id := int64(imp["id"].(float64))

	assert.Equal(t, "6761", failed[0].(map[string]any)["folder"])

	// 每个成功题目一条 problem.create 审计，target=problem，id=新题 id。
	var count int64
	require.NoError(t, f.db.Table("admin_audit_logs").
		Where("action = ? AND target_type = ? AND target_id = ?", "problem.create", "problem", id).
		Count(&count).Error)
	assert.Equal(t, int64(1), count, "one audit row per imported problem")
}

// newImportEngine builds a minimal router with overridable import limits so the
// body/file size checks can be exercised with tiny payloads.
func newImportEngine(t *testing.T, maxFile, maxBody int64) (http.Handler, string, *problem.Service) {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := problem.NewService(problem.NewGormRepository(g), store)

	require.NoError(t, g.Exec(`INSERT INTO users (username, nickname, email, password_hash, created_at, updated_at)
		VALUES ('importer', 'Importer', 'importer@example.com', 'x', now(), now())`).Error)
	var uid int64
	require.NoError(t, g.Raw(`SELECT id FROM users WHERE username = 'importer'`).Scan(&uid).Error)
	testutil.AssignRole(t, g, uid, rbac.RoleCodeContentAdmin)
	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	token, err := issuer.IssueAccess(&user.User{ID: uid, Role: user.RoleUser})
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(1<<20))
	problemJSON := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(4<<20))
	uploads := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(40<<20))
	imports := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(maxBody))
	problemapi.RegisterRoutes(api, problemJSON, uploads, imports, svc, nil, nil,
		problemapi.Options{RBAC: rbacSvc, MaxImportFileBytes: maxFile})
	return r, token, svc
}

func importMultipart(t *testing.T, h http.Handler, token string, fileBody []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "p.zip")
	require.NoError(t, err)
	_, err = part.Write(fileBody)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/problems/import/hydro", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestImportHydroHandlerBodyLimit(t *testing.T) {
	const (
		maxFile = 1 << 10 // 1 KiB
		maxBody = 4 << 10 // 4 KiB
	)
	h, token, _ := newImportEngine(t, maxFile, maxBody)

	// 文件超过 1 KiB 但整包在 body 限制内 → handler 返回 40001。
	w, resp := importMultipart(t, h, token, bytes.Repeat([]byte("a"), maxFile+1))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])

	// 整包超过 4 KiB → 中间件返回 413。
	w, resp = importMultipart(t, h, token, bytes.Repeat([]byte("a"), maxBody+1))
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
	assert.EqualValues(t, 41300, resp["code"])
}

func TestImportHydroHandlerMissingFile(t *testing.T) {
	f := newFx(t)
	w, resp := f.multipart("/api/problems/import/hydro", f.token("problem_admin"), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
}
