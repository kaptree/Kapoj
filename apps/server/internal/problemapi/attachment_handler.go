package problemapi

import (
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func (h *handler) listAttachments(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	list, err := h.svc.ListAttachments(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]problem.AttachmentDTO, len(list))
	for i := range list {
		out[i] = problem.ToAttachmentDTO(&list[i])
	}
	httpx.OK(c, out)
}

func (h *handler) downloadAttachment(c *gin.Context) {
	h.serveAttachment(c, false)
}

func (h *handler) statementImage(c *gin.Context) {
	h.serveAttachment(c, true)
}

func (h *handler) serveAttachment(c *gin.Context, inline bool) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	aid, ok := pathID(c, "aid")
	if !ok {
		return
	}
	content, err := h.svc.OpenAttachment(c.Request.Context(), viewer(c), id, aid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = content.Reader.Close() }()

	if inline && content.Attachment.Kind != problem.AttachmentKindStatementImage {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if !inline && content.Attachment.Kind != problem.AttachmentKindFile {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if inline {
		c.Header("Content-Disposition", "inline")
	} else {
		c.Header("Content-Disposition", problem.AttachmentDisposition(content.Attachment.Filename))
	}
	c.DataFromReader(http.StatusOK, content.Attachment.Size, content.Attachment.ContentType, content.Reader, nil)
}

func (h *handler) uploadAttachment(c *gin.Context) {
	h.uploadAttachmentKind(c, false)
}

func (h *handler) uploadStatementImage(c *gin.Context) {
	h.uploadAttachmentKind(c, true)
}

func (h *handler) uploadAttachmentKind(c *gin.Context, image bool) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	header, ok := attachmentFormFile(c)
	if !ok {
		return
	}
	file, err := header.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = file.Close() }()

	claims, _ := middleware.CurrentClaims(c)
	up := problem.AttachmentUpload{
		Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Reader: file,
	}
	if image {
		a, err := h.svc.AddStatementImage(c.Request.Context(), id, claims.UserID, up)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		dto := problem.ToAttachmentDTO(a)
		httpx.Created(c, gin.H{"url": dto.URL, "markdown": "![" + a.Filename + "](" + dto.URL + ")"})
		return
	}
	a, err := h.svc.AddAttachment(c.Request.Context(), id, claims.UserID, up)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, problem.ToAttachmentDTO(a))
}

func (h *handler) deleteAttachment(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	aid, ok := pathID(c, "aid")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	if err := h.svc.DeleteAttachment(c.Request.Context(), id, aid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func attachmentFormFile(c *gin.Context) (*multipart.FileHeader, bool) {
	header, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 file"))
		return nil, false
	}
	return header, true
}
