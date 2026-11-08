package user

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	// maxAvatarBytes caps an uploaded avatar at 2 MiB.
	maxAvatarBytes = 2 << 20
	// avatarKeyPrefix is the storage prefix for uploaded avatars.
	avatarKeyPrefix = "avatars/"
)

// AvatarImage is an uploaded avatar read from storage, ready to serve.
type AvatarImage struct {
	Data        []byte
	ContentType string
}

// UpdateMeInput is the user-editable subset of the profile.
type UpdateMeInput struct {
	Nickname  *string `json:"nickname"`
	Bio       *string `json:"bio"`
	AvatarURL *string `json:"avatarUrl"`
}

// Option configures a Service; used to attach optional dependencies.
type Option func(*Service)

// WithStorage attaches the storage used for avatar upload/serve. Without it
// avatar upload/delete/read fail safely (upload/delete return internal error,
// read returns not found).
func WithStorage(s storage.Storage) Option {
	return func(svc *Service) { svc.store = s }
}

type Service struct {
	repo  Repository
	store storage.Storage
}

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Me returns the authenticated user. A valid token for a user that no longer
// exists is treated as unauthenticated.
func (s *Service) Me(ctx context.Context, id int64) (*User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrUnauthorized
	}
	return u, err
}

func (s *Service) UpdateMe(ctx context.Context, id int64, in UpdateMeInput) (*User, error) {
	var changes ProfileChanges
	if in.Nickname != nil {
		nick := strings.TrimSpace(*in.Nickname)
		if err := ValidateNickname(nick); err != nil {
			return nil, err
		}
		changes.Nickname = &nick
	}
	if in.Bio != nil {
		if err := ValidateBio(*in.Bio); err != nil {
			return nil, err
		}
		changes.Bio = in.Bio
	}

	// Avatar URL semantics: a non-empty http(s) link replaces any uploaded
	// avatar (clearing its key and deleting the old file); an empty link keeps
	// an existing upload but clears a pure external link.
	var oldKey string
	if in.AvatarURL != nil {
		current, err := s.repo.FindByID(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrUnauthorized
		}
		if err != nil {
			return nil, err
		}
		oldKey = current.AvatarKey
		avatar := strings.TrimSpace(*in.AvatarURL)
		if err := ValidateAvatarURL(avatar); err != nil {
			return nil, err
		}
		switch {
		case avatar != "":
			changes.AvatarURL = &avatar
			empty := ""
			changes.AvatarKey = &empty
			changes.AvatarContentType = &empty
		case current.AvatarKey == "":
			// No upload exists: an empty link clears the external URL.
			changes.AvatarURL = &avatar
		}
	}

	err := s.repo.UpdateProfile(ctx, id, changes)
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}

	// Best-effort: delete the replaced upload only after the DB write succeeded.
	if oldKey != "" && (changes.AvatarKey != nil && *changes.AvatarKey == "") && s.store != nil {
		_ = s.store.Delete(ctx, oldKey)
	}
	return s.Me(ctx, id)
}

// UploadAvatar validates and stores an avatar image, then points avatar_url at
// the internal image endpoint. The previous upload is deleted on success.
func (s *Service) UploadAvatar(ctx context.Context, id int64, r io.Reader) (*User, error) {
	if s.store == nil {
		return nil, httpx.ErrInternal
	}
	data, err := io.ReadAll(io.LimitReader(r, maxAvatarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, httpx.ErrValidation("请选择要上传的图片")
	}
	if len(data) > maxAvatarBytes {
		return nil, httpx.ErrValidation("图片不能超过 2 MiB")
	}
	contentType := http.DetectContentType(data)
	if !allowedAvatarImageType(contentType) {
		return nil, httpx.ErrValidation("仅支持 PNG、JPEG、WebP 或 GIF 图片")
	}

	current, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}

	suffix := randomHex(16)
	key := fmt.Sprintf("%s%d/%s", avatarKeyPrefix, id, suffix)
	if _, err := s.store.Put(ctx, key, bytes.NewReader(data), maxAvatarBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return nil, httpx.ErrValidation("图片不能超过 2 MiB")
		}
		return nil, err
	}

	url := fmt.Sprintf("/api/users/%d/avatar?v=%s", id, suffix)
	err = s.repo.UpdateProfile(ctx, id, ProfileChanges{
		AvatarURL:         &url,
		AvatarKey:         &key,
		AvatarContentType: &contentType,
	})
	if err != nil {
		// DB write failed: remove the just-stored file so no orphan remains.
		_ = s.store.Delete(ctx, key)
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrUnauthorized
		}
		return nil, err
	}

	if current.AvatarKey != "" && current.AvatarKey != key {
		_ = s.store.Delete(ctx, current.AvatarKey)
	}
	return s.Me(ctx, id)
}

// DeleteAvatar clears the avatar URL and uploaded file; it is idempotent.
func (s *Service) DeleteAvatar(ctx context.Context, id int64) (*User, error) {
	current, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	empty := ""
	err = s.repo.UpdateProfile(ctx, id, ProfileChanges{
		AvatarURL:         &empty,
		AvatarKey:         &empty,
		AvatarContentType: &empty,
	})
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if current.AvatarKey != "" && s.store != nil {
		_ = s.store.Delete(ctx, current.AvatarKey)
	}
	return s.Me(ctx, id)
}

// AvatarImage returns the uploaded avatar bytes and content type, or not found
// when the user has no upload or the file is missing.
func (s *Service) AvatarImage(ctx context.Context, id int64) (*AvatarImage, error) {
	u, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if u.AvatarKey == "" || s.store == nil {
		return nil, httpx.ErrNotFound
	}
	f, err := s.store.Open(ctx, u.AvatarKey)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxAvatarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAvatarBytes {
		return nil, httpx.ErrNotFound
	}
	return &AvatarImage{Data: data, ContentType: u.AvatarContentType}, nil
}

func (s *Service) Profile(ctx context.Context, id int64) (*User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrNotFound
	}
	return u, err
}

func (s *Service) List(ctx context.Context, page, pageSize int) (httpx.Page[MeDTO], error) {
	users, total, err := s.repo.List(ctx, (page-1)*pageSize, pageSize)
	if err != nil {
		return httpx.Page[MeDTO]{}, err
	}
	items := make([]MeDTO, len(users))
	for i := range users {
		items[i] = ToMe(&users[i])
	}
	return httpx.Page[MeDTO]{Total: total, Items: items}, nil
}

func allowedAvatarImageType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
