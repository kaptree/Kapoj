package problem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	AttachmentKindFile           = "attachment"
	AttachmentKindStatementImage = "statement_image"

	maxAttachmentBytes       = 20 << 20
	maxStatementImageBytes   = 5 << 20
	maxAttachmentsPerProblem = 50
)

// ProblemAttachment is a public attachment or an inline statement image.
// Test data remains in problem_testcases and is never exposed here.
type ProblemAttachment struct {
	ID          int64     `gorm:"primaryKey"`
	ProblemID   int64     `gorm:"not null;index:idx_problem_attachments_problem_kind,priority:1"`
	Kind        string    `gorm:"size:20;not null;index:idx_problem_attachments_problem_kind,priority:2"`
	Filename    string    `gorm:"size:255;not null"`
	StorageKey  string    `gorm:"size:255;not null"`
	ContentType string    `gorm:"size:100;not null;default:''"`
	Size        int64     `gorm:"not null;default:0"`
	CreatedBy   int64     `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (ProblemAttachment) TableName() string { return "problem_attachments" }

// AttachmentUpload carries one uploaded attachment. The handler owns the file
// handle and closes it after the service returns.
type AttachmentUpload struct {
	Filename    string
	ContentType string
	Reader      io.Reader
}

// AttachmentContent is one attachment plus its open storage stream.
type AttachmentContent struct {
	Attachment *ProblemAttachment
	Reader     io.ReadCloser
}

// AttachmentDTO is the public list shape; StorageKey is never exposed.
type AttachmentDTO struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"createdAt"`
	URL         string    `json:"url"`
}

func ToAttachmentDTO(a *ProblemAttachment) AttachmentDTO {
	base := fmt.Sprintf("/api/problems/%d/attachments/%d", a.ProblemID, a.ID)
	if a.Kind == AttachmentKindStatementImage {
		base = fmt.Sprintf("/api/problems/%d/assets/%d", a.ProblemID, a.ID)
	}
	return AttachmentDTO{
		ID: a.ID, Filename: a.Filename, ContentType: a.ContentType,
		Size: a.Size, CreatedAt: a.CreatedAt.UTC(), URL: base,
	}
}

// AddAttachment validates and stores a public attachment.
func (s *Service) AddAttachment(ctx context.Context, problemID, userID int64, up AttachmentUpload) (*ProblemAttachment, error) {
	return s.addAttachment(ctx, problemID, userID, AttachmentKindFile, up)
}

// AddStatementImage validates and stores an inline statement image.
func (s *Service) AddStatementImage(ctx context.Context, problemID, userID int64, up AttachmentUpload) (*ProblemAttachment, error) {
	return s.addAttachment(ctx, problemID, userID, AttachmentKindStatementImage, up)
}

func (s *Service) addAttachment(ctx context.Context, problemID, userID int64, kind string, up AttachmentUpload) (*ProblemAttachment, error) {
	if _, err := s.repo.FindByID(ctx, problemID); err != nil {
		return nil, mapRepoErr(err)
	}
	count, err := s.repo.CountAttachments(ctx, problemID)
	if err != nil {
		return nil, err
	}
	if count >= maxAttachmentsPerProblem {
		return nil, httpx.ErrValidation(fmt.Sprintf("题目附件数量不能超过 %d 个", maxAttachmentsPerProblem))
	}

	filename := sanitizeAttachmentFilename(up.Filename)
	maxBytes := int64(maxAttachmentBytes)
	if kind == AttachmentKindStatementImage {
		maxBytes = maxStatementImageBytes
	}
	data, err := io.ReadAll(io.LimitReader(up.Reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		if kind == AttachmentKindStatementImage {
			return nil, httpx.ErrValidation("题面图片不能超过 5 MiB")
		}
		return nil, httpx.ErrValidation("附件不能超过 20 MiB")
	}
	contentType := http.DetectContentType(data)
	if kind == AttachmentKindStatementImage {
		if !allowedAttachmentImage(contentType) {
			return nil, httpx.ErrValidation("题面图片仅支持 PNG、JPEG、WebP 或 GIF")
		}
	} else if up.ContentType != "" && (contentType == "application/octet-stream" || contentType == "text/plain") {
		contentType = up.ContentType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	ext := safeAttachmentExt(filename)
	if kind == AttachmentKindStatementImage {
		ext = imageExt(contentType)
	}
	key := fmt.Sprintf("problems/%d/assets/%s%s", problemID, randomHex(), ext)
	if kind == AttachmentKindFile {
		key = fmt.Sprintf("problems/%d/attachments/%s%s", problemID, randomHex(), ext)
	}
	if _, err := s.store.Put(ctx, key, bytes.NewReader(data), maxBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return nil, httpx.ErrValidation("附件超过大小限制")
		}
		return nil, err
	}

	a := &ProblemAttachment{
		ProblemID: problemID, Kind: kind, Filename: filename, StorageKey: key,
		ContentType: contentType, Size: int64(len(data)), CreatedBy: userID,
	}
	if err := s.repo.CreateAttachment(ctx, a); err != nil {
		s.deleteFiles(ctx, []string{key})
		return nil, err
	}
	return a, nil
}

// ListAttachments returns public attachments of a visible problem. Statement
// images are managed by the editor and are not included in this list.
func (s *Service) ListAttachments(ctx context.Context, v Viewer, problemID int64) ([]ProblemAttachment, error) {
	if _, err := s.Get(ctx, v, problemID); err != nil {
		return nil, err
	}
	return s.repo.ListAttachments(ctx, problemID, AttachmentKindFile)
}

// OpenAttachment opens a visible attachment or statement image.
func (s *Service) OpenAttachment(ctx context.Context, v Viewer, problemID, attachmentID int64) (*AttachmentContent, error) {
	if _, err := s.Get(ctx, v, problemID); err != nil {
		return nil, err
	}
	a, err := s.repo.FindAttachment(ctx, problemID, attachmentID)
	if err != nil {
		return nil, err
	}
	f, err := s.store.Open(ctx, a.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &AttachmentContent{Attachment: a, Reader: f}, nil
}

// DeleteAttachment removes the database row and best-effort deletes the file.
func (s *Service) DeleteAttachment(ctx context.Context, problemID, attachmentID int64) error {
	a, err := s.repo.DeleteAttachment(ctx, problemID, attachmentID)
	if err != nil {
		return err
	}
	s.deleteFiles(ctx, []string{a.StorageKey})
	return nil
}

func sanitizeAttachmentFilename(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(strings.ReplaceAll(name, "\\", "_"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, ".")
	if name == "" {
		return "file"
	}
	runes := []rune(name)
	if len(runes) > 150 {
		name = string(runes[:150])
	}
	return name
}

func safeAttachmentExt(filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	if len(ext) < 2 || len(ext) > 12 {
		return ""
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return ext
}

func allowedAttachmentImage(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}

func imageExt(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	return ""
}

// AttachmentDisposition builds an RFC 2231-safe download header.
func AttachmentDisposition(filename string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}
