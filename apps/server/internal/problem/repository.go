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
	DifficultyMin int
	DifficultyMax int
	IncludeHidden bool
	IncludeLocked bool
	OwnerID       int64
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
	// CreateWithHistory inserts the problem and, when history is not nil, its
	// initial difficulty history row in one transaction.
	CreateWithHistory(ctx context.Context, p *Problem, history *DifficultyHistory) error
	FindByID(ctx context.Context, id int64) (*Problem, error)
	FindBySlug(ctx context.Context, slug string) (*Problem, error)
	List(ctx context.Context, f Filter, offset, limit int) ([]Problem, int64, error)
	// Update writes only the editable columns, never the checker path,
	// counters or creator.
	Update(ctx context.Context, p *Problem) error
	// UpdateWithHistory writes the editable columns and, when history is not
	// nil, appends a difficulty history row in the same transaction.
	UpdateWithHistory(ctx context.Context, p *Problem, history *DifficultyHistory) error
	// UpdateDifficultyWithHistory writes only the difficulty metadata columns,
	// plus the optional history row.
	UpdateDifficultyWithHistory(ctx context.Context, p *Problem, history *DifficultyHistory) error
	// ListDifficultyHistory returns one page of history, newest first.
	ListDifficultyHistory(ctx context.Context, problemID int64, offset, limit int) ([]DifficultyHistory, int64, error)
	SetCheckerPath(ctx context.Context, id int64, path string) error
	UpdateSolution(ctx context.Context, id int64, solutionMD, solutionCode string, price int64) error
	// AllocateAttachmentIDs reserves attachment ids before a staged overwrite.
	AllocateAttachmentIDs(ctx context.Context, count int) ([]int64, error)
	// ReplaceContent atomically replaces a problem's testcases/attachments and
	// editable content, returning storage keys that are no longer referenced.
	ReplaceContent(ctx context.Context, p *Problem, testcases []*Testcase, attachments []*ProblemAttachment) ([]string, error)
	// ReplaceContentWithHistory is ReplaceContent plus an optional difficulty
	// history row written in the same transaction. old_score is taken from the
	// locked row so a concurrent edit cannot make the log lie.
	ReplaceContentWithHistory(ctx context.Context, p *Problem, testcases []*Testcase, attachments []*ProblemAttachment, history *DifficultyHistory) ([]string, error)
	Delete(ctx context.Context, id int64) (DeleteResult, error)

	// FindAuthor returns the public summary of a user (id/username/nickname),
	// or (nil, nil) when the user does not exist.
	FindAuthor(ctx context.Context, userID int64) (*AuthorSummary, error)

	AddTestcase(ctx context.Context, tc *Testcase) error
	AddTestcases(ctx context.Context, problemID int64, cases []*Testcase) error
	ListTestcases(ctx context.Context, problemID int64) ([]Testcase, error)
	FindTestcase(ctx context.Context, problemID, id int64) (*Testcase, error)
	UpdateTestcase(ctx context.Context, problemID, id int64, score *int, isSample *bool) error
	UpdateTestcaseFiles(ctx context.Context, problemID, id int64, inputFile, outputFile string, inputSize, outputSize int64) error
	DeleteTestcase(ctx context.Context, problemID, id int64) (*Testcase, error)
	ReorderTestcases(ctx context.Context, problemID int64, ids []int64) error
	CountTestcases(ctx context.Context, problemID int64) (int, error)

	// StatusCounts returns the visible submission distribution for a problem.
	StatusCounts(ctx context.Context, problemID int64, f SubmissionVisibility) ([]StatusCount, error)
	// ListSubmissionMetrics returns visible submission metrics without code or
	// submission IDs, plus the total number of matching rows.
	ListSubmissionMetrics(ctx context.Context, problemID int64, f SubmissionMetricsFilter) ([]SubmissionMetricRow, int64, error)

	CreateAttachment(ctx context.Context, a *ProblemAttachment) error
	ListAttachments(ctx context.Context, problemID int64, kind string) ([]ProblemAttachment, error)
	FindAttachment(ctx context.Context, problemID, id int64) (*ProblemAttachment, error)
	DeleteAttachment(ctx context.Context, problemID, id int64) (*ProblemAttachment, error)
	CountAttachments(ctx context.Context, problemID int64) (int64, error)
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

