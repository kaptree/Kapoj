package gamification_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type badgeEnv struct {
	svc   *gamification.Service
	repo  *gamification.Repository
	db    *gorm.DB
	store storage.Storage
	user  *user.User
	admin *user.User
}

func newBadgeEnv(t *testing.T, maxEquipped int) *badgeEnv {
	t.Helper()
	g := testutil.PG(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)

	u := &user.User{
		Username:     "badge-user",
		Nickname:     "徽章用户",
		Email:        "badge-user@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	admin := &user.User{
		Username:     "badge-admin",
		Nickname:     "徽章管理员",
		Email:        "badge-admin@example.com",
		PasswordHash: "x",
		Role:         user.RoleSuperAdmin,
	}
	require.NoError(t, g.Create(admin).Error)

	repo := gamification.NewRepository(g)
	svc := gamification.NewServiceWithOptions(repo, gamification.ServiceOptions{
		MaxEquippedBadges: maxEquipped,
		Storage:           store,
	})
	return &badgeEnv{svc: svc, repo: repo, db: g, store: store, user: u, admin: admin}
}

func (e *badgeEnv) addUser(t *testing.T, name string) *user.User {
	t.Helper()
	u := &user.User{
		Username:     name,
		Nickname:     name,
		Email:        name + "@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, e.db.Create(u).Error)
	return u
}

func badgeInput(name string, price int64) gamification.BadgeInput {
	return gamification.BadgeInput{
		Name:        name,
		Description: "徽章描述",
		Category:    "测试",
		PriceCoins:  price,
	}
}

func limitedBadgeInput(name string, price, stock int64) gamification.BadgeInput {
	in := badgeInput(name, price)
	in.IsLimited = true
	in.Stock = stock
	return in
}

func (e *badgeEnv) createBadge(t *testing.T, in gamification.BadgeInput) *gamification.Badge {
	t.Helper()
	b, err := e.svc.CreateBadge(context.Background(), adminViewer(e.admin.ID), in)
	require.NoError(t, err)
	return b
}

func (e *badgeEnv) setCoins(t *testing.T, userID, coins int64) {
	t.Helper()
	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", userID).
		Update("coins", coins).Error)
}

func (e *badgeEnv) balance(t *testing.T, userID int64) int64 {
	t.Helper()
	balance, err := e.repo.Balance(context.Background(), userID)
	require.NoError(t, err)
	return balance
}

func (e *badgeEnv) addAcceptedProblem(t *testing.T, slug string) {
	t.Helper()
	p := &problem.Problem{Title: slug, Slug: slug}
	require.NoError(t, e.db.Create(p).Error)
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main() {}', ?, now())`,
		e.user.ID, p.ID, submission.StatusAccepted,
	).Error)
}

func findBadgeItem(t *testing.T, items []gamification.BadgeItem, id int64) gamification.BadgeItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("badge %d not found in %+v", id, items)
	return gamification.BadgeItem{}
}

var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R'}

func TestBadgePurchaseAndEquipLimit(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()

	badges := make([]*gamification.Badge, 0, 4)
	for _, name := range []string{"徽章一", "徽章二", "徽章三", "徽章四"} {
		badges = append(badges, e.createBadge(t, badgeInput(name, 0)))
	}
	for _, b := range badges {
		require.NoError(t, e.svc.PurchaseBadge(ctx, e.user.ID, b.ID))
	}

	for i := 0; i < 3; i++ {
		require.NoError(t, e.svc.EquipBadge(ctx, e.user.ID, badges[i].ID))
	}
	assertCode(t, e.svc.EquipBadge(ctx, e.user.ID, badges[3].ID), 40001)

	require.NoError(t, e.svc.UnequipBadge(ctx, e.user.ID, badges[0].ID))
	require.NoError(t, e.svc.EquipBadge(ctx, e.user.ID, badges[3].ID))

	items, err := e.svc.ListBadges(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	require.Len(t, items, 4)
	equipped := 0
	for _, item := range items {
		assert.True(t, item.Owned)
		assert.NotNil(t, item.PurchasedAt)
		if item.Equipped {
			equipped++
		}
	}
	assert.Equal(t, 3, equipped)

	// 0 金币购买也要留下 amount=0 的流水记录。
	var txCount int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ? AND reason = ?", e.user.ID, "badge_purchase").
		Count(&txCount).Error)
	assert.EqualValues(t, 4, txCount)
}

