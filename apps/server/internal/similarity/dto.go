package similarity

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// Overview is the per-contest similarity dashboard. It lists every contest
// problem with its saved settings (or the default threshold) and, when one
// exists, the latest run of any status.
type Overview struct {
	ContestID int64         `json:"contestId"`
	Problems  []ProblemView `json:"problems"`
}

// ProblemView is one contest problem with its similarity settings and the
// latest run of any status.
type ProblemView struct {
	ProblemID        int64    `json:"problemId"`
	DisplayID        string   `json:"displayId"`
	Title            string   `json:"title"`
	Slug             string   `json:"slug"`
	ThresholdPercent int      `json:"thresholdPercent"`
	IgnoreCommonCode bool     `json:"ignoreCommonCode"`
	BaseLanguage     string   `json:"baseLanguage"`
	SubmissionCount  int      `json:"submissionCount"`
	MatchCount       int      `json:"matchCount"`
	LatestRun        *RunView `json:"latestRun"`
}

// SettingsInput is the request body for saving a problem's similarity
// settings.
type SettingsInput struct {
	ThresholdPercent int    `json:"thresholdPercent"`
	IgnoreCommonCode bool   `json:"ignoreCommonCode"`
	BaseLanguage     string `json:"baseLanguage"`
	BaseCode         string `json:"baseCode"`
}

// RunView is one similarity run as exposed to the API.
type RunView struct {
	ID               int64      `json:"id"`
	ContestID        int64      `json:"contestId"`
	ProblemID        int64      `json:"problemId"`
	Status           string     `json:"status"`
	ThresholdPercent int        `json:"thresholdPercent"`
	IgnoreCommonCode bool       `json:"ignoreCommonCode"`
	Engine           string     `json:"engine"`
	EngineVersion    string     `json:"engineVersion"`
	RequestedBy      int64      `json:"requestedBy"`
	SubmissionCount  int        `json:"submissionCount"`
	MatchCount       int        `json:"matchCount"`
	ErrorMessage     string     `json:"errorMessage"`
	StartedAt        *time.Time `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
}

// MatchView is one similarity pair as exposed in the list.
type MatchView struct {
	ID                   int64               `json:"id"`
	RunID                int64               `json:"runId"`
	ContestID            int64               `json:"contestId"`
	ProblemID            int64               `json:"problemId"`
	OriginalSubmissionID int64               `json:"originalSubmissionId"`
	SuspectSubmissionID  int64               `json:"suspectSubmissionId"`
	OriginalUserID       int64               `json:"originalUserId"`
	OriginalUsername     string              `json:"originalUsername"`
	OriginalNickname     string              `json:"originalNickname"`
	SuspectUserID        int64               `json:"suspectUserId"`
	SuspectUsername      string              `json:"suspectUsername"`
	SuspectNickname      string              `json:"suspectNickname"`
	Language             submission.Language `json:"language"`
	SimilarityPercent    float64             `json:"similarityPercent"`
	DicePercent          float64             `json:"dicePercent"`
	ThresholdPercent     int                 `json:"thresholdPercent"`
	OriginalCreatedAt    time.Time           `json:"originalCreatedAt"`
	SuspectCreatedAt     time.Time           `json:"suspectCreatedAt"`
	FinishedAt           *time.Time          `json:"finishedAt"`
}

// MatchDetail adds both code payloads and judge statuses to a MatchView.
type MatchDetail struct {
	MatchView
	OriginalCode   string            `json:"originalCode"`
	SuspectCode    string            `json:"suspectCode"`
	OriginalStatus submission.Status `json:"originalStatus"`
	SuspectStatus  submission.Status `json:"suspectStatus"`
}

// MatchPage is a page of match results.
type MatchPage struct {
	Total int64       `json:"total"`
	Items []MatchView `json:"items"`
}
