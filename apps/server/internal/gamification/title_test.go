package gamification_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type titleEnv struct {
	svc   *gamification.Service
	repo  *gamification.Repository
	db    *gorm.DB
	user  *user.User
	admin *user.User
}

func newTitleEnv(t *testing.T) *titleEnv {
	t.Helper()
	g := testutil.PG(t)
	u := &user.User{
		Username:     "title-user",
		Nickname:     "称号用户",
		Email:        "title-user@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	admin := &user.User{
		Username:     "title-admin",
		Nickname:     "称号管理员",
		Email:        "title-admin@example.com",
		PasswordHash: "x",
		Role:         user.RoleSuperAdmin,
	}
	require.NoError(t, g.Create(admin).Error)
	repo := gamification.NewRepository(g)
	svc := gamification.NewService(repo)
	return &titleEnv{svc: svc, repo: repo, db: g, user: u, admin: admin}
}

func titleInput(name, color string, t gamification.TitleUnlockType, cfg gamification.TitleUnlockConfig, price int64) gamification.TitleInput {
	return gamification.TitleInput{
		Name:                  name,
		Color:                 color,
		UnlockConditionType:   t,
		UnlockConditionConfig: cfg,
		PriceCoins:            price,
		SortOrder:             1,
	}
}

func (e *titleEnv) createTitle(t *testing.T, in gamification.TitleInput) *gamification.Title {
	t.Helper()
	title, err := e.svc.CreateTitle(context.Background(), adminViewer(e.admin.ID), in)
	require.NoError(t, err)
	return title
}

func (e *titleEnv) setCoins(t *testing.T, coins int64) {
	t.Helper()
	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", e.user.ID).
		Update("coins", coins).Error)
}

func (e *titleEnv) balance(t *testing.T) int64 {
	t.Helper()
	balance, err := e.repo.Balance(context.Background(), e.user.ID)
	require.NoError(t, err)
	return balance
}

func (e *titleEnv) addAcceptedProblem(t *testing.T, slug string) {
	t.Helper()
	p := &problem.Problem{Title: slug, Slug: slug}
	require.NoError(t, e.db.Create(p).Error)
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main() {}', ?, now())`,
		e.user.ID, p.ID, submission.StatusAccepted,
	).Error)
}

func findTitleItem(t *testing.T, items []gamification.TitleItem, id int64) gamification.TitleItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("title %d not found in %+v", id, items)
	return gamification.TitleItem{}
}

func (e *titleEnv) userTitle(t *testing.T, titleID int64) gamification.UserTitle {
	t.Helper()
	var row gamification.UserTitle
	require.NoError(t, e.db.Where("user_id = ? AND title_id = ?", e.user.ID, titleID).First(&row).Error)
	return row
}

