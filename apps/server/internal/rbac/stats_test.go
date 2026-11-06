package rbac_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestStatsCounts(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	activeID := createSeedUser(t, g, "stats_active", user.RoleUser)
	bannedID := createSeedUser(t, g, "stats_banned", user.RoleUser)
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", bannedID).
		Update("status", user.StatusBanned).Error)

	pub := &problem.Problem{Title: "公开题", Slug: "stats-public", IsPublic: true, Type: problem.TypeTraditional}
	require.NoError(t, g.Create(pub).Error)
	priv := &problem.Problem{Title: "隐藏题", Slug: "stats-private", IsPublic: false, Type: problem.TypeTraditional}
	require.NoError(t, g.Create(priv).Error)

	now := time.Now()
	y, m, d := now.Date()
	todayStart := time.Date(y, m, d, 0, 0, 0, 0, now.Location())

	ct := &contest.Contest{
		Title:     "统计比赛",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		Mode:      contest.ModeICPC,
	}
	require.NoError(t, g.Create(ct).Error)

	// 今日与昨日各一条提交。昨日提交固定为“今日本地零点前一小时”，
	// 必然早于“今日”起点，且不受 DST 影响。
	todaySub := &submission.Submission{
		UserID: activeID, ProblemID: pub.ID, Language: submission.LanguageCPP,
		Code: "int main(){}", CreatedAt: now,
	}
	require.NoError(t, g.Create(todaySub).Error)
	yesterdaySub := &submission.Submission{
		UserID: activeID, ProblemID: pub.ID, Language: submission.LanguageCPP,
		Code: "int main(){}", CreatedAt: todayStart.Add(-time.Hour),
	}
	require.NoError(t, g.Create(yesterdaySub).Error)

	stats, err := svc.Stats(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.Users)
	assert.Equal(t, int64(1), stats.ActiveUsers)
	assert.Equal(t, int64(1), stats.BannedUsers)
	assert.Equal(t, int64(2), stats.Problems)
	assert.Equal(t, int64(1), stats.PublicProblems)
	assert.Equal(t, int64(1), stats.Contests)
	assert.Equal(t, int64(2), stats.Submissions)
	assert.Equal(t, int64(1), stats.TodaySubmissions)
	assert.Equal(t, int64(7), stats.Roles)
	assert.NotNil(t, stats.RecentAudit)
	assert.Empty(t, stats.RecentAudit)
}

func TestStatsRecentAudit(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	base := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		insertAuditRow(t, g, int64(i+1), "role.create", "role", nil, base.Add(time.Duration(i)*time.Minute))
	}

	// includeAudit=false 不返回最近审计。
	stats, err := svc.Stats(ctx, false)
	require.NoError(t, err)
	assert.NotNil(t, stats.RecentAudit)
	assert.Empty(t, stats.RecentAudit)

	// includeAudit=true 返回最近 10 条，按时间倒序。
	stats, err = svc.Stats(ctx, true)
	require.NoError(t, err)
	require.Len(t, stats.RecentAudit, 10)
	assert.Equal(t, int64(12), stats.RecentAudit[0].ActorID)
	assert.True(t, stats.RecentAudit[0].CreatedAt.Equal(base.Add(11*time.Minute)))
	assert.Equal(t, int64(3), stats.RecentAudit[9].ActorID)
	assert.True(t, stats.RecentAudit[9].CreatedAt.Equal(base.Add(2*time.Minute)))
}
