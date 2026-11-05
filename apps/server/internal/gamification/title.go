package gamification

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// TitleUnlockType 是称号解锁条件的类型。
type TitleUnlockType string

const (
	TitleUnlockRating      TitleUnlockType = "rating"
	TitleUnlockACCount     TitleUnlockType = "ac_count"
	TitleUnlockAchievement TitleUnlockType = "achievement"
)

const maxTitlePriceCoins = 1_000_000

var titleColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// ListTitles 返回称号商店。游客只看到上架称号；登录用户会先补算，并额外看到
// 自己已解锁但已下线的称号；管理员 includeInactive=true 时看到全部。
// 每个称号都带当前用户视角的解锁、购买、装备状态与条件进度。
func (s *Service) ListTitles(ctx context.Context, v Viewer, includeInactive bool) ([]TitleItem, error) {
	if includeInactive && !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	if v.UserID > 0 {
		if err := s.EvaluateUser(ctx, v.UserID); err != nil {
			return nil, err
		}
	}
	titles, err := s.repo.ListTitles(ctx)
	if err != nil {
		return nil, err
	}
	userTitles := map[int64]UserTitle{}
	if v.UserID > 0 {
		userTitles, err = s.repo.UserTitlesForUser(ctx, v.UserID)
		if err != nil {
			return nil, err
		}
	}

	items := make([]TitleItem, 0, len(titles))
	for _, title := range titles {
		userTitle, isUnlocked := userTitles[title.ID]
		if !includeInactive && !title.IsActive && !isUnlocked {
			continue
		}
		unlockType, cfg, err := titleUnlockConfigFromModel(title)
		if err != nil {
			slog.Warn("跳过解锁配置损坏的称号", "titleId", title.ID, "error", err)
			continue
		}
		_, progress, err := s.evaluateTitleUnlock(ctx, v.UserID, unlockType, cfg)
		if err != nil {
			return nil, err
		}
		var item TitleItem
		if isUnlocked {
			item = toTitleItem(title, unlockType, cfg, &userTitle, progress)
		} else {
			item = toTitleItem(title, unlockType, cfg, nil, progress)
		}
		items = append(items, item)
	}
	return items, nil
}

// CreateTitle 创建称号；isActive 缺省为上架。
func (s *Service) CreateTitle(ctx context.Context, v Viewer, in TitleInput) (*Title, error) {
	if !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	in, err := normalizeTitleInput(in)
	if err != nil {
		return nil, err
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	title := &Title{
		Name:                  in.Name,
		Color:                 in.Color,
		UnlockConditionType:   string(in.UnlockConditionType),
		UnlockConditionConfig: titleUnlockConfigToJSON(in.UnlockConditionType, in.UnlockConditionConfig),
		PriceCoins:            in.PriceCoins,
		SortOrder:             in.SortOrder,
		IsActive:              active,
	}
	if err := s.repo.CreateTitle(ctx, title); err != nil {
		return nil, err
	}
	return title, nil
}

// UpdateTitle 更新称号；isActive 缺省时保留原值。
func (s *Service) UpdateTitle(ctx context.Context, v Viewer, id int64, in TitleInput) (*Title, error) {
	if !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	if id <= 0 {
		return nil, httpx.ErrNotFound
	}
	// 先确认资源存在，保持 404 优先于输入校验；实际写入仍在事务内重新加锁。
	if _, err := s.findTitle(ctx, id); err != nil {
		return nil, err
	}
	in, err := normalizeTitleInput(in)
	if err != nil {
		return nil, err
	}
	return s.repo.UpdateTitleLocked(ctx, id, func(existing *Title) error {
		existing.Name = in.Name
		existing.Color = in.Color
		existing.UnlockConditionType = string(in.UnlockConditionType)
		existing.UnlockConditionConfig = titleUnlockConfigToJSON(in.UnlockConditionType, in.UnlockConditionConfig)
		existing.PriceCoins = in.PriceCoins
		existing.SortOrder = in.SortOrder
		if in.IsActive != nil {
			existing.IsActive = *in.IsActive
		}
		return nil
	})
}

// DeleteTitle 软删除称号，保留已解锁、已购买记录与装备状态。
func (s *Service) DeleteTitle(ctx context.Context, v Viewer, id int64) error {
	if !v.IsAdmin {
		return httpx.ErrForbidden
	}
	if _, err := s.findTitle(ctx, id); err != nil {
		return err
	}
	return s.repo.DeactivateTitle(ctx, id)
}

// PurchaseTitle 购买称号；解锁状态、余额、流水与购买时间在同一事务内校验并写入。
// 已下线的称号只要已解锁仍可购买。
func (s *Service) PurchaseTitle(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return httpx.ErrUnauthorized
	}
	if id <= 0 {
		return httpx.ErrNotFound
	}
	return s.repo.PurchaseTitle(ctx, userID, id)
}

