package problem_test

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type env struct {
	svc   *problem.Service
	repo  problem.Repository
	store storage.Storage
	db    *gorm.DB
	root  string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := testutil.PG(t)
	root := filepath.Join(t.TempDir(), "store")
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	repo := problem.NewGormRepository(g)
	return &env{svc: problem.NewService(repo, store), repo: repo, store: store, db: g, root: root}
}

type failingCheckerRepo struct {
	problem.Repository
}

func (failingCheckerRepo) SetCheckerPath(context.Context, int64, string) error {
	return errors.New("database unavailable")
}

func (e *env) files(t *testing.T) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(e.root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func input(slug string) problem.Input {
	return problem.Input{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
	}
}

func (e *env) create(t *testing.T, in problem.Input) *problem.Problem {
	t.Helper()
	p, err := e.svc.Create(context.Background(), 1, in)
	require.NoError(t, err)
	return p
}

func upload(in, out string) problem.TestcaseUpload {
	return problem.TestcaseUpload{Input: strings.NewReader(in), Output: strings.NewReader(out), Score: 10}
}

func (e *env) addCase(t *testing.T, id int64) *problem.Testcase {
	t.Helper()
	tc, err := e.svc.AddTestcase(context.Background(), id, upload("1 2", "3"))
	require.NoError(t, err)
	return tc
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func zeroBytes(n int64) io.Reader { return io.LimitReader(zeros{}, n) }

var (
	guest   = problem.Viewer{}
	manager = problem.Viewer{CanViewHidden: true}
)

func TestCreateForcesHidden(t *testing.T) {
	e := newEnv(t)
	in := input("force")
	in.IsPublic = true
	p := e.create(t, in)
	assert.False(t, p.IsPublic)
	assert.Equal(t, int64(1), p.CreatedBy)
}

func TestCreateDuplicateSlug(t *testing.T) {
	e := newEnv(t)
	e.create(t, input("dup"))
	_, err := e.svc.Create(context.Background(), 1, input("DUP"))
	assertCode(t, err, 40903)
}

func TestCreateValidates(t *testing.T) {
	e := newEnv(t)
	in := input("bad")
	in.Difficulty = 9
	_, err := e.svc.Create(context.Background(), 1, in)
	assertCode(t, err, 40001)
}

func TestGetHiddenAsNonManager(t *testing.T) {
	e := newEnv(t)
	p := e.create(t, input("hidden"))
	ctx := context.Background()

	_, err := e.svc.Get(ctx, guest, p.ID)
	assertCode(t, err, 40400)
	_, err = e.svc.Statistics(ctx, guest, p.ID)
	assertCode(t, err, 40400)
	got, err := e.svc.Get(ctx, manager, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)

	_, err = e.svc.Get(ctx, guest, 999999)
	assertCode(t, err, 40400)
}

func TestListHidesNonPublicUnlessManager(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	pub := e.create(t, input("public"))
	e.addCase(t, pub.ID)
	in := input("public")
	in.IsPublic = true
	_, err := e.svc.Update(ctx, pub.ID, in)
	require.NoError(t, err)
	e.create(t, input("secret"))

	page, err := e.svc.List(ctx, guest, problem.ListQuery{IncludeHidden: true}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total, "includeHidden is ignored for non-managers")
	require.Len(t, page.Items, 1)
	assert.Equal(t, "public", page.Items[0].Slug)

	page, err = e.svc.List(ctx, manager, problem.ListQuery{IncludeHidden: true}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(2), page.Total)

	page, err = e.svc.List(ctx, manager, problem.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total, "managers also get only public problems unless they ask")
}

func TestListHugePageIsEmpty(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("one"))
	e.addCase(t, p.ID)
	in := input("one")
	in.IsPublic = true
	_, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)

	page, err := e.svc.List(ctx, manager, problem.ListQuery{}, math.MaxInt, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	assert.Empty(t, page.Items, "an absurd page number must not wrap around to the first page")
	assert.NotNil(t, page.Items)
}

func TestPublishRequiresTestcase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("pub"))
	in := input("pub")
	in.IsPublic = true

	_, err := e.svc.Update(ctx, p.ID, in)
	assertCode(t, err, 40001)

	e.addCase(t, p.ID)
	updated, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.True(t, updated.IsPublic)
	_, err = e.svc.Get(ctx, guest, p.ID)
	assert.NoError(t, err)
}

