package gamification

import (
	"context"

	"gorm.io/gorm"
)

// Repository 提供游戏化模块的数据库访问。
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CountAcceptedProblems 返回用户去重后的 AC 题目数。
func (r *Repository) CountAcceptedProblems(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Raw(
		`SELECT COUNT(DISTINCT problem_id) FROM submissions
		 WHERE user_id = ? AND status = 'accepted'`,
		userID,
	).Scan(&count).Error
	return count, err
}

// CountContestParticipations 返回用户报名过的比赛数。
func (r *Repository) CountContestParticipations(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM contest_participants WHERE user_id = ?`,
		userID,
	).Scan(&count).Error
	return count, err
}

// CountCreatedContests 返回用户创建的比赛数。
func (r *Repository) CountCreatedContests(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM contests WHERE created_by = ?`,
		userID,
	).Scan(&count).Error
	return count, err
}
