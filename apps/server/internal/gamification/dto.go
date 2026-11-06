package gamification

import (
	"encoding/json"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

// Viewer 是游戏化读取接口的调用者；游客 UserID 为 0。各管理能力字段由
// 权限解析派生，授权判断必须使用具体字段而非组合字段。
type Viewer struct {
	UserID                int64
	CanManageAchievements bool
	CanManageTasks        bool
	CanManageBadges       bool
	CanManageTitles       bool
	CanAdjustCoins        bool
}

// AchievementInput 是管理员创建或更新成就的输入。
type AchievementInput struct {
	Code            string          `json:"code"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	IconURL         string          `json:"iconUrl"`
	ConditionType   ConditionType   `json:"conditionType"`
	ConditionConfig ConditionConfig `json:"conditionConfig"`
	RewardCoins     int64           `json:"rewardCoins"`
	RewardBadgeID   *int64          `json:"rewardBadgeId"`
	SortOrder       int             `json:"sortOrder"`
	IsActive        *bool           `json:"isActive"`
}

// AchievementItem 是成就的 API 表示，包含当前用户视角的解锁状态与进度。
type AchievementItem struct {
	ID              int64               `json:"id"`
	Code            string              `json:"code"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	IconURL         string              `json:"iconUrl"`
	ConditionType   ConditionType       `json:"conditionType"`
	ConditionConfig ConditionConfig     `json:"conditionConfig"`
	RewardCoins     int64               `json:"rewardCoins"`
	RewardBadgeID   *int64              `json:"rewardBadgeId"`
	SortOrder       int                 `json:"sortOrder"`
	IsActive        bool                `json:"isActive"`
	Unlocked        bool                `json:"unlocked"`
	UnlockedAt      *time.Time          `json:"unlockedAt"`
	Progress        []ConditionProgress `json:"progress"`
}

// TitleUnlockConfig 是称号解锁条件配置；每种类型只使用对应字段。
type TitleUnlockConfig struct {
	Rating          int    `json:"rating,omitempty"`
	Threshold       int64  `json:"threshold,omitempty"`
	AchievementCode string `json:"achievementCode,omitempty"`
}

// TitleInput 是管理员创建或更新称号的输入。
type TitleInput struct {
	Name                  string            `json:"name"`
	Color                 string            `json:"color"`
	UnlockConditionType   TitleUnlockType   `json:"unlockConditionType"`
	UnlockConditionConfig TitleUnlockConfig `json:"unlockConditionConfig"`
	PriceCoins            int64             `json:"priceCoins"`
	SortOrder             int               `json:"sortOrder"`
	IsActive              *bool             `json:"isActive"`
}

// TitleProgress 是称号解锁条件的当前进度。
type TitleProgress struct {
	Type            TitleUnlockType `json:"type"`
	Current         int64           `json:"current"`
	Target          int64           `json:"target"`
	Done            bool            `json:"done"`
	AchievementCode string          `json:"achievementCode"`
}

// TitleItem 是称号的 API 表示，包含当前用户的解锁、购买与装备状态。
type TitleItem struct {
	ID                    int64             `json:"id"`
	Name                  string            `json:"name"`
	Color                 string            `json:"color"`
	UnlockConditionType   TitleUnlockType   `json:"unlockConditionType"`
	UnlockConditionConfig TitleUnlockConfig `json:"unlockConditionConfig"`
	PriceCoins            int64             `json:"priceCoins"`
	SortOrder             int               `json:"sortOrder"`
	IsActive              bool              `json:"isActive"`
	Unlocked              bool              `json:"unlocked"`
	UnlockedAt            *time.Time        `json:"unlockedAt"`
	Purchased             bool              `json:"purchased"`
	PurchasedAt           *time.Time        `json:"purchasedAt"`
	Equipped              bool              `json:"equipped"`
	Progress              TitleProgress     `json:"progress"`
}

// BadgeInput 是管理员创建或更新徽章的输入。
// Stock 使用指针区分“未提交”和 0：更新时省略表示保留当前库存，创建时省略为 0。
// ImageURL 在更新时为空表示保留现有图片（图片上传接口会单独写入）。
type BadgeInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageURL    string `json:"imageUrl"`
	Category    string `json:"category"`
	PriceCoins  int64  `json:"priceCoins"`
	IsLimited   bool   `json:"isLimited"`
	Stock       *int64 `json:"stock"`
	SortOrder   int    `json:"sortOrder"`
	IsActive    *bool  `json:"isActive"`
}