func TestTitleUnlockConditions(t *testing.T) {
	e := newTitleEnv(t)
	ctx := context.Background()

	ratingTitle := e.createTitle(t, titleInput(
		"高分段位", "#7C3AED",
		gamification.TitleUnlockRating, gamification.TitleUnlockConfig{Rating: 1600}, 100,
	))
	acTitle := e.createTitle(t, titleInput(
		"百题斩", "#2563EB",
		gamification.TitleUnlockACCount, gamification.TitleUnlockConfig{Threshold: 1}, 50,
	))
	achTitle := e.createTitle(t, titleInput(
		"首次 AC 称号", "#059669",
		gamification.TitleUnlockAchievement, gamification.TitleUnlockConfig{AchievementCode: "first-ac"}, 0,
	))

	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	items, err := e.svc.ListTitles(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	require.Len(t, items, 3)
	for _, item := range items {
		assert.False(t, item.Unlocked)
		assert.False(t, item.Purchased)
		assert.Nil(t, item.UnlockedAt)
		assert.Nil(t, item.PurchasedAt)
	}
	ratingItem := findTitleItem(t, items, ratingTitle.ID)
	assert.Equal(t, gamification.TitleUnlockRating, ratingItem.Progress.Type)
	assert.EqualValues(t, 1500, ratingItem.Progress.Current)
	assert.EqualValues(t, 1600, ratingItem.Progress.Target)
	assert.False(t, ratingItem.Progress.Done)
	acItem := findTitleItem(t, items, acTitle.ID)
	assert.EqualValues(t, 0, acItem.Progress.Current)
	assert.EqualValues(t, 1, acItem.Progress.Target)
	achItem := findTitleItem(t, items, achTitle.ID)
	assert.Equal(t, "first-ac", achItem.Progress.AchievementCode)
	assert.EqualValues(t, 0, achItem.Progress.Current)
	assert.EqualValues(t, 1, achItem.Progress.Target)

	// 损坏的存储配置只跳过该称号，不影响整个列表与补算。
	broken := &gamification.Title{
		Name:                  "损坏称号",
		Color:                 "#123456",
		UnlockConditionType:   string(gamification.TitleUnlockACCount),
		UnlockConditionConfig: dbtypes.JSON[map[string]any]{Data: map[string]any{"threshold": "bad"}},
		PriceCoins:            0,
		IsActive:              true,
	}
	require.NoError(t, e.db.Create(broken).Error)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	items, err = e.svc.ListTitles(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	for _, item := range items {
		assert.NotEqual(t, broken.ID, item.ID)
	}

	// 达成三种条件：Rating 提升、1 道 AC、解锁 first-ac 成就。
	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", e.user.ID).
		Update("rating", 1600).Error)
	e.addAcceptedProblem(t, "title-rating-problem")
	_, err = e.svc.CreateAchievement(ctx, adminViewer(e.admin.ID), achievementInput(
		"first-ac", "首次 AC",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 0,
	))
	require.NoError(t, err)

	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	items, err = e.svc.ListTitles(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	require.Len(t, items, 3)

	ratingItem = findTitleItem(t, items, ratingTitle.ID)
	assert.True(t, ratingItem.Unlocked)
	assert.NotNil(t, ratingItem.UnlockedAt)
	assert.False(t, ratingItem.Purchased)
	assert.EqualValues(t, 1600, ratingItem.Progress.Current)
	assert.True(t, ratingItem.Progress.Done)

	acItem = findTitleItem(t, items, acTitle.ID)
	assert.True(t, acItem.Unlocked)
	assert.NotNil(t, acItem.UnlockedAt)
	assert.EqualValues(t, 1, acItem.Progress.Current)
	assert.True(t, acItem.Progress.Done)

	achItem = findTitleItem(t, items, achTitle.ID)
	assert.True(t, achItem.Unlocked)
	assert.Equal(t, "first-ac", achItem.Progress.AchievementCode)
	assert.EqualValues(t, 1, achItem.Progress.Current)
	assert.True(t, achItem.Progress.Done)

	for _, title := range []*gamification.Title{ratingTitle, acTitle, achTitle} {
		row := e.userTitle(t, title.ID)
		assert.False(t, row.UnlockedAt.IsZero())
		assert.Nil(t, row.PurchasedAt)
		assert.False(t, row.IsEquipped)
	}

	// 重复补算不重复插入解锁记录，也不产生金币流水。
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	var unlockCount, txCount int64
	require.NoError(t, e.db.Model(&gamification.UserTitle{}).
		Where("user_id = ?", e.user.ID).Count(&unlockCount).Error)
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 3, unlockCount)
	assert.EqualValues(t, 0, txCount)
}

func TestTitlePurchaseAndEquipExclusive(t *testing.T) {
	e := newTitleEnv(t)
	ctx := context.Background()

	a := e.createTitle(t, titleInput(
		"称号 A", "#7C3AED",
		gamification.TitleUnlockACCount, gamification.TitleUnlockConfig{Threshold: 1}, 100,
	))
	b := e.createTitle(t, titleInput(
		"称号 B", "#DC2626",
		gamification.TitleUnlockACCount, gamification.TitleUnlockConfig{Threshold: 1}, 200,
	))
	locked := e.createTitle(t, titleInput(
		"高门槛称号", "#111111",
		gamification.TitleUnlockRating, gamification.TitleUnlockConfig{Rating: 3000}, 0,
	))

	// 未解锁不能购买。
	assertCode(t, e.svc.PurchaseTitle(ctx, e.user.ID, a.ID), 40001)
	assertCode(t, e.svc.PurchaseTitle(ctx, e.user.ID, locked.ID), 40001)

	e.addAcceptedProblem(t, "title-purchase-problem")
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

	// 已解锁但余额不足。
	e.setCoins(t, 50)
	assertCode(t, e.svc.PurchaseTitle(ctx, e.user.ID, a.ID), 40001)
	assert.EqualValues(t, 50, e.balance(t))
	var rows int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&rows).Error)
	assert.EqualValues(t, 0, rows)

	// 购买 A 扣款并写流水。
	e.setCoins(t, 500)
	require.NoError(t, e.svc.PurchaseTitle(ctx, e.user.ID, a.ID))
	assert.EqualValues(t, 400, e.balance(t))
	assertCode(t, e.svc.PurchaseTitle(ctx, e.user.ID, a.ID), 40001)
	assert.EqualValues(t, 400, e.balance(t))

	var txs []gamification.CoinTransaction
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).Find(&txs).Error)
	require.Len(t, txs, 1)
	assert.EqualValues(t, -100, txs[0].Amount)
	assert.Equal(t, gamification.CoinSpend, txs[0].Type)
	assert.Equal(t, "title_purchase", txs[0].Reason)
	assert.Equal(t, "title", txs[0].RefType)
	assert.EqualValues(t, a.ID, txs[0].RefID)

	rowA := e.userTitle(t, a.ID)
	assert.NotNil(t, rowA.PurchasedAt)

	// 未购买不能装备。
	assertCode(t, e.svc.EquipTitle(ctx, e.user.ID, b.ID), 40001)

	require.NoError(t, e.svc.EquipTitle(ctx, e.user.ID, a.ID))
	rowA = e.userTitle(t, a.ID)
	rowB := e.userTitle(t, b.ID)
	assert.True(t, rowA.IsEquipped)
	assert.False(t, rowB.IsEquipped)

	// 购买并装备 B 自动卸下 A，同一用户同时只装备 1 个称号。
	require.NoError(t, e.svc.PurchaseTitle(ctx, e.user.ID, b.ID))
	assert.EqualValues(t, 200, e.balance(t))
	require.NoError(t, e.svc.EquipTitle(ctx, e.user.ID, b.ID))
	rowA = e.userTitle(t, a.ID)
	rowB = e.userTitle(t, b.ID)
	assert.False(t, rowA.IsEquipped)
	assert.True(t, rowB.IsEquipped)
	var equipped int64
	require.NoError(t, e.db.Model(&gamification.UserTitle{}).
		Where("user_id = ? AND is_equipped = ?", e.user.ID, true).Count(&equipped).Error)
	assert.EqualValues(t, 1, equipped)

	// 重复装备幂等；卸下未装备的称号也幂等。
	require.NoError(t, e.svc.EquipTitle(ctx, e.user.ID, b.ID))
	require.NoError(t, e.svc.UnequipTitle(ctx, e.user.ID, a.ID))
	assert.True(t, e.userTitle(t, b.ID).IsEquipped)
	require.NoError(t, e.svc.UnequipTitle(ctx, e.user.ID, b.ID))
	assert.False(t, e.userTitle(t, b.ID).IsEquipped)

	// 未解锁的称号不能购买或装备。
	assertCode(t, e.svc.EquipTitle(ctx, e.user.ID, locked.ID), 40001)
	assertCode(t, e.svc.PurchaseTitle(ctx, e.user.ID, locked.ID), 40001)
}