var editableColumns = []string{
	"title", "slug", "statement_md", "input_format_md", "output_format_md",
	"samples_json", "hint_md", "type", "answers_json",
	"tags_json", "difficulty", "difficulty_prior", "difficulty_prior_source",
	"difficulty_auto", "difficulty_source", "difficulty_reason",
	"difficulty_updated_at", "time_limit_ms", "memory_limit_mb",
	"is_public", "source", "updated_at",
}

// difficultyColumns are the columns owned by the difficulty subsystem.
var difficultyColumns = []string{
	"difficulty", "difficulty_prior", "difficulty_prior_source",
	"difficulty_auto", "difficulty_source", "difficulty_reason",
	"difficulty_updated_at", "updated_at",
}

func (r *gormRepository) Create(ctx context.Context, p *Problem) error {
	return mapErr(r.db.WithContext(ctx).Create(p).Error)
}

func (r *gormRepository) CreateWithHistory(ctx context.Context, p *Problem, history *DifficultyHistory) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// GORM replaces a false zero value with the `default:true` tag when it
		// builds an INSERT, so a locked problem would silently become auto.
		// Insert first, then force the intended value in the same transaction.
		auto := p.DifficultyAuto
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		if !auto {
			if err := tx.Model(&Problem{}).Where("id = ?", p.ID).
				UpdateColumn("difficulty_auto", false).Error; err != nil {
				return err
			}
			p.DifficultyAuto = false
		}
		return createHistory(tx, p.ID, history)
	})
	return mapErr(err)
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*Problem, error) {
	var p Problem
	if err := r.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, mapErr(err)
	}
	return &p, nil
}

func (r *gormRepository) FindBySlug(ctx context.Context, slug string) (*Problem, error) {
	var p Problem
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
	if f.OwnerID > 0 {
		q = q.Where("created_by = ?", f.OwnerID)
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
	if f.DifficultyMin > 0 {
		q = q.Where("difficulty >= ?", f.DifficultyMin)
	}
	if f.DifficultyMax > 0 {
		q = q.Where("difficulty <= ?", f.DifficultyMax)
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
	return mapErr(r.update(ctx, p, nil, editableColumns))
}

func (r *gormRepository) UpdateWithHistory(ctx context.Context, p *Problem, history *DifficultyHistory) error {
	return mapErr(r.update(ctx, p, history, editableColumns))
}

func (r *gormRepository) UpdateDifficultyWithHistory(ctx context.Context, p *Problem, history *DifficultyHistory) error {
	return mapErr(r.update(ctx, p, history, difficultyColumns))
}

// update writes the given columns under a row lock. Publishability is checked
// in the same transaction so a concurrent testcase deletion cannot leave a
// public problem empty or without a checker.
func (r *gormRepository) update(ctx context.Context, p *Problem, history *DifficultyHistory, columns []string) error {
	p.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Problem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, p.ID).Error; err != nil {
			return err
		}
		// Objective problems are answered directly, so they need no testcases.
		if p.IsPublic {
			if p.Type != TypeObjective {
				var n int64
				if err := tx.Model(&Testcase{}).Where("problem_id = ?", p.ID).Count(&n).Error; err != nil {
					return err
				}
				if n == 0 {
					return ErrPublishNeedsTestcase
				}
			}
			if p.Type == TypeSpecial && current.CheckerPath == "" {
				return ErrPublishNeedsChecker
			}
		}
		res := tx.Model(&Problem{}).Where("id = ?", p.ID).Select(columns).Updates(p)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return createHistory(tx, p.ID, history)
	})
}

// createHistory appends one history row; a nil history is a no-op.
func createHistory(tx *gorm.DB, problemID int64, history *DifficultyHistory) error {
	if history == nil {
		return nil
	}
	history.ProblemID = problemID
	return tx.Create(history).Error
}

