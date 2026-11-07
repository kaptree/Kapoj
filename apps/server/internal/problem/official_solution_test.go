package problem_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type osEnv struct {
	svc  *problem.OfficialSolutionService
	repo problem.Repository
	db   *gorm.DB
}

func newOSEnv(t *testing.T) *osEnv {
	t.Helper()
	g := testutil.PG(t)
	repo := problem.NewGormRepository(g)
	return &osEnv{svc: problem.NewOfficialSolutionService(g, repo), repo: repo, db: g}
}

// addUser inserts a user with the given id and coin balance.
func (e *osEnv) addUser(t *testing.T, id, coins int64) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, coins, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'x', ?, now(), now())`,
		id, fmt.Sprintf("u%d", id), fmt.Sprintf("u%d", id), fmt.Sprintf("u%d@example.com", id), coins,
	).Error)
}

// addProblem inserts a problem owned by createdBy with the given visibility,
// price and official solution content.
func (e *osEnv) addProblem(t *testing.T, slug string, createdBy int64, public bool, price int64, md, code string) *problem.Problem {
	t.Helper()
	p := &problem.Problem{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		CreatedBy: createdBy, IsPublic: public,
		SolutionMD: md, SolutionCode: code, SolutionPriceCoins: price,
	}
	require.NoError(t, e.repo.Create(context.Background(), p))
	return p
}

// addAccepted marks the user as having accepted the problem.
func (e *osEnv) addAccepted(t *testing.T, userID, problemID int64) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'accepted', now())`, userID, problemID,
	).Error)
}

// addUnlock records a direct unlock row for the user/problem.
func (e *osEnv) addUnlock(t *testing.T, userID, problemID int64) {
	t.Helper()
	require.NoError(t, e.db.Create(&problem.ProblemSolutionUnlock{
		UserID: userID, ProblemID: problemID, PricePaid: 100,
	}).Error)
}

func (e *osEnv) coins(t *testing.T, userID int64) int64 {
	t.Helper()
	var c int64
	require.NoError(t, e.db.Raw(`SELECT coins FROM users WHERE id = ?`, userID).Scan(&c).Error)
	return c
}

func (e *osEnv) unlockCount(t *testing.T, userID, problemID int64) int64 {
	t.Helper()
	var n int64
	require.NoError(t, e.db.Model(&problem.ProblemSolutionUnlock{}).
		Where("user_id = ? AND problem_id = ?", userID, problemID).Count(&n).Error)
	return n
}

func assertOSCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestOfficialSolutionAccessStates(t *testing.T) {
	e := newOSEnv(t)
	ctx := context.Background()

	const (
		creator  = int64(10)
		locked   = int64(11)
		accepted = int64(12)
		buyer    = int64(13)
	)
	e.addUser(t, creator, 0)
	e.addUser(t, locked, 0)
	e.addUser(t, accepted, 0)
	e.addUser(t, buyer, 0)

	p := e.addProblem(t, "paid", creator, true, 100, "PAID_MD", "PAID_CODE")
	e.addAccepted(t, accepted, p.ID)
	e.addUnlock(t, buyer, p.ID)

	// guest -> guest, no content, cannot purchase (not logged in)
	v, err := e.svc.Get(ctx, 0, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "guest", v.Access)
	assert.Equal(t, int64(100), v.PriceCoins)
	assert.False(t, v.CanEdit)
	assert.False(t, v.CanPurchase)
	assert.Nil(t, v.SolutionMD)
	assert.Nil(t, v.SolutionCode)

	// locked user -> locked, no content, can purchase
	v, err = e.svc.Get(ctx, locked, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "locked", v.Access)
	assert.True(t, v.CanPurchase)
	assert.Nil(t, v.SolutionMD)
	assert.Nil(t, v.SolutionCode)

	// accepted -> accepted, content
	v, err = e.svc.Get(ctx, accepted, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "accepted", v.Access)
	assert.False(t, v.CanPurchase)
	assert.Equal(t, "PAID_MD", *v.SolutionMD)
	assert.Equal(t, "PAID_CODE", *v.SolutionCode)

	// purchased -> purchased, content
	v, err = e.svc.Get(ctx, buyer, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "purchased", v.Access)
	assert.False(t, v.CanPurchase)
	assert.Equal(t, "PAID_MD", *v.SolutionMD)

	// creator -> creator, content, editable
	v, err = e.svc.Get(ctx, creator, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "creator", v.Access)
	assert.True(t, v.CanEdit)
	assert.False(t, v.CanPurchase)
	assert.Equal(t, "PAID_MD", *v.SolutionMD)

	// super -> creator, content, editable
	v, err = e.svc.Get(ctx, 999, p.ID, problem.Viewer{}, true)
	require.NoError(t, err)
	assert.Equal(t, "creator", v.Access)
	assert.True(t, v.CanEdit)
	assert.False(t, v.CanPurchase)
	assert.Equal(t, "PAID_MD", *v.SolutionMD)

	// price 0 -> free for everyone, content
	free := e.addProblem(t, "free", creator, true, 0, "FREE_MD", "FREE_CODE")
	v, err = e.svc.Get(ctx, 0, free.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "free", v.Access)
	assert.False(t, v.CanEdit)
	assert.False(t, v.CanPurchase)
	assert.Equal(t, "FREE_MD", *v.SolutionMD)
	assert.Equal(t, "FREE_CODE", *v.SolutionCode)
}

