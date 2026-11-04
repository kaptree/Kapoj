package submission

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

var ErrNotFound = errors.New("submission not found")

// ListRow is a submission joined with its user and problem.
type ListRow struct {
	Submission
	Username         string
	Nickname         string
	ProblemTitle     string     `gorm:"column:problem_title"`
	ProblemSlug      string     `gorm:"column:problem_slug"`
	ProblemIsPublic  bool       `gorm:"column:problem_is_public"`
	ContestStartTime *time.Time `gorm:"column:contest_start_time"`
	ContestEndTime   *time.Time `gorm:"column:contest_end_time"`
}

type DetailRow = ListRow

type Repository interface {
	Create(ctx context.Context, sub *Submission) error
	FindByID(ctx context.Context, id int64) (*Submission, error)
	MarkSystemError(ctx context.Context, id int64, message string) error
	List(ctx context.Context, f ListFilter, offset, limit int) ([]ListRow, int64, error)
	FindDetail(ctx context.Context, id int64) (*DetailRow, error)
	HasAccepted(ctx context.Context, userID, problemID int64) (bool, error)
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, sub *Submission) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&problem.Problem{}).Where("id = ?", sub.ProblemID).
			UpdateColumn("submit_count", gorm.Expr("submit_count + 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return problem.ErrNotFound
		}
		return tx.Create(sub).Error
	})
	return mapErr(err)
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*Submission, error) {
	var sub Submission
	if err := r.db.WithContext(ctx).First(&sub, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

// MarkSystemError moves an open submission to system_error. Terminal
// submissions are left untouched so a late retry cannot rewrite a verdict.
func (r *gormRepository) MarkSystemError(ctx context.Context, id int64, message string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&Submission{}).
		Where("id = ? AND status IN ?", id, []Status{StatusPending, StatusJudging}).
		Updates(map[string]any{
			"status":        StatusSystemError,
			"error_message": message,
			"judged_at":     now,
		}).Error
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *gormRepository) List(ctx context.Context, f ListFilter, offset, limit int) ([]ListRow, int64, error) {
	q := r.db.WithContext(ctx).Table("submissions AS s").
		Joins("JOIN users AS u ON u.id = s.user_id").
		Joins("JOIN problems AS p ON p.id = s.problem_id").
		Joins("LEFT JOIN contests AS c ON c.id = s.contest_id")
	if f.ProblemID != 0 {
		q = q.Where("s.problem_id = ?", f.ProblemID)
	}
	if f.ContestID != nil {
		if *f.ContestID == 0 {
			q = q.Where("s.contest_id IS NULL")
		} else {
			q = q.Where("s.contest_id = ?", *f.ContestID)
		}
	}
	if f.UserID != 0 {
		q = q.Where("s.user_id = ?", f.UserID)
	}
	if f.Language != "" {
		q = q.Where("s.language = ?", f.Language)
	}
	if f.Status != "" {
		q = q.Where("s.status = ?", f.Status)
	}
	if f.Nickname != "" {
		q = q.Where(`u.nickname ILIKE ? ESCAPE '\'`, "%"+escapeLike(f.Nickname)+"%")
	}
	if !f.CanSeeHidden {
		if f.ViewerID != 0 {
			q = q.Where("(p.is_public = true OR s.user_id = ?)", f.ViewerID)
		} else {
			q = q.Where("p.is_public = true")
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ListRow
	err := q.Select(`s.id, s.user_id, s.problem_id, s.contest_id, s.language, s.status,
		s.time_used_ms, s.memory_used_kb, s.score, s.created_at, s.judged_at,
		u.username, u.nickname, p.title AS problem_title, p.slug AS problem_slug,
		p.is_public AS problem_is_public,
		c.start_time AS contest_start_time, c.end_time AS contest_end_time`).
		Order("s.id DESC").Offset(offset).Limit(limit).Scan(&rows).Error
	return rows, total, err
}

func (r *gormRepository) FindDetail(ctx context.Context, id int64) (*DetailRow, error) {
	var row DetailRow
	err := r.db.WithContext(ctx).Table("submissions AS s").
		Select(`s.*, u.username, u.nickname, p.title AS problem_title, p.slug AS problem_slug,
			p.is_public AS problem_is_public,
			c.start_time AS contest_start_time, c.end_time AS contest_end_time`).
		Joins("JOIN users AS u ON u.id = s.user_id").
		Joins("JOIN problems AS p ON p.id = s.problem_id").
		Joins("LEFT JOIN contests AS c ON c.id = s.contest_id").
		Where("s.id = ?", id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *gormRepository) HasAccepted(ctx context.Context, userID, problemID int64) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(
		`SELECT EXISTS(SELECT 1 FROM submissions WHERE user_id = ? AND problem_id = ? AND status = 'accepted')`,
		userID, problemID,
	).Scan(&ok).Error
	return ok, err
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return problem.ErrNotFound
	}
	return err
}
