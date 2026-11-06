package gamification_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type achievementEnv struct {
	svc  *gamification.Service
	repo *gamification.Repository
	db   *gorm.DB
	user *user.User
}

func newAchievementEnv(t *testing.T) *achievementEnv {
	t.Helper()
	g := testutil.PG(t)
	u := &user.User{
		Username:     "achiever",
		Nickname:     "成就用户",
		Email:        "achiever@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	repo := gamification.NewRepository(g)
	return &achievementEnv{svc: gamification.NewService(repo), repo: repo, db: g, user: u}
}

func adminViewer(userID int64) gamification.Viewer {
	return gamification.Viewer{
		UserID:                userID,
		CanManageAchievements: true,
		CanManageTasks:        true,
		CanManageBadges:       true,
		CanManageTitles:       true,
		CanAdjustCoins:        true,
	}
}

func achievementInput(code, name string, t gamification.ConditionType, cfg gamification.ConditionConfig, reward int64) gamification.AchievementInput {
	return gamification.AchievementInput{
		Code:            code,
		Name:            name,
		Description:     "测试成就",
		ConditionType:   t,
		ConditionConfig: cfg,
		RewardCoins:     reward,
		SortOrder:       1,
	}
}

func (e *achievementEnv) create(t *testing.T, in gamification.AchievementInput) *gamification.Achievement {
	t.Helper()
	a, err := e.svc.CreateAchievement(context.Background(), adminViewer(e.user.ID), in)
	require.NoError(t, err)
	return a
}

func (e *achievementEnv) addAcceptedProblem(t *testing.T, slug string) int64 {
	t.Helper()
	p := &problem.Problem{Title: slug, Slug: slug}
	require.NoError(t, e.db.Create(p).Error)
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main() {}', ?, now())`,
		e.user.ID, p.ID, submission.StatusAccepted,
	).Error)
	return p.ID
}

func (e *achievementEnv) addContestParticipation(t *testing.T) {
	t.Helper()
	now := time.Now()
	c := &contest.Contest{
		Title:        "成就比赛",
		StartTime:    now.Add(-time.Hour),
		EndTime:      now.Add(time.Hour),
		Mode:         contest.ModeICPC,
		IsPublic:     true,
		CreatedBy:    e.user.ID,
		RegisterType: contest.RegisterOpen,
	}
	require.NoError(t, e.db.Create(c).Error)
	require.NoError(t, e.db.Create(&contest.ContestParticipant{
		ContestID: c.ID, UserID: e.user.ID, RegisterTime: now,
	}).Error)
}

func findAchievement(t *testing.T, items []gamification.AchievementItem, id int64) gamification.AchievementItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("achievement %d not found in %+v", id, items)
	return gamification.AchievementItem{}
}

func TestAchievementUnlocksAndRewardsOnce(t *testing.T) {
	e := newAchievementEnv(t)
	a := e.create(t, achievementInput(
		"first-ac", "首次 AC",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 50,
	))
	e.addAcceptedProblem(t, "first-ac-problem")

	require.NoError(t, e.svc.EvaluateUser(context.Background(), e.user.ID))

	items, err := e.svc.ListAchievements(context.Background(), gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	item := findAchievement(t, items, a.ID)
	assert.True(t, item.Unlocked)
	assert.NotNil(t, item.UnlockedAt)
	require.Len(t, item.Progress, 1)
	assert.Equal(t, gamification.ConditionACCount, item.Progress[0].Type)
	assert.EqualValues(t, 1, item.Progress[0].Current)
	assert.EqualValues(t, 1, item.Progress[0].Target)
	assert.True(t, item.Progress[0].Done)

	balance, err := e.repo.Balance(context.Background(), e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 50, balance)

	// 重复补算不得重复发奖。
	require.NoError(t, e.svc.EvaluateUser(context.Background(), e.user.ID))
	balance, err = e.repo.Balance(context.Background(), e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 50, balance)

	var unlockedCount, txCount int64
	require.NoError(t, e.db.Model(&gamification.UserAchievement{}).
		Where("user_id = ?", e.user.ID).Count(&unlockedCount).Error)
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, unlockedCount)
	assert.EqualValues(t, 1, txCount)

	var tx gamification.CoinTransaction
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).First(&tx).Error)
	assert.EqualValues(t, 50, tx.Amount)
	assert.Equal(t, gamification.CoinEarn, tx.Type)
	assert.Equal(t, "achievement", tx.Reason)
	assert.Equal(t, "achievement", tx.RefType)
	assert.EqualValues(t, a.ID, tx.RefID)
}

