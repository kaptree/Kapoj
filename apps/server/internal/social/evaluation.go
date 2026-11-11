package social

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const (
	evaluationContentMaxRunes = 40
	evaluationCloudLimit      = 24
)

// EvaluationInput 是保存评价的请求体。
type EvaluationInput struct {
	Content string `json:"content"`
}

// EvaluationItem 是一条对外的用户评价。
type EvaluationItem struct {
	ID        int64       `json:"id"`
	Content   string      `json:"content"`
	Author    UserSummary `json:"author"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
	CanDelete bool        `json:"canDelete"`
}

// EvaluationCloudItem 是词云中的一项：相同内容聚合后的文本与次数。
type EvaluationCloudItem struct {
	Text  string `json:"text"`
	Count int64  `json:"count"`
}

// EvaluationPage 是用户主页的评价列表、词云与当前访问者的权限信息。
type EvaluationPage struct {
	Total        int64                 `json:"total"`
	Items        []EvaluationItem      `json:"items"`
	Cloud        []EvaluationCloudItem `json:"cloud"`
	MyEvaluation *EvaluationItem       `json:"myEvaluation"`
	CanEvaluate  bool                  `json:"canEvaluate"`
}

// evaluationRow 是评价与作者摘要的联表查询结果。
type evaluationRow struct {
	ID        int64     `gorm:"column:id"`
	Content   string    `gorm:"column:content"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
	AuthorID  int64     `gorm:"column:author_id"`
	Username  string    `gorm:"column:username"`
	Nickname  string    `gorm:"column:nickname"`
	AvatarURL string    `gorm:"column:avatar_url"`
	Rating    int       `gorm:"column:rating"`
}

func (r evaluationRow) item(canDelete bool) EvaluationItem {
	return EvaluationItem{
		ID:      r.ID,
		Content: r.Content,
		Author: UserSummary{
			ID: r.AuthorID, Username: r.Username, Nickname: r.Nickname,
			AvatarURL: r.AvatarURL, Rating: r.Rating,
		},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		CanDelete: canDelete,
	}
}

// normalizeEvaluationContent 校验并规范化评价内容：trim 首尾空白，限制 1-40
// 个 rune，拒绝换行、制表符与其他控制字符。
func normalizeEvaluationContent(content string) (string, error) {
	for _, r := range content {
		if r == '\n' || r == '\r' || r == '\t' || unicode.IsControl(r) {
			return "", httpx.ErrValidation("评价内容不能包含换行或控制字符")
		}
	}
	trimmed := strings.TrimSpace(content)
	n := len([]rune(trimmed))
	if n < 1 || n > evaluationContentMaxRunes {
		return "", httpx.ErrValidation("评价内容需为 1-40 个字符")
	}
	return trimmed, nil
}

// SaveEvaluation 创建或更新当前作者对目标用户的评价。同一作者对同一目标只保留
// 一条：重复保存更新内容与 updatedAt，保留 createdAt。
func (s *Service) SaveEvaluation(ctx context.Context, authorID, targetID int64, content string) (*EvaluationItem, error) {
	if authorID <= 0 || targetID <= 0 {
		return nil, httpx.ErrValidation("用户 ID 必须为正数")
	}
	if authorID == targetID {
		return nil, httpx.ErrValidation("不能评价自己")
	}
	if err := s.requireActiveTarget(ctx, targetID); err != nil {
		// requireActiveTarget 复用关注逻辑的文案，这里换成评价语义。
		var appErr *httpx.AppError
		if errors.As(err, &appErr) && appErr.Code == 40001 {
			return nil, httpx.ErrValidation("不能评价该用户")
		}
		return nil, err
	}
	normalized, err := normalizeEvaluationContent(content)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(time.Local)
	if err := s.db.WithContext(ctx).Exec(
		`INSERT INTO user_evaluations (author_user_id, target_user_id, content, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (author_user_id, target_user_id)
		 DO UPDATE SET content = EXCLUDED.content, updated_at = now()`,
		authorID, targetID, normalized, now, now,
	).Error; err != nil {
		return nil, err
	}

	rows, err := s.evaluationRows(ctx, targetID, authorID, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, httpx.ErrNotFound
	}
	item := rows[0].item(true)
	return &item, nil
}

