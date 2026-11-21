package problem_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

const mib = int64(1 << 20)

// newLimitsEnv builds a service whose provider returns limits/err.
func newLimitsEnv(t *testing.T, limits problem.TestcaseLimits, providerErr error) (*env, *int) {
	t.Helper()
	g := testutil.PG(t)
	root := filepath.Join(t.TempDir(), "store")
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	repo := problem.NewGormRepository(g)
	calls := 0
	svc := problem.NewService(repo, store, problem.WithTestcaseLimitsProvider(
		problem.TestcaseLimitsProviderFunc(func(context.Context) (problem.TestcaseLimits, error) {
			calls++
			return limits, providerErr
		}),
	))
	return &env{svc: svc, repo: repo, store: store, db: g, root: root}, &calls
}

func TestDefaultTestcaseLimitsMatchLegacyConstants(t *testing.T) {
	got := problem.DefaultTestcaseLimits()
	assert.Equal(t, int64(16<<20), got.MaxInputBytes)
	assert.Equal(t, int64(16<<20), got.MaxOutputBytes)
	assert.Equal(t, int64(200<<20), got.MaxBatchTotalBytes)
	assert.Equal(t, int64(200<<20), got.MaxProblemTotalBytes)
}

func TestTestcaseUploadUsesConfiguredLimits(t *testing.T) {
	e, calls := newLimitsEnv(t, problem.TestcaseLimits{
		MaxInputBytes:        4 * mib,
		MaxOutputBytes:       2 * mib,
		MaxBatchTotalBytes:   200 * mib,
		MaxProblemTotalBytes: 200 * mib,
	}, nil)
	ctx := context.Background()
	p := e.create(t, input("configured-single"))

	_, err := e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{
		Input: zeroBytes(4*mib + 1), Output: strings.NewReader("out"), Score: 10,
	})
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "输入文件不能超过 4 MiB")
	assert.Empty(t, e.files(t))
	assert.Equal(t, 1, *calls, "limits must be read once per operation")

	_, err = e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{
		Input: strings.NewReader("in"), Output: zeroBytes(2*mib + 1), Score: 10,
	})
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "输出文件不能超过 2 MiB")
	assert.Empty(t, e.files(t), "the input staged before the output overflow must be deleted")
	assert.Equal(t, 2, *calls)

	tc, err := e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{
		Input: zeroBytes(4 * mib), Output: zeroBytes(2 * mib), Score: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 4*mib, tc.InputSize)
	assert.Equal(t, 2*mib, tc.OutputSize)
	assert.Equal(t, 3, *calls)
}

func TestTestcaseUploadFallsBackToDefaultLimits(t *testing.T) {
	e, _ := newLimitsEnv(t, problem.TestcaseLimits{}, errors.New("settings unavailable"))
	ctx := context.Background()
	p := e.create(t, input("fallback-single"))

	_, err := e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{
		Input: zeroBytes(16*mib + 1), Output: strings.NewReader("out"), Score: 10,
	})
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "输入文件不能超过 16 MiB")
	assert.Empty(t, e.files(t))
}

func TestBatchUploadUsesConfiguredLimits(t *testing.T) {
	e, calls := newLimitsEnv(t, problem.TestcaseLimits{
		MaxInputBytes:        4 * mib,
		MaxOutputBytes:       2 * mib,
		MaxBatchTotalBytes:   6 * mib,
		MaxProblemTotalBytes: 200 * mib,
	}, nil)
	ctx := context.Background()
	p := e.create(t, input("configured-batch"))

	_, err := e.svc.AddTestcasesBatch(ctx, p.ID, []problem.BatchTestcaseFile{
		{Name: "big.in", Size: 4*mib + 1, Reader: strings.NewReader("")},
		{Name: "big.out", Size: 1, Reader: strings.NewReader("")},
	}, 10)
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "输入文件不能超过 4 MiB")
	assert.Empty(t, e.files(t))
	assert.Equal(t, 1, *calls, "limits must be read once per operation")

	_, err = e.svc.AddTestcasesBatch(ctx, p.ID, []problem.BatchTestcaseFile{
		{Name: "big.in", Size: 1, Reader: strings.NewReader("")},
		{Name: "big.out", Size: 2*mib + 1, Reader: strings.NewReader("")},
	}, 10)
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "输出文件不能超过 2 MiB")
	assert.Empty(t, e.files(t))

	_, err = e.svc.AddTestcasesBatch(ctx, p.ID, []problem.BatchTestcaseFile{
		{Name: "1.in", Size: 4 * mib, Reader: strings.NewReader("")},
		{Name: "1.out", Size: 2 * mib, Reader: strings.NewReader("")},
		{Name: "2.in", Size: 1, Reader: strings.NewReader("")},
		{Name: "2.out", Size: 1, Reader: strings.NewReader("")},
	}, 10)
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "批量测试数据不能超过 6 MiB")
	assert.Empty(t, e.files(t))

	// Headers fit, so the write itself must enforce the configured limits.
	_, err = e.svc.AddTestcasesBatch(ctx, p.ID, []problem.BatchTestcaseFile{
		{Name: "actual.in", Size: 1, Reader: zeroBytes(4*mib + 1)},
		{Name: "actual.out", Size: 1, Reader: strings.NewReader("out")},
	}, 10)
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "输入文件不能超过 4 MiB")
	assert.Empty(t, e.files(t))

	_, err = e.svc.AddTestcasesBatch(ctx, p.ID, []problem.BatchTestcaseFile{
		{Name: "actual.in", Size: 1, Reader: strings.NewReader("in")},
		{Name: "actual.out", Size: 1, Reader: zeroBytes(2*mib + 1)},
	}, 10)
	assertCode(t, err, 40001)
	assert.Contains(t, err.Error(), "输出文件不能超过 2 MiB")
	assert.Empty(t, e.files(t))

	before := *calls
	cases, err := e.svc.AddTestcasesBatch(ctx, p.ID, []problem.BatchTestcaseFile{
		{Name: "ok.in", Size: 4 * mib, Reader: zeroBytes(4 * mib)},
		{Name: "ok.out", Size: 2 * mib, Reader: zeroBytes(2 * mib)},
	}, 10)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	assert.Equal(t, 4*mib, cases[0].InputSize)
	assert.Equal(t, 2*mib, cases[0].OutputSize)
	assert.Equal(t, before+1, *calls)
}

func TestBatchUploadFallsBackToDefaultLimits(t *testing.T) {
	tests := []struct {
		name  string
		slug  string
		build func(*testing.T) *env
	}{
		{"nil provider", "fallback-batch-nil", newEnv},
		{"error provider", "fallback-batch-error", func(t *testing.T) *env {
			e, _ := newLimitsEnv(t, problem.TestcaseLimits{}, errors.New("settings unavailable"))
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.build(t)
			p := e.create(t, input(tt.slug))
			_, err := e.svc.AddTestcasesBatch(context.Background(), p.ID, []problem.BatchTestcaseFile{
				{Name: "big.in", Size: 16*mib + 1, Reader: strings.NewReader("")},
				{Name: "big.out", Size: 1, Reader: strings.NewReader("")},
			}, 10)
			assertCode(t, err, 40001)
			assert.Contains(t, err.Error(), "输入文件不能超过 16 MiB")
			assert.Empty(t, e.files(t))
		})
	}
}
