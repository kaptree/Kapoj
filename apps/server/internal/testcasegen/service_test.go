package testcasegen_test

import (
	"context"
	"errors"
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
	store, err := storage.NewLocal(filepath.Join(t.TempDir(), "store"))
	require.NoError(t, err)
	return testcasegen.NewService(testcasegen.NewGormRepository(g), probs, store, scripts, enq)
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
