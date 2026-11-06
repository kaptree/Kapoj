package rbac

import (
	"context"
	"fmt"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// recentAuditLimit 是仪表盘附带“最近审计”的条数上限。
const recentAuditLimit = 10

// Stats 是管理仪表盘的聚合计数；除 RecentAudit 外不返回任何明细。
// “今日”按服务器本地日历日（当天 00:00 起）计算。
type Stats struct {
	Users            int64       `json:"users"`
	ActiveUsers      int64       `json:"activeUsers"`
	BannedUsers      int64       `json:"bannedUsers"`
	Problems         int64       `json:"problems"`
	PublicProblems   int64       `json:"publicProblems"`
	Contests         int64       `json:"contests"`
	Submissions      int64       `json:"submissions"`
	TodaySubmissions int64       `json:"todaySubmissions"`
	Roles            int64       `json:"roles"`
	RecentAudit      []AuditItem `json:"recentAudit"`
}

// Stats 统计用户/题目/比赛/提交/角色数量；includeAudit 为 true 时附带最近
// 10 条审计（按时间倒序）。
func (s *Service) Stats(ctx context.Context, includeAudit bool) (*Stats, error) {
	db := s.db.WithContext(ctx)
	out := &Stats{RecentAudit: []AuditItem{}}

	if err := db.Model(&user.User{}).Count(&out.Users).Error; err != nil {
		return nil, fmt.Errorf("rbac: count users: %w", err)
	}
	if err := db.Model(&user.User{}).Where("status = ?", user.StatusActive).Count(&out.ActiveUsers).Error; err != nil {
		return nil, fmt.Errorf("rbac: count active users: %w", err)
	}
	if err := db.Model(&user.User{}).Where("status = ?", user.StatusBanned).Count(&out.BannedUsers).Error; err != nil {
		return nil, fmt.Errorf("rbac: count banned users: %w", err)
	}
	if err := db.Model(&problem.Problem{}).Count(&out.Problems).Error; err != nil {
		return nil, fmt.Errorf("rbac: count problems: %w", err)
	}
	if err := db.Model(&problem.Problem{}).Where("is_public = ?", true).Count(&out.PublicProblems).Error; err != nil {
		return nil, fmt.Errorf("rbac: count public problems: %w", err)
	}
	if err := db.Model(&contest.Contest{}).Count(&out.Contests).Error; err != nil {
		return nil, fmt.Errorf("rbac: count contests: %w", err)
	}
	if err := db.Model(&submission.Submission{}).Count(&out.Submissions).Error; err != nil {
		return nil, fmt.Errorf("rbac: count submissions: %w", err)
	}
	todayStart := startOfLocalDay(time.Now())
	if err := db.Model(&submission.Submission{}).Where("created_at >= ?", todayStart).Count(&out.TodaySubmissions).Error; err != nil {
		return nil, fmt.Errorf("rbac: count today submissions: %w", err)
	}
	if err := db.Model(&Role{}).Count(&out.Roles).Error; err != nil {
		return nil, fmt.Errorf("rbac: count roles: %w", err)
	}

	if includeAudit {
		page, err := s.ListAudit(ctx, AuditFilter{}, 1, recentAuditLimit)
		if err != nil {
			return nil, err
		}
		out.RecentAudit = page.Items
	}
	return out, nil
}

// startOfLocalDay 返回 now 所在服务器本地日历日的 00:00:00。
func startOfLocalDay(now time.Time) time.Time {
	y, m, d := now.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}
