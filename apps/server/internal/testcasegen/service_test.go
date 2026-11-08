package testcasegen_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type fakeProblems struct {
	byID map[int64]*problem.Problem
	err  error
}

func (f *fakeProblems) FindByID(_ context.Context, id int64) (*problem.Problem, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, problem.ErrNotFound
}

type fakeScripts struct {
	script string
	err    error
	calls  []ai.TestcaseScriptInput
	userID int64
}

func (f *fakeScripts) GenerateTestcaseScript(_ context.Context, userID int64, in ai.TestcaseScriptInput) (string, error) {
	f.calls = append(f.calls, in)
	f.userID = userID
	if f.err != nil {
		return "", f.err
	}
	return f.script, nil
}

type fakeEnqueuer struct {
	ids []int64
	err error
}

func (f *fakeEnqueuer) EnqueueTestcaseGeneration(_ context.Context, id int64) error {
	f.ids = append(f.ids, id)
	return f.err
}

func newService(t *testing.T, g *gorm.DB, probs *fakeProblems, scripts *fakeScripts, enq *fakeEnqueuer) *testcasegen.Service {
	t.Helper()
	svc, _ := newServiceStore(t, g, probs, scripts, enq)
	return svc
}

// newServiceStore also returns the storage root so tests can seed files.
func newServiceStore(t *testing.T, g *gorm.DB, probs *fakeProblems, scripts *fakeScripts, enq *fakeEnqueuer) (*testcasegen.Service, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "store")
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	return testcasegen.NewService(testcasegen.NewGormRepository(g), probs, store, scripts, enq), root
}

// generatedKey mirrors the deterministic key the judge writes files under.
func generatedKey(problemID, genID int64, n int, ext string) string {
	return fmt.Sprintf("problems/%d/generated/%d/%d.%s", problemID, genID, n, ext)
}

func insertGeneration(t *testing.T, g *gorm.DB, problemID int64, status testcasegen.Status, count int) int64 {
	t.Helper()
	gen := &testcasegen.Generation{
		ProblemID:     problemID,
		UserID:        1,
		Status:        status,
		Language:      "cpp",
		StandardCode:  "int main(){}",
		GeneratorCode: "print(1)",
		TestcaseCount: count,
		TimeLimitMS:   1000,
		MemoryLimitMB: 256,
	}
	require.NoError(t, g.Create(gen).Error)
	return gen.ID
}

// insertCases adds count cases whose files use the deterministic key layout.
func insertCases(t *testing.T, g *gorm.DB, problemID, genID int64, count int) []testcasegen.GenerationCase {
	t.Helper()
	cases := make([]testcasegen.GenerationCase, count)
	for i := 0; i < count; i++ {
		c := testcasegen.GenerationCase{
			GenerationID: genID,
			SortOrder:    i + 1,
			InputFile:    generatedKey(problemID, genID, i+1, "in"),
			OutputFile:   generatedKey(problemID, genID, i+1, "out"),
			InputSize:    10,
			OutputSize:   4,
		}
		require.NoError(t, g.Create(&c).Error)
		cases[i] = c
	}
	return cases
}

func writeStoreFile(t *testing.T, root, key, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func storeFileExists(root, key string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(key)))
	return err == nil
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, code, appErr.Code)
}

func TestCreateRequiresTraditionalProblem(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{byID: map[int64]*problem.Problem{
		1: {ID: 1, Type: problem.TypeSpecial, Title: "special"},
		2: {ID: 2, Type: problem.TypeInteractive, Title: "interactive"},
	}}
	scripts := &fakeScripts{script: "print(1)"}
	enq := &fakeEnqueuer{}
	svc := newService(t, g, probs, scripts, enq)

	for _, id := range []int64{1, 2} {
		_, err := svc.Create(ctx, id, 1, testcasegen.CreateInput{Language: "cpp", SourceCode: "int main(){}", TestcaseCount: 3})
		assertCode(t, err, 40001)
	}
	assert.Empty(t, scripts.calls, "AI must not be called for non-traditional problems")
	assert.Empty(t, enq.ids)
}