func TestTitleInactiveOwnedVisibility(t *testing.T) {
	e := newTitleEnv(t)
	ctx := context.Background()

	owned := e.createTitle(t, titleInput(
		"已解锁下线", "#7C3AED",
		gamification.TitleUnlockACCount, gamification.TitleUnlockConfig{Threshold: 1}, 0,
	))
	locked := e.createTitle(t, titleInput(
		"未解锁下线", "#DC2626",
		gamification.TitleUnlockRating, gamification.TitleUnlockConfig{Rating: 2000}, 0,
	))
	active := e.createTitle(t, titleInput(
		"上架称号", "#059669",
		gamification.TitleUnlockRating, gamification.TitleUnlockConfig{Rating: 1500}, 0,
	))

	e.addAcceptedProblem(t, "title-inactive-problem")
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	require.NoError(t, e.svc.DeleteTitle(ctx, adminViewer(e.admin.ID), owned.ID))
	require.NoError(t, e.svc.DeleteTitle(ctx, adminViewer(e.admin.ID), locked.ID))

	// 游客只看到上架称号。
	guestItems, err := e.svc.ListTitles(ctx, gamification.Viewer{}, false)
	require.NoError(t, err)
	require.Len(t, guestItems, 1)
	assert.Equal(t, active.ID, guestItems[0].ID)

	// 登录用户看到上架称号与自己已解锁的下线称号。
	userItems, err := e.svc.ListTitles(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	require.Len(t, userItems, 2)
	assert.True(t, findTitleItem(t, userItems, owned.ID).Unlocked)
	assert.False(t, findTitleItem(t, userItems, owned.ID).IsActive)
	for _, item := range userItems {
		assert.NotEqual(t, locked.ID, item.ID)
	}

	// 已解锁的下线称号仍可购买与装备，价格 0 也写 amount=0 的流水。
	require.NoError(t, e.svc.PurchaseTitle(ctx, e.user.ID, owned.ID))
	require.NoError(t, e.svc.EquipTitle(ctx, e.user.ID, owned.ID))
	var txs []gamification.CoinTransaction
	require.NoError(t, e.db.Where("user_id = ? AND reason = ?", e.user.ID, "title_purchase").
		Find(&txs).Error)
	require.Len(t, txs, 1)
	assert.EqualValues(t, 0, txs[0].Amount)
	assert.EqualValues(t, owned.ID, txs[0].RefID)
	assert.True(t, e.userTitle(t, owned.ID).IsEquipped)

	// 未解锁且下线的称号不能购买。
	assertCode(t, e.svc.PurchaseTitle(ctx, e.user.ID, locked.ID), 40001)

	// 管理员可看全部，普通用户不能请求 includeInactive。
	adminItems, err := e.svc.ListTitles(ctx, adminViewer(e.admin.ID), true)
	require.NoError(t, err)
	assert.Len(t, adminItems, 3)
	_, err = e.svc.ListTitles(ctx, gamification.Viewer{UserID: e.user.ID}, true)
	assertCode(t, err, 40300)
}

func TestTitleConcurrentPurchaseChargesOnce(t *testing.T) {
	e := newTitleEnv(t)
	ctx := context.Background()

	a := e.createTitle(t, titleInput(
		"并发称号 A", "#7C3AED",
		gamification.TitleUnlockRating, gamification.TitleUnlockConfig{Rating: 1500}, 30,
	))
	b := e.createTitle(t, titleInput(
		"并发称号 B", "#2563EB",
		gamification.TitleUnlockRating, gamification.TitleUnlockConfig{Rating: 1500}, 30,
	))
	e.setCoins(t, 100)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

	// 同一用户并发购买同一称号，只能扣款一次。
	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = e.svc.PurchaseTitle(ctx, e.user.ID, a.ID)
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		assertCode(t, err, 40001)
	}
	assert.Equal(t, 1, successes)
	assert.EqualValues(t, 70, e.balance(t))
	var purchaseCount int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ? AND reason = ?", e.user.ID, "title_purchase").
		Count(&purchaseCount).Error)
	assert.EqualValues(t, 1, purchaseCount)

	require.NoError(t, e.svc.PurchaseTitle(ctx, e.user.ID, b.ID))
	assert.EqualValues(t, 40, e.balance(t))

	// 并发装备两个已购买称号，最终仍只装备 1 个。
	equipErrs := make([]error, workers)
	equipStart := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-equipStart
			titleID := a.ID
			if i%2 == 1 {
				titleID = b.ID
			}
			equipErrs[i] = e.svc.EquipTitle(ctx, e.user.ID, titleID)
		}(i)
	}
	close(equipStart)
	wg.Wait()
	for _, err := range equipErrs {
		require.NoError(t, err)
	}
	var equipped int64
	require.NoError(t, e.db.Model(&gamification.UserTitle{}).
		Where("user_id = ? AND is_equipped = ?", e.user.ID, true).Count(&equipped).Error)
	assert.EqualValues(t, 1, equipped)
}

