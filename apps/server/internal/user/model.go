package user

import "time"

type Role string

const (
	RoleUser         Role = "user"
	RoleProblemAdmin Role = "problem_admin"
	RoleContestAdmin Role = "contest_admin"
	RoleSuperAdmin   Role = "super_admin"
)

type Status string

const (
	StatusActive Status = "active"
	StatusBanned Status = "banned"
)

// InitialRating is the contest rating every new user starts with (DEVELOPMENT.md 5.5).
const InitialRating = 1500

type User struct {
	ID           int64  `gorm:"primaryKey"`
	Username     string `gorm:"size:32;not null;uniqueIndex:uniq_users_username"`
	Nickname     string `gorm:"size:64;not null"`
	Email        string `gorm:"size:254;not null;uniqueIndex:uniq_users_email"`
	PasswordHash string `gorm:"size:100;not null" json:"-"`
	AvatarURL    string `gorm:"size:512;not null;default:''"`
	// AvatarKey is the storage key of an uploaded avatar; empty means none.
	AvatarKey         string    `gorm:"size:255;not null;default:''"`
	AvatarContentType string    `gorm:"size:100;not null;default:''"`
	Role              Role      `gorm:"size:20;not null;default:user"`
	Rating            int       `gorm:"not null;default:1500"`
	Coins             int64     `gorm:"not null;default:0;check:chk_users_coins_non_negative,coins >= 0"`
	Bio               string    `gorm:"size:800;not null;default:''"`
	Status            Status    `gorm:"size:20;not null;default:active"`
	CreatedAt         time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt         time.Time `gorm:"type:timestamptz;not null"`
}
