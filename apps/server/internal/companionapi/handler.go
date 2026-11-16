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

// activityLevels are the only values the settings endpoint accepts; they mirror
// the quiet | normal | lively choice in the companion design document.
var activityLevels = map[string]struct{}{
	"quiet":  {},
	"normal": {},
	"lively": {},
}

// RegisterRoutes mounts the authenticated companion endpoints. The group must
// already run middleware.Authenticate so a valid bearer token populates the
// claims that middleware.RequireAuth checks.
func RegisterRoutes(r gin.IRouter, s *companion.Service) {
	h := &handler{svc: s}
	auth := middleware.RequireAuth()
	r.GET("/companion", auth, h.state)
	r.GET("/companion/events", auth, h.events)
	r.POST("/companion/events/read", auth, h.markRead)
	r.POST("/companion/settings", auth, h.updateSettings)
	r.POST("/companion/pet", auth, h.pet)
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

// settingsRequest uses pointers so an omitted field keeps its stored value
// instead of resetting to the zero value.
type settingsRequest struct {
	ActivityLevel *string `json:"activityLevel"`
	DockEnabled   *bool   `json:"dockEnabled"`
	PublicProfile *bool   `json:"publicProfile"`
	EquippedSkin  *string `json:"equippedSkin"`
}

func (h *handler) updateSettings(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	req, ok := httpx.BindJSON[settingsRequest](c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	current, err := h.svc.State(ctx, userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	in := companion.SettingsInput{
		ActivityLevel: current.ActivityLevel,
		DockEnabled:   current.DockEnabled,
		PublicProfile: current.PublicProfile,
		EquippedSkin:  current.EquippedSkin,
	}
	if req.ActivityLevel != nil {
		if _, valid := activityLevels[*req.ActivityLevel]; !valid {
			httpx.Fail(c, httpx.ErrValidation("活动频率无效"))
			return
		}
		in.ActivityLevel = *req.ActivityLevel
	}
	if req.DockEnabled != nil {
		in.DockEnabled = *req.DockEnabled
	}
	if req.PublicProfile != nil {
		in.PublicProfile = *req.PublicProfile
	}
	if req.EquippedSkin != nil {
		in.EquippedSkin = *req.EquippedSkin
	}
	view, err := h.svc.UpdateSettings(ctx, userID, in)
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
