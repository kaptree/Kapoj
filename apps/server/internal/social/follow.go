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
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// Follow 关注一个 active 用户。重复关注幂等；自己/不存在/非 active 分别返回
// 40001/404/40001。首次关注会写一条 follow 通知并在提交后 best-effort 记活跃度。
func (s *Service) Follow(ctx context.Context, followerID, followeeID int64) (*FollowResult, error) {
	if followerID <= 0 || followeeID <= 0 {
		return nil, httpx.ErrValidation("用户 ID 必须为正数")
	}
	if followerID == followeeID {
		return nil, httpx.ErrValidation("不能关注自己")
	}
	if err := s.requireActiveTarget(ctx, followeeID); err != nil {
		return nil, err
	}

	now := time.Now().In(time.Local)
	inserted := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Follow{
			FollowerID: followerID, FolloweeID: followeeID, CreatedAt: now,
		})
		if res.Error != nil {
			return res.Error
		}
		inserted = res.RowsAffected > 0
		if inserted {
			return insertNotification(tx, &Notification{
				UserID:     followeeID,
				ActorID:    followerID,
				Type:       TypeFollow,
				DedupeKey:  fmt.Sprintf("follow:%d:%d", followerID, followeeID),
				TargetType: targetTypeUser,
				TargetID:   followerID,
				Link:       fmt.Sprintf("/users/%d", followerID),
				CreatedAt:  now,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if inserted && s.activity != nil {
		if err := s.activity.RecordFollow(ctx, followerID, followeeID); err != nil {
			slog.Warn("social: record follow activity failed",
				"follower", followerID, "followee", followeeID, "err", err)
		}
	}
	return s.followResult(ctx, followerID, followeeID)
}

// Unfollow 取消关注；未关注时幂等，返回当前关注状态与粉丝数。
func (s *Service) Unfollow(ctx context.Context, followerID, followeeID int64) (*FollowResult, error) {
	if followerID <= 0 || followeeID <= 0 {
		return nil, httpx.ErrValidation("用户 ID 必须为正数")
	}
	if followerID == followeeID {
		return nil, httpx.ErrValidation("不能关注自己")
	}
	res := s.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Delete(&Follow{})
	if res.Error != nil {
		return nil, res.Error
	}
	return s.followResult(ctx, followerID, followeeID)
}

// SocialView 返回目标用户的粉丝数/关注数与 viewer 是否已关注。目标不存在 404；
// 已存在但封禁/非 active 的用户仍可读。
func (s *Service) SocialView(ctx context.Context, viewerID, targetID int64) (*SocialView, error) {
	if err := s.requireUserExists(ctx, targetID); err != nil {
		return nil, err
	}
	var followers, following int64
	if err := s.db.WithContext(ctx).Model(&Follow{}).
		Where("followee_id = ?", targetID).Count(&followers).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&Follow{}).
		Where("follower_id = ?", targetID).Count(&following).Error; err != nil {
		return nil, err
	}
	isFollowing := false
	if viewerID > 0 && viewerID != targetID {
		var n int64
		if err := s.db.WithContext(ctx).Model(&Follow{}).
			Where("follower_id = ? AND followee_id = ?", viewerID, targetID).Count(&n).Error; err != nil {
			return nil, err
		}
		isFollowing = n > 0
	}
	return &SocialView{FollowersCount: followers, FollowingCount: following, IsFollowing: isFollowing}, nil
}

// Followers 返回目标用户的粉丝分页列表，按关注时间倒序、id 倒序。
func (s *Service) Followers(ctx context.Context, targetID int64, page, size int) (httpx.Page[UserSummary], error) {
	return s.followUsers(ctx, targetID, page, size, false)
}

// Following 返回目标用户的关注分页列表，按关注时间倒序、id 倒序。
func (s *Service) Following(ctx context.Context, targetID int64, page, size int) (httpx.Page[UserSummary], error) {
	return s.followUsers(ctx, targetID, page, size, true)
}

// followUsers 查询粉丝（following=false）或关注（following=true）列表。
func (s *Service) followUsers(ctx context.Context, targetID int64, page, size int, following bool) (httpx.Page[UserSummary], error) {
	if err := s.requireUserExists(ctx, targetID); err != nil {
		return httpx.Page[UserSummary]{}, err
	}
	page, size = normalizePaging(page, size)

	countQuery := s.db.WithContext(ctx).Model(&Follow{})
	if following {
		countQuery = countQuery.Where("follower_id = ?", targetID)
	} else {
		countQuery = countQuery.Where("followee_id = ?", targetID)
	}
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return httpx.Page[UserSummary]{}, err
	}

	var rows []struct {
		ID        int64  `gorm:"column:id"`
		Username  string `gorm:"column:username"`
		Nickname  string `gorm:"column:nickname"`
		AvatarURL string `gorm:"column:avatar_url"`
		Rating    int    `gorm:"column:rating"`
	}
	query := s.db.WithContext(ctx).
		Table("user_follows AS f").
		Select("u.id, u.username, u.nickname, u.avatar_url, u.rating").
		Order("f.created_at DESC, f.id DESC").
		Offset((page - 1) * size).Limit(size)
	if following {
		query = query.Joins("JOIN users AS u ON u.id = f.followee_id").
			Where("f.follower_id = ?", targetID)
	} else {
		query = query.Joins("JOIN users AS u ON u.id = f.follower_id").
			Where("f.followee_id = ?", targetID)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return httpx.Page[UserSummary]{}, err
	}

	items := make([]UserSummary, len(rows))
	for i, r := range rows {
		items[i] = UserSummary{ID: r.ID, Username: r.Username, Nickname: r.Nickname, AvatarURL: r.AvatarURL, Rating: r.Rating}
	}
	return httpx.Page[UserSummary]{Total: total, Items: items}, nil
}

// followResult 返回关注状态与目标粉丝数。
func (s *Service) followResult(ctx context.Context, followerID, followeeID int64) (*FollowResult, error) {
	var following int64
	if err := s.db.WithContext(ctx).Model(&Follow{}).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Count(&following).Error; err != nil {
		return nil, err
	}
	var followersCount int64
	if err := s.db.WithContext(ctx).Model(&Follow{}).
		Where("followee_id = ?", followeeID).Count(&followersCount).Error; err != nil {
		return nil, err
	}
	return &FollowResult{Following: following > 0, FollowersCount: followersCount}, nil
}

// requireUserExists 校验用户存在；不存在返回 404。
func (s *Service) requireUserExists(ctx context.Context, id int64) error {
	if id <= 0 {
		return httpx.ErrNotFound
	}
	var count int64
	if err := s.db.WithContext(ctx).Table("users").Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// requireActiveTarget 校验关注目标存在且 active；不存在 404，非 active 40001。
func (s *Service) requireActiveTarget(ctx context.Context, id int64) error {
	var target struct {
		Status user.Status
	}
	err := s.db.WithContext(ctx).Table("users").
		Select("status").Where("id = ?", id).Take(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	if target.Status != user.StatusActive {
		return httpx.ErrValidation("不能关注该用户")
	}
	return nil
}
