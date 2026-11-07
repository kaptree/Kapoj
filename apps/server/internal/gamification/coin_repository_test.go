package gamification_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

const coinUserID = int64(1)

type coinEnv struct {
	repo *gamification.Repository
	db   *gorm.DB
}

func newCoinEnv(t *testing.T) *coinEnv {
	t.Helper()
	g := testutil.PG(t)
	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (?, 'coinuser', 'coinuser', 'coinuser@example.com', 'x', now(), now())`,
		coinUserID,
	).Error)
	return &coinEnv{repo: gamification.NewRepository(g), db: g}
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestCreditCoinsIsIdempotent(t *testing.T) {
	e := newCoinEnv(t)
	ok, err := e.repo.CreditCoins(context.Background(), coinUserID, 50, "achievement", "achievement", 7)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = e.repo.CreditCoins(context.Background(), coinUserID, 50, "achievement", "achievement", 7)
	require.NoError(t, err)
	assert.False(t, ok)
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 50, balance)
}

func TestCreditCoinsConcurrent(t *testing.T) {
	e := newCoinEnv(t)
	const workers = 8
	var wg sync.WaitGroup
	results := make(chan bool, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ok, err := e.repo.CreditCoins(context.Background(), coinUserID, 50, "achievement", "achievement", 7)
			results <- ok
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	var credited int
	for ok := range results {
		if ok {
			credited++
		}
	}
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, credited)
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 50, balance)
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestSpendCoinsRejectsInsufficientBalance(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 10, "测试"))
	err := e.repo.SpendCoins(context.Background(), coinUserID, 11, "badge_purchase", "badge", 1)
	assertCode(t, err, 40001)
	balance, _ := e.repo.Balance(context.Background(), coinUserID)
	assert.EqualValues(t, 10, balance)
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("type = ?", gamification.CoinSpend).Count(&count).Error)
	assert.Zero(t, count, "失败扣款必须回滚流水")
}

func TestSpendCoinsConcurrent(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 10, "测试"))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	start := make(chan struct{})
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go func(refID int64) {
			defer wg.Done()
			<-start
			errs <- e.repo.SpendCoins(context.Background(), coinUserID, 8, "badge_purchase", "badge", refID)
		}(int64(i))
	}
	close(start)
	wg.Wait()
	close(errs)

	var success int
	for err := range errs {
		if err == nil {
			success++
			continue
		}
		assertCode(t, err, 40001)
	}
	assert.Equal(t, 1, success)
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, balance)
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("type = ?", gamification.CoinSpend).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestSpendCoinsTxWithinCallerTransaction(t *testing.T) {
	e := newCoinEnv(t)
	ctx := context.Background()
	require.NoError(t, e.repo.AdminAdjustCoins(ctx, coinUserID, 50, "初始"))

	// 在调用方事务内扣款，写入流水并扣减余额。
	require.NoError(t, e.db.Transaction(func(tx *gorm.DB) error {
		return gamification.SpendCoinsTx(tx, coinUserID, 30, "problem_solution", "problem_solution", 5)
	}))
	balance, err := e.repo.Balance(ctx, coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 20, balance)

	// 相同引用幂等，不重复扣款。
	require.NoError(t, e.db.Transaction(func(tx *gorm.DB) error {
		return gamification.SpendCoinsTx(tx, coinUserID, 30, "problem_solution", "problem_solution", 5)
	}))
	balance, err = e.repo.Balance(ctx, coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 20, balance)

	// 余额不足返回 40001 且整体回滚（无流水、余额不变）。
	err = e.db.Transaction(func(tx *gorm.DB) error {
		return gamification.SpendCoinsTx(tx, coinUserID, 999, "problem_solution", "problem_solution", 6)
	})
	assertCode(t, err, 40001)
	balance, err = e.repo.Balance(ctx, coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 20, balance)
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("reason = ? AND ref_id = ?", "problem_solution", 6).Count(&count).Error)
	assert.Zero(t, count, "失败扣款必须回滚流水")
}

func TestSpendCoinsTxRejectsInvalidReference(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 10, "初始"))

	err := e.db.Transaction(func(tx *gorm.DB) error {
		return gamification.SpendCoinsTx(tx, coinUserID, 5, "problem_solution", "", 1)
	})
	assertCode(t, err, 40001)

	err = e.db.Transaction(func(tx *gorm.DB) error {
		return gamification.SpendCoinsTx(tx, coinUserID, 5, "problem_solution", "problem_solution", 0)
	})
	assertCode(t, err, 40001)

	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 10, balance)
}

func TestAdminAdjustCoinsAllowsRepeatedNotes(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 5, "活动"))
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 5, "活动"))
	balance, _ := e.repo.Balance(context.Background(), coinUserID)
	assert.EqualValues(t, 10, balance)
}

func TestAdminAdjustCoinsRejectsNegativeBalance(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 5, "初始"))
	err := e.repo.AdminAdjustCoins(context.Background(), coinUserID, -6, "扣减")
	assertCode(t, err, 40001)
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 5, balance)
}

func TestAdminAdjustCoinsWithAuditRollsBackOnAuditError(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 10, "初始"))

	err := e.repo.AdminAdjustCoinsWithAudit(
		context.Background(), coinUserID, 50, "活动",
		func(tx *gorm.DB) error { return errors.New("audit boom") },
	)
	require.Error(t, err)

	// 审计失败必须整体回滚：余额不变、无新增流水。
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 10, balance)
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("type = ?", gamification.CoinAdminAdjust).Count(&count).Error)
	assert.EqualValues(t, 1, count, "审计失败必须回滚新增的调整流水")
}

func TestAdminAdjustCoinsWithAuditWritesAuditOnSuccess(t *testing.T) {
	e := newCoinEnv(t)
	var auditCalled bool
	err := e.repo.AdminAdjustCoinsWithAudit(
		context.Background(), coinUserID, 20, "活动",
		func(tx *gorm.DB) error {
			auditCalled = true
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, auditCalled)
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 20, balance)
}

func TestServiceAdminAdjustCoinsRequiresCanAdjustCoins(t *testing.T) {
	e := newCoinEnv(t)
	svc := gamification.NewService(e.repo)

	_, err := svc.AdminAdjustCoins(context.Background(), gamification.Viewer{UserID: coinUserID}, coinUserID, 10, "活动")
	assertCode(t, err, 40300)

	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, balance)

	_, err = svc.AdminAdjustCoinsWithAudit(context.Background(), gamification.Viewer{CanAdjustCoins: true}, coinUserID, 10, "活动", nil)
	require.NoError(t, err)
	balance, err = e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 10, balance)
}

func TestListCoinTransactionsReturnsBalanceAndTotal(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 10, "活动"))
	ok, err := e.repo.CreditCoins(context.Background(), coinUserID, 5, "achievement", "achievement", 1)
	require.NoError(t, err)
	require.True(t, ok)

	items, total, balance, err := e.repo.ListCoinTransactions(context.Background(), coinUserID, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.EqualValues(t, 15, balance)
	require.Len(t, items, 2)
	assert.EqualValues(t, 5, items[0].Amount)
	assert.Equal(t, gamification.CoinEarn, items[0].Type)
	assert.Equal(t, gamification.CoinAdminAdjust, items[1].Type)
	assert.Equal(t, "活动", items[1].Note)
}

func TestCreditCoinsRejectsInvalidReference(t *testing.T) {
	e := newCoinEnv(t)
	cases := []struct {
		name    string
		refType string
		refID   int64
		want    string
	}{
		{"空引用类型", "", 1, "引用类型不能为空"},
		{"零引用ID", "achievement", 0, "引用 ID 必须为正数"},
		{"负数引用ID", "achievement", -1, "引用 ID 必须为正数"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := e.repo.CreditCoins(context.Background(), coinUserID, 50, "achievement", tc.refType, tc.refID)
			assert.False(t, ok)
			assertCode(t, err, 40001)
			var appErr *httpx.AppError
			require.ErrorAs(t, err, &appErr)
			assert.Equal(t, tc.want, appErr.Message)
		})
	}
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.Zero(t, balance)
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSpendCoinsRejectsInvalidReference(t *testing.T) {
	e := newCoinEnv(t)
	require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), coinUserID, 10, "初始"))
	cases := []struct {
		name    string
		refType string
		refID   int64
		want    string
	}{
		{"空引用类型", "", 1, "引用类型不能为空"},
		{"零引用ID", "badge", 0, "引用 ID 必须为正数"},
		{"负数引用ID", "badge", -1, "引用 ID 必须为正数"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.repo.SpendCoins(context.Background(), coinUserID, 8, "badge_purchase", tc.refType, tc.refID)
			assertCode(t, err, 40001)
			var appErr *httpx.AppError
			require.ErrorAs(t, err, &appErr)
			assert.Equal(t, tc.want, appErr.Message)
		})
	}
	balance, err := e.repo.Balance(context.Background(), coinUserID)
	require.NoError(t, err)
	assert.EqualValues(t, 10, balance)
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("type = ?", gamification.CoinSpend).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateInactiveRecordsPersistsFalse(t *testing.T) {
	e := newCoinEnv(t)

	achievement := &gamification.Achievement{
		Code:            "inactive-achievement",
		Name:            "未上架成就",
		ConditionType:   "ac_count",
		ConditionConfig: dbtypes.JSON[map[string]any]{Data: map[string]any{"threshold": 1}},
		IsActive:        false,
	}
	require.NoError(t, e.db.Create(achievement).Error)
	var gotAchievement gamification.Achievement
	require.NoError(t, e.db.First(&gotAchievement, achievement.ID).Error)
	assert.False(t, gotAchievement.IsActive)

	now := time.Now()
	task := &gamification.Task{
		Title:      "未上架任务",
		ProblemIDs: dbtypes.List[int64]{1},
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		IsActive:   false,
	}
	require.NoError(t, e.db.Create(task).Error)
	var gotTask gamification.Task
	require.NoError(t, e.db.First(&gotTask, task.ID).Error)
	assert.False(t, gotTask.IsActive)
}
