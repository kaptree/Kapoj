package social

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

// insertNotification 插入通知；dedupe_key 冲突时静默忽略，保证幂等。
func insertNotification(tx *gorm.DB, n *Notification) error {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().In(time.Local)
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(n).Error
}

// ListNotifications 返回用户自己的通知，按 created_at DESC, id DESC 排序。
func (s *Service) ListNotifications(ctx context.Context, userID int64, page, size int) (*NotificationPage, error) {
	if userID <= 0 {
		return nil, httpx.ErrValidation("用户 ID 必须为正数")
	}
	page, size = normalizePaging(page, size)

	var total int64
	if err := s.db.WithContext(ctx).Model(&Notification{}).
		Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, err
	}

	var rows []struct {
		ID         int64      `gorm:"column:id"`
		Type       string     `gorm:"column:type"`
		ActorID    int64      `gorm:"column:actor_id"`
		TargetType string     `gorm:"column:target_type"`
		TargetID   int64      `gorm:"column:target_id"`
		Link       string     `gorm:"column:link"`
		ReadAt     *time.Time `gorm:"column:read_at"`
		CreatedAt  time.Time  `gorm:"column:created_at"`
		Username   string     `gorm:"column:username"`
		Nickname   string     `gorm:"column:nickname"`
		AvatarURL  string     `gorm:"column:avatar_url"`
		Rating     int        `gorm:"column:rating"`
	}
	err := s.db.WithContext(ctx).
		Table("notifications AS n").
		Select("n.id, n.type, n.actor_id, n.target_type, n.target_id, n.link, n.read_at, n.created_at, u.username, u.nickname, u.avatar_url, u.rating").
		Joins("LEFT JOIN users AS u ON u.id = n.actor_id").
		Where("n.user_id = ?", userID).
		Order("n.created_at DESC, n.id DESC").
		Offset((page - 1) * size).Limit(size).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]NotificationItem, 0, len(rows))
	for _, r := range rows {
		var actor *UserSummary
		if r.Username != "" {
			actor = &UserSummary{ID: r.ActorID, Username: r.Username, Nickname: r.Nickname, AvatarURL: r.AvatarURL, Rating: r.Rating}
		}
		items = append(items, NotificationItem{
			ID:         r.ID,
			Type:       r.Type,
			Actor:      actor,
			TargetType: r.TargetType,
			TargetID:   r.TargetID,
			Link:       r.Link,
			ReadAt:     r.ReadAt,
			CreatedAt:  r.CreatedAt.UTC(),
		})
	}
	return &NotificationPage{Total: total, Items: items}, nil
}

// UnreadCount 返回用户未读通知数。
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, httpx.ErrValidation("用户 ID 必须为正数")
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// MarkRead 标记单条通知已读；非本人返回 404；重复标记幂等。
func (s *Service) MarkRead(ctx context.Context, userID, notificationID int64) error {
	if userID <= 0 {
		return httpx.ErrValidation("用户 ID 必须为正数")
	}
	if notificationID <= 0 {
		return httpx.ErrNotFound
	}
	res := s.db.WithContext(ctx).Model(&Notification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Update("read_at", time.Now().In(time.Local))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// MarkAllRead 将用户全部未读通知标记为已读；幂等。
func (s *Service) MarkAllRead(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return httpx.ErrValidation("用户 ID 必须为正数")
	}
	return s.db.WithContext(ctx).Model(&Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", time.Now().In(time.Local)).Error
}

// OnSolutionReply 在论坛回复创建成功后调用：给帖子作者写通知（自己回复自己
// 不通知），并 best-effort 记回复活跃度（自己回复自己也计分）。
func (s *Service) OnSolutionReply(ctx context.Context, postID, replyID, actorID int64) error {
	if postID <= 0 || replyID <= 0 || actorID <= 0 {
		return httpx.ErrValidation("参数必须为正数")
	}
	var post problem.SolutionPost
	if err := s.db.WithContext(ctx).Where("id = ?", postID).First(&post).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		return err
	}

	if s.activity != nil {
		if err := s.activity.RecordSolutionReply(ctx, actorID, replyID); err != nil {
			slog.Warn("social: record reply activity failed", "actor", actorID, "reply", replyID, "err", err)
		}
	}

	if post.UserID == actorID {
		return nil
	}

	return insertNotification(s.db, &Notification{
		UserID:     post.UserID,
		ActorID:    actorID,
		Type:       TypeSolutionReply,
		DedupeKey:  fmt.Sprintf("reply:%d", replyID),
		TargetType: targetTypeSolutionPost,
		TargetID:   postID,
		Link:       fmt.Sprintf("/problems/%d/solutions/%d", post.ProblemID, postID),
	})
}
