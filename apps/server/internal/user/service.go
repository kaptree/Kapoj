package user

import (
	"context"
	"errors"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

type UpdateMeInput struct {
	Nickname  *string `json:"nickname"`
	Bio       *string `json:"bio"`
	AvatarURL *string `json:"avatarUrl"`
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

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
	if in.AvatarURL != nil {
		avatar := strings.TrimSpace(*in.AvatarURL)
		if err := ValidateAvatarURL(avatar); err != nil {
			return nil, err
		}
		changes.AvatarURL = &avatar
	}
	err := s.repo.UpdateProfile(ctx, id, changes)
	if errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return s.Me(ctx, id)
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