func TestCreateValidatesInput(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{byID: map[int64]*problem.Problem{1: {ID: 1, Type: problem.TypeTraditional}}}
	scripts := &fakeScripts{script: "print(1)"}
	enq := &fakeEnqueuer{}
	svc := newService(t, g, probs, scripts, enq)

	cases := []testcasegen.CreateInput{
		{Language: "rust", SourceCode: "x", TestcaseCount: 3},
		{Language: "cpp", SourceCode: "x", TestcaseCount: 0},
		{Language: "cpp", SourceCode: "x", TestcaseCount: 11},
		{Language: "cpp", SourceCode: "   ", TestcaseCount: 3},
		{Language: "cpp", SourceCode: strings.Repeat("a", 64*1024+1), TestcaseCount: 3},
	}
	for _, in := range cases {
		_, err := svc.Create(ctx, 1, 1, in)
		assertCode(t, err, 40001)
	}
	assert.Empty(t, scripts.calls)
	assert.Empty(t, enq.ids)
}

func TestCreateCallsAIAndEnqueues(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{byID: map[int64]*problem.Problem{1: {
		ID: 1, Type: problem.TypeTraditional, Title: "Sum",
		StatementMD: "desc", InputFormatMD: "if", OutputFormatMD: "of",
		Samples:     dbtypes.List[problem.Sample]{{Input: "1 2", Output: "3"}},
		TimeLimitMS: 2000, MemoryLimitMB: 512,
	}}}
	scripts := &fakeScripts{script: "print(1)"}
	enq := &fakeEnqueuer{}
	svc := newService(t, g, probs, scripts, enq)

	view, err := svc.Create(ctx, 1, 42, testcasegen.CreateInput{Language: "cpp", SourceCode: "int main(){}", TestcaseCount: 3})
	require.NoError(t, err)
	assert.Equal(t, testcasegen.StatusPending, view.Status)
	assert.Equal(t, "print(1)", view.GeneratorCode)
	assert.Equal(t, 3, view.TestcaseCount)
	assert.Empty(t, view.Cases)
	assert.NotZero(t, view.ID)

	require.Len(t, scripts.calls, 1)
	call := scripts.calls[0]
	assert.Equal(t, "Sum", call.Title)
	assert.Equal(t, "int main(){}", call.StandardCode)
	assert.Equal(t, "cpp", call.Language)
	assert.Equal(t, 3, call.TestcaseCount)
	require.Len(t, call.Samples, 1)
	assert.Equal(t, "1 2", call.Samples[0].Input)
	assert.Equal(t, int64(42), scripts.userID)

	require.Len(t, enq.ids, 1)
	assert.Equal(t, view.ID, enq.ids[0])

	var gen testcasegen.Generation
	require.NoError(t, g.First(&gen, view.ID).Error)
	assert.Equal(t, testcasegen.StatusPending, gen.Status)
	assert.Equal(t, "print(1)", gen.GeneratorCode)
	assert.Equal(t, "int main(){}", gen.StandardCode)
	assert.Equal(t, 2000, gen.TimeLimitMS)
	assert.Equal(t, 512, gen.MemoryLimitMB)
}

func TestCreateAIErrorWritesNothing(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{byID: map[int64]*problem.Problem{1: {ID: 1, Type: problem.TypeTraditional}}}
	scripts := &fakeScripts{err: httpx.ErrBadGateway}
	enq := &fakeEnqueuer{}
	svc := newService(t, g, probs, scripts, enq)

	_, err := svc.Create(ctx, 1, 1, testcasegen.CreateInput{Language: "cpp", SourceCode: "x", TestcaseCount: 3})
	assertCode(t, err, 50200)

	var n int64
	require.NoError(t, g.Model(&testcasegen.Generation{}).Count(&n).Error)
	assert.Zero(t, n, "no generation row may be written when AI fails")
	assert.Empty(t, enq.ids)
}

func TestCreateEnqueueFailureMarksFailed(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{byID: map[int64]*problem.Problem{1: {ID: 1, Type: problem.TypeTraditional}}}
	scripts := &fakeScripts{script: "print(1)"}
	enq := &fakeEnqueuer{err: errors.New("redis down")}
	svc := newService(t, g, probs, scripts, enq)

	view, err := svc.Create(ctx, 1, 1, testcasegen.CreateInput{Language: "cpp", SourceCode: "x", TestcaseCount: 3})
	assertCode(t, err, 50300)
	require.Nil(t, view)

	var gen testcasegen.Generation
	require.NoError(t, g.First(&gen).Error)
	assert.Equal(t, testcasegen.StatusFailed, gen.Status)
	assert.Equal(t, "任务投递失败", gen.ErrorMessage)
}

