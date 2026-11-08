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

// solutionSharedType 是 shared_items 中题解帖子的类型，共享分公式按 5 * like_count
// 计入作者共享分。
const solutionSharedType = "solution"

// LikeResult 是点赞/取消点赞的返回。
type LikeResult struct {
	Liked     bool  `json:"liked"`
	LikeCount int64 `json:"likeCount"`
}

// LikeSolutionPost 给解题思路帖子点赞。帖子必须存在且未删除；不能赞自己的帖子；
// 重复点赞幂等。首次点赞会给帖子作者写一条 like 通知（dedupe like:<actor>:<post>），
// 并在提交后 best-effort 记点赞活跃度与共享分。
func (s *Service) LikeSolutionPost(ctx context.Context, actorID, postID int64) (*LikeResult, error) {
	if actorID <= 0 || postID <= 0 {
		return nil, httpx.ErrValidation("参数必须为正数")
	}
	post, err := s.findSolutionPost(ctx, postID)
	if err != nil {
		return nil, err
	}
	if post.UserID == actorID {
		return nil, httpx.ErrValidation("不能点赞自己的帖子")
	}

	now := time.Now().In(time.Local)
	inserted := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&problem.SolutionPostLike{
			UserID: actorID, PostID: postID, CreatedAt: now,
		})
		if res.Error != nil {
			return res.Error
		}
		inserted = res.RowsAffected > 0
		if !inserted {
			return nil
		}
		// 作者共享分按点赞数 +1。
		if err := tx.Exec(
			`INSERT INTO shared_items (user_id, type, ref_id, like_count, created_at)
			 VALUES (?, ?, ?, 1, ?)
			 ON CONFLICT (type, ref_id) DO UPDATE SET like_count = shared_items.like_count + 1`,
			post.UserID, solutionSharedType, postID, now,
		).Error; err != nil {
			return err
		}
		if post.UserID != actorID {
			return insertNotification(tx, &Notification{
				UserID:     post.UserID,
				ActorID:    actorID,
				Type:       TypeSolutionLike,
				DedupeKey:  fmt.Sprintf("like:%d:%d", actorID, postID),
				TargetType: targetTypeSolutionPost,
				TargetID:   postID,
				Link:       solutionPostLink(post.ProblemID, postID),
				CreatedAt:  now,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if inserted && s.activity != nil {
		if err := s.activity.RecordSolutionLike(ctx, actorID, postID); err != nil {
			slog.Warn("social: record like activity failed", "actor", actorID, "post", postID, "err", err)
		}
		s.activity.BumpSharingVersion(ctx)
	}
	return s.likeResult(ctx, postID, true)
}

// UnlikeSolutionPost 取消点赞；未点赞时幂等。删除成功后 shared_items -1（不低于 0），
// 不写通知、不扣活跃度；提交后 best-effort 递增共享榜版本。
func (s *Service) UnlikeSolutionPost(ctx context.Context, actorID, postID int64) (*LikeResult, error) {
	if actorID <= 0 || postID <= 0 {
		return nil, httpx.ErrValidation("参数必须为正数")
	}

	deleted := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("user_id = ? AND post_id = ?", actorID, postID).
			Delete(&problem.SolutionPostLike{})
		if res.Error != nil {
			return res.Error
		}
		deleted = res.RowsAffected > 0
		if !deleted {
			return nil
		}
		return tx.Exec(
			`UPDATE shared_items SET like_count = GREATEST(0, like_count - 1)
			 WHERE type = ? AND ref_id = ?`,
			solutionSharedType, postID,
		).Error
	})
	if err != nil {
		return nil, err
	}

	if deleted && s.activity != nil {
		s.activity.BumpSharingVersion(ctx)
	}
	return s.likeResult(ctx, postID, false)
}

// CleanupSolutionPost 在帖子软删除后清理其点赞与共享项；幂等，通知与活跃度日志
// 保留。提交后 best-effort 递增共享榜版本。
func (s *Service) CleanupSolutionPost(ctx context.Context, postID int64) error {
	if postID <= 0 {
		return httpx.ErrValidation("参数必须为正数")
	}
	removed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("post_id = ?", postID).Delete(&problem.SolutionPostLike{})
		if res.Error != nil {
			return res.Error
		}
		removed = res.RowsAffected > 0
		res = tx.Exec(`DELETE FROM shared_items WHERE type = ? AND ref_id = ?`,
			solutionSharedType, postID)
		if res.Error != nil {
			return res.Error
		}
		removed = removed || res.RowsAffected > 0
		return nil
	})
	if err != nil {
		return err
	}
	if removed && s.activity != nil {
		s.activity.BumpSharingVersion(ctx)
	}
	return nil
}

// findSolutionPost 加载未删除的帖子；不存在或已删除返回 404。
func (s *Service) findSolutionPost(ctx context.Context, postID int64) (*problem.SolutionPost, error) {
	var post problem.SolutionPost
	err := s.db.WithContext(ctx).Where("id = ?", postID).First(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &post, nil
}

// likeResult 返回点赞状态与当前点赞数。
func (s *Service) likeResult(ctx context.Context, postID int64, liked bool) (*LikeResult, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&problem.SolutionPostLike{}).
		Where("post_id = ?", postID).Count(&count).Error; err != nil {
		return nil, err
	}
	return &LikeResult{Liked: liked, LikeCount: count}, nil
}