func (r *gormRepository) ListDifficultyHistory(ctx context.Context, problemID int64, offset, limit int) ([]DifficultyHistory, int64, error) {
	q := r.db.WithContext(ctx).Model(&DifficultyHistory{}).Where("problem_id = ?", problemID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []DifficultyHistory
	if err := q.Order("created_at DESC, id DESC").Offset(offset).Limit(limit).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
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

func (r *gormRepository) UpdateSolution(ctx context.Context, id int64, solutionMD, solutionCode string, price int64) error {
	res := r.db.WithContext(ctx).Model(&Problem{}).Where("id = ?", id).Updates(map[string]any{
		"solution_md": solutionMD, "solution_code": solutionCode,
		"solution_price_coins": price, "updated_at": time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) AllocateAttachmentIDs(ctx context.Context, count int) ([]int64, error) {
	if count <= 0 {
		return nil, nil
	}
	rows, err := r.db.WithContext(ctx).Raw(
		`SELECT nextval(pg_get_serial_sequence('problem_attachments', 'id')) FROM generate_series(1, ?)`,
		count).Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, count)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReplaceContent stages an overwrite: old testcase/attachment rows and content
// are swapped in one transaction. The returned storage keys are deleted by the
// caller only after a successful commit.
func (r *gormRepository) ReplaceContent(
	ctx context.Context,
	p *Problem,
	testcases []*Testcase,
	attachments []*ProblemAttachment,
) ([]string, error) {
	return r.ReplaceContentWithHistory(ctx, p, testcases, attachments, nil)
}

// ReplaceContentWithHistory also appends a difficulty history row. The locked
// row's score is authoritative for old_score.
func (r *gormRepository) ReplaceContentWithHistory(
	ctx context.Context,
	p *Problem,
	testcases []*Testcase,
	attachments []*ProblemAttachment,
	history *DifficultyHistory,
) ([]string, error) {
	var oldKeys []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Problem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, p.ID).Error; err != nil {
			return err
		}
		if history != nil {
			history.OldScore = current.Difficulty
		}
		if p.IsPublic && p.Type != TypeObjective && len(testcases) == 0 {
			return ErrPublishNeedsTestcase
		}
		if p.IsPublic && p.Type == TypeSpecial && p.CheckerPath == "" {
			return ErrPublishNeedsChecker
		}

		var oldCases []Testcase
		if err := tx.Where("problem_id = ?", p.ID).Find(&oldCases).Error; err != nil {
			return err
		}
		for _, tc := range oldCases {
			oldKeys = append(oldKeys, tc.InputFile, tc.OutputFile)
		}
		if current.CheckerPath != "" {
			oldKeys = append(oldKeys, current.CheckerPath)
		}
		var oldAttachments []ProblemAttachment
		if err := tx.Where("problem_id = ?", p.ID).Find(&oldAttachments).Error; err != nil {
			return err
		}
		for _, attachment := range oldAttachments {
			oldKeys = append(oldKeys, attachment.StorageKey)
		}

		if err := tx.Where("problem_id = ?", p.ID).Delete(&Testcase{}).Error; err != nil {
			return err
		}
		if err := tx.Where("problem_id = ?", p.ID).Delete(&ProblemAttachment{}).Error; err != nil {
			return err
		}

		p.UpdatedAt = time.Now()
		res := tx.Model(&Problem{}).Where("id = ?", p.ID).Updates(map[string]any{
			"title": p.Title, "slug": p.Slug,
			"statement_md": p.StatementMD, "input_format_md": p.InputFormatMD,
			"output_format_md": p.OutputFormatMD, "samples_json": p.Samples,
			"hint_md": p.HintMD, "type": p.Type, "answers_json": p.Answers,
			"tags_json": p.Tags, "difficulty": p.Difficulty,
			"difficulty_prior": p.DifficultyPrior, "difficulty_prior_source": p.DifficultyPriorSource,
			"difficulty_auto": p.DifficultyAuto, "difficulty_source": p.DifficultySource,
			"difficulty_reason": p.DifficultyReason, "difficulty_updated_at": p.DifficultyUpdatedAt,
			"time_limit_ms": p.TimeLimitMS, "memory_limit_mb": p.MemoryLimitMB,
			"is_public": p.IsPublic, "source": p.Source,
			"solution_md": p.SolutionMD, "solution_code": p.SolutionCode,
			"solution_price_coins": p.SolutionPriceCoins,
			"checker_path":         p.CheckerPath, "updated_at": p.UpdatedAt,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}

		for i := range testcases {
			testcases[i].ProblemID = p.ID
			testcases[i].SortOrder = i + 1
		}
		if len(testcases) > 0 {
			if err := tx.Create(testcases).Error; err != nil {
				return err
			}
		}
		for i := range attachments {
			attachments[i].ProblemID = p.ID
		}
		if len(attachments) > 0 {
			if err := tx.Create(attachments).Error; err != nil {
				return err
			}
		}
		return createHistory(tx, p.ID, history)
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return oldKeys, nil
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
		var attachments []ProblemAttachment
		if err := tx.Where("problem_id = ?", id).Find(&attachments).Error; err != nil {
			return err
		}
		for _, a := range attachments {
			result.FileKeys = append(result.FileKeys, a.StorageKey)
		}
		if err := tx.Where("problem_id = ?", id).Delete(&ProblemAttachment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("problem_id = ?", id).Delete(&DifficultyHistory{}).Error; err != nil {
			return err
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

func (r *gormRepository) AddTestcases(ctx context.Context, problemID int64, cases []*Testcase) error {
	if len(cases) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Problem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&p, problemID).Error; err != nil {
			return mapErr(err)
		}
		var max int
		if err := tx.Model(&Testcase{}).Where("problem_id = ?", problemID).
			Select("COALESCE(MAX(sort_order), 0)").Scan(&max).Error; err != nil {
			return err
		}
		for i, tc := range cases {
			tc.ProblemID = problemID
			tc.SortOrder = max + i + 1
		}
		return tx.Create(&cases).Error
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

func (r *gormRepository) UpdateTestcaseFiles(ctx context.Context, problemID, id int64, inputFile, outputFile string, inputSize, outputSize int64) error {
	res := r.db.WithContext(ctx).Model(&Testcase{}).
		Where("problem_id = ? AND id = ?", problemID, id).
		Updates(map[string]any{
			"input_file": inputFile, "output_file": outputFile,
			"input_size": inputSize, "output_size": outputSize,
		})
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
			Select("id", "type", "is_public").First(&p, problemID).Error; err != nil {
			return err
		}
		if err := tx.Where("problem_id = ? AND id = ?", problemID, id).First(&tc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTestcaseNotFound
			}
			return err
		}
		if p.IsPublic && p.Type != TypeObjective {
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

func (r *gormRepository) CreateAttachment(ctx context.Context, a *ProblemAttachment) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormRepository) ListAttachments(ctx context.Context, problemID int64, kind string) ([]ProblemAttachment, error) {
	var items []ProblemAttachment
	err := r.db.WithContext(ctx).
		Where("problem_id = ? AND kind = ?", problemID, kind).
		Order("id").Find(&items).Error
	return items, err
}

func (r *gormRepository) FindAttachment(ctx context.Context, problemID, id int64) (*ProblemAttachment, error) {
	var a ProblemAttachment
	err := r.db.WithContext(ctx).Where("problem_id = ? AND id = ?", problemID, id).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *gormRepository) DeleteAttachment(ctx context.Context, problemID, id int64) (*ProblemAttachment, error) {
	a, err := r.FindAttachment(ctx, problemID, id)
	if err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Delete(&ProblemAttachment{}, a.ID).Error; err != nil {
		return nil, err
	}
	return a, nil
}

func (r *gormRepository) CountAttachments(ctx context.Context, problemID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&ProblemAttachment{}).Where("problem_id = ?", problemID).Count(&n).Error
	return n, err
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