func TestGetReturnsCasesAndNotFound(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{byID: map[int64]*problem.Problem{1: {ID: 1, Type: problem.TypeTraditional}}}
	scripts := &fakeScripts{script: "print(1)"}
	enq := &fakeEnqueuer{}
	svc := newService(t, g, probs, scripts, enq)

	view, err := svc.Create(ctx, 1, 1, testcasegen.CreateInput{Language: "python3", SourceCode: "print(1)", TestcaseCount: 2})
	require.NoError(t, err)
	assert.NotNil(t, view.Cases, "cases must be an empty slice, not null")
	assert.Empty(t, view.Cases)

	require.NoError(t, g.Create(&testcasegen.GenerationCase{
		GenerationID: view.ID, SortOrder: 1, InputFile: "p/1/g/1.in", OutputFile: "p/1/g/1.out", InputSize: 10, OutputSize: 4,
	}).Error)

	got, err := svc.Get(ctx, 1, view.ID)
	require.NoError(t, err)
	assert.Equal(t, testcasegen.StatusPending, got.Status)
	require.Len(t, got.Cases, 1)
	assert.Equal(t, 1, got.Cases[0].SortOrder)
	assert.Equal(t, int64(10), got.Cases[0].InputSize)
	assert.Equal(t, int64(4), got.Cases[0].OutputSize)

	_, err = svc.Get(ctx, 1, 999999)
	assertCode(t, err, 40400)
	_, err = svc.Get(ctx, 999999, view.ID)
	assertCode(t, err, 40400)
}

func TestPreviewTruncatesAndSanitizes(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{}
	svc, root := newServiceStore(t, g, probs, &fakeScripts{}, &fakeEnqueuer{})

	genID := insertGeneration(t, g, 1, testcasegen.StatusReady, 1)
	cases := insertCases(t, g, 1, genID, 1)

	raw := make([]byte, 70*1024)
	for i := range raw {
		raw[i] = 'a'
	}
	raw[100] = 0xff // invalid UTF-8
	writeStoreFile(t, root, cases[0].InputFile, string(raw))

	pv, err := svc.Preview(ctx, 1, genID, cases[0].ID, "input")
	require.NoError(t, err)
	assert.True(t, pv.Truncated)
	assert.LessOrEqual(t, len(pv.Content), 64*1024)
	assert.Contains(t, pv.Content, "\uFFFD")
	assert.Equal(t, "input", pv.Kind)
}

func TestPreviewRejectsKindAndWrongCase(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{}
	svc, root := newServiceStore(t, g, probs, &fakeScripts{}, &fakeEnqueuer{})

	genID := insertGeneration(t, g, 1, testcasegen.StatusReady, 1)
	cases := insertCases(t, g, 1, genID, 1)
	writeStoreFile(t, root, cases[0].InputFile, "1 2")

	// 非法 kind → 40001。
	_, err := svc.Preview(ctx, 1, genID, cases[0].ID, "stderr")
	assertCode(t, err, 40001)

	// case 不属于 generation → 404。
	_, err = svc.Preview(ctx, 1, genID, 999999, "input")
	assertCode(t, err, 40400)

	// generation 不属于 problem → 404。
	_, err = svc.Preview(ctx, 999999, genID, cases[0].ID, "input")
	assertCode(t, err, 40400)

	// 文件缺失 → 404。
	_, err = svc.Preview(ctx, 1, genID, cases[0].ID, "output")
	assertCode(t, err, 40400)
}

func TestConfirmInsertsTestcasesAtEnd(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{}
	svc, _ := newServiceStore(t, g, probs, &fakeScripts{}, &fakeEnqueuer{})

	p := &problem.Problem{Title: "Sum", Slug: "sum"}
	require.NoError(t, g.Create(p).Error)
	require.NoError(t, g.Create(&problem.Testcase{
		ProblemID: p.ID, InputFile: "1.in", OutputFile: "1.out", Score: 10, SortOrder: 1,
	}).Error)

	genID := insertGeneration(t, g, p.ID, testcasegen.StatusReady, 2)
	insertCases(t, g, p.ID, genID, 2)

	res, err := svc.Confirm(ctx, p.ID, genID)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Count)

	var tcs []problem.Testcase
	require.NoError(t, g.Where("problem_id = ?", p.ID).Order("sort_order").Find(&tcs).Error)
	require.Len(t, tcs, 3)
	assert.Equal(t, 1, tcs[0].SortOrder)
	assert.Equal(t, 2, tcs[1].SortOrder)
	assert.Equal(t, 10, tcs[1].Score)
	assert.False(t, tcs[1].IsSample)
	assert.Equal(t, 3, tcs[2].SortOrder)
	assert.Equal(t, 10, tcs[2].Score)

	var gen testcasegen.Generation
	require.NoError(t, g.First(&gen, genID).Error)
	assert.Equal(t, testcasegen.StatusConfirmed, gen.Status)

	// 重复确认幂等：返回 2 且不新增测试点。
	res, err = svc.Confirm(ctx, p.ID, genID)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Count)
	var n int64
	require.NoError(t, g.Model(&problem.Testcase{}).Where("problem_id = ?", p.ID).Count(&n).Error)
	assert.Equal(t, int64(3), n)
}

