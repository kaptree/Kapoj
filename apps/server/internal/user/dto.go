package user

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
)

// RoleSummary 是用户管理角色摘要，用于 /users/me 权限展示。
type RoleSummary struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// MeDTO is what a user sees about themselves (and what admins see).
type MeDTO struct {
	ID             int64         `json:"id"`
	Username       string        `json:"username"`
	Nickname       string        `json:"nickname"`
	Email          string        `json:"email"`
	AvatarURL      string        `json:"avatarUrl"`
	AvatarUploaded bool          `json:"avatarUploaded"`
	Role           Role          `json:"role"`
	Rating         int           `json:"rating"`
	Coins          int64         `json:"coins"`
	Bio            string        `json:"bio"`
	Status         Status        `json:"status"`
	Roles          []RoleSummary `json:"roles"`
	Permissions    []string      `json:"permissions"`
	IsSuperAdmin   bool          `json:"isSuperAdmin"`
	CreatedAt      time.Time     `json:"createdAt"`
}

// ProfileDTO is the public view of a user. Equipped badges and title are
// filled by the API layer; defaults are an empty slice and nil.
type ProfileDTO struct {
	ID             int64                       `json:"id"`
	Username       string                      `json:"username"`
	Nickname       string                      `json:"nickname"`
	AvatarURL      string                      `json:"avatarUrl"`
	AvatarUploaded bool                        `json:"avatarUploaded"`
	Bio            string                      `json:"bio"`
	Rating         int                         `json:"rating"`
	Coins          int64                       `json:"coins"`
	EquippedBadges []gamification.BadgeSummary `json:"equippedBadges"`
	EquippedTitle  *gamification.TitleSummary  `json:"equippedTitle"`
	CreatedAt      time.Time                   `json:"createdAt"`
}

func ToMe(u *User) MeDTO {
	return MeDTO{
		ID: u.ID, Username: u.Username, Nickname: u.Nickname, Email: u.Email,
		AvatarURL: u.AvatarURL, AvatarUploaded: u.AvatarKey != "",
		Role: u.Role, Rating: u.Rating, Coins: u.Coins,
		Bio: u.Bio, Status: u.Status,
		Roles: []RoleSummary{}, Permissions: []string{}, IsSuperAdmin: false,
		CreatedAt: u.CreatedAt.UTC(),
	}
}

func ToProfile(u *User) ProfileDTO {
	return ProfileDTO{
		ID: u.ID, Username: u.Username, Nickname: u.Nickname, AvatarURL: u.AvatarURL,
		AvatarUploaded: u.AvatarKey != "",
		Bio:            u.Bio, Rating: u.Rating, Coins: u.Coins,
		EquippedBadges: []gamification.BadgeSummary{}, EquippedTitle: nil,
		CreatedAt: u.CreatedAt.UTC(),
	}
}
