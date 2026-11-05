package submission

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
)

// Created is returned by POST /submissions.
type Created struct {
	ID     int64  `json:"id"`
	Status Status `json:"status"`
}

func ToCreated(sub *Submission) Created {
	return Created{ID: sub.ID, Status: sub.Status}
}

// ListItem is one row of GET /submissions.
type ListItem struct {
	ID             int64                       `json:"id"`
	UserID         int64                       `json:"userId"`
	Username       string                      `json:"username"`
	Nickname       string                      `json:"nickname"`
	EquippedBadges []gamification.BadgeSummary `json:"equippedBadges"`
	EquippedTitle  *gamification.TitleSummary  `json:"equippedTitle"`
	ProblemID      int64                       `json:"problemId"`
	ProblemTitle   string                      `json:"problemTitle"`
	ProblemSlug    string                      `json:"problemSlug"`
	ContestID      *int64                      `json:"contestId,omitempty"`
	Language       Language                    `json:"language"`
	Status         Status                      `json:"status"`
	TimeUsedMS     int                         `json:"timeUsedMs"`
	MemoryUsedKB   int                         `json:"memoryUsedKb"`
	Score          int                         `json:"score"`
	CreatedAt      time.Time                   `json:"createdAt"`
	JudgedAt       *time.Time                  `json:"judgedAt,omitempty"`
}

// Detail is GET /submissions/:id. Code and JudgeResult are hidden for other
// users' contest submissions (M2 contest protection).
type Detail struct {
	ListItem
	Code         string            `json:"code,omitempty"`
	CodeHidden   bool              `json:"codeHidden"`
	ErrorMessage string            `json:"errorMessage,omitempty"`
	JudgeResult  []JudgeCaseResult `json:"judgeResult"`
}

func toListItem(row ListRow) ListItem {
	s := row.Submission
	return ListItem{
		ID: s.ID, UserID: s.UserID, Username: row.Username, Nickname: row.Nickname,
		EquippedBadges: []gamification.BadgeSummary{}, EquippedTitle: nil,
		ProblemID: s.ProblemID, ProblemTitle: row.ProblemTitle, ProblemSlug: row.ProblemSlug,
		ContestID: s.ContestID, Language: s.Language, Status: s.Status,
		TimeUsedMS: s.TimeUsedMS, MemoryUsedKB: s.MemoryUsedKB, Score: s.Score,
		CreatedAt: s.CreatedAt, JudgedAt: s.JudgedAt,
	}
}
