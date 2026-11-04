package gamification

import (
	"context"
	"errors"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
)

// ErrSubmissionNotFound 表示事件引用的提交不存在或已不是 accepted。
// worker 视其为无需重试的过期事件。
var ErrSubmissionNotFound = errors.New("accepted submission not found")

// EventPublisher 是游戏化事件发布器；测试与未配置 worker 时可缺省。
type EventPublisher interface {
	Publish(ctx context.Context, ev gamequeue.Event) error
}

// AcceptedSubmission 是从数据库重新读取的 AC 提交事实。
type AcceptedSubmission struct {
	ID     int64
	UserID int64
}

// OnLogin 先记录服务端本地日期的登录日，再投递 user.login 事件。
// 发布失败返回给 handler 记录日志，不影响登录响应。
func (s *Service) OnLogin(ctx context.Context, userID int64) error {
	today := time.Now().In(time.Local).Format(loginDateLayout)
	if err := s.repo.RecordLoginDay(ctx, userID, today); err != nil {
		return err
	}
	return s.publish(ctx, gamequeue.Event{Type: gamequeue.EventUserLogin, UserID: userID})
}

// OnContestJoin 在报名写库后投递 contest.join 事件；发布失败由 handler 记录日志。
func (s *Service) OnContestJoin(ctx context.Context, userID, contestID int64) error {
	return s.publish(ctx, gamequeue.Event{
		Type: gamequeue.EventContestJoin, UserID: userID, ContestID: contestID,
	})
}

func (s *Service) publish(ctx context.Context, ev gamequeue.Event) error {
	if s.publisher == nil {
		return nil
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	return s.publisher.Publish(ctx, ev)
}

// HandleGameEvent 重新读取事实并运行同一套幂等结算；未知类型直接忽略。
func (s *Service) HandleGameEvent(ctx context.Context, ev gamequeue.Event) error {
	switch ev.Type {
	case gamequeue.EventSubmissionAccepted:
		sub, err := s.repo.FindAcceptedSubmission(ctx, ev.SubmissionID)
		if errors.Is(err, ErrSubmissionNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return s.EvaluateUser(ctx, sub.UserID)
	case gamequeue.EventUserLogin, gamequeue.EventContestJoin:
		return s.EvaluateUser(ctx, ev.UserID)
	}
	return nil
}

// FindAcceptedSubmission 读取提交的 id、user_id 与状态；不存在或未 AC
// 都返回 ErrSubmissionNotFound，避免对过期事件重试。
func (r *Repository) FindAcceptedSubmission(ctx context.Context, submissionID int64) (*AcceptedSubmission, error) {
	var sub AcceptedSubmission
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id FROM submissions WHERE id = ? AND status = 'accepted'`,
		submissionID,
	).Scan(&sub).Error
	if err != nil {
		return nil, err
	}
	if sub.ID == 0 {
		return nil, ErrSubmissionNotFound
	}
	return &sub, nil
}