func TestTitleCRUDValidation(t *testing.T) {
	e := newTitleEnv(t)
	ctx := context.Background()
	userViewer := gamification.Viewer{UserID: e.user.ID}

	_, err := e.svc.CreateTitle(ctx, userViewer, titleInput(
		"无权限", "#7C3AED",
		gamification.TitleUnlockACCount, gamification.TitleUnlockConfig{Threshold: 1}, 0,
	))
	assertCode(t, err, 40300)

	cases := []struct {
		name   string
		mutate func(*gamification.TitleInput)
	}{
		{"名称为空", func(in *gamification.TitleInput) { in.Name = "  " }},
		{"名称过长", func(in *gamification.TitleInput) { in.Name = string(make([]rune, 101)) }},
		{"颜色缺少井号", func(in *gamification.TitleInput) { in.Color = "7C3AED" }},
		{"颜色长度错误", func(in *gamification.TitleInput) { in.Color = "#7C3AE" }},
		{"颜色非法字符", func(in *gamification.TitleInput) { in.Color = "#GGGGGG" }},
		{"价格为负", func(in *gamification.TitleInput) { in.PriceCoins = -1 }},
		{"价格过大", func(in *gamification.TitleInput) { in.PriceCoins = 1_000_001 }},
		{"未知解锁类型", func(in *gamification.TitleInput) {
			in.UnlockConditionType = gamification.TitleUnlockType("unknown")
		}},
		{"Rating 非正", func(in *gamification.TitleInput) {
			in.UnlockConditionType = gamification.TitleUnlockRating
			in.UnlockConditionConfig = gamification.TitleUnlockConfig{Rating: 0}
		}},
		{"阈值非正", func(in *gamification.TitleInput) {
			in.UnlockConditionType = gamification.TitleUnlockACCount
			in.UnlockConditionConfig = gamification.TitleUnlockConfig{Threshold: 0}
		}},
		{"成就 code 为空", func(in *gamification.TitleInput) {
			in.UnlockConditionType = gamification.TitleUnlockAchievement
			in.UnlockConditionConfig = gamification.TitleUnlockConfig{AchievementCode: ""}
		}},
		{"成就 code 格式错误", func(in *gamification.TitleInput) {
			in.UnlockConditionType = gamification.TitleUnlockAchievement
			in.UnlockConditionConfig = gamification.TitleUnlockConfig{AchievementCode: "First AC"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := titleInput("校验称号", "#7C3AED",
				gamification.TitleUnlockACCount, gamification.TitleUnlockConfig{Threshold: 1}, 0)
			tc.mutate(&in)
			_, err := e.svc.CreateTitle(ctx, adminViewer(e.admin.ID), in)
			assertCode(t, err, 40001)
		})
	}

	created := e.createTitle(t, titleInput(
		"可更新称号", "#7C3AED",
		gamification.TitleUnlockACCount, gamification.TitleUnlockConfig{Threshold: 1}, 100,
	))
	update := titleInput(
		"更新后的称号", "#2563EB",
		gamification.TitleUnlockRating, gamification.TitleUnlockConfig{Rating: 1800}, 200,
	)
	updated, err := e.svc.UpdateTitle(ctx, adminViewer(e.admin.ID), created.ID, update)
	require.NoError(t, err)
	assert.Equal(t, "更新后的称号", updated.Name)
	assert.Equal(t, "#2563EB", updated.Color)
	assert.Equal(t, string(gamification.TitleUnlockRating), updated.UnlockConditionType)
	assert.EqualValues(t, 200, updated.PriceCoins)
	assert.True(t, updated.IsActive, "更新省略 isActive 时保留原值")

	inactive := false
	update.IsActive = &inactive
	updated, err = e.svc.UpdateTitle(ctx, adminViewer(e.admin.ID), created.ID, update)
	require.NoError(t, err)
	assert.False(t, updated.IsActive)

	missingID := created.ID + 1000
	_, err = e.svc.UpdateTitle(ctx, adminViewer(e.admin.ID), missingID, update)
	assertCode(t, err, 40400)
	_, err = e.svc.UpdateTitle(ctx, userViewer, created.ID, update)
	assertCode(t, err, 40300)
	assertCode(t, e.svc.DeleteTitle(ctx, userViewer, created.ID), 40300)
	assertCode(t, e.svc.DeleteTitle(ctx, adminViewer(e.admin.ID), missingID), 40400)
	require.NoError(t, e.svc.DeleteTitle(ctx, adminViewer(e.admin.ID), created.ID))
}
