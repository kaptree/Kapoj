package gamification

import (
	"context"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// ConditionType 是成就条件的类型。
type ConditionType string

const (
	ConditionACCount        ConditionType = "ac_count"
	ConditionContestCount   ConditionType = "contest_count"
	ConditionLoginStreak    ConditionType = "login_streak"
	ConditionCreatedContest ConditionType = "created_contest"
	ConditionAllOf          ConditionType = "all_of"
)

const (
	minAllOfConditions = 2
	maxAllOfConditions = 8
)

// Condition 是 all_of 中的单个子条件。
type Condition struct {
	Type      ConditionType `json:"type"`
	Threshold int64         `json:"threshold"`
}

// ConditionConfig 是成就条件配置；单条件使用 Threshold，all_of 使用 Conditions。
type ConditionConfig struct {
	Threshold  int64       `json:"threshold"`
	Conditions []Condition `json:"conditions"`
}

// ConditionProgress 是给前端展示的条件进度。
type ConditionProgress struct {
	Type    ConditionType `json:"type"`
	Current int64         `json:"current"`
	Target  int64         `json:"target"`
	Done    bool          `json:"done"`
}

// ValidateCondition 校验成就条件配置。条件阈值必须为正整数；all_of 必须包含
// 2–8 个非 all_of 的子条件，避免任意嵌套。
func ValidateCondition(t ConditionType, c ConditionConfig) error {
	switch t {
	case ConditionACCount, ConditionContestCount, ConditionLoginStreak, ConditionCreatedContest:
		if c.Threshold <= 0 {
			return httpx.ErrValidation("条件阈值必须为正整数")
		}
		return nil
	case ConditionAllOf:
		if len(c.Conditions) < minAllOfConditions {
			return httpx.ErrValidation("all_of 条件至少需要 2 项")
		}
		if len(c.Conditions) > maxAllOfConditions {
			return httpx.ErrValidation("all_of 条件最多 8 项")
		}
		for _, sub := range c.Conditions {
			if sub.Type == ConditionAllOf {
				return httpx.ErrValidation("all_of 不允许嵌套")
			}
			if err := ValidateCondition(sub.Type, ConditionConfig{Threshold: sub.Threshold}); err != nil {
				return err
			}
		}
		return nil
	default:
		return httpx.ErrValidation("未知的条件类型")
	}
}

// EvaluateCondition 重新计算用户的单个条件或 all_of 组合条件。
func (s *Service) EvaluateCondition(ctx context.Context, userID int64, t ConditionType, c ConditionConfig) (bool, []ConditionProgress, error) {
	if err := ValidateCondition(t, c); err != nil {
		return false, nil, err
	}
	if t == ConditionAllOf {
		progress := make([]ConditionProgress, 0, len(c.Conditions))
		done := true
		for _, sub := range c.Conditions {
			subDone, subProgress, err := s.evaluateSingleCondition(ctx, userID, sub.Type, sub.Threshold)
			if err != nil {
				return false, nil, err
			}
			progress = append(progress, subProgress...)
			if !subDone {
				done = false
			}
		}
		return done, progress, nil
	}
	return s.evaluateSingleCondition(ctx, userID, t, c.Threshold)
}

func (s *Service) evaluateSingleCondition(ctx context.Context, userID int64, t ConditionType, threshold int64) (bool, []ConditionProgress, error) {
	current, err := s.conditionCurrent(ctx, userID, t)
	if err != nil {
		return false, nil, err
	}
	progress := []ConditionProgress{{
		Type:    t,
		Current: current,
		Target:  threshold,
		Done:    current >= threshold,
	}}
	return progress[0].Done, progress, nil
}

func (s *Service) conditionCurrent(ctx context.Context, userID int64, t ConditionType) (int64, error) {
	switch t {
	case ConditionACCount:
		return s.repo.CountAcceptedProblems(ctx, userID)
	case ConditionContestCount:
		return s.repo.CountContestParticipations(ctx, userID)
	case ConditionCreatedContest:
		return s.repo.CountCreatedContests(ctx, userID)
	case ConditionLoginStreak:
		today := time.Now().In(time.Local).Format(loginDateLayout)
		return s.repo.CurrentLoginStreak(ctx, userID, today)
	default:
		return 0, httpx.ErrValidation("未知的条件类型")
	}
}
