package problem_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

// zipEntry describes a single file (or directory/symlink) to place in an
// in-memory zip package built by buildZip.
type zipEntry struct {
	name    string
	body    []byte
	dir     bool
	symlink bool
	// forgedUncompressed, when non-zero, declares a different (forged)
	// UncompressedSize64 for a stored entry; used to simulate a forged header.
	forgedUncompressed uint64
}

func buildZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		switch {
		case e.dir:
			_, err := w.Create(e.name + "/")
			require.NoError(t, err)
		case e.symlink:
			fh := &zip.FileHeader{Name: e.name, Method: zip.Store}
			fh.SetMode(fs.ModeSymlink | 0o777)
			cw, err := w.CreateHeader(fh)
			require.NoError(t, err)
			_, err = cw.Write(e.body)
			require.NoError(t, err)
		case e.forgedUncompressed != 0:
			fh := &zip.FileHeader{
				Name: e.name, Method: zip.Store, CRC32: crc32.ChecksumIEEE(e.body),
				UncompressedSize64: e.forgedUncompressed, CompressedSize64: uint64(len(e.body)),
			}
			cw, err := w.CreateRaw(fh)
			require.NoError(t, err)
			_, err = cw.Write(e.body)
			require.NoError(t, err)
		default:
			cw, err := w.Create(e.name)
			require.NoError(t, err)
			_, err = cw.Write(e.body)
			require.NoError(t, err)
		}
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func importZip(t *testing.T, svc *problem.Service, createdBy int64, data []byte) (*problem.HydroImportResult, error) {
	t.Helper()
	return svc.ImportHydro(context.Background(), createdBy, bytes.NewReader(data), int64(len(data)))
}

func hydroProblem(name, title string, testcases int, extra ...zipEntry) []zipEntry {
	entries := []zipEntry{
		{name: name + "/problem.yaml", body: []byte("title: " + title + "\ntag:\n  - 循环\n  - DAI\n")},
		{name: name + "/problem_zh.md", body: []byte("## 题目描述\n求和 " + title + "\n")},
	}
	for i := 1; i <= testcases; i++ {
		entries = append(entries,
			zipEntry{name: fmt.Sprintf("%s/testdata/%d.in", name, i), body: []byte(fmt.Sprintf("in %d", i))},
			zipEntry{name: fmt.Sprintf("%s/testdata/%d.out", name, i), body: []byte(fmt.Sprintf("out %d", i))},
		)
	}
	entries = append(entries,
		zipEntry{name: name + "/testdata/gen.py", body: []byte("# gen")},
		zipEntry{name: name + "/testdata/std.cpp", body: []byte("int main(){}")},
	)
	return append(entries, extra...)
}

func TestImportHydroSamplePackage(t *testing.T) {
	e := newEnv(t)
	data := buildZip(t, hydroProblem("6760", "求和", 10)...)

	res, err := importZip(t, e.svc, 1, data)
	require.NoError(t, err)
	require.Empty(t, res.Failed)
	require.Len(t, res.Imported, 1)

	imp := res.Imported[0]
	assert.Equal(t, "求和", imp.Title)
	assert.Equal(t, "hydro-6760", imp.Slug)
	assert.Equal(t, 10, imp.TestcaseCount)

	p, err := e.repo.FindByID(context.Background(), imp.ID)
	require.NoError(t, err)
	assert.Equal(t, "求和", p.Title)
	assert.Equal(t, "hydro-6760", p.Slug)
	assert.Equal(t, "HydroOJ", p.Source)
	assert.False(t, p.IsPublic)
	assert.Equal(t, int64(1), p.CreatedBy)
	assert.Equal(t, []string{"循环", "DAI"}, []string(p.Tags))
	assert.Equal(t, "## 题目描述\n求和 求和\n", p.StatementMD)
	assert.Equal(t, problem.TypeTraditional, p.Type)
	assert.Equal(t, 1, p.Difficulty)
	assert.Equal(t, 1000, p.TimeLimitMS)
	assert.Equal(t, 256, p.MemoryLimitMB)

	// Testcases are ordered numerically (1,2,…,10), score 10.
	cases, err := e.repo.ListTestcases(context.Background(), imp.ID)
	require.NoError(t, err)
	require.Len(t, cases, 10)
	for i, tc := range cases {
		assert.Equal(t, 10, tc.Score)
		assert.Equal(t, i+1, tc.SortOrder)
		rc, err := e.store.Open(context.Background(), tc.InputFile)
		require.NoError(t, err)
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		assert.Equal(t, fmt.Sprintf("in %d", i+1), string(b), "numeric ordering must hold")
	}
}

