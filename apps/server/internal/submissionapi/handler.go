package submissionapi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// RegisterRoutes mounts the submission endpoints. r must already run the
// authentication middleware.
func RegisterRoutes(r gin.IRouter, s *submission.Service, streamer submissionevents.Streamer) {
	h := &handler{svc: s, streamer: streamer}
	r.GET("/submissions", h.list)
	r.GET("/submissions/stream", h.stream)
	r.GET("/submissions/:id", h.detail)
	r.POST("/submissions", middleware.RequireAuth(), h.create)
}

type handler struct {
	svc      *submission.Service
	streamer submissionevents.Streamer
}

func viewer(c *gin.Context) submission.Viewer {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return submission.Viewer{}
	}
	return submission.Viewer{
		UserID: claims.UserID,
		CanManage: claims.Role == user.RoleProblemAdmin ||
			claims.Role == user.RoleContestAdmin ||
			claims.Role == user.RoleSuperAdmin,
		CanManageProblems: claims.Role == user.RoleProblemAdmin ||
			claims.Role == user.RoleSuperAdmin,
	}
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
	httpx.Created(c, submission.ToCreated(sub))
}
