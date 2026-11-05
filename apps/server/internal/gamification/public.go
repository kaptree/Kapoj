package gamification

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
