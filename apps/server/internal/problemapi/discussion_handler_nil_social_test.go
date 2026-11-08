package problemapi_test

import (
	"context"
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
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// newNoSocialEngine 注册 problemapi 路由但省略 Options.Social，用于验证点赞/
// 取消赞处理器在 social 缺失时返回 500 而非 panic。
func newNoSocialEngine(t *testing.T) (http.Handler, string, int64, int64) {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := problem.NewService(problem.NewGormRepository(g), store)
	repo := problem.NewGormRepository(g)

	// 普通用户与访问 token。
	require.NoError(t, g.Exec(`INSERT INTO users (username, nickname, email, password_hash, created_at, updated_at)
		VALUES ('liker-nosocial', 'Liker', 'liker-nosocial@example.com', 'x', now(), now())`).Error)
	var uid int64
	require.NoError(t, g.Raw(`SELECT id FROM users WHERE username = 'liker-nosocial'`).Scan(&uid).Error)
	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	token, err := issuer.IssueAccess(&user.User{ID: uid, Role: user.RoleUser})
	require.NoError(t, err)

	// 公开题目 + 帖子，使点赞处理器越过可见性校验，真正走到 social 调用。
	p := &problem.Problem{
		Title: "No social", Slug: "no-social", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: uid,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	discussions := problem.NewDiscussionService(g, repo)
	post, err := discussions.CreatePost(context.Background(), uid, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "T", ContentMD: "body"})
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(1<<20))
	problemJSON := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(4<<20))
	uploads := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(40<<20))
	imports := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(1<<20))
	// 注意：这里不传 Options.Social。
	problemapi.RegisterRoutes(api, problemJSON, uploads, imports, svc, nil, nil,
		problemapi.Options{RBAC: rbacSvc, Discussions: discussions})
	return r, token, p.ID, post.ID
}

func TestLikeWithoutSocialReturns500(t *testing.T) {
	h, token, pid, postID := newNoSocialEngine(t)

	for _, c := range []struct{ method, p string }{
		{http.MethodPost, path("/api/problems/%d/solutions/%d/like", pid, postID)},
		{http.MethodDelete, path("/api/problems/%d/solutions/%d/like", pid, postID)},
	} {
		req := httptest.NewRequest(c.method, c.p, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		require.NotPanics(t, func() { h.ServeHTTP(w, req) }, c.method+" "+c.p)
		assert.Equal(t, http.StatusInternalServerError, w.Code, c.method+" "+c.p)
		assert.EqualValues(t, 50000, decode(w)["code"], c.method+" "+c.p)
	}
}