func TestConfirmRejectsNotReady(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{}
	svc, _ := newServiceStore(t, g, probs, &fakeScripts{}, &fakeEnqueuer{})

	p := &problem.Problem{Title: "Sum", Slug: "sum2"}
	require.NoError(t, g.Create(p).Error)

	for _, status := range []testcasegen.Status{
		testcasegen.StatusPending, testcasegen.StatusRunning, testcasegen.StatusFailed, testcasegen.StatusDiscarded,
	} {
		genID := insertGeneration(t, g, p.ID, status, 1)
		insertCases(t, g, p.ID, genID, 1)
		_, err := svc.Confirm(ctx, p.ID, genID)
		assertCode(t, err, 40001)
	}
}

func TestDiscardReadyDeletesCasesAndFiles(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{}
	svc, root := newServiceStore(t, g, probs, &fakeScripts{}, &fakeEnqueuer{})

	genID := insertGeneration(t, g, 1, testcasegen.StatusReady, 2)
	insertCases(t, g, 1, genID, 2)
	for n := 1; n <= 2; n++ {
		writeStoreFile(t, root, generatedKey(1, genID, n, "in"), fmt.Sprintf("in%d", n))
		writeStoreFile(t, root, generatedKey(1, genID, n, "out"), fmt.Sprintf("out%d", n))
	}

	require.NoError(t, svc.Discard(ctx, 1, genID))

	var gen testcasegen.Generation
	require.NoError(t, g.First(&gen, genID).Error)
	assert.Equal(t, testcasegen.StatusDiscarded, gen.Status)

	var n int64
	require.NoError(t, g.Model(&testcasegen.GenerationCase{}).Where("generation_id = ?", genID).Count(&n).Error)
	assert.Zero(t, n)

	for i := 1; i <= 2; i++ {
		assert.False(t, storeFileExists(root, generatedKey(1, genID, i, "in")))
		assert.False(t, storeFileExists(root, generatedKey(1, genID, i, "out")))
	}
}

func TestDiscardRejectsConfirmedAndRunning(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{}
	svc, root := newServiceStore(t, g, probs, &fakeScripts{}, &fakeEnqueuer{})

	// confirmed：40001 且文件保留。
	genID := insertGeneration(t, g, 1, testcasegen.StatusConfirmed, 1)
	insertCases(t, g, 1, genID, 1)
	writeStoreFile(t, root, generatedKey(1, genID, 1, "in"), "keep")
	err := svc.Discard(ctx, 1, genID)
	assertCode(t, err, 40001)
	assert.True(t, storeFileExists(root, generatedKey(1, genID, 1, "in")))

	// pending/running：40001。
	for _, status := range []testcasegen.Status{testcasegen.StatusPending, testcasegen.StatusRunning} {
		id := insertGeneration(t, g, 1, status, 1)
		insertCases(t, g, 1, id, 1)
		err = svc.Discard(ctx, 1, id)
		assertCode(t, err, 40001)
	}
}

func TestDiscardFailedCleansPartialFiles(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	probs := &fakeProblems{}
	svc, root := newServiceStore(t, g, probs, &fakeScripts{}, &fakeEnqueuer{})

	genID := insertGeneration(t, g, 1, testcasegen.StatusFailed, 2)
	insertCases(t, g, 1, genID, 2)
	// 只有部分确定性文件存在（2.out 缺失）。
	writeStoreFile(t, root, generatedKey(1, genID, 1, "in"), "1")
	writeStoreFile(t, root, generatedKey(1, genID, 1, "out"), "1")
	writeStoreFile(t, root, generatedKey(1, genID, 2, "in"), "2")

	require.NoError(t, svc.Discard(ctx, 1, genID))

	for _, ext := range []string{"in", "out"} {
		for n := 1; n <= 2; n++ {
			assert.False(t, storeFileExists(root, generatedKey(1, genID, n, ext)))
		}
	}
}