func TestAchievementUnlockConcurrent(t *testing.T) {
	e := newAchievementEnv(t)
	e.create(t, achievementInput(
		"concurrent-ac", "并发 AC",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 50,
	))
	e.addAcceptedProblem(t, "concurrent-problem")

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- e.svc.EvaluateUser(context.Background(), e.user.ID)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	balance, err := e.repo.Balance(context.Background(), e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 50, balance)
	var unlockedCount, txCount int64
	require.NoError(t, e.db.Model(&gamification.UserAchievement{}).
		Where("user_id = ?", e.user.ID).Count(&unlockedCount).Error)
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, unlockedCount)
	assert.EqualValues(t, 1, txCount)
}

func TestAchievementDuplicateCodeConcurrent(t *testing.T) {
	e := newAchievementEnv(t)
	in := achievementInput(
		"race-code", "并发 code",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 10,
	)

	const workers = 4
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := e.svc.CreateAchievement(context.Background(), adminViewer(e.user.ID), in)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	var created int
	for err := range errs {
		if err == nil {
			created++
			continue
		}
		assertCode(t, err, 40001)
	}
	assert.Equal(t, 1, created)
	var count int64
	require.NoError(t, e.db.Model(&gamification.Achievement{}).
		Where("code = ?", in.Code).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestAchievementMalformedConfigSkipped(t *testing.T) {
	e := newAchievementEnv(t)
	valid := e.create(t, achievementInput(
		"valid-config", "有效成就",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 10,
	))

	// 模拟历史坏数据：condition_config_json 的 threshold 类型错误。
	broken := &gamification.Achievement{
		Code:          "broken-config",
		Name:          "损坏配置成就",
		ConditionType: string(gamification.ConditionACCount),
		ConditionConfig: dbtypes.JSON[map[string]any]{
			Data: map[string]any{"threshold": "not-a-number"},
		},
		IsActive: true,
	}
	require.NoError(t, e.db.Create(broken).Error)
	e.addAcceptedProblem(t, "malformed-config-problem")

	require.NoError(t, e.svc.EvaluateUser(context.Background(), e.user.ID))
	items, err := e.svc.ListAchievements(
		context.Background(), gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, valid.ID, items[0].ID)
	assert.True(t, items[0].Unlocked)
}

func TestAchievementListKeepsUnlockedInactive(t *testing.T) {
	e := newAchievementEnv(t)
	unlocked := e.create(t, achievementInput(
		"unlocked-inactive", "已解锁下线",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 0,
	))
	locked := e.create(t, achievementInput(
		"locked-inactive", "未解锁下线",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 100}, 0,
	))
	e.addAcceptedProblem(t, "inactive-problem")
	require.NoError(t, e.svc.EvaluateUser(context.Background(), e.user.ID))

	inactive := false
	for _, a := range []*gamification.Achievement{unlocked, locked} {
		in := achievementInput(a.Code, a.Name,
			gamification.ConditionType(a.ConditionType),
			gamification.ConditionConfig{Threshold: 1}, a.RewardCoins)
		if a.ID == locked.ID {
			in.ConditionConfig.Threshold = 100
		}
		in.IsActive = &inactive
		_, err := e.svc.UpdateAchievement(context.Background(), adminViewer(e.user.ID), a.ID, in)
		require.NoError(t, err)
	}

	items, err := e.svc.ListAchievements(context.Background(), gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, unlocked.ID, items[0].ID)
	assert.True(t, items[0].Unlocked)
	assert.False(t, items[0].IsActive)

	guestItems, err := e.svc.ListAchievements(context.Background(), gamification.Viewer{}, false)
	require.NoError(t, err)
	assert.Empty(t, guestItems)

	adminItems, err := e.svc.ListAchievements(context.Background(), adminViewer(e.user.ID), true)
	require.NoError(t, err)
	assert.Len(t, adminItems, 2)

	_, err = e.svc.ListAchievements(context.Background(), gamification.Viewer{UserID: e.user.ID}, true)
	assertCode(t, err, 40300)
}

func TestAchievementProgressAndAllOf(t *testing.T) {
	e := newAchievementEnv(t)
	a := e.create(t, achievementInput(
		"all-rounder", "全能选手",
		gamification.ConditionAllOf,
		gamification.ConditionConfig{Conditions: []gamification.Condition{
			{Type: gamification.ConditionACCount, Threshold: 1},
			{Type: gamification.ConditionContestCount, Threshold: 1},
		}},
		0,
	))
	e.addAcceptedProblem(t, "all-of-problem")

	items, err := e.svc.ListAchievements(context.Background(), gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	item := findAchievement(t, items, a.ID)
	assert.False(t, item.Unlocked)
	require.Len(t, item.Progress, 2)
	assert.Equal(t, gamification.ConditionACCount, item.Progress[0].Type)
	assert.True(t, item.Progress[0].Done)
	assert.EqualValues(t, 1, item.Progress[0].Current)
	assert.Equal(t, gamification.ConditionContestCount, item.Progress[1].Type)
	assert.False(t, item.Progress[1].Done)
	assert.EqualValues(t, 0, item.Progress[1].Current)
	assert.EqualValues(t, 1, item.Progress[1].Target)

	e.addContestParticipation(t)
	require.NoError(t, e.svc.EvaluateUser(context.Background(), e.user.ID))
	items, err = e.svc.ListAchievements(context.Background(), gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	item = findAchievement(t, items, a.ID)
	assert.True(t, item.Unlocked)
	require.Len(t, item.Progress, 2)
	assert.True(t, item.Progress[0].Done)
	assert.True(t, item.Progress[1].Done)
}

func TestAchievementUpdateKeepsInactiveRewardBadge(t *testing.T) {
	e := newAchievementEnv(t)
	ctx := context.Background()
	viewer := adminViewer(e.user.ID)

	rewardBadge := &gamification.Badge{Name: "原奖励徽章", IsActive: true}
	require.NoError(t, e.db.Create(rewardBadge).Error)
	in := achievementInput("keep-inactive-badge", "保留下线奖励", gamification.ConditionACCount,
		gamification.ConditionConfig{Threshold: 1}, 0)
	in.RewardBadgeID = &rewardBadge.ID
	created, err := e.svc.CreateAchievement(ctx, viewer, in)
	require.NoError(t, err)

	require.NoError(t, e.svc.DeleteBadge(ctx, viewer, rewardBadge.ID))

	// 同名 rewardBadgeId 已下线也应允许无关字段更新。
	update := in
	update.Code = ""
	update.Name = "改名后的成就"
	updated, err := e.svc.UpdateAchievement(ctx, viewer, created.ID, update)
	require.NoError(t, err)
	assert.Equal(t, "改名后的成就", updated.Name)
	require.NotNil(t, updated.RewardBadgeID)
	assert.Equal(t, rewardBadge.ID, *updated.RewardBadgeID)

	// 新绑定的已下线徽章仍必须拒绝。
	another := &gamification.Badge{Name: "另一个下线徽章", IsActive: true}
	require.NoError(t, e.db.Create(another).Error)
	require.NoError(t, e.svc.DeleteBadge(ctx, viewer, another.ID))
	update.RewardBadgeID = &another.ID
	_, err = e.svc.UpdateAchievement(ctx, viewer, created.ID, update)
	assertCode(t, err, 40001)
}

func TestAchievementValidation(t *testing.T) {
	e := newAchievementEnv(t)
	viewer := adminViewer(e.user.ID)
	validCondition := gamification.ConditionConfig{Threshold: 1}

	badgeID := int64(1)
	cases := []struct {
		name   string
		mutate func(*gamification.AchievementInput)
		want   string
	}{
		{"code 非法", func(in *gamification.AchievementInput) { in.Code = "Bad Code" }, "code"},
		{"code 过短", func(in *gamification.AchievementInput) { in.Code = "a" }, "code"},
		{"未知条件", func(in *gamification.AchievementInput) { in.ConditionType = "bogus" }, "条件"},
		{"奖励徽章不存在", func(in *gamification.AchievementInput) { in.RewardBadgeID = &badgeID }, "徽章"},
		{"奖励金币为负", func(in *gamification.AchievementInput) { in.RewardCoins = -1 }, "奖励金币"},
		{"奖励金币过大", func(in *gamification.AchievementInput) { in.RewardCoins = 1_000_001 }, "奖励金币"},
		{"名称为空", func(in *gamification.AchievementInput) { in.Name = "  " }, "名称"},
		{"阈值非正", func(in *gamification.AchievementInput) { in.ConditionConfig.Threshold = 0 }, "阈值"},
		{"all_of 只有一项", func(in *gamification.AchievementInput) {
			in.ConditionType = gamification.ConditionAllOf
			in.ConditionConfig = gamification.ConditionConfig{Conditions: []gamification.Condition{
				{Type: gamification.ConditionACCount, Threshold: 1},
			}}
		}, "all_of"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := achievementInput("valid-code-"+string(rune('a'+i)), "测试成就",
				gamification.ConditionACCount, validCondition, 10)
			tc.mutate(&in)
			_, err := e.svc.CreateAchievement(context.Background(), viewer, in)
			assertCode(t, err, 40001)
			var appErr interface{ Error() string }
			require.ErrorAs(t, err, &appErr)
			assert.Contains(t, appErr.Error(), tc.want)
		})
	}

	in := achievementInput("valid-code", "有效成就",
		gamification.ConditionACCount, validCondition, 10)
	a := e.create(t, in)
	assert.True(t, a.IsActive, "创建时未指定 isActive 应默认上架")

	rewardBadge := &gamification.Badge{Name: "回馈徽章", IsActive: true}
	require.NoError(t, e.db.Create(rewardBadge).Error)
	withBadge := achievementInput("valid-badge-code", "徽章成就",
		gamification.ConditionACCount, validCondition, 0)
	withBadge.RewardBadgeID = &rewardBadge.ID
	badgeAchievement, err := e.svc.CreateAchievement(context.Background(), viewer, withBadge)
	require.NoError(t, err)
	require.NotNil(t, badgeAchievement.RewardBadgeID)
	assert.Equal(t, rewardBadge.ID, *badgeAchievement.RewardBadgeID)

	inactiveBadge := &gamification.Badge{Name: "下线徽章", IsActive: false}
	require.NoError(t, e.db.Create(inactiveBadge).Error)
	inactiveReward := achievementInput("inactive-badge-code", "下线徽章成就",
		gamification.ConditionACCount, validCondition, 0)
	inactiveReward.RewardBadgeID = &inactiveBadge.ID
	_, err = e.svc.CreateAchievement(context.Background(), viewer, inactiveReward)
	assertCode(t, err, 40001)

	// 更新时省略 rewardBadgeId 保留原奖励徽章。
	kept := withBadge
	kept.RewardBadgeID = nil
	updated, err := e.svc.UpdateAchievement(context.Background(), viewer, badgeAchievement.ID, kept)
	require.NoError(t, err)
	require.NotNil(t, updated.RewardBadgeID)
	assert.Equal(t, rewardBadge.ID, *updated.RewardBadgeID)

	duplicate := achievementInput("valid-code", "重复成就",
		gamification.ConditionACCount, validCondition, 10)
	_, err = e.svc.CreateAchievement(context.Background(), viewer, duplicate)
	assertCode(t, err, 40001)

	changed := in
	changed.Code = "changed-code"
	_, err = e.svc.UpdateAchievement(context.Background(), viewer, a.ID, changed)
	assertCode(t, err, 40001)

	_, err = e.svc.CreateAchievement(context.Background(), gamification.Viewer{UserID: e.user.ID}, duplicate)
	assertCode(t, err, 40300)
	err = e.svc.DeleteAchievement(context.Background(), gamification.Viewer{UserID: e.user.ID}, a.ID)
	assertCode(t, err, 40300)

	require.NoError(t, e.svc.DeleteAchievement(context.Background(), viewer, a.ID))
	deleted, err := e.repo.FindAchievement(context.Background(), a.ID)
	require.NoError(t, err)
	assert.False(t, deleted.IsActive)
}
