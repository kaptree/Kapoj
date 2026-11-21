package problem

import (
	"context"
	"sort"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/ratinglevel"
)

// StatusCount is one slice of a problem's visible submission distribution.
type StatusCount struct {
	Status     string  `json:"status"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// SubmissionMetric is one row of the public submission statistics table. It
// intentionally has no submission ID and no code fields.
type SubmissionMetric struct {
	UserID         int64                       `json:"userId"`
	Username       string                      `json:"username"`
	Nickname       string                      `json:"nickname"`
	AvatarURL      string                      `json:"avatarUrl"`
	EquippedBadges []gamification.BadgeSummary `json:"equippedBadges"`
	EquippedTitle  *gamification.TitleSummary  `json:"equippedTitle"`
	Level          ratinglevel.Level           `json:"level"`
	Language       string                      `json:"language"`
	Status         string                      `json:"status"`
	TimeUsedMS     int                         `json:"timeUsedMs"`
	MemoryUsedKB   int                         `json:"memoryUsedKb"`
	CodeLength     int                         `json:"codeLength"`
	CreatedAt      time.Time                   `json:"createdAt"`
}

// SubmissionMetricRow is the repository row before public identity decoration.
type SubmissionMetricRow struct {
	UserID       int64     `gorm:"column:user_id"`
	Username     string    `gorm:"column:username"`
	Nickname     string    `gorm:"column:nickname"`
	AvatarURL    string    `gorm:"column:avatar_url"`
	Language     string    `gorm:"column:language"`
	Status       string    `gorm:"column:status"`
	TimeUsedMS   int       `gorm:"column:time_used_ms"`
	MemoryUsedKB int       `gorm:"column:memory_used_kb"`
	CodeLength   int       `gorm:"column:code_length"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

// SubmissionVisibility carries the viewer-dependent contest protection state.
type SubmissionVisibility struct {
	ViewerID           int64
	CanViewLiveContest bool
	Now                time.Time
}

// SubmissionMetricsFilter is a validated, whitelisted metric query.
type SubmissionMetricsFilter struct {
	SubmissionVisibility
	Sort   string
	Order  string
	Limit  int
	Offset int
}

// SubmissionStatsQuery is the user-facing query for
// GET /problems/:id/submission-stats.
type SubmissionStatsQuery struct {
	Sort     string
	Order    string
	Page     int
	PageSize int
}

var submissionMetricSortColumns = map[string]string{
	"time":        "s.time_used_ms",
	"memory":      "s.memory_used_kb",
	"code_length": "s.code_length",
	"created_at":  "s.created_at",
}

// Normalize applies defaults and rejects unknown sort/order values. Only
// whitelisted column names ever reach SQL.
func (q SubmissionStatsQuery) Normalize() (SubmissionStatsQuery, error) {
	if q.Sort == "" {
		q.Sort = "created_at"
	}
	if _, ok := submissionMetricSortColumns[q.Sort]; !ok {
		return q, httpx.ErrValidation("sort 必须是 time、memory、code_length 或 created_at")
	}
	if q.Order == "" {
		q.Order = "desc"
	}
	if q.Order != "asc" && q.Order != "desc" {
		return q, httpx.ErrValidation("order 必须是 asc 或 desc")
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	return q, nil
}

// SubmissionStats returns the visible submission metrics for a problem.
func (s *Service) SubmissionStats(
	ctx context.Context, v Viewer, id int64, q SubmissionStatsQuery,
) (httpx.Page[SubmissionMetric], error) {
	if _, err := s.Get(ctx, v, id); err != nil {
		return httpx.Page[SubmissionMetric]{}, err
	}
	q, err := q.Normalize()
	if err != nil {
		return httpx.Page[SubmissionMetric]{}, err
	}
	rows, total, err := s.repo.ListSubmissionMetrics(ctx, id, SubmissionMetricsFilter{
		SubmissionVisibility: SubmissionVisibility{
			ViewerID: v.UserID, CanViewLiveContest: v.CanViewLiveContest, Now: time.Now(),
		},
		Sort: q.Sort, Order: q.Order,
		Limit: q.PageSize, Offset: (q.Page - 1) * q.PageSize,
	})
	if err != nil {
		return httpx.Page[SubmissionMetric]{}, err
	}
	items := make([]SubmissionMetric, len(rows))
	for i, row := range rows {
		items[i] = SubmissionMetric{
			UserID: row.UserID, Username: row.Username, Nickname: row.Nickname,
			AvatarURL:      row.AvatarURL,
			EquippedBadges: []gamification.BadgeSummary{}, EquippedTitle: nil,
			Level:    ratinglevel.LevelFor(0),
			Language: row.Language, Status: row.Status,
			TimeUsedMS: row.TimeUsedMS, MemoryUsedKB: row.MemoryUsedKB,
			CodeLength: row.CodeLength, CreatedAt: row.CreatedAt,
		}
	}
	return httpx.Page[SubmissionMetric]{Total: total, Items: items}, nil
}

var statusDisplayOrder = []string{
	"accepted", "wrong_answer", "time_limit_exceeded", "memory_limit_exceeded",
	"runtime_error", "compile_error", "system_error", "judging", "pending",
}

func orderStatusCounts(rows []StatusCount) []StatusCount {
	rank := make(map[string]int, len(statusDisplayOrder))
	for i, status := range statusDisplayOrder {
		rank[status] = i
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, iok := rank[rows[i].Status]
		rj, jok := rank[rows[j].Status]
		if !iok {
			ri = len(statusDisplayOrder)
		}
		if !jok {
			rj = len(statusDisplayOrder)
		}
		return ri < rj
	})
	return rows
}

const visibleSubmissionClause = `
	(s.contest_id IS NULL OR s.user_id = ? OR ? OR c.id IS NULL
	 OR c.start_time IS NULL OR c.end_time IS NULL
	 OR NOT (c.start_time <= ? AND ? < c.end_time))`

func (r *gormRepository) StatusCounts(
	ctx context.Context, problemID int64, f SubmissionVisibility,
) ([]StatusCount, error) {
	rows := make([]StatusCount, 0)
	now := f.Now
	if now.IsZero() {
		now = time.Now()
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.status AS status, COUNT(*) AS count
		FROM submissions s
		LEFT JOIN contests c ON c.id = s.contest_id
		WHERE s.problem_id = ? AND s.agent_id IS NULL AND `+visibleSubmissionClause+`
		GROUP BY s.status`,
		problemID, f.ViewerID, f.CanViewLiveContest, now, now,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return orderStatusCounts(rows), nil
}

func (r *gormRepository) ListSubmissionMetrics(
	ctx context.Context, problemID int64, f SubmissionMetricsFilter,
) ([]SubmissionMetricRow, int64, error) {
	now := f.Now
	if now.IsZero() {
		now = time.Now()
	}
	from := `
		FROM submissions s
		LEFT JOIN contests c ON c.id = s.contest_id
		JOIN users u ON u.id = s.user_id
		WHERE s.problem_id = ? AND s.agent_id IS NULL AND ` + visibleSubmissionClause
	args := []any{problemID, f.ViewerID, f.CanViewLiveContest, now, now}

	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*)`+from, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	column, ok := submissionMetricSortColumns[f.Sort]
	if !ok {
		column = "s.created_at"
	}
	direction := "DESC"
	if f.Order == "asc" {
		direction = "ASC"
	}
	rows := make([]SubmissionMetricRow, 0)
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.user_id, u.username, u.nickname, u.avatar_url, s.language, s.status,
		       s.time_used_ms, s.memory_used_kb, s.code_length, s.created_at`+from+
		` ORDER BY `+column+` `+direction+`, s.id `+direction+` LIMIT ? OFFSET ?`,
		append(args, f.Limit, f.Offset)...,
	).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
