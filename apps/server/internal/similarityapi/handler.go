package similarityapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/similarity"
)

// auditTargetContest is the content-management audit target type.
const auditTargetContest = "contest"

// Options configures the similarity API's optional dependencies.
type Options struct {
	RBAC *rbac.Service
}

// RegisterRoutes mounts the contest similarity endpoints. The service is
// required; callers must not pass nil.
func RegisterRoutes(r gin.IRouter, s *similarity.Service, opts ...Options) {
	if s == nil {
		return
	}
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{svc: s, rbac: o.RBAC}

	// A nil resolver makes LoadAccess a no-op and RequirePermission reject every
	// request; production always passes the RBAC service.
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)
	require := middleware.RequirePermission(resolver, string(rbac.PermContestUpdate))
	audit := middleware.AuditAdminAction(h.rbac, string(rbac.PermContestUpdate), auditTargetContest)

	r.GET("/contests/:id/similarity/overview", load, require, h.overview)
	r.PUT("/contests/:id/similarity/problems/:problemId", load, require, audit, h.saveSettings)
	r.POST("/contests/:id/similarity/runs", load, require, audit, h.startRuns)
	r.GET("/contests/:id/similarity/runs/:runId", load, require, h.run)
	r.GET("/contests/:id/similarity/matches", load, require, h.matches)
	r.GET("/contests/:id/similarity/matches/:matchId", load, require, h.matchDetail)
}

type handler struct {
	svc  *similarity.Service
	rbac *rbac.Service
}

// viewer builds the acting user's similarity identity from loaded RBAC access.
func viewer(c *gin.Context) similarity.Viewer {
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return similarity.Viewer{}
	}
	return similarity.Viewer{
		UserID:    access.UserID,
		CanManage: access.Has(string(rbac.PermContestUpdate)),
	}
}

// pathID parses a positive path parameter, writing 404 when it is invalid.
func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

// queryID parses an optional positive query parameter, writing 400 when it is
// present but invalid.
func queryID(c *gin.Context, name string) (int64, bool) {
	raw := c.Query(name)
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrValidation(name+" 必须是正整数"))
		return 0, false
	}
	return id, true
}

func (h *handler) overview(c *gin.Context) {
	contestID, ok := pathID(c, "id")
	if !ok {
		return
	}
	result, err := h.svc.Overview(c.Request.Context(), viewer(c), contestID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// settingsView is the JSON shape returned after saving settings. similarity.Settings
// is a persistence model without JSON tags, so it is not exposed directly.
type settingsView struct {
	ContestID        int64  `json:"contestId"`
	ProblemID        int64  `json:"problemId"`
	ThresholdPercent int    `json:"thresholdPercent"`
	IgnoreCommonCode bool   `json:"ignoreCommonCode"`
	BaseLanguage     string `json:"baseLanguage"`
	BaseCode         string `json:"baseCode"`
}

func (h *handler) saveSettings(c *gin.Context) {
	contestID, ok := pathID(c, "id")
	if !ok {
		return
	}
	problemID, ok := pathID(c, "problemId")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[similarity.SettingsInput](c)
	if !ok {
		return
	}
	settings, err := h.svc.SaveSettings(c.Request.Context(), viewer(c), contestID, problemID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, settingsView{
		ContestID:        settings.ContestID,
		ProblemID:        settings.ProblemID,
		ThresholdPercent: settings.ThresholdPercent,
		IgnoreCommonCode: settings.IgnoreCommonCode,
		BaseLanguage:     settings.BaseLanguage,
		BaseCode:         settings.BaseCode,
	})
}

// runRequest optionally narrows a run to one problem; zero means every problem.
type runRequest struct {
	ProblemID int64 `json:"problemId"`
}

func (h *handler) startRuns(c *gin.Context) {
	contestID, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in runRequest
	if c.Request.ContentLength != 0 {
		in, ok = httpx.BindJSON[runRequest](c)
		if !ok {
			return
		}
	}
	runs, err := h.svc.StartRuns(c.Request.Context(), viewer(c), contestID, in.ProblemID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Accepted(c, runs)
}

func (h *handler) run(c *gin.Context) {
	contestID, ok := pathID(c, "id")
	if !ok {
		return
	}
	runID, ok := pathID(c, "runId")
	if !ok {
		return
	}
	result, err := h.svc.Run(c.Request.Context(), viewer(c), contestID, runID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) matches(c *gin.Context) {
	contestID, ok := pathID(c, "id")
	if !ok {
		return
	}
	problemID, ok := queryID(c, "problemId")
	if !ok {
		return
	}
	runID, ok := queryID(c, "runId")
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.Matches(c.Request.Context(), viewer(c), contestID, problemID, runID, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.Page[similarity.MatchView]{Total: result.Total, Items: result.Items})
}

func (h *handler) matchDetail(c *gin.Context) {
	contestID, ok := pathID(c, "id")
	if !ok {
		return
	}
	matchID, ok := pathID(c, "matchId")
	if !ok {
		return
	}
	result, err := h.svc.MatchDetail(c.Request.Context(), viewer(c), contestID, matchID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
