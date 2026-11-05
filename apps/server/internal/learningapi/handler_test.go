package learningapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t  *testing.T
	h  http.Handler
	db *gorm.DB
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	h := router.New(router.Deps{
		DB: g, Redis: rdb, Storage: store, Enqueuer: noopQueue{},
		Learning: learning.NewService(learning.NewRepository(g)),
		Config: &config.Config{
			JWTSecret: testJWTSecret, AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
	})
	return &fx{t: t, h: h, db: g}
}

func (f *fx) user(username string, role user.Role) int64 {
	f.t.Helper()
	require.NoError(f.t, f.db.Exec(
		`INSERT INTO users (username, nickname, email, password_hash, role, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', ?, now(), now())`,
		username, "昵称-"+username, username+"@example.com", role,
	).Error)
	var id int64
	require.NoError(f.t, f.db.Raw(`SELECT id FROM users WHERE username = ?`, username).Scan(&id).Error)
	return id
}

func (f *fx) token(userID int64, role user.Role) string {
	f.t.Helper()
	token, err := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute).
		IssueAccess(&user.User{ID: userID, Role: role})
	require.NoError(f.t, err)
	return token
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

func responseData(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "expected data object, got %T", resp["data"])
	return data
}

func pageData(t *testing.T, resp map[string]any) (float64, []any) {
	t.Helper()
	data := responseData(t, resp)
	items, ok := data["items"].([]any)
	require.True(t, ok, "expected data.items array, got %T", data["items"])
	return data["total"].(float64), items
}

func requireCode(t *testing.T, resp map[string]any, code int) {
	t.Helper()
	assert.EqualValues(t, code, resp["code"], resp)
}

