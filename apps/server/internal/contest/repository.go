package contest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNotFound             = errors.New("contest not found")
	ErrAnnouncementNotFound = errors.New("announcement not found")
	ErrDuplicateProblem     = errors.New("problem already in contest")
	ErrDuplicateDisplay     = errors.New("display id already used")
	ErrProblemNotFound      = errors.New("problem not found")
	ErrProblemNotInContest  = errors.New("problem is not in this contest")
	ErrHasSubmissions       = errors.New("contest has submissions")
	ErrStartFrozen          = errors.New("contest start time is frozen")
	ErrModeFrozen           = errors.New("contest mode is frozen")
	ErrProblemsFrozen       = errors.New("contest problems are frozen")
	ErrRatedFrozen          = errors.New("contest rated flag is frozen")
)

type ListFilter struct {
	Keyword           string
	Mode              Mode
	Status            Status
	IncludeHidden     bool
	Now               time.Time
	ViewerID          int64
	ParticipantUserID int64
	ProblemID         int64
}

type ProblemRow struct {
	ContestProblem
	ProblemTitle         string `gorm:"column:problem_title"`
	ProblemSlug          string `gorm:"column:problem_slug"`
	ProblemType          string `gorm:"column:problem_type"`
	ProblemTimeLimitMS   int    `gorm:"column:problem_time_limit_ms"`
	ProblemMemoryLimitMB int    `gorm:"column:problem_memory_limit_mb"`
	ProblemIsPublic      bool   `gorm:"column:problem_is_public"`
}

// ProblemStat 是单场比赛内某题的赛后聚合统计。
type ProblemStat struct {
	SubmitCount int `gorm:"column:submit_count"`
	AcceptCount int `gorm:"column:accept_count"`
}

// UserProblemStat 是当前用户在单场比赛内某题的个人统计。
type UserProblemStat struct {
	Attempts   int
	Accepted   bool
	LastStatus string
}

type ListRow struct {
	Contest
	ProblemCount     int64 `gorm:"column:problem_count"`
	ParticipantCount int64 `gorm:"column:participant_count"`
	Registered       bool  `gorm:"column:registered"`
}

type ParticipantRow struct {
	UserID       int64
	Username     string
	Nickname     string
	RegisterTime time.Time
}