func TestOfficialSolutionHiddenProblem(t *testing.T) {
	e := newOSEnv(t)
	ctx := context.Background()
	creator := int64(20)
	e.addUser(t, creator, 0)
	p := e.addProblem(t, "hidden", creator, false, 100, "HIDDEN_MD", "HIDDEN_CODE")

	// 无 view_hidden / super -> 404，且不泄露正文。
	_, err := e.svc.Get(ctx, 0, p.ID, problem.Viewer{}, false)
	assertOSCode(t, err, 40400)
	_, err = e.svc.Get(ctx, int64(30), p.ID, problem.Viewer{}, false)
	assertOSCode(t, err, 40400)

	// view_hidden -> 200（locked，不泄露正文）。
	v, err := e.svc.Get(ctx, int64(30), p.ID, problem.Viewer{CanViewHidden: true}, false)
	require.NoError(t, err)
	assert.Equal(t, "locked", v.Access)
	assert.Nil(t, v.SolutionMD)

	// super -> 200 creator。
	v, err = e.svc.Get(ctx, int64(30), p.ID, problem.Viewer{}, true)
	require.NoError(t, err)
	assert.Equal(t, "creator", v.Access)
	assert.Equal(t, "HIDDEN_MD", *v.SolutionMD)
}

func TestOfficialSolutionUpdateOnlyCreatorOrSuper(t *testing.T) {
	e := newOSEnv(t)
	ctx := context.Background()
	creator := int64(40)
	other := int64(41)
	e.addUser(t, creator, 0)
	e.addUser(t, other, 0)
	p := e.addProblem(t, "edit", creator, true, 100, "", "")

	in := problem.OfficialSolutionInput{SolutionMD: "NEW_MD", SolutionCode: "NEW_CODE", PriceCoins: 200}

	// 其他用户 -> 403。
	_, err := e.svc.Update(ctx, other, p.ID, false, in)
	assertOSCode(t, err, 40300)

	// creator -> 成功。
	v, err := e.svc.Update(ctx, creator, p.ID, false, in)
	require.NoError(t, err)
	assert.Equal(t, "creator", v.Access)
	assert.Equal(t, "NEW_MD", *v.SolutionMD)
	assert.Equal(t, int64(200), v.PriceCoins)

	// super -> 成功。
	in2 := problem.OfficialSolutionInput{SolutionMD: "SUPER_MD", SolutionCode: "SUPER_CODE", PriceCoins: 300}
	v, err = e.svc.Update(ctx, int64(99), p.ID, true, in2)
	require.NoError(t, err)
	assert.Equal(t, "SUPER_MD", *v.SolutionMD)
	assert.Equal(t, int64(300), v.PriceCoins)

	// 价格越界。
	for _, bad := range []int64{-1, 1_000_001} {
		_, err = e.svc.Update(ctx, creator, p.ID, false, problem.OfficialSolutionInput{SolutionMD: "x", PriceCoins: bad})
		assertOSCode(t, err, 40001)
	}

	// 说明超长。
	_, err = e.svc.Update(ctx, creator, p.ID, false, problem.OfficialSolutionInput{
		SolutionMD: strings.Repeat("字", 50001), PriceCoins: 0,
	})
	assertOSCode(t, err, 40001)

	// 代码超长。
	_, err = e.svc.Update(ctx, creator, p.ID, false, problem.OfficialSolutionInput{
		SolutionMD: "x", SolutionCode: strings.Repeat("a", 65537), PriceCoins: 0,
	})
	assertOSCode(t, err, 40001)
}

