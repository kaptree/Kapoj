package similarity

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// ErrNotFound is returned when a requested similarity row does not exist.
var ErrNotFound = errors.New("similarity record not found")

// SubmissionRow is a participant code submission inside the contest window.
type SubmissionRow struct {
	ID        int64
	UserID    int64
	Username  string
	Nickname  string
	Language  submission.Language
	Code      string
	CreatedAt time.Time
}

// MatchFilter selects a page of matches.
type MatchFilter struct {
	ContestID int64
	ProblemID int64
	RunID     int64
	Page      int
	PageSize  int
}

// MatchRow is a match joined with its run and both users' identities.
type MatchRow struct {
	Match
	ThresholdPercent int        `gorm:"column:threshold_percent"`
	FinishedAt       *time.Time `gorm:"column:finished_at"`
	OriginalUsername string
	OriginalNickname string
	SuspectUsername  string
	SuspectNickname  string
}

// MatchDetailRow is a match joined with its run, both submissions and both
// users.
type MatchDetailRow struct {
	Match
	ThresholdPercent int        `gorm:"column:threshold_percent"`
	FinishedAt       *time.Time `gorm:"column:finished_at"`
	OriginalCode     string
	SuspectCode      string
	OriginalStatus   submission.Status
	SuspectStatus    submission.Status
	OriginalScore    int
	SuspectScore     int
	OriginalUsername string
	OriginalNickname string
	SuspectUsername  string
	SuspectNickname  string
}

// Repository persists similarity settings, runs and matches.
type Repository interface {
	GetSettings(ctx context.Context, contestID, problemID int64) (*Settings, error)
	ListSettings(ctx context.Context, contestID int64) ([]Settings, error)
	UpsertSettings(ctx context.Context, s *Settings) error
	CreateRun(ctx context.Context, r *Run) error
	FindRun(ctx context.Context, id int64) (*Run, error)
	FindActiveRun(ctx context.Context, contestID, problemID int64) (*Run, error)
	UpdateRun(ctx context.Context, r *Run) error
	ReplaceMatches(ctx context.Context, runID int64, matches []Match) error
	LatestCompletedRuns(ctx context.Context, contestID int64) (map[int64]*Run, error)
	LatestRuns(ctx context.Context, contestID int64) (map[int64]*Run, error)
	ListMatches(ctx context.Context, f MatchFilter) ([]MatchRow, int64, error)
	FindMatch(ctx context.Context, id int64) (*MatchDetailRow, error)
	ListProblemSubmissions(ctx context.Context, contestID, problemID int64) ([]SubmissionRow, error)
	MarkStaleRunsFailed(ctx context.Context, before time.Time) error
}

type gormRepository struct{ db *gorm.DB }

// NewGormRepository returns the GORM-backed Repository.
func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

func (r *gormRepository) GetSettings(ctx context.Context, contestID, problemID int64) (*Settings, error) {
	var s Settings
	err := r.db.WithContext(ctx).
		Where("contest_id = ? AND problem_id = ?", contestID, problemID).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *gormRepository) ListSettings(ctx context.Context, contestID int64) ([]Settings, error) {
	items := make([]Settings, 0)
	err := r.db.WithContext(ctx).
		Where("contest_id = ?", contestID).
		Order("problem_id, id").
		Find(&items).Error
	return items, err
}

