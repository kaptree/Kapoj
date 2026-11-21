package activityapi

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/ratinglevel"
)

// IdentityLookup batch-loads public equipped identities; gamification.Service
// implements it.
type IdentityLookup interface {
	Identities(ctx context.Context, userIDs []int64) (map[int64]gamification.Identity, error)
}

// Options configures optional activity API dependencies.
type Options struct {
	IdentityLookup IdentityLookup
	Companion      *companion.Service
}

// RegisterRoutes mounts public activity/sharing leaderboards plus the
// authenticated sign-in and summary endpoints. The router group must already
// run middleware.Authenticate so leaderboard guests are treated as anonymous.
func RegisterRoutes(r gin.IRouter, s *activity.Service, opts ...Options) {
	h := &handler{svc: s}
	if len(opts) > 0 {
		h.identities = opts[0].IdentityLookup
		h.comp = opts[0].Companion
	}
	r.GET("/leaderboard/activity", h.activityLeaderboard)
	r.GET("/leaderboard/sharing", h.sharingLeaderboard)
	r.GET("/leaderboard/wealth", h.wealthLeaderboard)
	r.GET("/activity/summary", middleware.RequireAuth(), h.summary)
	r.POST("/activity/signin", middleware.RequireAuth(), h.signIn)
}

type handler struct {
	svc        *activity.Service
	identities IdentityLookup
	comp       *companion.Service
}

// LeaderboardRow is the public API row: cached base fields plus equipped
// identity resolved after the cache read.
type LeaderboardRow struct {
	activity.LeaderboardRow
	EquippedBadges []gamification.BadgeSummary `json:"equippedBadges"`
	EquippedTitle  *gamification.TitleSummary  `json:"equippedTitle"`
	Level          ratinglevel.Level           `json:"level"`
}

func (h *handler) activityLeaderboard(c *gin.Context) {
	h.leaderboard(c, h.svc.ActivityLeaderboard)
}

func (h *handler) sharingLeaderboard(c *gin.Context) {
	h.leaderboard(c, h.svc.SharingLeaderboard)
}

func (h *handler) wealthLeaderboard(c *gin.Context) {
	h.leaderboard(c, h.svc.WealthLeaderboard)
}

func (h *handler) leaderboard(
	c *gin.Context,
	load func(context.Context, int, int) (httpx.Page[activity.LeaderboardRow], error),
) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	base, err := load(c.Request.Context(), page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	rows := make([]LeaderboardRow, len(base.Items))
	userIDs := make([]int64, 0, len(base.Items))
	for i, item := range base.Items {
		rows[i] = LeaderboardRow{
			LeaderboardRow: item,
			EquippedBadges: []gamification.BadgeSummary{},
			Level:          ratinglevel.LevelFor(0),
		}
		userIDs = append(userIDs, item.UserID)
	}

	if h.identities != nil && len(userIDs) > 0 {
		identities, err := h.identities.Identities(c.Request.Context(), userIDs)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		for i := range rows {
			identity, ok := identities[rows[i].UserID]
			if !ok {
				continue
			}
			if identity.Badges != nil {
				rows[i].EquippedBadges = identity.Badges
			}
			rows[i].EquippedTitle = identity.Title
			rows[i].Level = identity.Level
		}
	}

	httpx.OK(c, httpx.Page[LeaderboardRow]{Total: base.Total, Items: rows})
}

func (h *handler) signIn(c *gin.Context) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	result, err := h.svc.SignIn(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if result.Awarded && h.comp != nil {
		if err := h.comp.RecordSignIn(c.Request.Context(), claims.UserID, result.Date); err != nil {
			slog.Warn("companion sign-in hook failed",
				"user_id", claims.UserID, "date", result.Date, "err", err)
		}
	}
	httpx.OK(c, result)
}

func (h *handler) summary(c *gin.Context) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	result, err := h.svc.Summary(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
