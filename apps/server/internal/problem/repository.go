package problem

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNotFound             = errors.New("problem not found")
	ErrSlugTaken            = errors.New("problem slug taken")
	ErrTestcaseNotFound     = errors.New("testcase not found")
	ErrBadOrder             = errors.New("reorder ids must be exactly the problem's testcases")
	ErrPublishNeedsTestcase = errors.New("a public problem needs at least one testcase")
	ErrPublishNeedsChecker  = errors.New("a public special problem needs a checker")
	ErrLastTestcase         = errors.New("a public problem cannot lose its last testcase")
	ErrInUse                = errors.New("problem is referenced by a contest")
)

type Filter struct {
	Keyword       string
	Tag           string
	Difficulty    int
	IncludeHidden bool
	IncludeLocked bool
	Now           time.Time
}

type DeleteResult struct {
	// Offlined is true when the problem was only taken offline because it has
	// submissions; its files are kept.
	Offlined bool
	// FileKeys lists stored files that are no longer referenced.
	FileKeys []string
}

type Repository interface {
	Create(ctx context.Context, p *Problem) error
	FindByID(ctx context.Context, id int64) (*Problem, error)
	List(ctx context.Context, f Filter, offset, limit int) ([]Problem, int64, error)
	// Update writes only the editable columns, never the checker path,
	// counters or creator.
	Update(ctx context.Context, p *Problem) error
	SetCheckerPath(ctx context.Context, id int64, path string) error
	Delete(ctx context.Context, id int64) (DeleteResult, error)

	// FindAuthor returns the public summary of a user (id/username/nickname),
	// or (nil, nil) when the user does not exist.
	FindAuthor(ctx context.Context, userID int64) (*AuthorSummary, error)

	AddTestcase(ctx context.Context, tc *Testcase) error
	ListTestcases(ctx context.Context, problemID int64) ([]Testcase, error)
	FindTestcase(ctx context.Context, problemID, id int64) (*Testcase, error)
	UpdateTestcase(ctx context.Context, problemID, id int64, score *int, isSample *bool) error
	DeleteTestcase(ctx context.Context, problemID, id int64) (*Testcase, error)
	ReorderTestcases(ctx context.Context, problemID int64, ids []int64) error
	CountTestcases(ctx context.Context, problemID int64) (int, error)
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

var editableColumns = []string{
	"title", "slug", "statement_md", "input_format_md", "output_format_md",
	"samples_json", "hint_md", "type",
	"tags_json", "difficulty", "time_limit_ms", "memory_limit_mb",
	"is_public", "source", "updated_at",
}

func (r *gormRepository) Create(ctx context.Context, p *Problem) error {
	return mapErr(r.db.WithContext(ctx).Create(p).Error)
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*Problem, error) {
	var p Problem
	if err := r.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, mapErr(err)
	}
	return &p, nil
}

func (r *gormRepository) FindAuthor(ctx context.Context, userID int64) (*AuthorSummary, error) {
	var a AuthorSummary
	err := r.db.WithContext(ctx).
		Table("users").
		Select("id, username, nickname").
		Where("id = ?", userID).
		Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *gormRepository) List(ctx context.Context, f Filter, offset, limit int) ([]Problem, int64, error) {
	q := r.db.WithContext(ctx).Model(&Problem{})
	if !f.IncludeHidden {
		q = q.Where("is_public = ?", true)
	}
	if !f.IncludeLocked {
		now := f.Now
		if now.IsZero() {
			now = time.Now()
		}
		q = q.Where(`NOT EXISTS (
			SELECT 1 FROM contest_problems cp
			JOIN contests c ON c.id = cp.contest_id
			WHERE cp.problem_id = problems.id AND c.end_time > ?
		)`, now)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		pat := "%" + escapeLike(kw) + "%"
		q = q.Where(`(title ILIKE ? ESCAPE '\' OR slug ILIKE ? ESCAPE '\')`, pat, pat)
	}
	if f.Tag != "" {
		tag, _ := json.Marshal([]string{f.Tag})
		q = q.Where("tags_json @> ?::jsonb", string(tag))
	}
	if f.Difficulty != 0 {
		q = q.Where("difficulty = ?", f.Difficulty)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []Problem
	if err := q.Order("id").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) Update(ctx context.Context, p *Problem) error {
	p.UpdatedAt = time.Now()
	return mapErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Problem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, p.ID).Error; err != nil {
			return err
		}
		// Publishing is checked under the same row lock as the write, so a
		// concurrent testcase deletion cannot leave a public problem empty.
		if p.IsPublic {
			var n int64
			if err := tx.Model(&Testcase{}).Where("problem_id = ?", p.ID).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return ErrPublishNeedsTestcase
			}
			if p.Type == TypeSpecial && current.CheckerPath == "" {
				return ErrPublishNeedsChecker
			}
		}
		res := tx.Model(&Problem{}).Where("id = ?", p.ID).Select(editableColumns).Updates(p)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	}))
}

