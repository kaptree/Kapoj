package rating

import "time"

// HistoryKind distinguishes rating history entries. This phase only writes
// contest settlements; decay/season kinds are reserved for later phases.
type HistoryKind string

const (
	HistoryContest HistoryKind = "contest"
)

// RatingHistory is one immutable rating change for a user. ContestID is
// nullable so future decay/season entries can exist. The partial unique index
// guarantees one contest settlement per user and contest while still allowing
// unlimited rows with a NULL contest_id.
type RatingHistory struct {
	ID           int64       `gorm:"primaryKey"`
	UserID       int64       `gorm:"not null;uniqueIndex:uniq_rating_history_user_contest,priority:1"`
	ContestID    *int64      `gorm:"uniqueIndex:uniq_rating_history_user_contest,priority:2,where:contest_id IS NOT NULL"`
	Kind         HistoryKind `gorm:"size:20;not null"`
	Rank         int         `gorm:"not null"`
	RatingBefore int         `gorm:"not null"`
	RatingAfter  int         `gorm:"not null"`
	CreatedAt    time.Time   `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (RatingHistory) TableName() string { return "rating_history" }
