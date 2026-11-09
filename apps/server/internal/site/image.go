package site

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	// maxLogoBytes caps an uploaded logo at 2 MiB.
	maxLogoBytes = 2 << 20
	// maxFaviconBytes caps an uploaded favicon at 1 MiB.
	maxFaviconBytes = 1 << 20

	logoDir    = "site/logo"
	faviconDir = "site/favicon"
)

// ErrNotFound reports that no image has been uploaded or the stored object is
// missing.
var ErrNotFound = errors.New("site: image not found")

// Image is an uploaded site image ready to be served.
type Image struct {
	ContentType string
	Data        []byte
}

// UploadLogo validates and stores a logo image, then points the settings row at
// it. The previous upload is best-effort deleted after the DB write succeeds.
func (s *Service) UploadLogo(ctx context.Context, actorID int64, r io.Reader) (*AdminView, error) {
	return s.uploadImage(ctx, actorID, r, false)
}

// DeleteLogo clears the logo reference and removes the stored file. It is
// idempotent: deleting when no logo exists succeeds.
func (s *Service) DeleteLogo(ctx context.Context, actorID int64) (*AdminView, error) {
	return s.deleteImage(ctx, actorID, false)
}

// UploadFavicon validates and stores a favicon image, then points the settings
// row at it. The previous upload is best-effort deleted after the DB write
// succeeds.
func (s *Service) UploadFavicon(ctx context.Context, actorID int64, r io.Reader) (*AdminView, error) {
	return s.uploadImage(ctx, actorID, r, true)
}

// DeleteFavicon clears the favicon reference and removes the stored file. It is
// idempotent: deleting when no favicon exists succeeds.
func (s *Service) DeleteFavicon(ctx context.Context, actorID int64) (*AdminView, error) {
	return s.deleteImage(ctx, actorID, true)
}

// Logo returns the uploaded logo bytes and content type, or ErrNotFound when
// nothing has been uploaded.
func (s *Service) Logo(ctx context.Context) (*Image, error) {
	return s.readImage(ctx, false)
}

// Favicon returns the uploaded favicon bytes and content type, or ErrNotFound
// when nothing has been uploaded.
func (s *Service) Favicon(ctx context.Context) (*Image, error) {
	return s.readImage(ctx, true)
}

func (s *Service) uploadImage(ctx context.Context, actorID int64, r io.Reader, favicon bool) (*AdminView, error) {
	dir, label, maxBytes := logoDir, "Logo", int64(maxLogoBytes)
	if favicon {
		dir, label, maxBytes = faviconDir, "站点图标", int64(maxFaviconBytes)
	}

	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, httpx.ErrValidation("请选择要上传的图片")
	}
	if int64(len(data)) > maxBytes {
		return nil, httpx.ErrValidation(label + "不能超过 " + byteLimitLabel(maxBytes))
	}
	contentType := http.DetectContentType(data)
	if !allowedImageType(contentType) {
		return nil, httpx.ErrValidation("仅支持 PNG、JPEG、WebP 或 GIF 图片")
	}

	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("%s/%s", dir, randomHex(16))
	if _, err := s.store.Put(ctx, key, bytes.NewReader(data), maxBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return nil, httpx.ErrValidation(label + "不能超过 " + byteLimitLabel(maxBytes))
		}
		return nil, err
	}

	oldKey := settings.LogoKey
	if favicon {
		oldKey = settings.FaviconKey
		settings.FaviconKey = key
		settings.FaviconContentType = contentType
	} else {
		settings.LogoKey = key
		settings.LogoContentType = contentType
	}
	settings.UpdatedBy = actorID

	if err := s.repo.Update(ctx, settings); err != nil {
		// DB write failed: remove the just-stored file so no orphan remains.
		_ = s.store.Delete(ctx, key)
		return nil, err
	}
	if oldKey != "" && oldKey != key {
		_ = s.store.Delete(ctx, oldKey)
	}
	return s.Admin(ctx)
}

func (s *Service) deleteImage(ctx context.Context, actorID int64, favicon bool) (*AdminView, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}

	oldKey := settings.LogoKey
	if favicon {
		oldKey = settings.FaviconKey
	}
	if oldKey != "" {
		if favicon {
			settings.FaviconKey = ""
			settings.FaviconContentType = ""
		} else {
			settings.LogoKey = ""
			settings.LogoContentType = ""
		}
		settings.UpdatedBy = actorID
		if err := s.repo.Update(ctx, settings); err != nil {
			return nil, err
		}
		_ = s.store.Delete(ctx, oldKey)
	}
	return s.Admin(ctx)
}

func (s *Service) readImage(ctx context.Context, favicon bool) (*Image, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	key, contentType, maxBytes := settings.LogoKey, settings.LogoContentType, int64(maxLogoBytes)
	if favicon {
		key, contentType, maxBytes = settings.FaviconKey, settings.FaviconContentType, int64(maxFaviconBytes)
	}
	if key == "" || s.store == nil {
		return nil, ErrNotFound
	}

	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrNotFound
	}
	return &Image{ContentType: contentType, Data: data}, nil
}

func allowedImageType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}

func byteLimitLabel(maxBytes int64) string {
	if maxBytes == maxFaviconBytes {
		return "1 MiB"
	}
	return "2 MiB"
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
