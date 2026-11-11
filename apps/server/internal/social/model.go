package social

import "time"

const (
	// TypeSolutionReply 表示“回复了你的解题思路”。
	TypeSolutionReply = "solution_reply"
	// TypeSolutionLike 表示“赞了你的解题思路”。
	TypeSolutionLike = "solution_like"
	// TypeFollow 表示“关注了你”。
	TypeFollow = "follow"
)

const (
	targetTypeSolutionPost = "solution_post"
	targetTypeUser         = "user"
)

// Follow 是关注关系；follower 关注 followee。
type Follow struct {
	ID         int64     `gorm:"primaryKey"`
	FollowerID int64     `gorm:"not null;uniqueIndex:uniq_user_follows,priority:1;check:chk_user_follows_no_self,follower_id <> followee_id"`
	FolloweeID int64     `gorm:"not null;uniqueIndex:uniq_user_follows,priority:2;index:idx_user_follows_followee"`
	CreatedAt  time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (Follow) TableName() string { return "user_follows" }

// Notification 是站内通知；dedupe_key 全局唯一用于幂等去重。
type Notification struct {
	ID         int64      `gorm:"primaryKey"`
	UserID     int64      `gorm:"not null;index:idx_notifications_user_created,priority:1;index:idx_notifications_user_read,priority:1"`
	ActorID    int64      `gorm:"not null"`
	Type       string     `gorm:"size:20;not null"`
	DedupeKey  string     `gorm:"column:dedupe_key;size:100;not null;uniqueIndex:uniq_notifications_dedupe"`
	TargetType string     `gorm:"column:target_type;size:20;not null"`
	TargetID   int64      `gorm:"column:target_id;not null"`
	Link       string     `gorm:"size:255;not null"`
	ReadAt     *time.Time `gorm:"type:timestamptz;index:idx_notifications_user_read,priority:2"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null;index:idx_notifications_user_created,priority:2"`
}

// TableName keeps the table name from the design document.
func (Notification) TableName() string { return "notifications" }

// UserEvaluation 是用户主页上的一条评价；每位作者对同一目标只能有一条。
type UserEvaluation struct {
	ID           int64     `gorm:"primaryKey"`
	TargetUserID int64     `gorm:"not null;uniqueIndex:uniq_user_evaluations_author_target,priority:2;index:idx_user_evaluations_target_updated,priority:1"`
	AuthorUserID int64     `gorm:"not null;uniqueIndex:uniq_user_evaluations_author_target,priority:1"`
	Content      string    `gorm:"size:40;not null"`
	CreatedAt    time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt    time.Time `gorm:"type:timestamptz;not null;index:idx_user_evaluations_target_updated,priority:2"`
}

// TableName keeps the table name from the design document.
func (UserEvaluation) TableName() string { return "user_evaluations" }

// UserSummary 是用户公开摘要，供粉丝/关注列表与通知 actor 使用；不含邮箱。
type UserSummary struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	Rating    int    `json:"rating"`
}

// FollowResult 是关注/取消关注的返回。
type FollowResult struct {
	Following      bool  `json:"following"`
	FollowersCount int64 `json:"followersCount"`
}

// SocialView 是用户主页的社交统计。
type SocialView struct {
	FollowersCount int64 `json:"followersCount"`
	FollowingCount int64 `json:"followingCount"`
	IsFollowing    bool  `json:"isFollowing"`
}

// NotificationItem 是一条通知的对外表示。
type NotificationItem struct {
	ID         int64        `json:"id"`
	Type       string       `json:"type"`
	Actor      *UserSummary `json:"actor"`
	TargetType string       `json:"targetType"`
	TargetID   int64        `json:"targetId"`
	Link       string       `json:"link"`
	ReadAt     *time.Time   `json:"readAt"`
	CreatedAt  time.Time    `json:"createdAt"`
}

// NotificationPage 是通知分页列表。
type NotificationPage struct {
	Total int64              `json:"total"`
	Items []NotificationItem `json:"items"`
}
