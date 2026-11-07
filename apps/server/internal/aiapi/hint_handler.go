package aiapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// hint handles POST /api/ai/hint. Auth is enforced by the route; canViewHidden
// is derived from the Access loaded by middleware.LoadAccess (super always
// passes, otherwise the problem.view_hidden permission is required).
func (h *handler) hint(c *gin.Context) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	in, ok := httpx.BindJSON[ai.HintInput](c)
	if !ok {
		return
	}
	canViewHidden := false
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		canViewHidden = access.IsSuper || access.Has(string(rbac.PermProblemViewHidden))
	}
	view, err := h.svc.Hint(c.Request.Context(), claims.UserID, in.ProblemID, canViewHidden)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
