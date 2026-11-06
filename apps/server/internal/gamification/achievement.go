package gamification

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// EvaluateUser 重新计算用户所有上架成就、已领取任务与上架称号；满足条件的成就
// 原子解锁并发奖，截止前集齐题目的任务原子完成并发奖，满足解锁条件的称号写入
// 解锁记录（不扣金币）。只使用仓储读取，避免调用 ListAchievements 造成递归。
func (s *Service) EvaluateUser(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	if err := s.EvaluateTasks(ctx, userID); err != nil {
		return err
	}
	achievements, err := s.repo.ListAchievements(ctx, false)
	if err != nil {
		return err
	}
	if len(achievements) > 0 {
		unlocked, err := s.repo.UnlockedAchievementIDs(ctx, userID)
		if err != nil {
			return err
		}
		for _, a := range achievements {
			if _, ok := unlocked[a.ID]; ok {
				continue
			}
			cfg, err := conditionConfigFromJSON(a.ConditionConfig)
			if err != nil {
				slog.Warn("跳过条件配置损坏的成就", "achievementId", a.ID, "error", err)
				continue
			}
			done, _, err := s.EvaluateCondition(ctx, userID, ConditionType(a.ConditionType), cfg)
			if err != nil {
				return err
			}
			if !done {
				continue
			}
			if _, err := s.repo.GrantAchievement(ctx, userID, a); err != nil {
				return err
			}
		}
	}
	// 成就解锁后可能满足 achievement 类型称号的解锁条件，因此称号补算最后执行。
	return s.EvaluateTitles(ctx, userID)
}

// ListAchievements 返回成就列表。游客只能看到上架成就；登录用户会先补算，
// 并额外看到自己已解锁但已下线的成就；管理员传 includeInactive 时看到全部。
func (s *Service) ListAchievements(ctx context.Context, v Viewer, includeInactive bool) ([]AchievementItem, error) {
	if includeInactive && !v.CanManageAchievements {
		return nil, httpx.ErrForbidden
	}
	if v.UserID > 0 {
		if err := s.EvaluateUser(ctx, v.UserID); err != nil {
			return nil, err
		}
	}
	var unlocked map[int64]time.Time
	if v.UserID > 0 {
		var err error
		unlocked, err = s.repo.UnlockedAchievementIDs(ctx, v.UserID)
		if err != nil {
			return nil, err
		}
	}

	all := includeInactive || v.UserID > 0
	achievements, err := s.repo.ListAchievements(ctx, all)
	if err != nil {
		return nil, err
	}
	items := make([]AchievementItem, 0, len(achievements))
	for _, a := range achievements {
		unlockedAt, isUnlocked := unlocked[a.ID]
		if !a.IsActive && !isUnlocked && !includeInactive {
			continue
		}
		cfg, err := conditionConfigFromJSON(a.ConditionConfig)
		if err != nil {
			slog.Warn("跳过条件配置损坏的成就", "achievementId", a.ID, "error", err)
			continue
		}
		_, progress, err := s.EvaluateCondition(ctx, v.UserID, ConditionType(a.ConditionType), cfg)
		if err != nil {
			return nil, err
		}
		var at *time.Time
		if isUnlocked {
			at = &unlockedAt
		}
		items = append(items, toAchievementItem(a, cfg, at, progress))
	}
	return items, nil
}

// CreateAchievement 创建成就；code 必须符合格式，isActive 缺省为 true。
func (s *Service) CreateAchievement(ctx context.Context, v Viewer, in AchievementInput) (*Achievement, error) {
	if !v.CanManageAchievements {
		return nil, httpx.ErrForbidden
	}
	in, err := normalizeAchievementInput(in, true)
	if err != nil {
		return nil, err
	}
	if err := s.validateRewardBadge(ctx, in.RewardBadgeID, nil); err != nil {
		return nil, err
	}
	taken, err := s.repo.AchievementCodeTaken(ctx, in.Code, 0)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, httpx.ErrValidation("成就 code 已存在")
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	a := &Achievement{
		Code:            in.Code,
		Name:            in.Name,
		Description:     in.Description,
		IconURL:         in.IconURL,
		ConditionType:   string(in.ConditionType),
		ConditionConfig: conditionConfigToJSON(in.ConditionType, in.ConditionConfig),
		RewardCoins:     in.RewardCoins,
		RewardBadgeID:   in.RewardBadgeID,
		SortOrder:       in.SortOrder,
		IsActive:        active,
	}
	if err := s.repo.CreateAchievement(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAchievement 更新成就；code 创建后不可修改，输入为空时保持原值。
func (s *Service) UpdateAchievement(ctx context.Context, v Viewer, id int64, in AchievementInput) (*Achievement, error) {
	if !v.CanManageAchievements {
		return nil, httpx.ErrForbidden
	}
	existing, err := s.repo.FindAchievement(ctx, id)
	if err != nil {
		return nil, err
	}
	in, err = normalizeAchievementInput(in, false)
	if err != nil {
		return nil, err
	}
	if in.Code != "" && in.Code != existing.Code {
		return nil, httpx.ErrValidation("成就 code 创建后不可修改")
	}
	if err := s.validateRewardBadge(ctx, in.RewardBadgeID, existing.RewardBadgeID); err != nil {
		return nil, err
	}

	existing.Name = in.Name
	existing.Description = in.Description
	existing.IconURL = in.IconURL
	existing.ConditionType = string(in.ConditionType)
	existing.ConditionConfig = conditionConfigToJSON(in.ConditionType, in.ConditionConfig)
	existing.RewardCoins = in.RewardCoins
	// nil 表示保持原奖励徽章；M3-B 暂不支持清除奖励徽章。
	if in.RewardBadgeID != nil {
		existing.RewardBadgeID = in.RewardBadgeID
	}
	existing.SortOrder = in.SortOrder
	if in.IsActive != nil {
		existing.IsActive = *in.IsActive
	}
	if err := s.repo.UpdateAchievement(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteAchievement 软删除成就，保留已解锁记录与金币流水。
func (s *Service) DeleteAchievement(ctx context.Context, v Viewer, id int64) error {
	if !v.CanManageAchievements {
		return httpx.ErrForbidden
	}
	if _, err := s.repo.FindAchievement(ctx, id); err != nil {
		return err
	}
	return s.repo.DeactivateAchievement(ctx, id)
}

// validateRewardBadge 要求奖励徽章存在且上架；nil 表示不配置奖励徽章。
// 更新成就时，与现有奖励徽章相同的 id 允许保留（即使徽章后来下线），避免
// 改名等无关更新被已下线的旧奖励阻塞；新绑定的徽章仍必须上架。
func (s *Service) validateRewardBadge(ctx context.Context, badgeID, currentBadgeID *int64) error {
	if badgeID == nil {
		return nil
	}
	if *badgeID <= 0 {
		return httpx.ErrValidation("奖励徽章不存在")
	}
	badge, err := s.repo.FindBadge(ctx, *badgeID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.ErrValidation("奖励徽章不存在")
	}
	if err != nil {
		return err
	}
	if !badge.IsActive {
		if currentBadgeID != nil && *currentBadgeID == *badgeID {
			return nil
		}
		return httpx.ErrValidation("奖励徽章已下线")
	}
	return nil
}
