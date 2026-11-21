package problem_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func configuredImportLimits() problem.TestcaseLimits {
	return problem.TestcaseLimits{
		MaxInputBytes:        4 * mib,
		MaxOutputBytes:       2 * mib,
		MaxBatchTotalBytes:   6 * mib,
		MaxProblemTotalBytes: 6 * mib,
	}
}

// hydroLimitsProblem builds a HydroOJ problem folder whose testdata entries are
// named relative to testdata/.
func hydroLimitsProblem(folder string, testdata ...zipEntry) []zipEntry {
	entries := []zipEntry{
		{name: folder + "/problem.yaml", body: []byte("title: " + folder + "\n")},
		{name: folder + "/problem_zh.md", body: []byte("## statement\n")},
	}
	for _, entry := range testdata {
		entry.name = folder + "/testdata/" + entry.name
		entries = append(entries, entry)
	}
	return entries
}

func TestHydroImportUsesConfiguredTestcaseLimits(t *testing.T) {
	e, calls := newLimitsEnv(t, configuredImportLimits(), nil)

	oversizedInput := bytes.Repeat([]byte("i"), int(4*mib+1))
	oversizedOutput := bytes.Repeat([]byte("o"), int(2*mib+1))

	cases := []struct {
		name    string
		folder  string
		file    string
		entries []zipEntry
		want    string
	}{
		{
			name:   "input over limit",
			folder: "input-limit",
			file:   "testdata/1.in",
			entries: []zipEntry{
				{name: "1.in", body: oversizedInput},
				{name: "1.out", body: []byte("out")},
			},
			want: "输入文件不能超过 4 MiB",
		},
		{
			name:   "out over limit",
			folder: "out-limit",
			file:   "testdata/1.out",
			entries: []zipEntry{
				{name: "1.in", body: []byte("in")},
				{name: "1.out", body: oversizedOutput},
			},
			want: "输出文件不能超过 2 MiB",
		},
		{
			name:   "ans over limit",
			folder: "ans-limit",
			file:   "testdata/1.ans",
			entries: []zipEntry{
				{name: "1.in", body: []byte("in")},
				{name: "1.ans", body: oversizedOutput},
			},
			want: "输出文件不能超过 2 MiB",
		},
		{
			name:   "problem total over limit",
			folder: "total-limit",
			entries: []zipEntry{
				{name: "1.in", body: bytes.Repeat([]byte("i"), int(4*mib))},
				{name: "1.out", body: bytes.Repeat([]byte("o"), int(2*mib))},
				{name: "2.in", body: []byte("in")},
				{name: "2.out", body: []byte("out")},
			},
			want: "单题测试数据不能超过 6 MiB",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildZip(t, hydroLimitsProblem(tc.folder, tc.entries...)...)
			res, err := importZip(t, e.svc, 1, data)
			require.NoError(t, err)
			require.Empty(t, res.Imported)
			require.Len(t, res.Failed, 1)
			assert.Contains(t, res.Failed[0].Error, tc.want)
			if tc.file != "" {
				assert.Contains(t, res.Failed[0].Error, tc.file)
			}
			assert.Empty(t, e.files(t), "a rejected import must not write testcase files")
		})
	}

	// Files exactly at the configured per-file and per-problem limits import.
	data := buildZip(t, hydroLimitsProblem("at-limit",
		zipEntry{name: "1.in", body: bytes.Repeat([]byte("i"), int(4*mib))},
		zipEntry{name: "1.out", body: bytes.Repeat([]byte("o"), int(2*mib))},
	)...)
	res, err := importZip(t, e.svc, 1, data)
	require.NoError(t, err)
	require.Empty(t, res.Failed)
	require.Len(t, res.Imported, 1)
	assert.Equal(t, 1, res.Imported[0].TestcaseCount)
	assert.GreaterOrEqual(t, *calls, 1, "the configured limits must be read")
}

func TestPackageImportUsesConfiguredTestcaseLimits(t *testing.T) {
	e, calls := newLimitsEnv(t, configuredImportLimits(), nil)
	ctx := context.Background()

	// A HydroOJ package delegates to ImportHydro, which must apply the same
	// per-file limits as a direct import.
	hydroData := buildZip(t, hydroLimitsProblem("package-hydro",
		zipEntry{name: "1.in", body: bytes.Repeat([]byte("i"), int(4*mib+1))},
		zipEntry{name: "1.out", body: []byte("out")},
	)...)
	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(hydroData), int64(len(hydroData)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	require.NoError(t, err)
	require.Empty(t, result.Imported)
	require.Len(t, result.Failed, 1)
	assert.Contains(t, result.Failed[0].Error, "输入文件不能超过 4 MiB")
	assert.Contains(t, result.Failed[0].Error, "testdata/1.in")
	assert.Empty(t, e.files(t))
	assert.GreaterOrEqual(t, *calls, 1, "the configured limits must be read")

	// The package envelope limit still comes from PackageImportOptions and is
	// independent of the judge testcase settings.
	_, err = e.svc.ImportPackage(ctx, 1, bytes.NewReader(hydroData), int64(len(hydroData)),
		problem.PackageImportOptions{ConflictPolicy: "skip", MaxZipBytes: int64(len(hydroData)) - 1})
	requireImportErrorCode(t, err, 40001)
	assert.Contains(t, err.Error(), "题包文件不能超过")

	// A kapOJ package applies the same configured input/output/total limits.
	p := e.create(t, input("package-limits-source"))
	e.addCase(t, p.ID)
	e.addCase(t, p.ID)
	var exported bytes.Buffer
	_, err = e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)

	kapOJCases := []struct {
		name  string
		slug  string
		files map[string]string
		want  string
	}{
		{
			name:  "input over limit",
			slug:  "package-limits-input",
			files: map[string]string{"testdata/001.in": string(bytes.Repeat([]byte("i"), int(4*mib+1)))},
			want:  "输入文件不能超过 4 MiB：testdata/001.in",
		},
		{
			name:  "output over limit",
			slug:  "package-limits-output",
			files: map[string]string{"testdata/001.out": string(bytes.Repeat([]byte("o"), int(2*mib+1)))},
			want:  "输出文件不能超过 2 MiB：testdata/001.out",
		},
		{
			name: "problem total over limit",
			slug: "package-limits-total",
			files: map[string]string{
				"testdata/001.in":  string(bytes.Repeat([]byte("i"), int(4*mib))),
				"testdata/001.out": string(bytes.Repeat([]byte("o"), int(2*mib))),
			},
			want: "单题测试数据不能超过 6 MiB",
		},
	}
	for _, tc := range kapOJCases {
		t.Run(tc.name, func(t *testing.T) {
			data := renamePackageSlug(t, exported.Bytes(), tc.slug)
			for name, content := range tc.files {
				data = replacePackageFile(t, data, name, content)
			}
			before := e.files(t)
			result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
				problem.PackageImportOptions{ConflictPolicy: "skip"})
			require.NoError(t, err)
			require.Empty(t, result.Imported)
			require.Len(t, result.Failed, 1)
			assert.Contains(t, result.Failed[0].Error, tc.want)
			assert.Equal(t, before, e.files(t), "a rejected import must not write testcase files")
			missing, err := e.repo.FindBySlug(ctx, tc.slug)
			require.NoError(t, err)
			assert.Nil(t, missing)
		})
	}
}
