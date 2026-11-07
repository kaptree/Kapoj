package learning

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// IsProblemInActivePath reports whether the problem is referenced by any node
// of an active (not soft-deleted) learning path.
func (r *Repository) IsProblemInActivePath(ctx context.Context, problemID int64) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM learning_path_nodes AS n
			JOIN learning_paths AS p ON p.id = n.path_id
			WHERE p.is_active = true
			  AND n.problem_ids_json @> jsonb_build_array(?::bigint)
		)`, problemID).Scan(&exists).Error
	return exists, err
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

func (r *Repository) CreateNode(ctx context.Context, n *LearningPathNode) error {
	return r.db.WithContext(ctx).Create(n).Error
}

func (r *Repository) FindNode(ctx context.Context, id int64) (*LearningPathNode, error) {
	var n LearningPathNode
	if err := r.db.WithContext(ctx).First(&n, id).Error; err != nil {
		return nil, mapRepoError(err)
	}
	return &n, nil
}

func (r *Repository) UpdateNode(ctx context.Context, n *LearningPathNode) error {
	n.UpdatedAt = time.Now()
	updates := map[string]any{
		"title":            n.Title,
		"description_md":   n.DescriptionMD,
		"parent_id":        n.ParentID,
		"problem_ids_json": n.ProblemIDs,
		"sort_order":       n.SortOrder,
		"required_count":   n.RequiredCount,
		"updated_at":       n.UpdatedAt,
	}
	res := r.db.WithContext(ctx).Model(&LearningPathNode{}).
		Where("id = ? AND path_id = ?", n.ID, n.PathID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteNode removes the node and its per-user progress rows atomically.
func (r *Repository) DeleteNode(ctx context.Context, n *LearningPathNode) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", n.ID).Delete(&UserLearningProgress{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND path_id = ?", n.ID, n.PathID).Delete(&LearningPathNode{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (r *Repository) CountChildren(ctx context.Context, nodeID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&LearningPathNode{}).
		Where("parent_id = ?", nodeID).Count(&count).Error
	return count, err
}

// ListNodes returns a path's nodes ordered by sort order then id.
func (r *Repository) ListNodes(ctx context.Context, pathID int64) ([]LearningPathNode, error) {
	var nodes []LearningPathNode
	err := r.db.WithContext(ctx).Where("path_id = ?", pathID).
		Order("sort_order ASC, id ASC").Find(&nodes).Error
	return nodes, err
}

// ProblemSummaries returns public problem summaries keyed by problem id.
// Missing or private problems are simply absent from the result.
func (r *Repository) ProblemSummaries(ctx context.Context, ids []int64) (map[int64]ProblemSummary, error) {
	out := make(map[int64]ProblemSummary, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []ProblemSummary
	err := r.db.WithContext(ctx).Table("problems").
		Select("id AS problem_id, title, slug, difficulty").
		Where("id IN ? AND is_public = ?", ids, true).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ProblemID] = row
	}
	return out, nil
}

// ListActivePathIDsForUser returns the IDs of active paths visible to the
// user: official, public, or created by them.
func (r *Repository) ListActivePathIDsForUser(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&LearningPath{}).
		Where("is_active = ?", true).
		Where("(is_official = ? OR is_public = ? OR created_by = ?)", true, true, userID).
		Pluck("id", &ids).Error
	return ids, err
}

// ListNodesByPathIDs returns the nodes of the given paths ordered by path id,
// sort order and id. An empty path list yields nil without querying.
func (r *Repository) ListNodesByPathIDs(ctx context.Context, pathIDs []int64) ([]LearningPathNode, error) {
	if len(pathIDs) == 0 {
		return nil, nil
	}
	var nodes []LearningPathNode
	err := r.db.WithContext(ctx).Where("path_id IN ?", pathIDs).
		Order("path_id ASC, sort_order ASC, id ASC").Find(&nodes).Error
	return nodes, err
}

// ListProgressForPaths returns the user's progress rows for the given paths,
// keyed by node ID. Guests (userID<=0) and empty path lists return an empty map.
func (r *Repository) ListProgressForPaths(ctx context.Context, userID int64, pathIDs []int64) (map[int64]UserLearningProgress, error) {
	out := make(map[int64]UserLearningProgress)
	if userID <= 0 || len(pathIDs) == 0 {
		return out, nil
	}
	var rows []UserLearningProgress
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND path_id IN ?", userID, pathIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].NodeID] = rows[i]
	}
	return out, nil
}

// AcceptedProblemIDs returns the distinct problem IDs the user has an accepted
// submission for.
func (r *Repository) AcceptedProblemIDs(ctx context.Context, userID int64) (map[int64]struct{}, error) {
	out := make(map[int64]struct{})
	var ids []int64
	if err := r.db.WithContext(ctx).Table("submissions").
		Where("user_id = ? AND status = ?", userID, "accepted").
		Distinct("problem_id").
		Pluck("problem_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// UpsertProgress inserts or updates progress rows, keyed by the unique
// (user_id, node_id) constraint, updating status, solved_count, completed_at
// and updated_at on conflict.
func (r *Repository) UpsertProgress(ctx context.Context, rows []UserLearningProgress) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "solved_count", "completed_at", "updated_at"}),
	}).Create(&rows).Error
}

func mapRepoError(err error) error {
	if errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
