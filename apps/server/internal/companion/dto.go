package companion

import (
	"context"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
)

// SettingsInput is the user-editable companion settings payload. The service
// layer validates and defaults these before the repository persists them.
type SettingsInput struct {
	ActivityLevel     string
	DockEnabled       bool
	PublicProfile     bool
	EquippedSkin      string
	PersonalityPrompt string
	AIDialogueEnabled bool
}

// SettingsPatch is a partial settings update. Only non-nil fields are written,
// so concurrent requests touching different fields cannot clobber each other.
// The service validates the activity enum and the skin length before the
// repository applies the patch under a state-row lock.
type SettingsPatch struct {
	ActivityLevel     *string `json:"activityLevel"`
	DockEnabled       *bool   `json:"dockEnabled"`
	PublicProfile     *bool   `json:"publicProfile"`
	EquippedSkin      *string `json:"equippedSkin"`
	PersonalityPrompt *string `json:"personalityPrompt"`
	AIDialogueEnabled *bool   `json:"aiDialogueEnabled"`
}

// DialogueGenerator is the AI seam used by the companion dialogue service.
type DialogueGenerator interface {
	Chat(ctx context.Context, userID int64, messages []ai.ChatMessage, maxTokens int) (string, error)
}

// AIStatus reports whether the user has a usable AI model configuration.
type AIStatus interface {
	HasActiveModel(ctx context.Context, userID int64) (bool, error)
}

// CompanionNameProvider returns the site-configured companion name.
type CompanionNameProvider interface {
	CompanionName(ctx context.Context) (string, error)
}

// DialogueRequest is one companion interaction. The client owns the visual
// expression; the server uses it only as prompt context.
type DialogueRequest struct {
	Trigger     string   `json:"trigger"`
	Expression  string   `json:"expression"`
	GreetingKey string   `json:"greetingKey"`
	RecentLines []string `json:"recentLines"`
}

// DialogueResult is either an AI line or a static fallback key. The endpoint
// always returns 200 for AI unavailability; only malformed requests fail.
type DialogueResult struct {
	Source         string `json:"source"`
	Line           string `json:"line,omitempty"`
	LineKey        string `json:"lineKey,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RemainingToday int    `json:"remainingToday"`
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
	Bond              int         `json:"bond"`
	Level             int         `json:"level"`
	LevelName         string      `json:"levelName"`
	NextThreshold     int         `json:"nextThreshold"`
	DailyGain         int         `json:"dailyGain"`
	DailyCap          int         `json:"dailyCap"`
	ActivityLevel     string      `json:"activityLevel"`
	DockEnabled       bool        `json:"dockEnabled"`
	PublicProfile     bool        `json:"publicProfile"`
	EquippedSkin      string      `json:"equippedSkin"`
	UnlockedSkins     []string    `json:"unlockedSkins"`
	PersonalityPrompt string      `json:"personalityPrompt"`
	AIDialogueEnabled bool        `json:"aiDialogueEnabled"`
	AIReady           bool        `json:"aiReady"`
	AIRemainingToday  int         `json:"aiRemainingToday"`
	UnreadCount       int         `json:"unreadCount"`
	TodayEvents       []EventView `json:"todayEvents"`
}

// PublicStateView is the privacy-filtered companion summary shown on a user's
// profile. It intentionally omits settings and event details that belong to the
// owner-only state view.
type PublicStateView struct {
	UserID            int64    `json:"userId"`
	Bond              int      `json:"bond"`
	Level             int      `json:"level"`
	LevelName         string   `json:"levelName"`
	NextThreshold     int      `json:"nextThreshold"`
	EquippedSkin      string   `json:"equippedSkin"`
	UnlockedSkins     []string `json:"unlockedSkins"`
	PublicProfile     bool     `json:"publicProfile"`
	DaysTogether      int      `json:"daysTogether"`
	FavoriteEventType string   `json:"favoriteEventType,omitempty"`
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