// EquipTitle 装备已购买的称号；同一事务内先卸下该用户全部称号，再装备目标称号。
func (s *Service) EquipTitle(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return httpx.ErrUnauthorized
	}
	if id <= 0 {
		return httpx.ErrNotFound
	}
	return s.repo.EquipTitle(ctx, userID, id)
}

// UnequipTitle 卸下目标称号；未装备时幂等成功。
func (s *Service) UnequipTitle(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return httpx.ErrUnauthorized
	}
	if id <= 0 {
		return httpx.ErrNotFound
	}
	return s.repo.UnequipTitle(ctx, userID, id)
}

// EvaluateTitles 补算用户的上架称号解锁状态。满足条件时写入 user_titles，
// 唯一约束保证重复或并发补算只插入一次；解锁不产生金币流水。
// 存储中的解锁配置损坏时记录警告并跳过该称号，不影响其余称号。
func (s *Service) EvaluateTitles(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	titles, err := s.repo.ListActiveTitles(ctx)
	if err != nil {
		return err
	}
	if len(titles) == 0 {
		return nil
	}
	unlocked, err := s.repo.UserTitlesForUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, title := range titles {
		if _, ok := unlocked[title.ID]; ok {
			continue
		}
		unlockType, cfg, err := titleUnlockConfigFromModel(title)
		if err != nil {
			slog.Warn("跳过解锁配置损坏的称号", "titleId", title.ID, "error", err)
			continue
		}
		done, _, err := s.evaluateTitleUnlock(ctx, userID, unlockType, cfg)
		if err != nil {
			return err
		}
		if !done {
			continue
		}
		if err := s.repo.UnlockTitle(ctx, userID, title.ID); err != nil {
			return err
		}
	}
	return nil
}

// evaluateTitleUnlock 计算当前用户对单个称号的解锁条件进度。
func (s *Service) evaluateTitleUnlock(ctx context.Context, userID int64, t TitleUnlockType, cfg TitleUnlockConfig) (bool, TitleProgress, error) {
	progress := TitleProgress{Type: t}
	switch t {
	case TitleUnlockRating:
		var current int64
		if userID > 0 {
			rating, err := s.repo.UserRating(ctx, userID)
			if err != nil {
				return false, progress, err
			}
			current = rating
		}
		progress.Current = current
		progress.Target = int64(cfg.Rating)
	case TitleUnlockACCount:
		var current int64
		if userID > 0 {
			count, err := s.repo.CountAcceptedProblems(ctx, userID)
			if err != nil {
				return false, progress, err
			}
			current = count
		}
		progress.Current = current
		progress.Target = cfg.Threshold
	case TitleUnlockAchievement:
		var current int64
		if userID > 0 {
			has, err := s.repo.UserHasAchievementCode(ctx, userID, cfg.AchievementCode)
			if err != nil {
				return false, progress, err
			}
			if has {
				current = 1
			}
		}
		progress.Current = current
		progress.Target = 1
		progress.AchievementCode = cfg.AchievementCode
	default:
		return false, progress, httpx.ErrValidation("未知的称号解锁条件类型")
	}
	progress.Done = progress.Current >= progress.Target
	return progress.Done, progress, nil
}