func TestLearningPathAPIAuthValidationAndCRUD(t *testing.T) {
	f := newFx(t)
	authorID := f.user("path-author", user.RoleUser)
	otherID := f.user("path-other", user.RoleUser)
	adminID := f.user("path-admin", user.RoleProblemAdmin)
	authorToken := f.token(authorID, user.RoleUser)
	otherToken := f.token(otherID, user.RoleUser)
	adminToken := f.token(adminID, user.RoleProblemAdmin)

	w, resp := f.do(http.MethodGet, "/api/learning-paths", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items := pageData(t, resp)
	assert.EqualValues(t, 0, total)
	assert.Empty(t, items)

	w, resp = f.do(http.MethodPost, "/api/learning-paths", "", map[string]any{"title": "游客路线"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	requireCode(t, resp, 40100)

	w, resp = f.do(http.MethodPost, "/api/learning-paths", authorToken, map[string]any{"title": ""})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	requireCode(t, resp, 40001)

	w, resp = f.do(http.MethodPost, "/api/learning-paths", authorToken,
		map[string]any{"title": "路线", "coverUrl": "ftp://bad"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	requireCode(t, resp, 40001)

	w, resp = f.do(http.MethodPost, "/api/learning-paths", authorToken,
		map[string]any{"title": "我的路线", "descriptionMd": "描述", "sortOrder": 3})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := responseData(t, resp)
	pathID := int64(created["id"].(float64))
	assert.Equal(t, "我的路线", created["title"])
	assert.Equal(t, false, created["isPublic"])
	assert.Equal(t, false, created["isOfficial"])
	assert.Equal(t, true, created["isActive"])
	assert.EqualValues(t, 0, created["nodeCount"])
	assert.EqualValues(t, 0, created["myCompletedCount"])
	assert.Equal(t, "path-author", created["author"].(map[string]any)["username"])

	w, resp = f.do(http.MethodGet, "/api/learning-paths", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 0, total, "私有路线不出现在公开列表")
	assert.Empty(t, items)

	pathURL := "/api/learning-paths/" + itoa(pathID)
	w, resp = f.do(http.MethodGet, pathURL, "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodGet, pathURL, otherToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodPut, pathURL, otherToken, map[string]any{"title": "他人修改"})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodPut, pathURL, authorToken, map[string]any{"title": "改名"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "改名", responseData(t, resp)["title"])

	w, resp = f.do(http.MethodPut, pathURL, authorToken,
		map[string]any{"title": "改名", "isOfficial": true})
	assert.Equal(t, http.StatusForbidden, w.Code)
	requireCode(t, resp, 40300)

	w, resp = f.do(http.MethodPut, pathURL, authorToken,
		map[string]any{"title": "改名", "isActive": false})
	assert.Equal(t, http.StatusForbidden, w.Code)
	requireCode(t, resp, 40300)

	w, resp = f.do(http.MethodPut, pathURL, authorToken,
		map[string]any{"title": "改名", "isPublic": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, responseData(t, resp)["isPublic"])

	w, resp = f.do(http.MethodGet, "/api/learning-paths", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, "path-author", items[0].(map[string]any)["author"].(map[string]any)["username"])

	w, _ = f.do(http.MethodGet, pathURL, "", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodDelete, pathURL, otherToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodDelete, pathURL, authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, responseData(t, resp)["deleted"])

	w, resp = f.do(http.MethodGet, pathURL, "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodGet, pathURL, adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, false, responseData(t, resp)["isActive"])

	w, resp = f.do(http.MethodGet, "/api/learning-paths?includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, false, items[0].(map[string]any)["isActive"])

	w, resp = f.do(http.MethodPut, pathURL, adminToken,
		map[string]any{"title": "改名", "isActive": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, responseData(t, resp)["isActive"])

	w, resp = f.do(http.MethodDelete, "/api/learning-paths/999999", adminToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodGet, "/api/learning-paths/not-a-number", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)
}

func TestLearningPathAPIOfficialPermissions(t *testing.T) {
	f := newFx(t)
	authorID := f.user("official-author", user.RoleUser)
	adminID := f.user("official-admin", user.RoleProblemAdmin)
	authorToken := f.token(authorID, user.RoleUser)
	adminToken := f.token(adminID, user.RoleProblemAdmin)

	w, resp := f.do(http.MethodPost, "/api/learning-paths", authorToken,
		map[string]any{"title": "冒充官方", "isOfficial": true})
	assert.Equal(t, http.StatusForbidden, w.Code)
	requireCode(t, resp, 40300)

	w, resp = f.do(http.MethodPost, "/api/learning-paths", adminToken,
		map[string]any{"title": "官方路线", "isOfficial": true, "isPublic": false})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := responseData(t, resp)
	pathID := int64(created["id"].(float64))
	assert.Equal(t, true, created["isOfficial"])
	assert.Equal(t, true, created["isPublic"])
	assert.Equal(t, true, created["isActive"])

	pathURL := "/api/learning-paths/" + itoa(pathID)
	w, resp = f.do(http.MethodGet, pathURL, "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, responseData(t, resp)["isOfficial"])

	w, resp = f.do(http.MethodPut, pathURL, adminToken, map[string]any{"title": "官方路线2"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := responseData(t, resp)
	assert.Equal(t, true, updated["isOfficial"])
	assert.Equal(t, true, updated["isPublic"])
	assert.Equal(t, true, updated["isActive"])

	w, resp = f.do(http.MethodPut, pathURL, adminToken,
		map[string]any{"title": "官方路线2", "isOfficial": false})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	requireCode(t, resp, 40001)

	w, resp = f.do(http.MethodPut, pathURL, authorToken, map[string]any{"title": "篡改官方"})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodDelete, pathURL, authorToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, _ = f.do(http.MethodDelete, pathURL, adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp = f.do(http.MethodGet, "/api/learning-paths?includeInactive=true", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, _ := pageData(t, resp)
	assert.EqualValues(t, 0, total, "inactive 路线只在管理员列表中")

	w, resp = f.do(http.MethodPut, pathURL, adminToken,
		map[string]any{"title": "官方路线2", "isActive": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, responseData(t, resp)["isActive"])

	w, resp = f.do(http.MethodPost, "/api/learning-paths", adminToken,
		map[string]any{"title": "管理员下架路线", "isActive": false})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, false, responseData(t, resp)["isActive"])

	w, resp = f.do(http.MethodGet, "/api/learning-paths", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, _ = pageData(t, resp)
	assert.EqualValues(t, 1, total, "inactive 不进入公开列表")

	w, resp = f.do(http.MethodGet, "/api/learning-paths?includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items := pageData(t, resp)
	assert.EqualValues(t, 2, total)
	assert.Len(t, items, 2)
}

func TestLearningPathAPIListFilters(t *testing.T) {
	f := newFx(t)
	authorID := f.user("filter-author", user.RoleUser)
	otherID := f.user("filter-other", user.RoleUser)
	adminID := f.user("filter-admin", user.RoleProblemAdmin)
	authorToken := f.token(authorID, user.RoleUser)
	otherToken := f.token(otherID, user.RoleUser)
	adminToken := f.token(adminID, user.RoleProblemAdmin)

	create := func(token string, body map[string]any) int64 {
		w, resp := f.do(http.MethodPost, "/api/learning-paths", token, body)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		return int64(responseData(t, resp)["id"].(float64))
	}
	officialB := create(adminToken, map[string]any{"title": "官方B", "isOfficial": true, "sortOrder": 1})
	officialA := create(adminToken, map[string]any{"title": "官方A", "isOfficial": true, "sortOrder": 2})
	public := create(authorToken, map[string]any{"title": "公开自定义", "isPublic": true})
	private := create(authorToken, map[string]any{"title": "作者私有"})
	otherPrivate := create(otherToken, map[string]any{"title": "他人私有"})

	node := &learning.LearningPathNode{
		PathID: public, Title: "节点1", ProblemIDs: dbtypes.List[int64]{}, RequiredCount: 1,
	}
	require.NoError(t, f.db.Create(node).Error)
	require.NoError(t, f.db.Create(&learning.LearningPathNode{
		PathID: public, Title: "节点2", ProblemIDs: dbtypes.List[int64]{}, RequiredCount: 1,
	}).Error)
	require.NoError(t, f.db.Create(&learning.UserLearningProgress{
		UserID: authorID, PathID: public, NodeID: node.ID,
		Status: learning.ProgressCompleted, SolvedCount: 1, UpdatedAt: time.Now(),
	}).Error)

	w, resp := f.do(http.MethodGet, "/api/learning-paths", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items := pageData(t, resp)
	assert.EqualValues(t, 3, total)
	assert.Equal(t, []float64{float64(officialB), float64(officialA), float64(public)}, itemIDs(items))
	assert.EqualValues(t, 2, items[2].(map[string]any)["nodeCount"])

	w, resp = f.do(http.MethodGet, "/api/learning-paths", authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 4, total)
	assert.EqualValues(t, 1, items[2].(map[string]any)["myCompletedCount"])

	w, resp = f.do(http.MethodGet, "/api/learning-paths?mine=true", authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, []float64{float64(public), float64(private)}, itemIDs(items))

	w, resp = f.do(http.MethodGet, "/api/learning-paths?mine=true", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, _ = pageData(t, resp)
	assert.EqualValues(t, 0, total)

	w, resp = f.do(http.MethodGet, "/api/learning-paths?includeInactive=true", authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, _ = pageData(t, resp)
	assert.EqualValues(t, 4, total, "非管理员 includeInactive 被忽略")

	w, resp = f.do(http.MethodGet, "/api/learning-paths?includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 5, total)
	assert.Equal(t, []float64{
		float64(officialB), float64(officialA), float64(public), float64(private), float64(otherPrivate),
	}, itemIDs(items))

	w, _ = f.do(http.MethodDelete, "/api/learning-paths/"+itoa(private), authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp = f.do(http.MethodGet, "/api/learning-paths?mine=true", authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, []float64{float64(public)}, itemIDs(items))

	w, resp = f.do(http.MethodGet, "/api/learning-paths?includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 5, total)
	assert.Equal(t, false, items[3].(map[string]any)["isActive"])

	w, resp = f.do(http.MethodGet, "/api/learning-paths?mine=true&includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	total, items = pageData(t, resp)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, []float64{float64(officialB), float64(officialA)}, itemIDs(items))
}

func itemIDs(items []any) []float64 {
	out := make([]float64, len(items))
	for i, item := range items {
		out[i] = item.(map[string]any)["id"].(float64)
	}
	return out
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}

func TestLearningPathAPINodesAndTree(t *testing.T) {
	f := newFx(t)
	authorID := f.user("node-author", user.RoleUser)
	otherID := f.user("node-other", user.RoleUser)
	adminID := f.user("node-admin", user.RoleProblemAdmin)
	authorToken := f.token(authorID, user.RoleUser)
	otherToken := f.token(otherID, user.RoleUser)
	adminToken := f.token(adminID, user.RoleProblemAdmin)

	addProblem := func(slug string, public bool) *problem.Problem {
		t.Helper()
		p := &problem.Problem{
			Title: "题目-" + slug, Slug: slug, StatementMD: "题干",
			Type: problem.TypeTraditional, Difficulty: 2,
			TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: public,
		}
		require.NoError(t, f.db.Create(p).Error)
		return p
	}
	pub := addProblem("api-node-pub", true)
	hidden := addProblem("api-node-hidden", false)

	w, resp := f.do(http.MethodPost, "/api/learning-paths", authorToken,
		map[string]any{"title": "API 节点路线"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	pathID := int64(responseData(t, resp)["id"].(float64))
	pathURL := "/api/learning-paths/" + itoa(pathID)
	nodesURL := pathURL + "/nodes"

	w, resp = f.do(http.MethodPost, nodesURL, "", map[string]any{"title": "游客节点"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	requireCode(t, resp, 40100)

	w, resp = f.do(http.MethodPost, nodesURL, otherToken, map[string]any{"title": "他人节点"})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodPost, nodesURL, authorToken,
		map[string]any{"title": "", "problemIds": []int64{pub.ID}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	requireCode(t, resp, 40001)

	w, resp = f.do(http.MethodPost, nodesURL, authorToken,
		map[string]any{"title": "私有题节点", "problemIds": []int64{hidden.ID}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	requireCode(t, resp, 40001)

	w, resp = f.do(http.MethodPost, nodesURL, authorToken, map[string]any{
		"title": "根节点", "descriptionMd": "根描述",
		"problemIds": []int64{pub.ID}, "sortOrder": 2,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	root := responseData(t, resp)
	rootID := int64(root["id"].(float64))
	assert.Equal(t, "根节点", root["title"])
	assert.Nil(t, root["parentId"])
	assert.EqualValues(t, 1, root["requiredCount"])
	assert.EqualValues(t, 0, root["solvedCount"])
	assert.Equal(t, learning.ProgressActive, root["status"])
	problems := root["problems"].([]any)
	require.Len(t, problems, 1)
	summary := problems[0].(map[string]any)
	assert.Len(t, summary, 4, "树只返回 problemId/title/slug/difficulty")
	assert.EqualValues(t, pub.ID, summary["problemId"])
	assert.Equal(t, "题目-api-node-pub", summary["title"])
	assert.Equal(t, "api-node-pub", summary["slug"])
	assert.EqualValues(t, 2, summary["difficulty"])

	w, resp = f.do(http.MethodPost, nodesURL, authorToken, map[string]any{
		"title": "子节点", "parentId": rootID, "problemIds": []int64{pub.ID},
		"requiredCount": 1,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	child := responseData(t, resp)
	childID := int64(child["id"].(float64))
	assert.Equal(t, learning.ProgressLocked, child["status"])
	assert.EqualValues(t, rootID, child["parentId"])

	// 私有路线游客不可见。
	w, resp = f.do(http.MethodGet, pathURL+"/tree", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, _ = f.do(http.MethodPut, pathURL, authorToken,
		map[string]any{"title": "API 节点路线", "isPublic": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp = f.do(http.MethodGet, pathURL+"/tree", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	tree := responseData(t, resp)
	assert.Equal(t, "API 节点路线", tree["path"].(map[string]any)["title"])
	rootNodes := tree["nodes"].([]any)
	require.Len(t, rootNodes, 1)
	rootNode := rootNodes[0].(map[string]any)
	assert.Equal(t, "根节点", rootNode["title"])
	assert.Equal(t, learning.ProgressActive, rootNode["status"])
	children := rootNode["children"].([]any)
	require.Len(t, children, 1)
	assert.Equal(t, "子节点", children[0].(map[string]any)["title"])
	assert.Equal(t, learning.ProgressLocked, children[0].(map[string]any)["status"])

	w, resp = f.do(http.MethodPut, nodesURL+"/"+itoa(childID), authorToken,
		map[string]any{"title": "改名子节点", "parentId": rootID, "problemIds": []int64{pub.ID}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "改名子节点", responseData(t, resp)["title"])

	w, resp = f.do(http.MethodPut, nodesURL+"/"+itoa(childID), otherToken,
		map[string]any{"title": "他人改名", "problemIds": []int64{pub.ID}})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodDelete, nodesURL+"/"+itoa(rootID), authorToken, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	requireCode(t, resp, 40001)

	// 节点 id 必须属于 URL 中的路线。
	otherPath := func() int64 {
		w, resp := f.do(http.MethodPost, "/api/learning-paths", authorToken,
			map[string]any{"title": "另一条路线"})
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		return int64(responseData(t, resp)["id"].(float64))
	}()
	w, resp = f.do(http.MethodPut,
		"/api/learning-paths/"+itoa(otherPath)+"/nodes/"+itoa(childID), authorToken,
		map[string]any{"title": "跨路线", "problemIds": []int64{pub.ID}})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodDelete, nodesURL+"/"+itoa(childID), authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, responseData(t, resp)["deleted"])
	w, _ = f.do(http.MethodDelete, nodesURL+"/"+itoa(rootID), authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 官方路线只有管理员可以写节点。
	w, resp = f.do(http.MethodPost, "/api/learning-paths", adminToken,
		map[string]any{"title": "API 官方路线", "isOfficial": true})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	officialURL := "/api/learning-paths/" + itoa(int64(responseData(t, resp)["id"].(float64)))
	w, resp = f.do(http.MethodPost, officialURL+"/nodes", authorToken,
		map[string]any{"title": "作者写官方", "problemIds": []int64{pub.ID}})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)
	w, _ = f.do(http.MethodPost, officialURL+"/nodes", adminToken,
		map[string]any{"title": "官方节点", "problemIds": []int64{pub.ID}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// inactive 路线只有管理员可以写节点。
	w, resp = f.do(http.MethodPost, "/api/learning-paths", authorToken,
		map[string]any{"title": "API 下架路线"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	inactiveURL := "/api/learning-paths/" + itoa(int64(responseData(t, resp)["id"].(float64)))
	w, _ = f.do(http.MethodDelete, inactiveURL, authorToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp = f.do(http.MethodPost, inactiveURL+"/nodes", authorToken,
		map[string]any{"title": "作者写下架", "problemIds": []int64{pub.ID}})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)
	w, _ = f.do(http.MethodPost, inactiveURL+"/nodes", adminToken,
		map[string]any{"title": "管理员写下架", "problemIds": []int64{pub.ID}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w, resp = f.do(http.MethodGet, "/api/learning-paths/999999/tree", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)

	w, resp = f.do(http.MethodPut, nodesURL+"/not-a-number", authorToken,
		map[string]any{"title": "非法 id", "problemIds": []int64{pub.ID}})
	assert.Equal(t, http.StatusNotFound, w.Code)
	requireCode(t, resp, 40400)
}