// BadgeItem 是徽章的 API 表示，包含当前用户的拥有与装备状态。
type BadgeItem struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	ImageURL    string     `json:"imageUrl"`
	Category    string     `json:"category"`
	PriceCoins  int64      `json:"priceCoins"`
	IsLimited   bool       `json:"isLimited"`
	Stock       int64      `json:"stock"`
	SortOrder   int        `json:"sortOrder"`
	IsActive    bool       `json:"isActive"`
	Owned       bool       `json:"owned"`
	Equipped    bool       `json:"equipped"`
	PurchasedAt *time.Time `json:"purchasedAt"`
}

// BadgeImage 是公开图片接口返回的原始图片。
type BadgeImage struct {
	Data        []byte
	ContentType string
}

// CoinTransactionItem 是金币流水的 API 表示。
type CoinTransactionItem struct {
	ID        int64     `json:"id"`
	Amount    int64     `json:"amount"`
	Type      CoinType  `json:"type"`
	Reason    string    `json:"reason"`
	RefType   string    `json:"refType"`
	RefID     int64     `json:"refId"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

// CoinTransactionPage 是金币流水分页响应，余额以服务端为准。
type CoinTransactionPage struct {
	Balance int64                 `json:"balance"`
	Total   int64                 `json:"total"`
	Items   []CoinTransactionItem `json:"items"`
}

// MarshalJSON 只输出条件配置中实际使用的字段，避免单条件出现 conditions: null、
// all_of 出现 threshold: 0。
func (c ConditionConfig) MarshalJSON() ([]byte, error) {
	if len(c.Conditions) > 0 {
		return json.Marshal(struct {
			Conditions []Condition `json:"conditions"`
		}{Conditions: c.Conditions})
	}
	return json.Marshal(struct {
		Threshold int64 `json:"threshold"`
	}{Threshold: c.Threshold})
}

func toCoinTransactionItem(tx CoinTransaction) CoinTransactionItem {
	return CoinTransactionItem{
		ID: tx.ID, Amount: tx.Amount, Type: tx.Type, Reason: tx.Reason,
		RefType: tx.RefType, RefID: tx.RefID, Note: tx.Note,
		CreatedAt: tx.CreatedAt.UTC(),
	}
}

func toAchievementItem(a Achievement, cfg ConditionConfig, unlockedAt *time.Time, progress []ConditionProgress) AchievementItem {
	item := AchievementItem{
		ID: a.ID, Code: a.Code, Name: a.Name, Description: a.Description,
		IconURL: a.IconURL, ConditionType: ConditionType(a.ConditionType),
		ConditionConfig: cfg, RewardCoins: a.RewardCoins,
		RewardBadgeID: a.RewardBadgeID, SortOrder: a.SortOrder, IsActive: a.IsActive,
		Unlocked: unlockedAt != nil, UnlockedAt: unlockedAt, Progress: progress,
	}
	if item.Progress == nil {
		item.Progress = []ConditionProgress{}
	}
	return item
}

// AchievementItemFromModel 将存储层成就转换为 API 表示；用于创建/更新响应，
// 此时没有当前用户的解锁信息与进度。
func AchievementItemFromModel(a Achievement) AchievementItem {
	cfg, _ := conditionConfigFromJSON(a.ConditionConfig)
	return toAchievementItem(a, cfg, nil, nil)
}

// conditionConfigToJSON 把类型化条件配置写入存储列，保持设计文档中的形状。
func conditionConfigToJSON(t ConditionType, cfg ConditionConfig) dbtypes.JSON[map[string]any] {
	if t == ConditionAllOf {
		conditions := make([]map[string]any, 0, len(cfg.Conditions))
		for _, sub := range cfg.Conditions {
			conditions = append(conditions, map[string]any{
				"type": string(sub.Type), "threshold": sub.Threshold,
			})
		}
		return dbtypes.JSON[map[string]any]{Data: map[string]any{"conditions": conditions}}
	}
	return dbtypes.JSON[map[string]any]{Data: map[string]any{"threshold": cfg.Threshold}}
}

// conditionConfigFromJSON 把存储列中的 JSON 转回类型化条件配置。
func conditionConfigFromJSON(raw dbtypes.JSON[map[string]any]) (ConditionConfig, error) {
	data, err := json.Marshal(raw.Data)
	if err != nil {
		return ConditionConfig{}, err
	}
	var cfg ConditionConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ConditionConfig{}, err
	}
	return cfg, nil
}
