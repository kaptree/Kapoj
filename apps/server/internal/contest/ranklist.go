package contest

import (
	"context"
	"sort"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
)

type RankSubmission struct {
	UserID    int64     `gorm:"column:user_id"`
	ProblemID int64     `gorm:"column:problem_id"`
	Status    string    `gorm:"column:status"`
	Score     int       `gorm:"column:score"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

type RankProblem struct {
	ProblemID int64  `json:"problemId"`
	DisplayID string `json:"displayId"`
	Title     string `json:"title"`
}

type RankCell struct {
	Status     string     `json:"status,omitempty"`
	Attempts   int        `json:"attempts"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
	Score      int        `json:"score"`
	FirstBlood bool       `json:"firstBlood"`
}

type RankRow struct {
	Rank             int                         `json:"rank"`
	UserID           int64                       `json:"userId"`
	Username         string                      `json:"username"`
	Nickname         string                      `json:"nickname"`
	EquippedBadges   []gamification.BadgeSummary `json:"equippedBadges"`
	EquippedTitle    *gamification.TitleSummary  `json:"equippedTitle"`
	Solved           int                         `json:"solved"`
	Penalty          int                         `json:"penalty"`
	TotalScore       int                         `json:"totalScore"`
	LastAcceptedAt   *time.Time                  `json:"lastAcceptedAt,omitempty"`
	LastSubmissionAt *time.Time                  `json:"-"`
	Problems         []RankCell                  `json:"problems"`
}

