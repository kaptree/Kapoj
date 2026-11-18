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

// acceptedProblemReward 是每道题首次 AC 的金币数。
const acceptedProblemReward int64 = 3

// AcceptedSubmission 是从数据库重新读取的 AC 提交事实。
type AcceptedSubmission struct {
	ID        int64
	UserID    int64
	ProblemID int64
	CreatedAt time.Time
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
		// 同一用户同一题只发一次：只有本次 AC 之前没有更早的 AC 时才入账。
		// 唯一索引 (user_id, reason, ref_type, ref_id) 仍作为并发兜底。
		earlier, err := s.repo.HasEarlierAccepted(ctx, sub.UserID, sub.ProblemID, sub.CreatedAt, sub.ID)
		if err != nil {
			return err
		}
		if !earlier {
			if _, err := s.repo.CreditCoins(ctx, sub.UserID, acceptedProblemReward,
				"problem_accepted", "problem", sub.ProblemID); err != nil {
				return err
			}
		}
		return s.EvaluateUser(ctx, sub.UserID)
	case gamequeue.EventUserLogin, gamequeue.EventContestJoin:
		return s.EvaluateUser(ctx, ev.UserID)
	}
	return nil
}

// FindAcceptedSubmission 读取提交的 id、user_id、problem_id 与创建时间；
// 不存在或未 AC 都返回 ErrSubmissionNotFound，避免对过期事件重试。
func (r *Repository) FindAcceptedSubmission(ctx context.Context, submissionID int64) (*AcceptedSubmission, error) {
	var sub AcceptedSubmission
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, problem_id, created_at
		 FROM submissions WHERE id = ? AND status = 'accepted' AND agent_id IS NULL`,
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

// HasEarlierAccepted 判断同一用户同一题在 createdAt 之前是否已有 AC；同一时刻
// 用提交 ID 决定先后，保证并发事件最多只有一个被认定为首次 AC。
func (r *Repository) HasEarlierAccepted(
	ctx context.Context, userID, problemID int64, createdAt time.Time, submissionID int64,
) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(
		`SELECT EXISTS(
			SELECT 1 FROM submissions
			WHERE user_id = ? AND problem_id = ? AND status = 'accepted'
			  AND agent_id IS NULL
			  AND (created_at < ? OR (created_at = ? AND id < ?))
		)`, userID, problemID, createdAt, createdAt, submissionID,
	).Scan(&exists).Error
	return exists, err
}