type Repository interface {
	Create(ctx context.Context, c *Contest, problems []ContestProblem) error
	Update(ctx context.Context, c *Contest, problems []ContestProblem) error
	FindByID(ctx context.Context, id int64) (*Contest, error)
	FindProblems(ctx context.Context, contestID int64) ([]ProblemRow, error)
	FindProblem(ctx context.Context, contestID, problemID int64) (*ContestProblem, error)
	HasSubmissions(ctx context.Context, contestID int64) (bool, error)
	CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
	IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)
	LockedProblemIDs(ctx context.Context, problemIDs []int64, now time.Time) (map[int64]bool, error)
	AIBlockedForUser(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
	RankSubmissions(ctx context.Context, contestID int64) ([]RankSubmission, error)
	ProblemStats(ctx context.Context, contestID int64) (map[int64]ProblemStat, error)
	UserProblemStats(ctx context.Context, contestID, userID int64) (map[int64]UserProblemStat, error)
	List(ctx context.Context, f ListFilter, offset, limit int) ([]ListRow, int64, error)
	Delete(ctx context.Context, id int64) error

	IsParticipant(ctx context.Context, contestID, userID int64) (bool, error)
	AddParticipant(ctx context.Context, contestID, userID int64) error
	ListParticipants(ctx context.Context, contestID int64) ([]ParticipantRow, error)
	CountParticipants(ctx context.Context, contestID int64) (int64, error)

	AddAnnouncement(ctx context.Context, a *Announcement) error
	ListAnnouncements(ctx context.Context, contestID int64) ([]Announcement, error)
	FindAnnouncement(ctx context.Context, contestID, id int64) (*Announcement, error)
	UpdateAnnouncement(ctx context.Context, contestID, id int64, title, content string) error
	DeleteAnnouncement(ctx context.Context, contestID, id int64) error
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, c *Contest, problems []ContestProblem) error {
	disableAI := c.DisableAI
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		if !disableAI {
			// GORM 对带 default 标签的字段会用默认值替换零值，导致显式传入的
			// false 被写成 true；这里显式写回调用方请求的值。
			if err := tx.Model(&Contest{}).Where("id = ?", c.ID).
				UpdateColumn("disable_ai", false).Error; err != nil {
				return err
			}
			c.DisableAI = false
		}
		for i := range problems {
			problems[i].ContestID = c.ID
			problems[i].SortOrder = i + 1
		}
		if len(problems) > 0 {
			if err := tx.Omit("Problem").Create(&problems).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return mapErr(err)
}

func (r *gormRepository) Update(ctx context.Context, c *Contest, problems []ContestProblem) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Contest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, c.ID).Error; err != nil {
			return err
		}
		if !time.Now().Before(existing.StartTime) {
			if !c.StartTime.Truncate(time.Second).Equal(existing.StartTime.Truncate(time.Second)) {
				return ErrStartFrozen
			}
			if c.Mode != existing.Mode {
				return ErrModeFrozen
			}
			if c.IsRated != existing.IsRated {
				return ErrRatedFrozen
			}
			var current []ContestProblem
			if err := tx.Where("contest_id = ?", c.ID).Order("sort_order, id").Find(&current).Error; err != nil {
				return err
			}
			if !sameContestProblems(current, problems) {
				return ErrProblemsFrozen
			}
			c.StartTime = existing.StartTime
		}
		// Edits never change the settlement marker; a concurrent settlement
		// between the service read and this row lock must not be erased.
		c.RatingSettledAt = existing.RatingSettledAt
		if err := tx.Save(c).Error; err != nil {
			return err
		}
		if err := tx.Where("contest_id = ?", c.ID).Delete(&ContestProblem{}).Error; err != nil {
			return err
		}
		for i := range problems {
			problems[i].ContestID = c.ID
			problems[i].SortOrder = i + 1
		}
		if len(problems) > 0 {
			if err := tx.Omit("Problem").Create(&problems).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return mapErr(err)
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*Contest, error) {
	var c Contest
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, mapErr(err)
	}
	return &c, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *gormRepository) List(ctx context.Context, f ListFilter, offset, limit int) ([]ListRow, int64, error) {
	q := r.db.WithContext(ctx).Model(&Contest{})
	if f.ProblemID != 0 {
		// 关联比赛筛选：先限定包含该题的比赛，再按“公开 OR 本人已报名 OR
		// view_hidden/super”联合可见性，不复用 mine=true 的互斥语义。
		q = q.Where(`EXISTS (
			SELECT 1 FROM contest_problems cpb
			WHERE cpb.contest_id = contests.id AND cpb.problem_id = ?
		)`, f.ProblemID)
		if !f.IncludeHidden {
			if f.ViewerID != 0 {
				q = q.Where(`(is_public = true OR EXISTS (
					SELECT 1 FROM contest_participants cpc
					WHERE cpc.contest_id = contests.id AND cpc.user_id = ?
				))`, f.ViewerID)
			} else {
				q = q.Where("is_public = true")
			}
		}
	} else if f.ParticipantUserID != 0 {
		q = q.Where(`EXISTS (
			SELECT 1 FROM contest_participants cp
			WHERE cp.contest_id = contests.id AND cp.user_id = ?
		)`, f.ParticipantUserID)
	} else if !f.IncludeHidden {
		q = q.Where("is_public = true")
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		q = q.Where(`title ILIKE ? ESCAPE '\'`, "%"+escapeLike(kw)+"%")
	}
	if f.Mode != "" {
		q = q.Where("mode = ?", f.Mode)
	}
	now := f.Now
	if now.IsZero() {
		now = time.Now()
	}
	switch f.Status {
	case StatusUpcoming:
		q = q.Where("start_time > ?", now)
	case StatusRunning:
		q = q.Where("start_time <= ? AND end_time > ?", now, now)
	case StatusEnded:
		q = q.Where("end_time <= ?", now)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ListRow
	registered := "false"
	args := []any{}
	if f.ViewerID != 0 {
		registered = `EXISTS (
			SELECT 1 FROM contest_participants cpv
			WHERE cpv.contest_id = contests.id AND cpv.user_id = ?
		)`
		args = append(args, f.ViewerID)
	}
	selectSQL := fmt.Sprintf(`contests.*,
		(SELECT COUNT(*) FROM contest_problems cp WHERE cp.contest_id = contests.id) AS problem_count,
		(SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = contests.id) AS participant_count,
		%s AS registered`, registered)
	err := q.Select(selectSQL, args...).
		Order("start_time DESC, id DESC").Offset(offset).Limit(limit).Scan(&rows).Error
	return rows, total, err
}

func (r *gormRepository) FindProblems(ctx context.Context, contestID int64) ([]ProblemRow, error) {
	var rows []ProblemRow
	err := r.db.WithContext(ctx).Table("contest_problems AS cp").
		Select(`cp.*, p.title AS problem_title, p.slug AS problem_slug,
			p.type AS problem_type,
			p.time_limit_ms AS problem_time_limit_ms,
			p.memory_limit_mb AS problem_memory_limit_mb,
			p.is_public AS problem_is_public`).
		Joins("JOIN problems AS p ON p.id = cp.problem_id").
		Where("cp.contest_id = ?", contestID).
		Order("cp.sort_order, cp.id").Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) FindProblem(ctx context.Context, contestID, problemID int64) (*ContestProblem, error) {
	var cp ContestProblem
	if err := r.db.WithContext(ctx).
		Where("contest_id = ? AND problem_id = ?", contestID, problemID).First(&cp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProblemNotInContest
		}
		return nil, err
	}
	return &cp, nil
}

