package contest

import (
	"context"
	"errors"
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
)

type ListFilter struct {
	Keyword       string
	Mode          Mode
	Status        Status
	IncludeHidden bool
	Now           time.Time
}

type ProblemRow struct {
	ContestProblem
	ProblemTitle string `gorm:"column:problem_title"`
	ProblemSlug  string `gorm:"column:problem_slug"`
}

type ListRow struct {
	Contest
	ProblemCount     int64 `gorm:"column:problem_count"`
	ParticipantCount int64 `gorm:"column:participant_count"`
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
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
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

func (r *gormRepository) Update(ctx context.Context, c *Contest, problems []ContestProblem) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Contest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, c.ID).Error; err != nil {
			return err
		}
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
	if !f.IncludeHidden {
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
	err := q.Select(`contests.*,
		(SELECT COUNT(*) FROM contest_problems cp WHERE cp.contest_id = contests.id) AS problem_count,
		(SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = contests.id) AS participant_count`).
		Order("start_time DESC, id DESC").Offset(offset).Limit(limit).Scan(&rows).Error
	return rows, total, err
}

func (r *gormRepository) FindProblems(ctx context.Context, contestID int64) ([]ProblemRow, error) {
	var rows []ProblemRow
	err := r.db.WithContext(ctx).Table("contest_problems AS cp").
		Select("cp.*, p.title AS problem_title, p.slug AS problem_slug").
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
// are registered in a contest that contains it and has already started.
func (r *gormRepository) CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM contest_participants cp
			JOIN contest_problems cpb ON cpb.contest_id = cp.contest_id
			JOIN contests c ON c.id = cp.contest_id
			WHERE cp.user_id = ? AND cpb.problem_id = ? AND c.start_time <= ?
		)`, userID, problemID, now).Scan(&ok).Error
	return ok, err
}

func (r *gormRepository) Delete(ctx context.Context, id int64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
