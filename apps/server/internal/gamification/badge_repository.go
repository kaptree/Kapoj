package gamification

import "context"

// FindBadge 返回指定徽章；不存在时返回 gorm.ErrRecordNotFound。
func (r *Repository) FindBadge(ctx context.Context, id int64) (*Badge, error) {
	var badge Badge
	if err := r.db.WithContext(ctx).First(&badge, id).Error; err != nil {
		return nil, err
	}
	return &badge, nil
}

// ListBadges 返回按 sort_order、id 排序的全部徽章。
// 后续任务在此签名上补充上架筛选与用户拥有状态。
func (r *Repository) ListBadges(ctx context.Context) ([]Badge, error) {
	var badges []Badge
	err := r.db.WithContext(ctx).Order("sort_order, id").Find(&badges).Error
	return badges, err
}
