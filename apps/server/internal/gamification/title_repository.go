package gamification

import "context"

// FindTitle 返回指定称号；不存在时返回 gorm.ErrRecordNotFound。
func (r *Repository) FindTitle(ctx context.Context, id int64) (*Title, error) {
	var title Title
	if err := r.db.WithContext(ctx).First(&title, id).Error; err != nil {
		return nil, err
	}
	return &title, nil
}

// ListTitles 返回按 sort_order、id 排序的全部称号。
// 后续任务在此签名上补充上架筛选与用户解锁状态。
func (r *Repository) ListTitles(ctx context.Context) ([]Title, error) {
	var titles []Title
	err := r.db.WithContext(ctx).Order("sort_order, id").Find(&titles).Error
	return titles, err
}
