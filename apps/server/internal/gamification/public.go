package gamification

import "context"

// BadgeSummary 是公开身份中展示的已装备徽章。
type BadgeSummary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

// TitleSummary 是公开身份中展示的已装备称号。
type TitleSummary struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Identity 是用户对外展示的装备身份；无装备时徽章为空切片、称号为 nil。
type Identity struct {
	Badges []BadgeSummary `json:"equippedBadges"`
	Title  *TitleSummary  `json:"equippedTitle"`
}

// emptyIdentity 返回可直接序列化为空数组的公开身份。
func emptyIdentity() Identity {
	return Identity{Badges: []BadgeSummary{}}
}

// Identities 批量返回用户公开身份。结果包含每个正数输入用户；无装备用户的
// Badges 是空切片而非 nil。已装备但后来下线的徽章/称号仍然展示。
func (s *Service) Identities(ctx context.Context, userIDs []int64) (map[int64]Identity, error) {
	identities := make(map[int64]Identity, len(userIDs))
	ids := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := identities[id]; ok {
			continue
		}
		identities[id] = emptyIdentity()
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return identities, nil
	}

	badges, err := s.repo.equippedBadges(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range badges {
		identity, ok := identities[row.UserID]
		if !ok {
			continue
		}
		identity.Badges = append(identity.Badges, BadgeSummary{
			ID: row.BadgeID, Name: row.Name, ImageURL: row.ImageURL,
		})
		identities[row.UserID] = identity
	}

	titles, err := s.repo.equippedTitles(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range titles {
		identity, ok := identities[row.UserID]
		if !ok || identity.Title != nil {
			continue
		}
		identity.Title = &TitleSummary{ID: row.TitleID, Name: row.Name, Color: row.Color}
		identities[row.UserID] = identity
	}
	return identities, nil
}

type equippedBadgeRow struct {
	UserID   int64  `gorm:"column:user_id"`
	BadgeID  int64  `gorm:"column:badge_id"`
	Name     string `gorm:"column:name"`
	ImageURL string `gorm:"column:image_url"`
}

type equippedTitleRow struct {
	UserID  int64  `gorm:"column:user_id"`
	TitleID int64  `gorm:"column:title_id"`
	Name    string `gorm:"column:name"`
	Color   string `gorm:"column:color"`
}

// equippedBadges 一次查询多个用户已装备的徽章，按 sort_order、id 排序。
// 不筛选 is_active，保证下线的已装备徽章仍然展示。
func (r *Repository) equippedBadges(ctx context.Context, userIDs []int64) ([]equippedBadgeRow, error) {
	rows := make([]equippedBadgeRow, 0)
	err := r.db.WithContext(ctx).
		Table("user_badges AS ub").
		Select("ub.user_id, b.id AS badge_id, b.name, b.image_url").
		Joins("JOIN badges AS b ON b.id = ub.badge_id").
		Where("ub.user_id IN ? AND ub.is_equipped = ?", userIDs, true).
		Order("ub.user_id, b.sort_order, b.id").
		Scan(&rows).Error
	return rows, err
}

// equippedTitles 一次查询多个用户已装备的称号，按 sort_order、id 排序。
// 不筛选 is_active；调用方只取每个用户的第一条以满足“称号最多 1 个”。
func (r *Repository) equippedTitles(ctx context.Context, userIDs []int64) ([]equippedTitleRow, error) {
	rows := make([]equippedTitleRow, 0)
	err := r.db.WithContext(ctx).
		Table("user_titles AS ut").
		Select("ut.user_id, t.id AS title_id, t.name, t.color").
		Joins("JOIN titles AS t ON t.id = ut.title_id").
		Where("ut.user_id IN ? AND ut.is_equipped = ?", userIDs, true).
		Order("ut.user_id, t.sort_order, t.id").
		Scan(&rows).Error
	return rows, err
}
