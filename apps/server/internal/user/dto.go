package user

import "time"

// MeDTO is what a user sees about themselves (and what admins see).
type MeDTO struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatarUrl"`
	Role      Role      `json:"role"`
	Rating    int       `json:"rating"`
	Coins     int64     `json:"coins"`
	Bio       string    `json:"bio"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

// ProfileDTO is the public view of a user. Badges and titles are filled in
// once the gamification modules (M3) exist.
type ProfileDTO struct {
	ID             int64     `json:"id"`
	Username       string    `json:"username"`
	Nickname       string    `json:"nickname"`
	AvatarURL      string    `json:"avatarUrl"`
	Bio            string    `json:"bio"`
	Rating         int       `json:"rating"`
	Coins          int64     `json:"coins"`
	EquippedBadges []any     `json:"equippedBadges"`
	EquippedTitle  any       `json:"equippedTitle"`
	CreatedAt      time.Time `json:"createdAt"`
}

func ToMe(u *User) MeDTO {
	return MeDTO{
		ID: u.ID, Username: u.Username, Nickname: u.Nickname, Email: u.Email,
		AvatarURL: u.AvatarURL, Role: u.Role, Rating: u.Rating, Coins: u.Coins,
		Bio: u.Bio, Status: u.Status, CreatedAt: u.CreatedAt.UTC(),
	}
}

func ToProfile(u *User) ProfileDTO {
	return ProfileDTO{
		ID: u.ID, Username: u.Username, Nickname: u.Nickname, AvatarURL: u.AvatarURL,
		Bio: u.Bio, Rating: u.Rating, Coins: u.Coins,
		EquippedBadges: []any{}, EquippedTitle: nil, CreatedAt: u.CreatedAt.UTC(),
	}
}
