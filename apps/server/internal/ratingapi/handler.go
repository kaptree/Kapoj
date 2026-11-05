package ratingapi

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
)

// IdentityLookup batch-loads public equipped identities; gamification.Service
// implements it.
type IdentityLookup interface {
	Identities(ctx context.Context, userIDs []int64) (map[int64]gamification.Identity, error)
}

// Options configures optional rating API dependencies.
type Options struct {
	IdentityLookup IdentityLookup
}

// RegisterRoutes mounts public rating leaderboard and history endpoints. The
// router group must already run middleware.Authenticate so guests are treated
// as anonymous.
func RegisterRoutes(r gin.IRouter, s *rating.Service, opts ...Options) {
	h := &handler{svc: s}
	if len(opts) > 0 {
		h.identities = opts[0].IdentityLookup
	}
	r.GET("/leaderboard/rating", h.leaderboard)
	r.GET("/users/:id/rating-history", h.ratingHistory)
}

type handler struct {
	svc        *rating.Service
	identities IdentityLookup
}

// LeaderboardRow is the public API row: cached base fields plus equipped
// identity resolved after the cache read.
type LeaderboardRow struct {
	rating.LeaderboardRow
	EquippedBadges []gamification.BadgeSummary `json:"equippedBadges"`
	EquippedTitle  *gamification.TitleSummary  `json:"equippedTitle"`
}

func (h *handler) leaderboard(c *gin.Context) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	base, err := h.svc.Leaderboard(c.Request.Context(), page, size)
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
		}
	}

	httpx.OK(c, httpx.Page[LeaderboardRow]{Total: base.Total, Items: rows})
}

func (h *handler) ratingHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.RatingHistory(c.Request.Context(), id, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