func (r *gormRepository) SetCheckerPath(ctx context.Context, id int64, path string) error {
	res := r.db.WithContext(ctx).Model(&Problem{}).Where("id = ?", id).Update("checker_path", path)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) Delete(ctx context.Context, id int64) (DeleteResult, error) {
	var result DeleteResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Problem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return mapErr(err)
		}
		if p.SubmitCount > 0 {
			result.Offlined = true
			return tx.Model(&Problem{}).Where("id = ?", id).Update("is_public", false).Error
		}
		var cases []Testcase
		if err := tx.Where("problem_id = ?", id).Find(&cases).Error; err != nil {
			return err
		}
		for _, c := range cases {
			result.FileKeys = append(result.FileKeys, c.InputFile, c.OutputFile)
		}
		if p.CheckerPath != "" {
			result.FileKeys = append(result.FileKeys, p.CheckerPath)
		}
		return tx.Delete(&Problem{}, id).Error
	})
	if err != nil {
		return DeleteResult{}, mapErr(err)
	}
	return result, nil
}

func (r *gormRepository) AddTestcase(ctx context.Context, tc *Testcase) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Problem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&p, tc.ProblemID).Error; err != nil {
			return mapErr(err)
		}
		var max int
		if err := tx.Model(&Testcase{}).Where("problem_id = ?", tc.ProblemID).
			Select("COALESCE(MAX(sort_order), 0)").Scan(&max).Error; err != nil {
			return err
		}
		tc.SortOrder = max + 1
		return tx.Create(tc).Error
	})
}

func (r *gormRepository) ListTestcases(ctx context.Context, problemID int64) ([]Testcase, error) {
	var cases []Testcase
	err := r.db.WithContext(ctx).Where("problem_id = ?", problemID).Order("sort_order").Find(&cases).Error
	return cases, err
}

func (r *gormRepository) FindTestcase(ctx context.Context, problemID, id int64) (*Testcase, error) {
	var tc Testcase
	err := r.db.WithContext(ctx).Where("problem_id = ? AND id = ?", problemID, id).First(&tc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTestcaseNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tc, nil
}

func (r *gormRepository) UpdateTestcase(ctx context.Context, problemID, id int64, score *int, isSample *bool) error {
	cols := map[string]any{}
	if score != nil {
		cols["score"] = *score
	}
	if isSample != nil {
		cols["is_sample"] = *isSample
	}
	if len(cols) == 0 {
		_, err := r.FindTestcase(ctx, problemID, id)
		return err
	}
	res := r.db.WithContext(ctx).Model(&Testcase{}).
		Where("problem_id = ? AND id = ?", problemID, id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTestcaseNotFound
	}
	return nil
}

func (r *gormRepository) DeleteTestcase(ctx context.Context, problemID, id int64) (*Testcase, error) {
	var tc Testcase
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Problem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "is_public").First(&p, problemID).Error; err != nil {
			return err
		}
		if err := tx.Where("problem_id = ? AND id = ?", problemID, id).First(&tc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTestcaseNotFound
			}
			return err
		}
		if p.IsPublic {
			var n int64
			if err := tx.Model(&Testcase{}).Where("problem_id = ?", problemID).Count(&n).Error; err != nil {
				return err
			}
			if n <= 1 {
				return ErrLastTestcase
			}
		}
		res := tx.Where("problem_id = ? AND id = ?", problemID, id).Delete(&Testcase{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrTestcaseNotFound
		}
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &tc, nil
}

func (r *gormRepository) ReorderTestcases(ctx context.Context, problemID int64, ids []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&Problem{}, problemID).Error; err != nil {
			return mapErr(err)
		}
		var current []int64
		if err := tx.Model(&Testcase{}).Where("problem_id = ?", problemID).Pluck("id", &current).Error; err != nil {
			return err
		}
		sorted := slices.Clone(ids)
		slices.Sort(sorted)
		slices.Sort(current)
		if len(ids) == 0 || !slices.Equal(sorted, current) {
			return ErrBadOrder
		}
		// Shift out of the way first so no intermediate state collides.
		if err := tx.Model(&Testcase{}).Where("problem_id = ?", problemID).
			Update("sort_order", gorm.Expr("sort_order + 1000000")).Error; err != nil {
			return err
		}
		for i, id := range ids {
			if err := tx.Model(&Testcase{}).Where("problem_id = ? AND id = ?", problemID, id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *gormRepository) CountTestcases(ctx context.Context, problemID int64) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&Testcase{}).Where("problem_id = ?", problemID).Count(&n).Error
	return int(n), err
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
		if pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_problems_slug" {
			return ErrSlugTaken
		}
		if pgErr.Code == "23503" {
			return ErrInUse
		}
	}
	return err
}
