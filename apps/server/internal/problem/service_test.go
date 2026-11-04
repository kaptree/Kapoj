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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type env struct {
	svc  *problem.Service
	repo problem.Repository
	db   *gorm.DB
	root string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := testutil.PG(t)
	root := filepath.Join(t.TempDir(), "store")
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	repo := problem.NewGormRepository(g)
	return &env{svc: problem.NewService(repo, store), repo: repo, db: g, root: root}
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
	manager = problem.Viewer{CanManage: true}
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

	assertCode(t, e.svc.UploadChecker(ctx, p.ID, zeroBytes(256<<10+1)), 40001)
	assertCode(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("")), 40001)

	got, _ := e.repo.FindByID(ctx, p.ID)
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(got.CheckerPath)))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(b), "failed uploads must not clobber the previous checker")

	assert.NoError(t, e.svc.UploadChecker(ctx, p.ID, zeroBytes(256<<10)), "exactly 256 KiB is allowed")
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
