package contestapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// GameHook 在比赛报名成功后触发；失败只记录日志，不影响报名响应。
type GameHook interface {
	OnContestJoin(ctx context.Context, userID, contestID int64) error
}

// IdentityLookup 批量读取用户公开身份；由 gamification.Service 实现。
type IdentityLookup interface {
	Identities(ctx context.Context, userIDs []int64) (map[int64]gamification.Identity, error)
}

// Options 配置比赛接口的可选依赖。
type Options struct {
	GameHook       GameHook
	IdentityLookup IdentityLookup
}

func RegisterRoutes(r gin.IRouter, s *contest.Service, opts ...Options) {
	h := &handler{svc: s}
	if len(opts) > 0 {
		h.gameHook = opts[0].GameHook
		h.identities = opts[0].IdentityLookup
	}
	manage := []gin.HandlerFunc{middleware.RequireAuth(), middleware.RequireRole(user.RoleContestAdmin)}
	with := func(fn gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, manage...), fn)
	}

	r.GET("/contests", h.list)
	r.GET("/contests/:id", h.detail)
	r.POST("/contests", with(h.create)...)
	r.PUT("/contests/:id", with(h.update)...)
	r.DELETE("/contests/:id", with(h.delete)...)
	r.POST("/contests/:id/clone", with(h.clone)...)
	r.POST("/contests/:id/register", middleware.RequireAuth(), h.register)
	r.GET("/contests/:id/participants", with(h.participants)...)
	r.GET("/contests/:id/ranklist", h.ranklist)
	r.GET("/contests/:id/announcements", h.announcements)
	r.POST("/contests/:id/announcements", with(h.createAnnouncement)...)
	r.PUT("/contests/:id/announcements/:aid", with(h.updateAnnouncement)...)
	r.DELETE("/contests/:id/announcements/:aid", with(h.deleteAnnouncement)...)
}

type handler struct {
	svc        *contest.Service
	gameHook   GameHook
	identities IdentityLookup
}

func viewer(c *gin.Context) contest.Viewer {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return contest.Viewer{}
	}
	return contest.Viewer{
		UserID:    claims.UserID,
		CanManage: claims.Role == user.RoleContestAdmin || claims.Role == user.RoleSuperAdmin,
	}
}

// bindContestInput distinguishes omitted required fields from empty values:
// a missing "problems" must not silently erase the whole contest problem list.
func bindContestInput(c *gin.Context) (contest.Input, bool) {
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return contest.Input{}, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return contest.Input{}, false
	}
	for _, key := range []string{
		"title", "descriptionMd", "startTime", "endTime", "mode",
		"isPublic", "registerType", "problems",
	} {
		if _, ok := raw[key]; !ok {
			httpx.Fail(c, httpx.ErrValidation("缺少字段 "+key))
			return contest.Input{}, false
		}
	}
	var in contest.Input
	if err := json.Unmarshal(data, &in); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return contest.Input{}, false
	}
	return in, true
}

func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) list(c *gin.Context) {
	q := contest.ListQuery{
		Keyword:       c.Query("keyword"),
		Mode:          contest.Mode(c.Query("mode")),
		Status:        contest.Status(c.Query("status")),
		IncludeHidden: c.Query("includeHidden") == "true",
		Mine:          c.Query("mine") == "true",
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.List(c.Request.Context(), viewer(c), q, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) detail(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
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
	in, ok := bindContestInput(c)
	if !ok {
		return
	}
	created, err := h.svc.Create(c.Request.Context(), viewer(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	detail, err := h.svc.Detail(c.Request.Context(), viewer(c), created.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, detail)
}

func (h *handler) update(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := bindContestInput(c)
	if !ok {
		return
	}
	updated, err := h.svc.Update(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	detail, err := h.svc.Detail(c.Request.Context(), viewer(c), updated.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

func (h *handler) delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), viewer(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) clone(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[contest.Input](c)
	if !ok {
		return
	}
	created, err := h.svc.Clone(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	detail, err := h.svc.Detail(c.Request.Context(), viewer(c), created.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, detail)
}

func (h *handler) register(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	in, ok := httpx.BindJSON[struct {
		Password   string `json:"password"`
		InviteCode string `json:"inviteCode"`
	}](c)
	if !ok {
		return
	}
	if err := h.svc.Register(c.Request.Context(), claims.UserID, id, in.Password, in.InviteCode); err != nil {
		httpx.Fail(c, err)
		return
	}
	if h.gameHook != nil {
		if err := h.gameHook.OnContestJoin(c.Request.Context(), claims.UserID, id); err != nil {
			slog.Warn("gamification contest join hook failed",
				"user_id", claims.UserID, "contest_id", id, "err", err)
		}
	}
	httpx.OK(c, gin.H{"registered": true})
}

func (h *handler) participants(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.Participants(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) ranklist(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	list, err := h.svc.Ranklist(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if h.identities != nil && len(list.Rows) > 0 {
		// OI selfOnly 已在 service 内过滤，这里只装饰保留下来的行。
		ids := make([]int64, 0, len(list.Rows))
		seen := make(map[int64]struct{}, len(list.Rows))
		for _, row := range list.Rows {
			if row.UserID <= 0 {
				continue
			}
			if _, ok := seen[row.UserID]; ok {
				continue
			}
			seen[row.UserID] = struct{}{}
			ids = append(ids, row.UserID)
		}
		identities, err := h.identities.Identities(c.Request.Context(), ids)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		for i := range list.Rows {
			identity, ok := identities[list.Rows[i].UserID]
			if !ok {
				continue
			}
			list.Rows[i].EquippedBadges = identity.Badges
			list.Rows[i].EquippedTitle = identity.Title
		}
	}
	httpx.OK(c, list)
}

func (h *handler) announcements(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	items, err := h.svc.Announcements(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func announcementInput(c *gin.Context) (title, content string, ok bool) {
	in, ok := httpx.BindJSON[struct {
		Title     string `json:"title"`
		ContentMD string `json:"contentMd"`
	}](c)
	if !ok {
		return "", "", false
	}
	return in.Title, in.ContentMD, true
}

func (h *handler) createAnnouncement(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	title, content, ok := announcementInput(c)
	if !ok {
		return
	}
	item, err := h.svc.CreateAnnouncement(c.Request.Context(), viewer(c), id, title, content)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, item)
}

func (h *handler) updateAnnouncement(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	aid, ok := pathID(c, "aid")
	if !ok {
		return
	}
	title, content, ok := announcementInput(c)
	if !ok {
		return
	}
	if err := h.svc.UpdateAnnouncement(c.Request.Context(), viewer(c), id, aid, title, content); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) deleteAnnouncement(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	aid, ok := pathID(c, "aid")
	if !ok {
		return
	}
	if err := h.svc.DeleteAnnouncement(c.Request.Context(), viewer(c), id, aid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