func TestImportHydroStatementFallbackToProblemMd(t *testing.T) {
	e := newEnv(t)
	data := buildZip(t,
		zipEntry{name: "p/problem.yaml", body: []byte("title: 回退\n")},
		zipEntry{name: "p/problem.md", body: []byte("英文题面")},
		zipEntry{name: "p/testdata/1.in", body: []byte("1")},
		zipEntry{name: "p/testdata/1.out", body: []byte("2")},
	)
	res, err := importZip(t, e.svc, 1, data)
	require.NoError(t, err)
	require.Len(t, res.Imported, 1)
	p, err := e.repo.FindByID(context.Background(), res.Imported[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "英文题面", p.StatementMD)
}

func TestImportHydroMultipleAndPartialFailure(t *testing.T) {
	e := newEnv(t)
	good := hydroProblem("6760", "好题", 2)
	// 坏题：缺 problem_zh.md 与 problem.md。
	bad := []zipEntry{
		{name: "6761/problem.yaml", body: []byte("title: 坏题\n")},
		{name: "6761/testdata/1.in", body: []byte("1")},
		{name: "6761/testdata/1.out", body: []byte("2")},
	}
	data := buildZip(t, append(append([]zipEntry{}, good...), bad...)...)

	res, err := importZip(t, e.svc, 1, data)
	require.NoError(t, err)
	require.Len(t, res.Imported, 1)
	require.Len(t, res.Failed, 1)
	assert.Equal(t, "hydro-6760", res.Imported[0].Slug)
	assert.Equal(t, "6761", res.Failed[0].Folder)
	assert.Equal(t, "坏题", res.Failed[0].Title)
	assert.Contains(t, res.Failed[0].Error, "题面")

	// 坏题无残留：数据库只有好题一行。
	var n int64
	require.NoError(t, e.db.Model(&problem.Problem{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

func TestImportHydroSlugConflictDoesNotOverwrite(t *testing.T) {
	e := newEnv(t)
	existing := e.create(t, input("hydro-6760"))

	data := buildZip(t, hydroProblem("6760", "新题", 1)...)
	res, err := importZip(t, e.svc, 1, data)
	require.NoError(t, err)
	require.Empty(t, res.Imported)
	require.Len(t, res.Failed, 1)
	assert.Contains(t, res.Failed[0].Error, "题目标识已存在")

	got, err := e.repo.FindByID(context.Background(), existing.ID)
	require.NoError(t, err)
	assert.Equal(t, "Problem hydro-6760", got.Title, "existing problem must not be overwritten")
}

func TestImportHydroDuplicateFoldersBothFail(t *testing.T) {
	e := newEnv(t)
	// 两个不同包装目录下的同名文件夹 → 相同 slug → 双双失败。
	data := buildZip(t,
		zipEntry{name: "a/6760/problem.yaml", body: []byte("title: A\n")},
		zipEntry{name: "a/6760/problem_zh.md", body: []byte("a")},
		zipEntry{name: "a/6760/testdata/1.in", body: []byte("1")},
		zipEntry{name: "a/6760/testdata/1.out", body: []byte("2")},
		zipEntry{name: "b/6760/problem.yaml", body: []byte("title: B\n")},
		zipEntry{name: "b/6760/problem_zh.md", body: []byte("b")},
		zipEntry{name: "b/6760/testdata/1.in", body: []byte("1")},
		zipEntry{name: "b/6760/testdata/1.out", body: []byte("2")},
	)
	res, err := importZip(t, e.svc, 1, data)
	require.NoError(t, err)
	require.Empty(t, res.Imported)
	require.Len(t, res.Failed, 2)
	for _, f := range res.Failed {
		assert.Contains(t, f.Error, "题目标识已存在")
	}
}

func TestImportHydroValidation(t *testing.T) {
	valid := hydroProblem("6760", "求和", 1)

	cases := []struct {
		name       string
		data       []byte
		wantErr    bool
		failSubstr []string // per-problem failure substrings (when !wantErr)
	}{
		{
			name:    "no problem folders",
			data:    buildZip(t, zipEntry{name: "readme.txt", body: []byte("hi")}),
			wantErr: true,
		},
		{
			name:    "broken zip",
			data:    []byte("this is not a zip archive"),
			wantErr: true,
		},
		{
			name: "missing testdata",
			data: buildZip(t,
				zipEntry{name: "6760/problem.yaml", body: []byte("title: 求和\n")},
				zipEntry{name: "6760/problem_zh.md", body: []byte("题面")},
			),
			failSubstr: []string{"测试数据"},
		},
		{
			name: "unpaired in",
			data: buildZip(t,
				zipEntry{name: "6760/problem.yaml", body: []byte("title: 求和\n")},
				zipEntry{name: "6760/problem_zh.md", body: []byte("题面")},
				zipEntry{name: "6760/testdata/1.in", body: []byte("1")},
			),
			failSubstr: []string{"不配对"},
		},
		{
			name: "illegal slug",
			data: buildZip(t,
				zipEntry{name: "###/problem.yaml", body: []byte("title: 求和\n")},
				zipEntry{name: "###/problem_zh.md", body: []byte("题面")},
				zipEntry{name: "###/testdata/1.in", body: []byte("1")},
				zipEntry{name: "###/testdata/1.out", body: []byte("2")},
			),
			failSubstr: []string{"标识"},
		},
		{
			name: "path traversal",
			data: buildZip(t, append(valid,
				zipEntry{name: "../evil/problem.yaml", body: []byte("title: x\n")})...),
			wantErr: true,
		},
		{
			name: "symlink",
			data: buildZip(t, append(valid,
				zipEntry{name: "6760/link", body: []byte("target"), symlink: true})...),
			wantErr: true,
		},
		{
			name: "too many entries",
			data: buildZip(t, func() []zipEntry {
				es := make([]zipEntry, 5001)
				for i := range es {
					es[i] = zipEntry{name: fmt.Sprintf("f%04d.txt", i), body: []byte("x")}
				}
				return es
			}()...),
			wantErr: true,
		},
		{
			name: "too many problems",
			data: buildZip(t, func() []zipEntry {
				es := make([]zipEntry, 0, 101)
				for i := 0; i < 101; i++ {
					es = append(es, zipEntry{name: fmt.Sprintf("p%03d/problem.yaml", i), body: []byte("title: x\n")})
				}
				return es
			}()...),
			wantErr: true,
		},
		{
			name: "forged uncompressed size",
			data: buildZip(t,
				zipEntry{name: "6760/problem.yaml", body: []byte("title: 求和\n"), forgedUncompressed: 1<<30 + 1},
			),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			res, err := importZip(t, e.svc, 1, tc.data)
			if tc.wantErr {
				var appErr *httpx.AppError
				require.Error(t, err)
				require.True(t, errors.As(err, &appErr), "expected AppError, got %v", err)
				assert.Equal(t, 40001, appErr.Code, appErr.Message)
				assert.Empty(t, e.files(t))
				return
			}
			require.NoError(t, err)
			require.Empty(t, res.Imported)
			require.NotEmpty(t, res.Failed, tc.name)
			for i, sub := range tc.failSubstr {
				if i < len(res.Failed) {
					assert.Contains(t, res.Failed[i].Error, sub, tc.name)
				}
			}
			assert.Empty(t, e.files(t))
		})
	}
}

// failingStore fails the Nth Put call (1-based) to exercise cleanup.
type failingStore struct {
	storage.Storage
	failOnPut int
	puts      int
}

func (s *failingStore) Put(ctx context.Context, key string, r io.Reader, max int64) (int64, error) {
	s.puts++
	if s.failOnPut == s.puts {
		return 0, errors.New("injected storage failure")
	}
	return s.Storage.Put(ctx, key, r, max)
}

func TestImportHydroCleanupOnTestcaseFailure(t *testing.T) {
	e := newEnv(t)
	// 第 4 次 Put（第二个测试点的输出）失败，触发 AddTestcase 与题目清理。
	fs := &failingStore{Storage: e.store, failOnPut: 4}
	svc := problem.NewService(e.repo, fs)

	data := buildZip(t, hydroProblem("6760", "求和", 2)...)
	res, err := importZip(t, svc, 1, data)
	require.NoError(t, err)
	require.Empty(t, res.Imported)
	require.Len(t, res.Failed, 1)
	assert.Equal(t, "6760", res.Failed[0].Folder)

	var n int64
	require.NoError(t, e.db.Model(&problem.Problem{}).Count(&n).Error)
	assert.Zero(t, n, "the partially imported problem must be deleted")
	assert.Empty(t, e.files(t), "all stored files must be removed")
}

func TestImportHydroRollbackOnZipLevelError(t *testing.T) {
	e := newEnv(t)
	// 解压预算 100 字节：第一题（约 13 字节）导入成功，第二题的大测试点把
	// 实际读取总量推到预算之上，触发整包错误并回滚已导入的题目与文件。
	svc := problem.NewService(e.repo, e.store, problem.WithHydroUncompressedLimit(100))

	data := buildZip(t,
		zipEntry{name: "6760/problem.yaml", body: []byte("title: A\n")},
		zipEntry{name: "6760/problem_zh.md", body: []byte("A")},
		zipEntry{name: "6760/testdata/1.in", body: []byte("in")},
		zipEntry{name: "6760/testdata/1.out", body: []byte("o")},
		zipEntry{name: "6761/problem.yaml", body: []byte("title: B\n")},
		zipEntry{name: "6761/problem_zh.md", body: []byte("B")},
		zipEntry{name: "6761/testdata/1.in", body: bytes.Repeat([]byte("x"), 200)},
		zipEntry{name: "6761/testdata/1.out", body: []byte("o")},
	)

	res, err := importZip(t, svc, 1, data)
	var appErr *httpx.AppError
	require.Error(t, err)
	require.True(t, errors.As(err, &appErr), "expected AppError, got %v", err)
	assert.Equal(t, 40001, appErr.Code, appErr.Message)
	assert.Nil(t, res, "a package-level error must not return a result")

	// 回滚：第一题与第二题的临时数据都不能残留。
	var n int64
	require.NoError(t, e.db.Model(&problem.Problem{}).Count(&n).Error)
	assert.Zero(t, n, "all imported problems must be rolled back")
	assert.Empty(t, e.files(t), "all stored files must be removed")
}

func TestImportHydroEmptyTestcaseFilesAllowed(t *testing.T) {
	e := newEnv(t)
	data := buildZip(t,
		zipEntry{name: "6760/problem.yaml", body: []byte("title: 求和\n")},
		zipEntry{name: "6760/problem_zh.md", body: []byte("题面")},
		zipEntry{name: "6760/testdata/1.in", body: []byte{}},
		zipEntry{name: "6760/testdata/1.out", body: []byte{}},
	)
	res, err := importZip(t, e.svc, 1, data)
	require.NoError(t, err)
	require.Len(t, res.Imported, 1)
	assert.Equal(t, 1, res.Imported[0].TestcaseCount)
	cases, err := e.repo.ListTestcases(context.Background(), res.Imported[0].ID)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	assert.Equal(t, int64(0), cases[0].InputSize)
	assert.Equal(t, int64(0), cases[0].OutputSize)
}
