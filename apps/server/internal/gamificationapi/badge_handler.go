package gamificationapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
)

func (h *handler) listBadges(c *gin.Context) {
	items, err := h.svc.ListBadges(
		c.Request.Context(), viewer(c), c.Query("includeInactive") == "true",
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) createBadge(c *gin.Context) {
	in, ok := httpx.BindJSON[gamification.BadgeInput](c)
	if !ok {
		return
	}
	badge, err := h.svc.CreateBadge(c.Request.Context(), viewer(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, gamification.BadgeItemFromModel(*badge))
}

func (h *handler) updateBadge(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[gamification.BadgeInput](c)
	if !ok {
		return
	}
	badge, err := h.svc.UpdateBadge(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gamification.BadgeItemFromModel(*badge))
}

func (h *handler) deleteBadge(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteBadge(c.Request.Context(), viewer(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) uploadBadgeImage(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 file"))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	if err := h.svc.UploadBadgeImage(c.Request.Context(), viewer(c), id, file); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) badgeImage(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	image, err := h.svc.BadgeImage(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, image.ContentType, image.Data)
}

func (h *handler) purchaseBadge(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	if err := h.svc.PurchaseBadge(c.Request.Context(), claims.UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) equipBadge(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	if err := h.svc.EquipBadge(c.Request.Context(), claims.UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) unequipBadge(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	if err := h.svc.UnequipBadge(c.Request.Context(), claims.UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) userBadges(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	items, err := h.svc.ListUserBadges(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}