func TestLimitedBadgeConcurrentPurchase(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()
	other := e.addUser(t, "badge-rival")
	e.setCoins(t, e.user.ID, 100)
	e.setCoins(t, other.ID, 100)
	b := e.createBadge(t, limitedBadgeInput("限量徽章", 10, 1))

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, userID := range []int64{e.user.ID, other.ID} {
		wg.Add(1)
		go func(i int, userID int64) {
			defer wg.Done()
			<-start
			errs[i] = e.svc.PurchaseBadge(ctx, userID, b.ID)
		}(i, userID)
	}
	close(start)
	wg.Wait()

	successes := 0
	failures := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		failures++
		assertCode(t, err, 40001)
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)

	var got gamification.Badge
	require.NoError(t, e.db.First(&got, b.ID).Error)
	assert.EqualValues(t, 0, got.Stock)
	var ownedCount int64
	require.NoError(t, e.db.Model(&gamification.UserBadge{}).
		Where("badge_id = ?", b.ID).Count(&ownedCount).Error)
	assert.EqualValues(t, 1, ownedCount)
}

func TestBadgePurchaseIdempotentAndBalance(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()
	e.setCoins(t, e.user.ID, 100)
	b := e.createBadge(t, badgeInput("付费徽章", 100))

	require.NoError(t, e.svc.PurchaseBadge(ctx, e.user.ID, b.ID))
	assert.EqualValues(t, 0, e.balance(t, e.user.ID))

	assertCode(t, e.svc.PurchaseBadge(ctx, e.user.ID, b.ID), 40001)
	assert.EqualValues(t, 0, e.balance(t, e.user.ID))

	var txs []gamification.CoinTransaction
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).Find(&txs).Error)
	require.Len(t, txs, 1)
	assert.EqualValues(t, -100, txs[0].Amount)
	assert.Equal(t, gamification.CoinSpend, txs[0].Type)
	assert.Equal(t, "badge_purchase", txs[0].Reason)
	assert.Equal(t, "badge", txs[0].RefType)
	assert.EqualValues(t, b.ID, txs[0].RefID)

	var ownedCount int64
	require.NoError(t, e.db.Model(&gamification.UserBadge{}).
		Where("user_id = ? AND badge_id = ?", e.user.ID, b.ID).Count(&ownedCount).Error)
	assert.EqualValues(t, 1, ownedCount)
}

func TestBadgeInsufficientBalance(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()
	e.setCoins(t, e.user.ID, 50)
	b := e.createBadge(t, badgeInput("昂贵徽章", 100))

	assertCode(t, e.svc.PurchaseBadge(ctx, e.user.ID, b.ID), 40001)
	assert.EqualValues(t, 50, e.balance(t, e.user.ID))
	var ownedCount int64
	require.NoError(t, e.db.Model(&gamification.UserBadge{}).
		Where("user_id = ? AND badge_id = ?", e.user.ID, b.ID).Count(&ownedCount).Error)
	assert.EqualValues(t, 0, ownedCount)
}

func TestConcurrentEquipLimit(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()
	badges := make([]*gamification.Badge, 0, 4)
	for _, name := range []string{"并发一", "并发二", "并发三", "并发四"} {
		b := e.createBadge(t, badgeInput(name, 0))
		require.NoError(t, e.svc.PurchaseBadge(ctx, e.user.ID, b.ID))
		badges = append(badges, b)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, len(badges))
	for i, b := range badges {
		wg.Add(1)
		go func(i int, badgeID int64) {
			defer wg.Done()
			<-start
			errs[i] = e.svc.EquipBadge(ctx, e.user.ID, badgeID)
		}(i, b.ID)
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
	assert.Equal(t, 3, successes)
	var equipped int64
	require.NoError(t, e.db.Model(&gamification.UserBadge{}).
		Where("user_id = ? AND is_equipped = ?", e.user.ID, true).Count(&equipped).Error)
	assert.EqualValues(t, 3, equipped)
}

func TestBadgeImageUploadAndServe(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()
	b := e.createBadge(t, badgeInput("图片徽章", 0))

	require.NoError(t, e.svc.UploadBadgeImage(ctx, adminViewer(e.admin.ID), b.ID, bytes.NewReader(pngMagic)))
	stored, err := e.repo.FindBadge(ctx, b.ID)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("/api/badges/%d/image", b.ID), stored.ImageURL)

	img, err := e.svc.BadgeImage(ctx, b.ID)
	require.NoError(t, err)
	assert.Equal(t, "image/png", img.ContentType)
	assert.Equal(t, pngMagic, img.Data)

	tooLarge := append(append([]byte{}, pngMagic...), make([]byte, 2<<20)...)
	assertCode(t, e.svc.UploadBadgeImage(ctx, adminViewer(e.admin.ID), b.ID, bytes.NewReader(tooLarge)), 40001)
	assertCode(t, e.svc.UploadBadgeImage(ctx, adminViewer(e.admin.ID), b.ID,
		bytes.NewReader([]byte("plain text"))), 40001)
	assertCode(t, e.svc.UploadBadgeImage(ctx, gamification.Viewer{UserID: e.user.ID}, b.ID,
		bytes.NewReader(pngMagic)), 40300)
	assertCode(t, e.svc.UploadBadgeImage(ctx, adminViewer(e.admin.ID), b.ID+1000,
		bytes.NewReader(pngMagic)), 40400)

	external := e.createBadge(t, func() gamification.BadgeInput {
		in := badgeInput("外链徽章", 0)
		in.ImageURL = "https://example.com/badge.png"
		return in
	}())
	_, err = e.svc.BadgeImage(ctx, external.ID)
	assertCode(t, err, 40400)

	_, err = e.svc.BadgeImage(ctx, b.ID+1000)
	assertCode(t, err, 40400)
}