// UpsertSettings inserts or updates the settings row keyed by
// (contest_id, problem_id). A map is used so an explicit IgnoreCommonCode=false
// is written verbatim; GORM would otherwise substitute the parsed default true
// for a zero bool.
func (r *gormRepository) UpsertSettings(ctx context.Context, s *Settings) error {
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = time.Now()
	}
	values := map[string]any{
		"contest_id":         s.ContestID,
		"problem_id":         s.ProblemID,
		"threshold_percent":  s.ThresholdPercent,
		"ignore_common_code": s.IgnoreCommonCode,
		"base_language":      s.BaseLanguage,
		"base_code":          s.BaseCode,
		"updated_by":         s.UpdatedBy,
		"updated_at":         s.UpdatedAt,
	}
	err := r.db.WithContext(ctx).Model(&Settings{}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "contest_id"}, {Name: "problem_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"threshold_percent", "ignore_common_code", "base_language", "base_code", "updated_by", "updated_at",
		}),
	}).Create(values).Error
	if err != nil {
		return err
	}
	if s.ID == 0 {
		var existing Settings
		if err := r.db.WithContext(ctx).Select("id").
			Where("contest_id = ? AND problem_id = ?", s.ContestID, s.ProblemID).
			First(&existing).Error; err != nil {
			return err
		}
		s.ID = existing.ID
	}
	return nil
}

func (r *gormRepository) CreateRun(ctx context.Context, run *Run) error {
	ignoreCommonCode := run.IgnoreCommonCode
	if err := r.db.WithContext(ctx).Create(run).Error; err != nil {
		return err
	}
	// GORM substitutes the parsed default (true) for a zero bool; write the
	// caller's explicit false value back.
	if !ignoreCommonCode {
		if err := r.db.WithContext(ctx).Model(&Run{}).
			Where("id = ?", run.ID).UpdateColumn("ignore_common_code", false).Error; err != nil {
			return err
		}
		run.IgnoreCommonCode = false
	}
	return nil
}

func (r *gormRepository) FindRun(ctx context.Context, id int64) (*Run, error) {
	var run Run
	if err := r.db.WithContext(ctx).First(&run, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &run, nil
}

// FindActiveRun returns the most recent queued or running run for a problem,
// or nil when none exists.
func (r *gormRepository) FindActiveRun(ctx context.Context, contestID, problemID int64) (*Run, error) {
	var run Run
	err := r.db.WithContext(ctx).
		Where("contest_id = ? AND problem_id = ? AND status IN ?",
			contestID, problemID, []string{RunStatusQueued, RunStatusRunning}).
		Order("id DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// UpdateRun writes the full row; callers must pass a populated Run (for
// example one loaded via FindRun).
func (r *gormRepository) UpdateRun(ctx context.Context, run *Run) error {
	return r.db.WithContext(ctx).Save(run).Error
}

// ReplaceMatches replaces every match of a run with the provided rows inside a
// single transaction, so a re-run never trips the unique pair constraint.
func (r *gormRepository) ReplaceMatches(ctx context.Context, runID int64, matches []Match) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("run_id = ?", runID).Delete(&Match{}).Error; err != nil {
			return err
		}
		if len(matches) == 0 {
			return nil
		}
		for i := range matches {
			matches[i].RunID = runID
		}
		return tx.Create(&matches).Error
	})
}

// LatestCompletedRuns returns the most recent completed run per problem.
func (r *gormRepository) LatestCompletedRuns(ctx context.Context, contestID int64) (map[int64]*Run, error) {
	var runs []Run
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (problem_id) *
		FROM contest_similarity_runs
		WHERE contest_id = ? AND status = ?
		ORDER BY problem_id, id DESC`, contestID, RunStatusCompleted).Scan(&runs).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*Run, len(runs))
	for i := range runs {
		out[runs[i].ProblemID] = &runs[i]
	}
	return out, nil
}

// LatestRuns returns the most recent run per problem of any status.
func (r *gormRepository) LatestRuns(ctx context.Context, contestID int64) (map[int64]*Run, error) {
	var runs []Run
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (problem_id) *
		FROM contest_similarity_runs
		WHERE contest_id = ?
		ORDER BY problem_id, id DESC`, contestID).Scan(&runs).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*Run, len(runs))
	for i := range runs {
		out[runs[i].ProblemID] = &runs[i]
	}
	return out, nil
}

