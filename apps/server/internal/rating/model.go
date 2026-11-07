package rating

import "time"

// HistoryKind distinguishes rating history entries.
type HistoryKind string

const (
	HistoryContest     HistoryKind = "contest"
	HistoryDecay       HistoryKind = "decay"
	HistorySeasonReset HistoryKind = "season_reset"
)

// RatingHistory is one immutable rating change for a user. ContestID is
// nullable so decay/season entries can exist. Two partial unique indexes
// guarantee one contest settlement per user and contest, and one decay/season
// record per user per kind+period.
type RatingHistory struct {
	ID           int64       `gorm:"primaryKey"`
	UserID       int64       `gorm:"not null;uniqueIndex:uniq_rating_history_user_contest,priority:1;uniqueIndex:uniq_rating_history_user_period,priority:1"`
	ContestID    *int64      `gorm:"uniqueIndex:uniq_rating_history_user_contest,priority:2,where:contest_id IS NOT NULL"`
	Kind         HistoryKind `gorm:"size:20;not null;uniqueIndex:uniq_rating_history_user_period,priority:2"`
	Rank         int         `gorm:"not null"`
	RatingBefore int         `gorm:"not null"`
	RatingAfter  int         `gorm:"not null"`
	PeriodKey    string      `gorm:"size:64;not null;default:'';uniqueIndex:uniq_rating_history_user_period,priority:3,where:period_key <> ''"`
	CreatedAt    time.Time   `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (RatingHistory) TableName() string { return "rating_history" }

// RatingSettings is the singleton configuration row (id=1) for decay and season
// reset. Missing rows are lazily created on first read.
type RatingSettings struct {
	ID                 int64     `gorm:"primaryKey"`
	DecayEnabled       bool      `gorm:"not null;default:true"`
	DecayThreshold     int       `gorm:"not null;default:2400"`
	DecayInactiveDays  int       `gorm:"not null;default:60"`
	DecayPercent       int       `gorm:"not null;default:3"`
	DecayMax           int       `gorm:"not null;default:100"`
	SeasonEnabled      bool      `gorm:"not null;default:true"`
	SeasonMonths       int       `gorm:"not null;default:6"`
	SeasonResetPercent int       `gorm:"not null;default:30"`
	SeasonAnchor       time.Time `gorm:"type:date;not null"`
	UpdatedBy          int64     `gorm:"not null;default:0"`
	UpdatedAt          time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (RatingSettings) TableName() string { return "rating_settings" }

// RatingRun is an idempotency marker for a decay/season pass. ProcessedAt is
// null until the pass finishes; the unique key prevents duplicate work.
type RatingRun struct {
	Key         string      `gorm:"size:64;primaryKey"`
	Kind        HistoryKind `gorm:"size:20;not null"`
	ProcessedAt *time.Time  `gorm:"type:timestamptz"`
	CreatedAt   time.Time   `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (RatingRun) TableName() string { return "rating_runs" }
