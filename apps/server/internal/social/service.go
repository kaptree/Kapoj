package social

import (
	"context"

	"gorm.io/gorm"
)

// ActivityRecorder 是社交动作写活跃度/共享分所需的接口；由 activity.Service 实现。
type ActivityRecorder interface {
	RecordSolutionReply(ctx context.Context, userID, replyID int64) error
	RecordSolutionLike(ctx context.Context, userID, postID int64) error
	RecordFollow(ctx context.Context, userID, followeeID int64) error
	BumpSharingVersion(ctx context.Context)
}

// Service 处理关注、通知与社交活跃度。
type Service struct {
	db       *gorm.DB
	activity ActivityRecorder
}

// NewService 构建社交服务。activity 可为 nil 以禁用活跃度写入。
func NewService(db *gorm.DB, activity ActivityRecorder) *Service {
	return &Service{db: db, activity: activity}
}

// normalizePaging 将页码/页大小归一化到合法区间。
func normalizePaging(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
