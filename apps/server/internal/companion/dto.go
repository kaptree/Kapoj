package companion

import "time"

// SettingsInput is the user-editable companion settings payload. The service
// layer validates and defaults these before the repository persists them.
type SettingsInput struct {
	ActivityLevel string
	DockEnabled   bool
	PublicProfile bool
	EquippedSkin  string
}

// EventInput describes one bond event to apply. EventDate is a YYYY-MM-DD
// local date supplied by the caller; the repository never derives it from the
// clock. SubCap, when > 0, limits how many events of the same Type may award
// bond on a single EventDate.
type EventInput struct {
	UserID         int64
	Key            string
	Type           string
	Bond           int
	EventDate      string
	Payload        map[string]any
	DailyCapBypass bool
	SubCap         int
}

// SubmissionInfo is the subset of a submission the event engine needs to
// decide the ac:daily and ac:first awards.
type SubmissionInfo struct {
	ID        int64
	UserID    int64
	ProblemID int64
	Status    string
	CreatedAt time.Time
	JudgedAt  *time.Time
}

// StateView is the read model returned by the API for the companion state. It
// carries the denormalized progress plus the unread event count and today's
// gain breakdown for the dock bond meter.
type StateView struct {
	Bond          int         `json:"bond"`
	Level         int         `json:"level"`
	LevelName     string      `json:"levelName"`
	NextThreshold int         `json:"nextThreshold"`
	DailyGain     int         `json:"dailyGain"`
	DailyCap      int         `json:"dailyCap"`
	ActivityLevel string      `json:"activityLevel"`
	DockEnabled   bool        `json:"dockEnabled"`
	PublicProfile bool        `json:"publicProfile"`
	EquippedSkin  string      `json:"equippedSkin"`
	UnlockedSkins []string    `json:"unlockedSkins"`
	UnreadCount   int         `json:"unreadCount"`
	TodayEvents   []EventView `json:"todayEvents"`
}

// EventView is the read model for one companion event.
type EventView struct {
	ID        int64          `json:"id"`
	Type      string         `json:"type"`
	Bond      int            `json:"bond"`
	Payload   map[string]any `json:"payload"`
	ReadAt    *time.Time     `json:"readAt,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

// PetResult describes the outcome of a pet interaction: whether bond was
// awarded for this date, and the resulting bond/level for the meter.
type PetResult struct {
	Awarded bool `json:"awarded"`
	Bond    int  `json:"bond"`
	Level   int  `json:"level"`
}
