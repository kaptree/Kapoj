package gamification_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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