type Ranklist struct {
	ContestID   int64         `json:"contestId"`
	Mode        Mode          `json:"mode"`
	Status      Status        `json:"status"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Problems    []RankProblem `json:"problems"`
	Rows        []RankRow     `json:"rows"`
	SelfOnly    bool          `json:"selfOnly"`
}

type rankState struct {
	row RankRow
}

func (s *Service) Ranklist(ctx context.Context, v Viewer, id int64) (*Ranklist, error) {
	c, err := s.findAccessible(ctx, v, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	status := StatusAt(c.StartTime, c.EndTime, now)
	if status == StatusUpcoming && !v.CanViewHidden {
		return &Ranklist{
			ContestID: id, Mode: c.Mode, Status: status, GeneratedAt: now,
			Problems: []RankProblem{}, Rows: []RankRow{}, SelfOnly: false,
		}, nil
	}
	problems, err := s.repo.FindProblems(ctx, id)
	if err != nil {
		return nil, err
	}
	participants, err := s.repo.ListParticipants(ctx, id)
	if err != nil {
		return nil, err
	}
	submissions, err := s.repo.RankSubmissions(ctx, id)
	if err != nil {
		return nil, err
	}

	problemIndex := make(map[int64]int, len(problems))
	rankProblems := make([]RankProblem, len(problems))
	for i, p := range problems {
		problemIndex[p.ProblemID] = i
		rankProblems[i] = RankProblem{ProblemID: p.ProblemID, DisplayID: p.DisplayID, Title: p.ProblemTitle}
	}
	states := make(map[int64]*rankState, len(participants))
	order := make([]int64, 0, len(participants))
	registerAt := make(map[int64]time.Time, len(participants))
	for _, p := range participants {
		cells := make([]RankCell, len(problems))
		states[p.UserID] = &rankState{row: RankRow{
			UserID: p.UserID, Username: p.Username, Nickname: p.Nickname,
			EquippedBadges: []gamification.BadgeSummary{}, Problems: cells,
		}}
		registerAt[p.UserID] = p.RegisterTime
		order = append(order, p.UserID)
	}

	bySubmission := make(map[int64][]RankSubmission)
	for _, sub := range submissions {
		if _, ok := states[sub.UserID]; !ok {
			continue
		}
		if sub.CreatedAt.Before(c.StartTime) || !sub.CreatedAt.Before(c.EndTime) {
			continue
		}
		if at, ok := registerAt[sub.UserID]; ok && sub.CreatedAt.Before(at) {
			continue
		}
		bySubmission[sub.UserID] = append(bySubmission[sub.UserID], sub)
	}

	if c.Mode == ModeICPC {
		s.computeICPC(states, bySubmission, problemIndex, c.StartTime)
	} else {
		s.computeScoreBased(states, bySubmission, problemIndex)
	}

	rows := make([]RankRow, 0, len(order))
	for _, userID := range order {
		state := states[userID]
		row := state.row
		row.Problems = append([]RankCell{}, row.Problems...)
		rows = append(rows, row)
	}
	markFirstBlood(rows)
	sortRankRows(rows, c.Mode)
	assignRanks(rows, c.Mode)

	selfOnly := c.Mode == ModeOI && status == StatusRunning && !v.CanViewHidden
	if selfOnly {
		filtered := rows[:0]
		for _, row := range rows {
			if row.UserID == v.UserID {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	return &Ranklist{
		ContestID: id, Mode: c.Mode, Status: status, GeneratedAt: now,
		Problems: rankProblems, Rows: rows, SelfOnly: selfOnly,
	}, nil
}

func (s *Service) computeICPC(
	states map[int64]*rankState,
	submissions map[int64][]RankSubmission,
	problemIndex map[int64]int,
	start time.Time,
) {
	for userID, subs := range submissions {
		state := states[userID]
		for _, sub := range subs {
			if state.row.LastSubmissionAt == nil || sub.CreatedAt.After(*state.row.LastSubmissionAt) {
				at := sub.CreatedAt
				state.row.LastSubmissionAt = &at
			}
			idx, ok := problemIndex[sub.ProblemID]
			if !ok {
				continue
			}
			cell := &state.row.Problems[idx]
			if cell.Status == "accepted" {
				continue
			}
			if sub.Status == "accepted" {
				minutes := int(sub.CreatedAt.Sub(start).Minutes())
				if minutes < 0 {
					minutes = 0
				}
				state.row.Solved++
				state.row.Penalty += minutes + 20*cell.Attempts
				cell.Status = "accepted"
				at := sub.CreatedAt
				cell.AcceptedAt = &at
				if state.row.LastAcceptedAt == nil || at.After(*state.row.LastAcceptedAt) {
					state.row.LastAcceptedAt = &at
				}
			} else {
				cell.Attempts++
				cell.Status = sub.Status
			}
		}
	}
}

func (s *Service) computeScoreBased(
	states map[int64]*rankState,
	submissions map[int64][]RankSubmission,
	problemIndex map[int64]int,
) {
	for userID, subs := range submissions {
		state := states[userID]
		for _, sub := range subs {
			idx, ok := problemIndex[sub.ProblemID]
			if !ok {
				continue
			}
			cell := &state.row.Problems[idx]
			cell.Attempts++
			if state.row.LastSubmissionAt == nil || sub.CreatedAt.After(*state.row.LastSubmissionAt) {
				at := sub.CreatedAt
				state.row.LastSubmissionAt = &at
			}
			if sub.Score > cell.Score {
				cell.Score = sub.Score
			}
			if sub.Status == "accepted" {
				cell.Status = "accepted"
				at := sub.CreatedAt
				if cell.AcceptedAt == nil || sub.CreatedAt.Before(*cell.AcceptedAt) {
					cell.AcceptedAt = &at
				}
				if state.row.LastAcceptedAt == nil || sub.CreatedAt.After(*state.row.LastAcceptedAt) {
					state.row.LastAcceptedAt = &at
				}
			} else if cell.Status != "accepted" {
				cell.Status = sub.Status
			}
		}
		for _, cell := range state.row.Problems {
			state.row.TotalScore += cell.Score
			if cell.Status == "accepted" {
				state.row.Solved++
			}
		}
	}
}

func markFirstBlood(rows []RankRow) {
	if len(rows) == 0 {
		return
	}
	for idx := range rows[0].Problems {
		var winner *RankCell
		var winnerID int64
		for i := range rows {
			cell := &rows[i].Problems[idx]
			if cell.Status != "accepted" || cell.AcceptedAt == nil {
				continue
			}
			if winner == nil || cell.AcceptedAt.Before(*winner.AcceptedAt) ||
				(cell.AcceptedAt.Equal(*winner.AcceptedAt) && rows[i].UserID < winnerID) {
				winner, winnerID = cell, rows[i].UserID
			}
		}
		if winner != nil {
			winner.FirstBlood = true
		}
	}
}

func sortRankRows(rows []RankRow, mode Mode) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if mode == ModeICPC {
			if a.Solved != b.Solved {
				return a.Solved > b.Solved
			}
			if a.Penalty != b.Penalty {
				return a.Penalty < b.Penalty
			}
			return earlier(a.LastAcceptedAt, b.LastAcceptedAt, a.UserID, b.UserID)
		}
		if a.TotalScore != b.TotalScore {
			return a.TotalScore > b.TotalScore
		}
		if !sameTime(a.LastSubmissionAt, b.LastSubmissionAt) {
			return earlier(a.LastSubmissionAt, b.LastSubmissionAt, a.UserID, b.UserID)
		}
		return a.UserID < b.UserID
	})
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func earlier(a, b *time.Time, aID, bID int64) bool {
	if a == nil && b == nil {
		return aID < bID
	}
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	if !a.Equal(*b) {
		return a.Before(*b)
	}
	return aID < bID
}

func assignRanks(rows []RankRow, mode Mode) {
	for i := range rows {
		if i == 0 {
			rows[i].Rank = 1
			continue
		}
		prev, cur := rows[i-1], rows[i]
		same := false
		if mode == ModeICPC {
			same = prev.Solved == cur.Solved && prev.Penalty == cur.Penalty
		} else {
			same = prev.TotalScore == cur.TotalScore
		}
		if same {
			cur.Rank = prev.Rank
			rows[i].Rank = cur.Rank
		} else {
			rows[i].Rank = i + 1
		}
	}
}
