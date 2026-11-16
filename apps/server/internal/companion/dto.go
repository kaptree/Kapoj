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