// ListEvaluations 返回目标用户主页的评价分页、词云与权限信息。游客可读；
// 不存在目标 404，非 active 目标仍可读但 canEvaluate 为 false。
func (s *Service) ListEvaluations(ctx context.Context, viewerID, targetID int64, page, size int) (*EvaluationPage, error) {
	if err := s.requireUserExists(ctx, targetID); err != nil {
		return nil, err
	}
	page, size = normalizePaging(page, size)

	var total int64
	if err := s.db.WithContext(ctx).Model(&UserEvaluation{}).
		Where("target_user_id = ?", targetID).Count(&total).Error; err != nil {
		return nil, err
	}

	rows, err := s.evaluationRowsByTarget(ctx, targetID, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]EvaluationItem, len(rows))
	for i, row := range rows {
		canDelete := viewerID > 0 && (viewerID == targetID || viewerID == row.AuthorID)
		items[i] = row.item(canDelete)
	}

	cloud := make([]EvaluationCloudItem, 0)
	if err := s.db.WithContext(ctx).Table("user_evaluations").
		Select("content AS text, COUNT(*) AS count").
		Where("target_user_id = ?", targetID).
		Group("content").
		Order("COUNT(*) DESC, MIN(id) ASC").
		Limit(evaluationCloudLimit).
		Scan(&cloud).Error; err != nil {
		return nil, err
	}

	var myEvaluation *EvaluationItem
	if viewerID > 0 && viewerID != targetID {
		mine, err := s.evaluationRows(ctx, targetID, viewerID, 1)
		if err != nil {
			return nil, err
		}
		if len(mine) > 0 {
			item := mine[0].item(true)
			myEvaluation = &item
		}
	}

	canEvaluate := false
	if viewerID > 0 && viewerID != targetID {
		var target struct {
			Status user.Status
		}
		err := s.db.WithContext(ctx).Table("users").
			Select("status").Where("id = ?", targetID).Take(&target).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		canEvaluate = err == nil && target.Status == user.StatusActive
	}

	return &EvaluationPage{
		Total: total, Items: items, Cloud: cloud,
		MyEvaluation: myEvaluation, CanEvaluate: canEvaluate,
	}, nil
}

// DeleteMyEvaluation 删除当前作者对目标用户的评价；不存在时幂等返回成功。
func (s *Service) DeleteMyEvaluation(ctx context.Context, authorID, targetID int64) error {
	if authorID <= 0 || targetID <= 0 {
		return httpx.ErrValidation("用户 ID 必须为正数")
	}
	return s.db.WithContext(ctx).
		Where("author_user_id = ? AND target_user_id = ?", authorID, targetID).
		Delete(&UserEvaluation{}).Error
}

// DeleteEvaluationAsTarget 由主页主人删除自己页面上的任意评价。viewer 必须是
// 目标用户本人，否则 403；评价不存在或不属于该页面返回 404。
func (s *Service) DeleteEvaluationAsTarget(ctx context.Context, viewerID, targetID, evaluationID int64) error {
	if viewerID <= 0 || viewerID != targetID {
		return httpx.ErrForbidden
	}
	if targetID <= 0 || evaluationID <= 0 {
		return httpx.ErrNotFound
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND target_user_id = ?", evaluationID, targetID).
		Delete(&UserEvaluation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// evaluationRowsByTarget 查询目标用户评价页，按最近更新倒序、id 倒序。
func (s *Service) evaluationRowsByTarget(ctx context.Context, targetID int64, page, size int) ([]evaluationRow, error) {
	var rows []evaluationRow
	err := s.evaluationQuery(ctx).
		Where("e.target_user_id = ?", targetID).
		Order("e.updated_at DESC, e.id DESC").
		Offset((page - 1) * size).Limit(size).
		Scan(&rows).Error
	return rows, err
}

// evaluationRows 查询某位作者对目标用户的评价；limit<=0 表示不限制。
func (s *Service) evaluationRows(ctx context.Context, targetID, authorID int64, limit int) ([]evaluationRow, error) {
	var rows []evaluationRow
	query := s.evaluationQuery(ctx).
		Where("e.target_user_id = ? AND e.author_user_id = ?", targetID, authorID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Scan(&rows).Error
	return rows, err
}

// evaluationQuery 是评价与作者公开摘要的联表基础查询。
func (s *Service) evaluationQuery(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).
		Table("user_evaluations AS e").
		Select(`e.id, e.content, e.created_at, e.updated_at,
			u.id AS author_id, u.username, u.nickname, u.avatar_url, u.rating`).
		Joins("JOIN users AS u ON u.id = e.author_user_id")
}
