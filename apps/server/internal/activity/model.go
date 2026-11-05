package activity

import "time"

// Action is the kind of activity recorded for a user.
type Action string

const (
	// ActionSubmission is awarded once per submission.
	ActionSubmission Action = "submission"
	// ActionContestJoin is awarded once per contest registration.
	ActionContestJoin Action = "contest_join"
	// ActionSignIn is awarded once per local day.
	ActionSignIn Action = "signin"
)

const (
	refTypeSubmission = "submission"
	refTypeContest    = "contest"
	refTypeDay        = "day"
)

// ActivityLog is one immutable activity award. The unique index on
// (user_id, action, ref_type, ref_id) makes every award idempotent per object.
type ActivityLog struct {
	ID        int64     `gorm:"primaryKey"`
	UserID    int64     `gorm:"not null;uniqueIndex:uniq_activity_logs_dedupe,priority:1;index:idx_activity_logs_user_created,priority:1"`
	Action    Action    `gorm:"size:20;not null;uniqueIndex:uniq_activity_logs_dedupe,priority:2"`
	Points    int       `gorm:"not null"`
	RefType   string    `gorm:"size:20;not null;uniqueIndex:uniq_activity_logs_dedupe,priority:3"`
	RefID     int64     `gorm:"not null;uniqueIndex:uniq_activity_logs_dedupe,priority:4"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null;index:idx_activity_logs_user_created,priority:2;index:idx_activity_logs_created"`
}

// TableName keeps the table name from the design document.
func (ActivityLog) TableName() string { return "activity_logs" }

// SharedItem records a shareable object owned by a user. This phase only
// writes type=contest; solution likes are reserved for a later phase.
type SharedItem struct {
	ID        int64     `gorm:"primaryKey"`
	UserID    int64     `gorm:"not null"`
	Type      string    `gorm:"size:20;not null;uniqueIndex:uniq_shared_items_type_ref,priority:1"`
	RefID     int64     `gorm:"not null;uniqueIndex:uniq_shared_items_type_ref,priority:2"`
	LikeCount int       `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (SharedItem) TableName() string { return "shared_items" }