func (r *gormRepository) HasSubmissions(ctx context.Context, contestID int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("submissions").Where("contest_id = ?", contestID).Count(&n).Error
	return n > 0, err
}

// CanViewProblem reports whether userID may open a hidden problem because they
// are registered in a contest that contains it and is currently running.
func (r *gormRepository) CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM contest_participants cp
			JOIN contest_problems cpb ON cpb.contest_id = cp.contest_id
			JOIN contests c ON c.id = cp.contest_id
			WHERE cp.user_id = ? AND cpb.problem_id = ?
			  AND c.start_time <= ? AND c.end_time > ?
		)`, userID, problemID, now, now).Scan(&ok).Error
	return ok, err
}

// IsProblemLocked reports whether a problem belongs to a contest that has not
// ended yet; upcoming and running contests both keep their problems locked.
func (r *gormRepository) IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error) {
	var locked bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM contest_problems cpb
			JOIN contests c ON c.id = cpb.contest_id
			WHERE cpb.problem_id = ? AND c.end_time > ?
		)`, problemID, now).Scan(&locked).Error
	return locked, err
}

// LockedProblemIDs returns the subset of problemIDs that still belong to a
// contest that has not ended yet.
func (r *gormRepository) LockedProblemIDs(ctx context.Context, problemIDs []int64, now time.Time) (map[int64]bool, error) {
	locked := make(map[int64]bool, len(problemIDs))
	if len(problemIDs) == 0 {
		return locked, nil
	}
	type problemIDRow struct {
		ProblemID int64 `gorm:"column:problem_id"`
	}
	var rows []problemIDRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT cp.problem_id
		FROM contest_problems cp
		JOIN contests c ON c.id = cp.contest_id
		WHERE cp.problem_id IN (?) AND c.end_time > ?`,
		problemIDs, now).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		locked[row.ProblemID] = true
	}
	return locked, nil
}

// AIBlockedForUser reports whether userID is participating in a running contest
// that contains the problem and has AI assistance disabled.
func (r *gormRepository) AIBlockedForUser(ctx context.Context, userID, problemID int64, now time.Time) (bool, error) {
	var blocked bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM contest_participants cp
			JOIN contest_problems cpb ON cpb.contest_id = cp.contest_id
			JOIN contests c ON c.id = cp.contest_id
			WHERE cp.user_id = ? AND cpb.problem_id = ?
			  AND c.disable_ai = true AND c.start_time <= ? AND c.end_time > ?
		)`, userID, problemID, now, now).Scan(&blocked).Error
	return blocked, err
}

func (r *gormRepository) RankSubmissions(ctx context.Context, contestID int64) ([]RankSubmission, error) {
	var rows []RankSubmission
	err := r.db.WithContext(ctx).Table("submissions").
		Select("user_id, problem_id, status, score, created_at").
		Where("contest_id = ? AND status NOT IN ('pending', 'judging')", contestID).
		Order("created_at, id").Scan(&rows).Error
	return rows, err
}