func TestPublishSpecialRequiresChecker(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := input("spj")
	in.Type = problem.TypeSpecial
	p := e.create(t, in)
	e.addCase(t, p.ID)
	in.IsPublic = true

	_, err := e.svc.Update(ctx, p.ID, in)
	assertCode(t, err, 40001)

	require.NoError(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("int main(){}")))
	_, err = e.svc.Update(ctx, p.ID, in)
	assert.NoError(t, err)
}

func TestChangingPublicProblemToSpecialNeedsChecker(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("flip"))
	e.addCase(t, p.ID)
	in := input("flip")
	in.IsPublic = true
	_, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)

	in.Type = problem.TypeSpecial
	_, err = e.svc.Update(ctx, p.ID, in)
	assertCode(t, err, 40001)
}

func TestUpdateMissingAndDuplicateSlug(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.Update(ctx, 999999, input("missing"))
	assertCode(t, err, 40400)

	e.create(t, input("one"))
	two := e.create(t, input("two"))
	_, err = e.svc.Update(ctx, two.ID, input("one"))
	assertCode(t, err, 40903)
}

func TestUpdateDoesNotTouchProtectedColumns(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("prot"))
	require.NoError(t, e.db.Exec(
		"UPDATE problems SET checker_path='c.cpp', submit_count=5, accept_count=2 WHERE id = ?", p.ID).Error)

	in := input("prot")
	in.Title = "Renamed"
	updated, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.Title)

	got, _ := e.repo.FindByID(ctx, p.ID)
	assert.Equal(t, "c.cpp", got.CheckerPath)
	assert.Equal(t, 5, got.SubmitCount)
	assert.Equal(t, 2, got.AcceptCount)
	assert.Equal(t, int64(1), got.CreatedBy)
}

func TestAddTestcaseStoresFilesAndRow(t *testing.T) {
	e := newEnv(t)
	p := e.create(t, input("store"))

	tc, err := e.svc.AddTestcase(context.Background(), p.ID, problem.TestcaseUpload{
		Input: strings.NewReader("12345"), Output: strings.NewReader("678"), Score: 25, IsSample: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, tc.SortOrder)
	assert.Equal(t, int64(5), tc.InputSize)
	assert.Equal(t, int64(3), tc.OutputSize)
	assert.Equal(t, 25, tc.Score)
	assert.True(t, tc.IsSample)

	require.Len(t, e.files(t), 2)
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(tc.InputFile)))
	require.NoError(t, err)
	assert.Equal(t, "12345", string(b))
	assert.True(t, strings.HasPrefix(tc.InputFile, "problems/"), tc.InputFile)

	list, err := e.svc.ListTestcases(context.Background(), p.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, tc.ID, list[0].ID)
	assert.Equal(t, int64(5), list[0].InputSize)
}

func TestAddTestcaseRejectsOversizeWithoutResidue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("big"))
	const limit = 16 << 20

	_, err := e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{Input: zeroBytes(limit + 1), Output: strings.NewReader("x"), Score: 10})
	assertCode(t, err, 40001)
	assert.Empty(t, e.files(t))

	_, err = e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{Input: strings.NewReader("ok"), Output: zeroBytes(limit + 1), Score: 10})
	assertCode(t, err, 40001)
	assert.Empty(t, e.files(t), "the already-stored input must be removed too")

	n, err := e.repo.CountTestcases(ctx, p.ID)
	require.NoError(t, err)
	assert.Zero(t, n)

	_, err = e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{Input: zeroBytes(limit), Output: zeroBytes(limit), Score: 10})
	assert.NoError(t, err, "exactly 16 MiB is allowed")
}