func TestAchievementRewardsBadgeWithoutStock(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()
	b := e.createBadge(t, limitedBadgeInput("成就徽章", 100, 0))

	in := achievementInput(
		"badge-reward", "徽章奖励",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 0,
	)
	in.RewardBadgeID = &b.ID
	a, err := e.svc.CreateAchievement(ctx, adminViewer(e.admin.ID), in)
	require.NoError(t, err)
	require.NotNil(t, a.RewardBadgeID)

	e.addAcceptedProblem(t, "badge-reward-problem")
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

	var owned gamification.UserBadge
	require.NoError(t, e.db.Where("user_id = ? AND badge_id = ?", e.user.ID, b.ID).
		First(&owned).Error)
	assert.False(t, owned.PurchasedAt.IsZero())

	stored, err := e.repo.FindBadge(ctx, b.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, stored.Stock, "成就奖励不占用商城库存")

	// 重复补算不重复插入 user_badges。
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	var count int64
	require.NoError(t, e.db.Model(&gamification.UserBadge{}).
		Where("user_id = ? AND badge_id = ?", e.user.ID, b.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	// 更新成就省略 rewardBadgeId 时保留原奖励徽章。
	update := in
	update.RewardBadgeID = nil
	updated, err := e.svc.UpdateAchievement(ctx, adminViewer(e.admin.ID), a.ID, update)
	require.NoError(t, err)
	require.NotNil(t, updated.RewardBadgeID)
	assert.Equal(t, b.ID, *updated.RewardBadgeID)
}

func TestAchievementBadgeRewardValidation(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()

	missing := int64(999999)
	in := achievementInput(
		"missing-badge", "缺失徽章",
		gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}, 0,
	)
	in.RewardBadgeID = &missing
	assertCode(t, func() error {
		_, err := e.svc.CreateAchievement(ctx, adminViewer(e.admin.ID), in)
		return err
	}(), 40001)

	inactive := e.createBadge(t, badgeInput("下线徽章", 0))
	require.NoError(t, e.svc.DeleteBadge(ctx, adminViewer(e.admin.ID), inactive.ID))
	in.Code = "inactive-badge"
	in.RewardBadgeID = &inactive.ID
	assertCode(t, func() error {
		_, err := e.svc.CreateAchievement(ctx, adminViewer(e.admin.ID), in)
		return err
	}(), 40001)
}

func TestInactiveOwnedBadgeVisibleAndEquippable(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()

	owned := e.createBadge(t, badgeInput("已拥有下线", 0))
	require.NoError(t, e.svc.PurchaseBadge(ctx, e.user.ID, owned.ID))
	require.NoError(t, e.svc.DeleteBadge(ctx, adminViewer(e.admin.ID), owned.ID))

	notOwned := e.createBadge(t, badgeInput("未拥有下线", 0))
	require.NoError(t, e.svc.DeleteBadge(ctx, adminViewer(e.admin.ID), notOwned.ID))

	guestItems, err := e.svc.ListBadges(ctx, gamification.Viewer{}, false)
	require.NoError(t, err)
	assert.Empty(t, guestItems)

	userItems, err := e.svc.ListBadges(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	require.Len(t, userItems, 1)
	item := findBadgeItem(t, userItems, owned.ID)
	assert.True(t, item.Owned)
	assert.False(t, item.IsActive)

	require.NoError(t, e.svc.EquipBadge(ctx, e.user.ID, owned.ID))
	userItems, err = e.svc.ListBadges(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	assert.True(t, findBadgeItem(t, userItems, owned.ID).Equipped)
	require.NoError(t, e.svc.UnequipBadge(ctx, e.user.ID, owned.ID))

	adminItems, err := e.svc.ListBadges(ctx, adminViewer(e.admin.ID), true)
	require.NoError(t, err)
	assert.Len(t, adminItems, 2)

	_, err = e.svc.ListBadges(ctx, gamification.Viewer{UserID: e.user.ID}, true)
	assertCode(t, err, 40300)

	userBadges, err := e.svc.ListUserBadges(ctx, e.user.ID)
	require.NoError(t, err)
	require.Len(t, userBadges, 1)
	assert.Equal(t, owned.ID, userBadges[0].ID)
	assert.True(t, userBadges[0].Owned)
	assert.False(t, userBadges[0].Equipped)

	_, err = e.svc.ListUserBadges(ctx, e.user.ID+1000)
	assertCode(t, err, 40400)
}

func TestBadgeCRUDValidation(t *testing.T) {
	e := newBadgeEnv(t, 3)
	ctx := context.Background()
	userViewer := gamification.Viewer{UserID: e.user.ID}

	_, err := e.svc.CreateBadge(ctx, userViewer, badgeInput("无权限", 0))
	assertCode(t, err, 40300)

	cases := []struct {
		name   string
		mutate func(*gamification.BadgeInput)
	}{
		{"名称为空", func(in *gamification.BadgeInput) { in.Name = "  " }},
		{"名称过长", func(in *gamification.BadgeInput) { in.Name = string(make([]rune, 101)) }},
		{"描述过长", func(in *gamification.BadgeInput) { in.Description = string(make([]rune, 501)) }},
		{"分类为空", func(in *gamification.BadgeInput) { in.Category = " " }},
		{"分类过长", func(in *gamification.BadgeInput) { in.Category = string(make([]rune, 21)) }},
		{"价格为负", func(in *gamification.BadgeInput) { in.PriceCoins = -1 }},
		{"价格过大", func(in *gamification.BadgeInput) { in.PriceCoins = 1_000_001 }},
		{"限量库存为负", func(in *gamification.BadgeInput) { in.IsLimited = true; in.Stock = -1 }},
		{"图片地址非法", func(in *gamification.BadgeInput) { in.ImageURL = "ftp://example.com/x.png" }},
		{"内部图片地址非法", func(in *gamification.BadgeInput) { in.ImageURL = "/api/badges/not-a-number/image" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := badgeInput("校验徽章", 0)
			tc.mutate(&in)
			_, err := e.svc.CreateBadge(ctx, adminViewer(e.admin.ID), in)
			assertCode(t, err, 40001)
		})
	}

	b := e.createBadge(t, badgeInput("可更新徽章", 100))
	update := badgeInput("更新后的徽章", 200)
	update.IsLimited = true
	update.Stock = 5
	updated, err := e.svc.UpdateBadge(ctx, adminViewer(e.admin.ID), b.ID, update)
	require.NoError(t, err)
	assert.Equal(t, "更新后的徽章", updated.Name)
	assert.True(t, updated.IsLimited)
	assert.EqualValues(t, 5, updated.Stock)
	assert.True(t, updated.IsActive, "更新省略 isActive 时保留原值")

	inactive := false
	update.IsActive = &inactive
	updated, err = e.svc.UpdateBadge(ctx, adminViewer(e.admin.ID), b.ID, update)
	require.NoError(t, err)
	assert.False(t, updated.IsActive)

	missingID := b.ID + 1000
	update.ImageURL = fmt.Sprintf("/api/badges/%d/image", missingID)
	_, err = e.svc.UpdateBadge(ctx, adminViewer(e.admin.ID), b.ID, update)
	assertCode(t, err, 40001)

	update.ImageURL = fmt.Sprintf("/api/badges/%d/image", b.ID)
	_, err = e.svc.UpdateBadge(ctx, adminViewer(e.admin.ID), b.ID, update)
	require.NoError(t, err)

	_, err = e.svc.UpdateBadge(ctx, adminViewer(e.admin.ID), missingID, update)
	assertCode(t, err, 40400)
	_, err = e.svc.UpdateBadge(ctx, userViewer, b.ID, update)
	assertCode(t, err, 40300)

	assertCode(t, e.svc.DeleteBadge(ctx, adminViewer(e.admin.ID), missingID), 40400)
	assertCode(t, e.svc.DeleteBadge(ctx, userViewer, b.ID), 40300)
	require.NoError(t, e.svc.DeleteBadge(ctx, adminViewer(e.admin.ID), b.ID))
}