// ProblemStats aggregates judged submissions for every problem in a contest,
// using the same participant/register-time/time-window filter as the ranklist.
func (r *gormRepository) ProblemStats(ctx context.Context, contestID int64) (map[int64]ProblemStat, error) {
	var rows []struct {
		ProblemID   int64 `gorm:"column:problem_id"`
		SubmitCount int   `gorm:"column:submit_count"`
		AcceptCount int   `gorm:"column:accept_count"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.problem_id,
		       COUNT(*) AS submit_count,
		       COUNT(DISTINCT s.user_id) FILTER (WHERE s.status = 'accepted') AS accept_count
		FROM submissions s
		JOIN contest_participants p
		  ON p.contest_id = s.contest_id AND p.user_id = s.user_id
		JOIN contests c ON c.id = s.contest_id
		WHERE s.contest_id = ?
		  AND s.status NOT IN ('pending', 'judging')
		  AND s.created_at >= c.start_time
		  AND s.created_at < c.end_time
		  AND s.created_at >= p.register_time
		GROUP BY s.problem_id`, contestID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	stats := make(map[int64]ProblemStat, len(rows))
	for _, row := range rows {
		stats[row.ProblemID] = ProblemStat{
			SubmitCount: row.SubmitCount, AcceptCount: row.AcceptCount,
		}
	}
	return stats, nil
}

// UserProblemStats aggregates one user's judged submissions per problem in a
// contest, with the same filter as ProblemStats.
func (r *gormRepository) UserProblemStats(ctx context.Context, contestID, userID int64) (map[int64]UserProblemStat, error) {
	var rows []struct {
		ProblemID int64  `gorm:"column:problem_id"`
		Status    string `gorm:"column:status"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.problem_id, s.status
		FROM submissions s
		JOIN contest_participants p
		  ON p.contest_id = s.contest_id AND p.user_id = s.user_id
		JOIN contests c ON c.id = s.contest_id
		WHERE s.contest_id = ?
		  AND s.user_id = ?
		  AND s.status NOT IN ('pending', 'judging')
		  AND s.created_at >= c.start_time
		  AND s.created_at < c.end_time
		  AND s.created_at >= p.register_time
		ORDER BY s.created_at, s.id`, contestID, userID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	stats := make(map[int64]UserProblemStat)
	for _, row := range rows {
		stat := stats[row.ProblemID]
		stat.Attempts++
		if row.Status == "accepted" {
			stat.Accepted = true
		}
		stat.LastStatus = row.Status
		stats[row.ProblemID] = stat
	}
	return stats, nil
}

func (r *gormRepository) Delete(ctx context.Context, id int64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Contest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&current, id).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Table("submissions").Where("contest_id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrHasSubmissions
		}
		res := tx.Where("id = ?", id).Delete(&Contest{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	return mapErr(err)
}

func (r *gormRepository) IsParticipant(ctx context.Context, contestID, userID int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&ContestParticipant{}).
		Where("contest_id = ? AND user_id = ?", contestID, userID).Count(&n).Error
	return n > 0, err
}

func (r *gormRepository) AddParticipant(ctx context.Context, contestID, userID int64) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&ContestParticipant{ContestID: contestID, UserID: userID, RegisterTime: time.Now()}).
		Error
}

func (r *gormRepository) ListParticipants(ctx context.Context, contestID int64) ([]ParticipantRow, error) {
	var rows []ParticipantRow
	err := r.db.WithContext(ctx).Table("contest_participants AS cp").
		Select("cp.user_id, u.username, u.nickname, cp.register_time").
		Joins("JOIN users AS u ON u.id = cp.user_id").
		Where("cp.contest_id = ?", contestID).
		Order("cp.register_time, cp.id").Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) CountParticipants(ctx context.Context, contestID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&ContestParticipant{}).
		Where("contest_id = ?", contestID).Count(&n).Error
	return n, err
}

func (r *gormRepository) AddAnnouncement(ctx context.Context, a *Announcement) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormRepository) ListAnnouncements(ctx context.Context, contestID int64) ([]Announcement, error) {
	var items []Announcement
	err := r.db.WithContext(ctx).Where("contest_id = ?", contestID).
		Order("id DESC").Find(&items).Error
	return items, err
}

func (r *gormRepository) FindAnnouncement(ctx context.Context, contestID, id int64) (*Announcement, error) {
	var a Announcement
	if err := r.db.WithContext(ctx).Where("contest_id = ? AND id = ?", contestID, id).First(&a).Error; err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

func (r *gormRepository) UpdateAnnouncement(ctx context.Context, contestID, id int64, title, content string) error {
	res := r.db.WithContext(ctx).Model(&Announcement{}).
		Where("contest_id = ? AND id = ?", contestID, id).
		Updates(map[string]any{"title": title, "content_md": content})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAnnouncementNotFound
	}
	return nil
}

func (r *gormRepository) DeleteAnnouncement(ctx context.Context, contestID, id int64) error {
	res := r.db.WithContext(ctx).Where("contest_id = ? AND id = ?", contestID, id).Delete(&Announcement{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAnnouncementNotFound
	}
	return nil
}

func sameContestProblems(current, wanted []ContestProblem) bool {
	if len(current) != len(wanted) {
		return false
	}
	for i := range current {
		if current[i].ProblemID != wanted[i].ProblemID ||
			current[i].DisplayID != wanted[i].DisplayID ||
			current[i].Score != wanted[i].Score {
			return false
		}
	}
	return true
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_contest_problems_problem":
			return ErrDuplicateProblem
		case pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_contest_problems_display":
			return ErrDuplicateDisplay
		case pgErr.Code == "23503":
			return ErrProblemNotFound
		}
	}
	return err
}