func TestAddTestcaseMissingProblemCleansFiles(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.AddTestcase(context.Background(), 999999, upload("1", "2"))
	assertCode(t, err, 40400)
	assert.Empty(t, e.files(t))
}

func TestAddTestcaseScoreRange(t *testing.T) {
	e := newEnv(t)
	p := e.create(t, input("score"))
	for _, s := range []int{-1, 1001} {
		up := upload("1", "2")
		up.Score = s
		_, err := e.svc.AddTestcase(context.Background(), p.ID, up)
		assertCode(t, err, 40001)
	}
	for _, s := range []int{0, 1000} {
		up := upload("1", "2")
		up.Score = s
		_, err := e.svc.AddTestcase(context.Background(), p.ID, up)
		assert.NoError(t, err, "score %d", s)
	}
	assert.Len(t, e.files(t), 4, "rejected uploads leave nothing; the two accepted ones leave two files each")
}

func TestUpdateTestcaseValidatesAndScopes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("tc-upd"))
	tc := e.addCase(t, p.ID)

	bad := 2000
	assertCode(t, e.svc.UpdateTestcase(ctx, p.ID, tc.ID, &bad, nil), 40001)
	assertCode(t, e.svc.UpdateTestcase(ctx, p.ID, 999999, nil, nil), 40400)
	assertCode(t, e.svc.UpdateTestcase(ctx, 999999, tc.ID, nil, nil), 40400)

	ok, yes := 77, true
	require.NoError(t, e.svc.UpdateTestcase(ctx, p.ID, tc.ID, &ok, &yes))
	list, _ := e.svc.ListTestcases(ctx, p.ID)
	assert.Equal(t, 77, list[0].Score)
	assert.True(t, list[0].IsSample)
}

func TestDeleteTestcaseRemovesFiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("tc-del"))
	tc := e.addCase(t, p.ID)
	require.Len(t, e.files(t), 2)

	require.NoError(t, e.svc.DeleteTestcase(ctx, p.ID, tc.ID))
	assert.Empty(t, e.files(t))
	assertCode(t, e.svc.DeleteTestcase(ctx, p.ID, tc.ID), 40400)
}

func TestCannotDeleteLastTestcaseOfPublicProblem(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("last"))
	first := e.addCase(t, p.ID)
	in := input("last")
	in.IsPublic = true
	_, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)

	assertCode(t, e.svc.DeleteTestcase(ctx, p.ID, first.ID), 40001)
	n, _ := e.repo.CountTestcases(ctx, p.ID)
	assert.Equal(t, 1, n)

	second := e.addCase(t, p.ID)
	assert.NoError(t, e.svc.DeleteTestcase(ctx, p.ID, first.ID))
	assertCode(t, e.svc.DeleteTestcase(ctx, p.ID, second.ID), 40001)
}

func TestServiceReorderRejectsBadIDs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("reo"))
	a, b := e.addCase(t, p.ID), e.addCase(t, p.ID)

	assertCode(t, e.svc.ReorderTestcases(ctx, p.ID, []int64{a.ID}), 40001)
	assertCode(t, e.svc.ReorderTestcases(ctx, 999999, []int64{a.ID}), 40400)
	require.NoError(t, e.svc.ReorderTestcases(ctx, p.ID, []int64{b.ID, a.ID}))
	list, _ := e.svc.ListTestcases(ctx, p.ID)
	assert.Equal(t, []int64{b.ID, a.ID}, []int64{list[0].ID, list[1].ID})
}

func TestUploadCheckerOnlyForSpecial(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	trad := e.create(t, input("trad"))
	assertCode(t, e.svc.UploadChecker(ctx, trad.ID, strings.NewReader("x")), 40001)
	assertCode(t, e.svc.UploadChecker(ctx, 999999, strings.NewReader("x")), 40400)

	in := input("spj")
	in.Type = problem.TypeSpecial
	spj := e.create(t, in)
	require.NoError(t, e.svc.UploadChecker(ctx, spj.ID, strings.NewReader("v1")))
	got, _ := e.repo.FindByID(ctx, spj.ID)
	assert.NotEmpty(t, got.CheckerPath)
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(got.CheckerPath)))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(b))
}

