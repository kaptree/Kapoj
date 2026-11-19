package companionapi

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
)

const (
	// eventsDefaultLimit is the page size returned when no valid limit is given.
	eventsDefaultLimit = 20
	// eventsMaxLimit is the hard cap accepted by the events endpoint.
	eventsMaxLimit = 20
	// markReadMaxIDs bounds one mark-read batch so a single request cannot build
	// an arbitrarily large IN clause.
	markReadMaxIDs = 100
)

// RegisterRoutes mounts the companion endpoints. The group must already run
// middleware.Authenticate so a valid bearer token populates the claims that
// middleware.RequireAuth checks. The public summary is deliberately mounted
// without RequireAuth: guests read it with viewerID 0.
func RegisterRoutes(r gin.IRouter, s *companion.Service) {
	h := &handler{svc: s}
	auth := middleware.RequireAuth()
	r.GET("/companion", auth, h.state)
	r.GET("/companion/events", auth, h.events)
	r.POST("/companion/events/read", auth, h.markRead)
	r.POST("/companion/settings", auth, h.updateSettings)
	r.POST("/companion/pet", auth, h.pet)
	r.POST("/companion/dialogue", auth, h.dialogue)
	r.GET("/users/:id/companion", h.publicState)
}

type handler struct {
	svc *companion.Service
}

// currentUserID returns the authenticated user id or writes 401 and returns
// false when the route was mounted without authentication.
func currentUserID(c *gin.Context) (int64, bool) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return 0, false
	}
	return claims.UserID, true
}

func (h *handler) state(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	view, err := h.svc.State(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// eventsResponse wraps the unread event list so the payload stays an object and
// the empty case serializes as [] rather than null.
type eventsResponse struct {
	Items []companion.EventView `json:"items"`
}

func (h *handler) events(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	limit := eventsLimit(c.Query("limit"))
	events, err := h.svc.UnreadEvents(c.Request.Context(), userID, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if events == nil {
		events = []companion.EventView{}
	}
	httpx.OK(c, eventsResponse{Items: events})
}

// eventsLimit clamps the limit query to 1..20; a non-numeric value falls back to
// the default page size.
func eventsLimit(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return eventsDefaultLimit
	}
	if n < 1 {
		return 1
	}
	if n > eventsMaxLimit {
		return eventsMaxLimit
	}
	return n
}

type markReadRequest struct {
	IDs []int64 `json:"ids"`
}

func (h *handler) markRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	req, ok := httpx.BindJSON[markReadRequest](c)
	if !ok {
		return
	}
	if len(req.IDs) > markReadMaxIDs {
		httpx.Fail(c, httpx.ErrValidation("一次最多标记 100 条事件"))
		return
	}
	if err := h.svc.MarkEventsRead(c.Request.Context(), userID, req.IDs); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// updateSettings applies a partial update. The service validates the activity
// enum and skin length; the repository writes only the provided fields.
func (h *handler) updateSettings(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	patch, ok := httpx.BindJSON[companion.SettingsPatch](c)
	if !ok {
		return
	}
	view, err := h.svc.UpdateSettingsPatch(c.Request.Context(), userID, patch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) pet(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	// Daily idempotency is keyed by the local date, matching the service's state
	// and event-date handling.
	date := time.Now().In(time.Local).Format("2006-01-02")
	result, err := h.svc.Pet(c.Request.Context(), userID, date)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func validDialogueTrigger(trigger string) bool {
	switch trigger {
	case "pet", "greet", "idle":
		return true
	default:
		return false
	}
}

func (h *handler) dialogue(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	req, ok := httpx.BindJSON[companion.DialogueRequest](c)
	if !ok {
		return
	}
	if !validDialogueTrigger(req.Trigger) {
		httpx.Fail(c, httpx.ErrValidation("互动类型无效"))
		return
	}
	if req.Expression != "" && !companion.ValidDialogueExpression(req.Expression) {
		httpx.Fail(c, httpx.ErrValidation("表情类型无效"))
		return
	}
	if req.GreetingKey != "" && !companion.ValidDialogueGreetingKey(req.GreetingKey) {
		httpx.Fail(c, httpx.ErrValidation("问候时段无效"))
		return
	}
	result, err := h.svc.Dialogue(c.Request.Context(), userID, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// pathID reads a positive path parameter; malformed ids are 404 so the summary
// never reveals whether a companion exists behind an invalid key.
func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

// publicState serves the privacy-filtered companion summary. A missing bearer
// token is not an error: the viewer is a guest (id 0) and only public profiles
// are visible.
func (h *handler) publicState(c *gin.Context) {
	userID, ok := pathID(c, "id")
	if !ok {
		return
	}
	var viewerID int64
	if claims, ok := middleware.CurrentClaims(c); ok {
		viewerID = claims.UserID
	}
	view, err := h.svc.PublicState(c.Request.Context(), userID, viewerID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
