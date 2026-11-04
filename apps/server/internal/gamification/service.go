package gamification

import (
	"context"
	"time"
)

// Service 是游戏化领域服务。
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// EvaluateUser 重新计算用户的成就与任务状态。本任务先占位，
// 成就与任务补算分别在后续任务填充。
func (s *Service) EvaluateUser(ctx context.Context, userID int64) error {
	return nil
}

// OnLogin 记录服务端本地日期的登录日；事件发布在后续任务接入。
func (s *Service) OnLogin(ctx context.Context, userID int64) error {
	today := time.Now().In(time.Local).Format(loginDateLayout)
	return s.repo.RecordLoginDay(ctx, userID, today)
}