func TestUploadCheckerSizeLimitKeepsPrevious(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := input("spj-size")
	in.Type = problem.TypeSpecial
	p := e.create(t, in)
	require.NoError(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("v1")))
	old, _ := e.repo.FindByID(ctx, p.ID)
	oldPath := old.CheckerPath

	assertCode(t, e.svc.UploadChecker(ctx, p.ID, zeroBytes(256<<10+1)), 40001)
	assertCode(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("")), 40001)

	got, _ := e.repo.FindByID(ctx, p.ID)
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(got.CheckerPath)))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(b), "failed uploads must not clobber the previous checker")

	assert.NoError(t, e.svc.UploadChecker(ctx, p.ID, zeroBytes(256<<10)), "exactly 256 KiB is allowed")

	got, _ = e.repo.FindByID(ctx, p.ID)
	assert.NotEqual(t, oldPath, got.CheckerPath, "a successful upload gets a fresh storage key")
	_, err = os.Stat(filepath.Join(e.root, filepath.FromSlash(oldPath)))
	assert.True(t, os.IsNotExist(err), "the replaced checker file must be removed")
}

func TestUploadCheckerCleansUpWhenDatabaseUpdateFails(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := input("spj-db")
	in.Type = problem.TypeSpecial
	p := e.create(t, in)
	require.NoError(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("v1")))
	old, _ := e.repo.FindByID(ctx, p.ID)

	svc := problem.NewService(failingCheckerRepo{e.repo}, e.store)
	require.Error(t, svc.UploadChecker(ctx, p.ID, strings.NewReader("v2")))

	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(old.CheckerPath)))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(b), "the previous checker must survive a failed DB update")
	assert.Len(t, e.files(t), 1, "the newly written checker must be removed")
}

func TestDeleteWithoutSubmissionsRemovesFiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := input("rm")
	in.Type = problem.TypeSpecial
	p := e.create(t, in)
	require.NoError(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("chk")))
	e.addCase(t, p.ID)
	e.addCase(t, p.ID)
	require.Len(t, e.files(t), 5)

	offlined, err := e.svc.Delete(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, offlined)
	assert.Empty(t, e.files(t))
	_, err = e.svc.Get(ctx, manager, p.ID)
	assertCode(t, err, 40400)
}

func TestDeleteWithSubmissionsKeepsFiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("keep"))
	e.addCase(t, p.ID)
	in := input("keep")
	in.IsPublic = true
	_, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	require.NoError(t, e.db.Exec("UPDATE problems SET submit_count = 3 WHERE id = ?", p.ID).Error)

	offlined, err := e.svc.Delete(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, offlined)
	assert.Len(t, e.files(t), 2)

	got, err := e.svc.Get(ctx, manager, p.ID)
	require.NoError(t, err)
	assert.False(t, got.IsPublic)
	_, err = e.svc.Get(ctx, guest, p.ID)
	assertCode(t, err, 40400)

	_, err = e.svc.Delete(ctx, 999999)
	assertCode(t, err, 40400)
}

type fakeGuard struct {
	blocked bool
	err     error
}

func (g fakeGuard) IsProblemInActivePath(context.Context, int64) (bool, error) {
	return g.blocked, g.err
}

