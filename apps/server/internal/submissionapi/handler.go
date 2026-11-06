package submissionapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
)

// IdentityLookup 批量读取用户公开身份；由 gamification.Service 实现。
type IdentityLookup interface {
	Identities(ctx context.Context, userIDs []int64) (map[int64]gamification.Identity, error)
}

// ActivityHook 在提交创建成功后记录活跃度；失败只记录日志，不影响提交响应。
type ActivityHook interface {
	RecordSubmission(ctx context.Context, userID, submissionID int64) error
}

// Options 配置测评记录接口的可选依赖。
type Options struct {
	IdentityLookup IdentityLookup
	ActivityHook   ActivityHook
	RBAC           *rbac.Service
}

// RegisterRoutes mounts the submission endpoints. r must already run the
// authentication middleware.
func RegisterRoutes(r gin.IRouter, s *submission.Service, streamer submissionevents.Streamer, opts ...Options) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{
		svc: s, streamer: streamer,
		identities: o.IdentityLookup, activityHook: o.ActivityHook, rbac: o.RBAC,
	}

	// resolver 为 nil 时 LoadAccess 为空操作；生产环境 router 始终传入 RBAC
	// service，权限能力按数据库实时解析。
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)

	r.GET("/submissions", load, h.list)
	r.GET("/submissions/stream", h.stream)
	r.GET("/submissions/:id", load, h.detail)
	r.POST("/submissions", middleware.RequireAuth(), load, h.create)
}

type handler struct {
	svc          *submission.Service
	streamer     submissionevents.Streamer
	identities   IdentityLookup
	activityHook ActivityHook
	rbac         *rbac.Service
}

// decorateItems 在 service 返回后批量填充当前页的公开身份，避免逐行查询。
// 它只增加展示字段，不影响赛时保护等原有隐藏逻辑。
func (h *handler) decorateItems(ctx context.Context, items []submission.ListItem) error {
	if h.identities == nil || len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item.UserID <= 0 {
			continue
		}
		if _, ok := seen[item.UserID]; ok {
			continue
		}
		seen[item.UserID] = struct{}{}
		ids = append(ids, item.UserID)
	}
	identities, err := h.identities.Identities(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		identity, ok := identities[items[i].UserID]
		if !ok {
			continue
		}
		items[i].EquippedBadges = identity.Badges
		items[i].EquippedTitle = identity.Title
	}
	return nil
}

func viewer(c *gin.Context) submission.Viewer {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return submission.Viewer{}
	}
	v := submission.Viewer{UserID: claims.UserID}
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		v.CanViewHiddenProblems = access.Has(string(rbac.PermProblemViewHidden))
		v.CanViewLiveContest = access.Has(string(rbac.PermSubmissionViewLiveContest))
		v.CanManageContest = access.Has(string(rbac.PermContestUpdate))
	}
	return v
}

func optionalInt64(c *gin.Context, name string) (int64, bool, error) {
	raw := c.Query(name)
	if raw == "" {
		return 0, false, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false, httpx.ErrValidation(name + " 必须是整数")
	}
	return n, true, nil
}

func (h *handler) list(c *gin.Context) {
	q := submission.ListQuery{Nickname: c.Query("nickname")}
	if v, ok, err := optionalInt64(c, "problemId"); err != nil {
		httpx.Fail(c, err)
		return
	} else if ok {
		q.ProblemID = v
	}
	if v, ok, err := optionalInt64(c, "contestId"); err != nil {
		httpx.Fail(c, err)
		return
	} else if ok {
		q.ContestID = &v
	}
	if v, ok, err := optionalInt64(c, "userId"); err != nil {
		httpx.Fail(c, err)
		return
	} else if ok {
		q.UserID = v
	}
	q.Language = submission.Language(c.Query("language"))
	q.Status = submission.Status(c.Query("status"))

	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.List(c.Request.Context(), viewer(c), q, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.decorateItems(c.Request.Context(), result.Items); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) stream(c *gin.Context) {
	events, err := h.streamer.Subscribe(c.Request.Context())
	if err != nil {
		httpx.Fail(c, httpx.ErrUnavailable)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
	if _, err := fmt.Fprint(c.Writer, ": connected\n\n"); err != nil {
		return
	}
	c.Writer.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
				return
			}
			c.Writer.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(c.Writer, ": keep-alive\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

func (h *handler) detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	detail, err := h.svc.Detail(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items := []submission.ListItem{detail.ListItem}
	if err := h.decorateItems(c.Request.Context(), items); err != nil {
		httpx.Fail(c, err)
		return
	}
	detail.ListItem = items[0]
	httpx.OK(c, detail)
}

func (h *handler) create(c *gin.Context) {
	in, ok := httpx.BindJSON[submission.Input](c)
	if !ok {
		return
	}
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	sub, err := h.svc.Create(c.Request.Context(), viewer(c), claims.UserID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if h.activityHook != nil {
		if err := h.activityHook.RecordSubmission(c.Request.Context(), claims.UserID, sub.ID); err != nil {
			slog.Warn("activity submission hook failed",
				"user_id", claims.UserID, "submission_id", sub.ID, "err", err)
		}
	}
	httpx.Created(c, submission.ToCreated(sub))
}
