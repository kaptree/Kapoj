package submissionapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// RegisterRoutes mounts the submission endpoints. r must already run the
// authentication middleware.
func RegisterRoutes(r gin.IRouter, s *submission.Service) {
	h := &handler{svc: s}
	r.POST("/submissions", middleware.RequireAuth(), h.create)
}

type handler struct{ svc *submission.Service }

func viewer(c *gin.Context) submission.Viewer {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return submission.Viewer{}
	}
	return submission.Viewer{
		CanManage: claims.Role == user.RoleProblemAdmin || claims.Role == user.RoleSuperAdmin,
	}
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