func TestProblemUpdateBlocksUnpublishWhenReferenced(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	guard := &fakeGuard{blocked: true}
	svc := problem.NewService(e.repo, e.store, problem.WithLearningGuard(guard))

	// 先发布一道题，再尝试下线：wasPublic=true 且改为 false 时被守卫拦截。
	pub := e.create(t, input("guard-pub"))
	e.addCase(t, pub.ID)
	in := input("guard-pub")
	in.IsPublic = true
	_, err := svc.Update(ctx, pub.ID, in)
	require.NoError(t, err)

	in.IsPublic = false
	_, err = svc.Update(ctx, pub.ID, in)
	assertCode(t, err, 40001)
	got, err := e.repo.FindByID(ctx, pub.ID)
	require.NoError(t, err)
	assert.True(t, got.IsPublic, "被拦截的下线操作必须保持题目仍为公开")

	// 保持公开（wasPublic=true 且仍为 true）即使被引用也允许。
	in.IsPublic = true
	_, err = svc.Update(ctx, pub.ID, in)
	require.NoError(t, err)

	// 原本就是私有的题目（wasPublic=false）不受守卫影响。
	priv := e.create(t, input("guard-priv"))
	in = input("guard-priv")
	_, err = svc.Update(ctx, priv.ID, in)
	require.NoError(t, err, "已是私有的题目允许更新")

	// 从私有发布为公开（wasPublic=false）即使被引用也允许。
	e.addCase(t, priv.ID)
	in.IsPublic = true
	_, err = svc.Update(ctx, priv.ID, in)
	require.NoError(t, err, "发布操作不受守卫影响")
}

func TestProblemDeleteBlocksWhenReferenced(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	blocked := problem.NewService(e.repo, e.store, problem.WithLearningGuard(&fakeGuard{blocked: true}))
	p := e.create(t, input("guard-del"))
	e.addCase(t, p.ID)

	_, err := blocked.Delete(ctx, p.ID)
	assertCode(t, err, 40001)
	_, err = e.repo.FindByID(ctx, p.ID)
	require.NoError(t, err, "被拦截的删除操作必须保留题目")

	allowed := problem.NewService(e.repo, e.store, problem.WithLearningGuard(&fakeGuard{}))
	offlined, err := allowed.Delete(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, offlined)
	_, err = e.repo.FindByID(ctx, p.ID)
	assert.Error(t, err, "未引用时删除正常执行")
}

func TestProblemGuardErrorPropagates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	dbErr := errors.New("database unavailable")
	svc := problem.NewService(e.repo, e.store,
		problem.WithLearningGuard(&fakeGuard{blocked: true, err: dbErr}))

	p := e.create(t, input("guard-err"))
	e.addCase(t, p.ID)
	in := input("guard-err")
	in.IsPublic = true
	_, err := svc.Update(ctx, p.ID, in)
	require.NoError(t, err)

	in.IsPublic = false
	_, err = svc.Update(ctx, p.ID, in)
	require.ErrorIs(t, err, dbErr, "守卫返回的数据库错误必须原样返回")
}

func TestListHidesContestLockedProblems(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("locked"))
	e.addCase(t, p.ID)
	in := input("locked")
	in.IsPublic = true
	_, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)

	repo := contest.NewGormRepository(e.db)
	now := time.Now()
	c := &contest.Contest{
		Title: "Lock", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, repo.Create(ctx, c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))

	page, err := e.svc.List(ctx, problem.Viewer{}, problem.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.Zero(t, page.Total, "locked problem must be hidden from the library")

	page, err = e.svc.List(ctx, problem.Viewer{CanViewLocked: true}, problem.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total, "managers can still see public locked problems")

	require.NoError(t, e.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	page, err = e.svc.List(ctx, problem.Viewer{}, problem.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total, "ended contest unlocks a public problem")
}

func TestStatisticsAcceptRate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("stats"))

	s, err := e.svc.Statistics(ctx, manager, p.ID)
	require.NoError(t, err)
	assert.Equal(t, problem.Stats{}, s, "no submissions: rate is 0, not NaN")

	require.NoError(t, e.db.Exec("UPDATE problems SET submit_count = 4, accept_count = 1 WHERE id = ?", p.ID).Error)
	s, err = e.svc.Statistics(ctx, manager, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 4, s.SubmitCount)
	assert.Equal(t, 1, s.AcceptCount)
	assert.InDelta(t, 0.25, s.AcceptRate, 1e-9)
}