func (s *Service) findTitle(ctx context.Context, id int64) (*Title, error) {
	if id <= 0 {
		return nil, httpx.ErrNotFound
	}
	title, err := s.repo.FindTitle(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return title, nil
}

// normalizeTitleInput 校验并规范化管理员输入，并只保留当前解锁类型使用的配置字段。
func normalizeTitleInput(in TitleInput) (TitleInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Color = strings.TrimSpace(in.Color)
	in.UnlockConditionConfig.AchievementCode = strings.TrimSpace(in.UnlockConditionConfig.AchievementCode)

	if err := validateText("称号名称", in.Name, 1, 100); err != nil {
		return in, err
	}
	if !titleColorPattern.MatchString(in.Color) {
		return in, httpx.ErrValidation("称号颜色必须为 #RRGGBB 格式")
	}
	if in.PriceCoins < 0 {
		return in, httpx.ErrValidation("称号价格不能为负数")
	}
	if in.PriceCoins > maxTitlePriceCoins {
		return in, httpx.ErrValidation("称号价格不能超过 " + strconv.Itoa(maxTitlePriceCoins))
	}
	if err := validateTitleUnlockConfig(in.UnlockConditionType, in.UnlockConditionConfig); err != nil {
		return in, err
	}

	switch in.UnlockConditionType {
	case TitleUnlockRating:
		in.UnlockConditionConfig = TitleUnlockConfig{Rating: in.UnlockConditionConfig.Rating}
	case TitleUnlockACCount:
		in.UnlockConditionConfig = TitleUnlockConfig{Threshold: in.UnlockConditionConfig.Threshold}
	case TitleUnlockAchievement:
		in.UnlockConditionConfig = TitleUnlockConfig{AchievementCode: in.UnlockConditionConfig.AchievementCode}
	}
	return in, nil
}

// validateTitleUnlockConfig 校验称号解锁条件；阈值与 Rating 必须为正整数，
// 成就 code 必须符合成就 code 格式。
func validateTitleUnlockConfig(t TitleUnlockType, cfg TitleUnlockConfig) error {
	switch t {
	case TitleUnlockRating:
		if cfg.Rating <= 0 {
			return httpx.ErrValidation("称号解锁 Rating 必须为正整数")
		}
		return nil
	case TitleUnlockACCount:
		if cfg.Threshold <= 0 {
			return httpx.ErrValidation("称号解锁条件阈值必须为正整数")
		}
		return nil
	case TitleUnlockAchievement:
		if !achievementCodePattern.MatchString(cfg.AchievementCode) {
			return httpx.ErrValidation("称号解锁成就 code 格式不正确")
		}
		return nil
	default:
		return httpx.ErrValidation("未知的称号解锁条件类型")
	}
}

// titleUnlockConfigFromModel 解析并校验存储层称号配置；损坏时返回错误供调用方跳过。
func titleUnlockConfigFromModel(title Title) (TitleUnlockType, TitleUnlockConfig, error) {
	t := TitleUnlockType(title.UnlockConditionType)
	cfg, err := titleUnlockConfigFromJSON(title.UnlockConditionConfig)
	if err != nil {
		return t, cfg, err
	}
	if err := validateTitleUnlockConfig(t, cfg); err != nil {
		return t, cfg, err
	}
	return t, cfg, nil
}

func titleUnlockConfigToJSON(t TitleUnlockType, cfg TitleUnlockConfig) dbtypes.JSON[map[string]any] {
	switch t {
	case TitleUnlockRating:
		return dbtypes.JSON[map[string]any]{Data: map[string]any{"rating": cfg.Rating}}
	case TitleUnlockACCount:
		return dbtypes.JSON[map[string]any]{Data: map[string]any{"threshold": cfg.Threshold}}
	case TitleUnlockAchievement:
		return dbtypes.JSON[map[string]any]{Data: map[string]any{"achievementCode": cfg.AchievementCode}}
	default:
		return dbtypes.JSON[map[string]any]{Data: map[string]any{}}
	}
}

func titleUnlockConfigFromJSON(raw dbtypes.JSON[map[string]any]) (TitleUnlockConfig, error) {
	data, err := json.Marshal(raw.Data)
	if err != nil {
		return TitleUnlockConfig{}, err
	}
	var cfg TitleUnlockConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return TitleUnlockConfig{}, err
	}
	return cfg, nil
}

func toTitleItem(title Title, t TitleUnlockType, cfg TitleUnlockConfig, userTitle *UserTitle, progress TitleProgress) TitleItem {
	item := TitleItem{
		ID:                    title.ID,
		Name:                  title.Name,
		Color:                 title.Color,
		UnlockConditionType:   t,
		UnlockConditionConfig: cfg,
		PriceCoins:            title.PriceCoins,
		SortOrder:             title.SortOrder,
		IsActive:              title.IsActive,
		Progress:              progress,
	}
	if userTitle != nil {
		unlockedAt := userTitle.UnlockedAt.UTC()
		item.Unlocked = true
		item.UnlockedAt = &unlockedAt
		item.Equipped = userTitle.IsEquipped
		if userTitle.PurchasedAt != nil {
			purchasedAt := userTitle.PurchasedAt.UTC()
			item.Purchased = true
			item.PurchasedAt = &purchasedAt
		}
	}
	return item
}

// TitleItemFromModel 将存储层称号转换为 API 表示；用于创建/更新响应，
// 此时没有当前用户的解锁、购买与装备状态，但保留条件目标值。
func TitleItemFromModel(title Title) TitleItem {
	t := TitleUnlockType(title.UnlockConditionType)
	cfg, err := titleUnlockConfigFromJSON(title.UnlockConditionConfig)
	if err != nil {
		return toTitleItem(title, t, TitleUnlockConfig{}, nil, TitleProgress{Type: t})
	}
	progress := TitleProgress{Type: t}
	switch t {
	case TitleUnlockRating:
		progress.Target = int64(cfg.Rating)
	case TitleUnlockACCount:
		progress.Target = cfg.Threshold
	case TitleUnlockAchievement:
		progress.Target = 1
		progress.AchievementCode = cfg.AchievementCode
	}
	return toTitleItem(title, t, cfg, nil, progress)
}