// ListMatches returns a page of matches. Unless an explicit RunID is given,
// only matches from the latest completed run of each problem are included.
func (r *gormRepository) ListMatches(ctx context.Context, f MatchFilter) ([]MatchRow, int64, error) {
	q := r.db.WithContext(ctx).
		Table("contest_similarity_matches AS m").
		Joins("JOIN contest_similarity_runs AS r ON r.id = m.run_id").
		Joins("JOIN users AS ou ON ou.id = m.original_user_id").
		Joins("JOIN users AS su ON su.id = m.suspect_user_id").
		Where("m.contest_id = ?", f.ContestID)

	if f.RunID != 0 {
		q = q.Where("m.run_id = ?", f.RunID)
	} else {
		latest := r.db.WithContext(ctx).
			Table("contest_similarity_runs").
			Select("DISTINCT ON (problem_id) id").
			Where("contest_id = ? AND status = ?", f.ContestID, RunStatusCompleted).
			Order("problem_id, id DESC")
		q = q.Where("m.run_id IN (?)", latest)
	}
	if f.ProblemID != 0 {
		q = q.Where("m.problem_id = ?", f.ProblemID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, size := normalizePage(f.Page, f.PageSize)
	rows := make([]MatchRow, 0)
	err := q.Select(`m.*, r.threshold_percent, r.finished_at,
		ou.username AS original_username, ou.nickname AS original_nickname,
		su.username AS suspect_username, su.nickname AS suspect_nickname`).
		Order("m.similarity_percent DESC, m.id").
		Offset((page - 1) * size).Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

// FindMatch returns a match joined with its run, both submissions' code/status
// and both users' identities.
func (r *gormRepository) FindMatch(ctx context.Context, id int64) (*MatchDetailRow, error) {
	var row MatchDetailRow
	err := r.db.WithContext(ctx).
		Table("contest_similarity_matches AS m").
		Select(`m.*,
			r.threshold_percent, r.finished_at,
			os.code AS original_code, os.status AS original_status, os.score AS original_score,
			ss.code AS suspect_code, ss.status AS suspect_status, ss.score AS suspect_score,
			ou.username AS original_username, ou.nickname AS original_nickname,
			su.username AS suspect_username, su.nickname AS suspect_nickname`).
		Joins("JOIN contest_similarity_runs AS r ON r.id = m.run_id").
		Joins("JOIN submissions AS os ON os.id = m.original_submission_id").
		Joins("JOIN submissions AS ss ON ss.id = m.suspect_submission_id").
		Joins("JOIN users AS ou ON ou.id = m.original_user_id").
		Joins("JOIN users AS su ON su.id = m.suspect_user_id").
		Where("m.id = ?", id).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// ListProblemSubmissions returns participant code submissions for one problem
// inside the contest time window, excluding objective answer submissions.
func (r *gormRepository) ListProblemSubmissions(ctx context.Context, contestID, problemID int64) ([]SubmissionRow, error) {
	rows := make([]SubmissionRow, 0)
	err := r.db.WithContext(ctx).
		Table("submissions AS s").
		Select("s.id, s.user_id, s.language, s.code, s.created_at, u.username, u.nickname").
		Joins("JOIN users AS u ON u.id = s.user_id").
		Joins("JOIN contest_participants AS cp ON cp.contest_id = s.contest_id AND cp.user_id = s.user_id").
		Joins("JOIN contests AS c ON c.id = s.contest_id").
		Where("s.contest_id = ? AND s.problem_id = ?", contestID, problemID).
		Where("s.language <> ?", submission.LanguageAnswer).
		Where("s.created_at >= c.start_time AND s.created_at < c.end_time").
		Order("s.created_at, s.id").
		Scan(&rows).Error
	return rows, err
}

// MarkStaleRunsFailed fails running runs whose started_at precedes before.
func (r *gormRepository) MarkStaleRunsFailed(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Model(&Run{}).
		Where("status = ? AND started_at < ?", RunStatusRunning, before).
		Updates(map[string]any{
			"status":        RunStatusFailed,
			"error_message": "任务超时",
		}).Error
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return page, size
}
