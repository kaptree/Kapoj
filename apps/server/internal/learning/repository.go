package learning

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound 表示学习路线不存在。
var ErrNotFound = errors.New("learning path not found")

// ListFilter controls which learning paths are visible to a viewer.
type ListFilter struct {
	ViewerID        int64
	IncludeInactive bool
	Mine            bool
}

// Repository owns learning path persistence and read queries.
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// pathItemColumns selects path metadata plus author, node count and the
// viewer's stored completed-node count. The first argument is the viewer ID.
const pathItemColumns = `p.id, p.title, p.description_md, p.cover_url,
	p.is_official, p.is_public, p.is_active, p.sort_order, p.created_at, p.updated_at,
	u.id AS author_id, u.username AS author_username, u.nickname AS author_nickname,
	(SELECT COUNT(*) FROM learning_path_nodes AS n WHERE n.path_id = p.id) AS node_count,
	(SELECT COUNT(*) FROM user_learning_progress AS pr
		WHERE pr.path_id = p.id AND pr.user_id = ? AND pr.status = ?) AS my_completed_count`

type pathRow struct {
	ID               int64
	Title            string
	DescriptionMD    string
	CoverURL         string
	IsOfficial       bool
	IsPublic         bool
	IsActive         bool
	SortOrder        int
	CreatedAt        time.Time
	UpdatedAt        time.Time
	AuthorID         int64
	AuthorUsername   string
	AuthorNickname   string
	NodeCount        int
	MyCompletedCount int
}

func (r pathRow) item() PathItem {
	return PathItem{
		ID:            r.ID,
		Title:         r.Title,
		DescriptionMD: r.DescriptionMD,
		CoverURL:      r.CoverURL,
		IsOfficial:    r.IsOfficial,
		IsPublic:      r.IsPublic,
		IsActive:      r.IsActive,
		Author: UserSummary{
			ID:       r.AuthorID,
			Username: r.AuthorUsername,
			Nickname: r.AuthorNickname,
		},
		NodeCount:        r.NodeCount,
		MyCompletedCount: r.MyCompletedCount,
		SortOrder:        r.SortOrder,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

func (r *Repository) Create(ctx context.Context, p *LearningPath) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*LearningPath, error) {
	var p LearningPath
	if err := r.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, mapRepoError(err)
	}
	return &p, nil
}

var pathUpdateColumns = []string{
	"title", "description_md", "cover_url", "is_official", "is_public",
	"is_active", "sort_order", "updated_at",
}

func (r *Repository) Update(ctx context.Context, p *LearningPath) error {
	p.UpdatedAt = time.Now()
	res := r.db.WithContext(ctx).Model(&LearningPath{}).Where("id = ?", p.ID).
		Select(pathUpdateColumns).Updates(p)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SetActive(ctx context.Context, id int64, active bool) error {
	res := r.db.WithContext(ctx).Model(&LearningPath{}).Where("id = ?", id).
		Updates(map[string]any{"is_active": active, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns learning paths ordered by official first, then sort order and id.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]PathItem, error) {
	q := r.db.WithContext(ctx).Table("learning_paths AS p").
		Select(pathItemColumns, f.ViewerID, ProgressCompleted).
		Joins("JOIN users AS u ON u.id = p.created_by")

	if !f.IncludeInactive {
		q = q.Where("p.is_active = ?", true).
			Where("(p.is_official = ? OR p.is_public = ? OR p.created_by = ?)",
				true, true, f.ViewerID)
	}
	if f.Mine {
		q = q.Where("p.created_by = ?", f.ViewerID)
	}

	var rows []pathRow
	if err := q.Order("p.is_official DESC, p.sort_order ASC, p.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]PathItem, len(rows))
	for i := range rows {
		items[i] = rows[i].item()
	}
	return items, nil
}

// Item returns one path's metadata with author and counts.
func (r *Repository) Item(ctx context.Context, viewerID, id int64) (*PathItem, error) {
	var row pathRow
	err := r.db.WithContext(ctx).Table("learning_paths AS p").
		Select(pathItemColumns, viewerID, ProgressCompleted).
		Joins("JOIN users AS u ON u.id = p.created_by").
		Where("p.id = ?", id).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	item := row.item()
	return &item, nil
}

// CountUnavailableNodeProblems counts distinct problems referenced by the
// path's nodes that no longer exist or are not public.
func (r *Repository) CountUnavailableNodeProblems(ctx context.Context, pathID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*)
		FROM (
			SELECT DISTINCT (elem)::bigint AS problem_id
			FROM learning_path_nodes
			CROSS JOIN LATERAL jsonb_array_elements_text(problem_ids_json) AS elem
			WHERE path_id = ?
		) AS refs
		LEFT JOIN problems AS p ON p.id = refs.problem_id
		WHERE p.id IS NULL OR p.is_public = false`, pathID).Scan(&count).Error
	return count, err
}

func mapRepoError(err error) error {
	if errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
