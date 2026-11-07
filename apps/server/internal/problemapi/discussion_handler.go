package problemapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// isModerator 报告当前用户是否拥有 problem.update（超级管理员 Has 恒为 true）。
func isModerator(c *gin.Context) bool {
	access, ok := middleware.CurrentAccess(c)
	return ok && access != nil && access.Has(string(rbac.PermProblemUpdate))
}

func (h *handler) listPosts(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.discussions.ListPosts(c.Request.Context(), currentUserID(c), id, viewer(c), currentIsSuper(c), isModerator(c), page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) createPost(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[problem.SolutionPostInput](c)
	if !ok {
		return
	}
	detail, err := h.discussions.CreatePost(c.Request.Context(), currentUserID(c), id, viewer(c), currentIsSuper(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, detail)
}

func (h *handler) getPost(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	postID, ok := pathID(c, "postId")
	if !ok {
		return
	}
	detail, err := h.discussions.GetPost(c.Request.Context(), currentUserID(c), id, postID, viewer(c), currentIsSuper(c), isModerator(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

func (h *handler) updatePost(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	postID, ok := pathID(c, "postId")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[problem.SolutionPostInput](c)
	if !ok {
		return
	}
	detail, err := h.discussions.UpdatePost(c.Request.Context(), currentUserID(c), id, postID, viewer(c), currentIsSuper(c), isModerator(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

func (h *handler) deletePost(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	postID, ok := pathID(c, "postId")
	if !ok {
		return
	}
	if err := h.discussions.DeletePost(c.Request.Context(), currentUserID(c), id, postID, viewer(c), currentIsSuper(c), isModerator(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) createReply(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	postID, ok := pathID(c, "postId")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[problem.SolutionReplyInput](c)
	if !ok {
		return
	}
	reply, err := h.discussions.CreateReply(c.Request.Context(), currentUserID(c), id, postID, viewer(c), currentIsSuper(c), isModerator(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, reply)
}

func (h *handler) updateReply(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	postID, ok := pathID(c, "postId")
	if !ok {
		return
	}
	replyID, ok := pathID(c, "replyId")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[problem.SolutionReplyInput](c)
	if !ok {
		return
	}
	reply, err := h.discussions.UpdateReply(c.Request.Context(), currentUserID(c), id, postID, replyID, viewer(c), currentIsSuper(c), isModerator(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, reply)
}

func (h *handler) deleteReply(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	postID, ok := pathID(c, "postId")
	if !ok {
		return
	}
	replyID, ok := pathID(c, "replyId")
	if !ok {
		return
	}
	if err := h.discussions.DeleteReply(c.Request.Context(), currentUserID(c), id, postID, replyID, viewer(c), currentIsSuper(c), isModerator(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