func TestOfficialSolutionPurchase(t *testing.T) {
	e := newOSEnv(t)
	ctx := context.Background()
	creator := int64(50)
	buyer := int64(51)
	accepted := int64(52)
	e.addUser(t, creator, 0)
	e.addUser(t, buyer, 100)
	e.addUser(t, accepted, 100)

	p := e.addProblem(t, "buy", creator, true, 100, "BUY_MD", "BUY_CODE")

	// 价格 0 不扣费。
	free := e.addProblem(t, "buy-free", creator, true, 0, "FREE_MD", "")
	v, err := e.svc.Purchase(ctx, buyer, free.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "free", v.Access)
	assert.Equal(t, int64(100), e.coins(t, buyer))
	assert.Zero(t, e.unlockCount(t, buyer, free.ID))

	// 已通过不扣费。
	e.addAccepted(t, accepted, p.ID)
	v, err = e.svc.Purchase(ctx, accepted, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "accepted", v.Access)
	assert.Equal(t, int64(100), e.coins(t, accepted))
	assert.Zero(t, e.unlockCount(t, accepted, p.ID))

	// 创建者不扣费。
	v, err = e.svc.Purchase(ctx, creator, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "creator", v.Access)
	assert.Equal(t, int64(0), e.coins(t, creator))
	assert.Zero(t, e.unlockCount(t, creator, p.ID))

	// 正常购买扣费并写解锁记录。
	v, err = e.svc.Purchase(ctx, buyer, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "purchased", v.Access)
	assert.Equal(t, "BUY_MD", *v.SolutionMD)
	assert.Equal(t, int64(0), e.coins(t, buyer))
	assert.Equal(t, int64(1), e.unlockCount(t, buyer, p.ID))

	// 重复购买幂等，不重复扣费、不重复写记录。
	v, err = e.svc.Purchase(ctx, buyer, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "purchased", v.Access)
	assert.Equal(t, int64(0), e.coins(t, buyer))
	assert.Equal(t, int64(1), e.unlockCount(t, buyer, p.ID))
}

func TestOfficialSolutionPurchaseConcurrentSingleCharge(t *testing.T) {
	e := newOSEnv(t)
	ctx := context.Background()
	creator := int64(70)
	buyer := int64(71)
	e.addUser(t, creator, 0)
	e.addUser(t, buyer, 100)

	p := e.addProblem(t, "buy-conc", creator, true, 100, "CONC_MD", "CONC_CODE")

	const workers = 4
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			_, err := e.svc.Purchase(ctx, buyer, p.ID, problem.Viewer{}, false)
			errs <- err
		}()
	}
	close(start)
	for i := 0; i < workers; i++ {
		require.NoError(t, <-errs)
	}

	// 只扣一次：余额 0，仅一条解锁记录。
	assert.Equal(t, int64(0), e.coins(t, buyer))
	assert.Equal(t, int64(1), e.unlockCount(t, buyer, p.ID))
}

func TestOfficialSolutionPurchaseInsufficientCoinsRollsBack(t *testing.T) {
	e := newOSEnv(t)
	ctx := context.Background()
	creator := int64(60)
	poor := int64(61)
	e.addUser(t, creator, 0)
	e.addUser(t, poor, 50)

	p := e.addProblem(t, "poor", creator, true, 100, "POOR_MD", "POOR_CODE")

	_, err := e.svc.Purchase(ctx, poor, p.ID, problem.Viewer{}, false)
	assertOSCode(t, err, 40001)

	// 余额不变、不写解锁记录。
	assert.Equal(t, int64(50), e.coins(t, poor))
	assert.Zero(t, e.unlockCount(t, poor, p.ID))

	// 仍处于 locked。
	v, err := e.svc.Get(ctx, poor, p.ID, problem.Viewer{}, false)
	require.NoError(t, err)
	assert.Equal(t, "locked", v.Access)
	assert.Nil(t, v.SolutionMD)
}
