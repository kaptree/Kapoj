package backup_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type backupEnv struct {
	svc      *backup.Service
	db       *gorm.DB
	files    storage.ObjectStore
	packages storage.ObjectStore
}

func newBackupEnv(t *testing.T) *backupEnv {
	t.Helper()
	g := testutil.PG(t)
	dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
	root := t.TempDir()
	files, err := storage.NewLocal(filepath.Join(root, "files"))
	require.NoError(t, err)
	packages, err := storage.NewLocal(filepath.Join(root, "packages"))
	require.NoError(t, err)
	svc, err := backup.NewService(g, dsn, backup.Options{
		Files: files, Packages: packages, WorkDir: filepath.Join(root, "work"),
	})
	require.NoError(t, err)
	require.NoError(t, svc.Ensure(context.Background()))
	return &backupEnv{svc: svc, db: g, files: files, packages: packages}
}

func TestPlatformBackupRestoresDatabaseAndFiles(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()

	u := &user.User{
		Username: "backup-user", Nickname: "原始昵称",
		Email: "backup-user@example.com", PasswordHash: "hash",
	}
	require.NoError(t, e.db.Create(u).Error)
	p := &problem.Problem{
		Title: "备份题", Slug: "backup-problem", StatementMD: "题干",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000,
		MemoryLimitMB: 256, IsPublic: true, CreatedBy: u.ID,
		Answers: dbtypes.List[string]{},
	}
	require.NoError(t, e.db.Create(p).Error)
	sub := &submission.Submission{
		UserID: u.ID, ProblemID: p.ID, Language: submission.LanguageCPP,
		Code: "int main(){}", Status: submission.StatusAccepted,
	}
	require.NoError(t, e.db.Create(sub).Error)
	_, err := e.files.Put(ctx, "problems/1/business.txt", strings.NewReader("original"), 100)
	require.NoError(t, err)

	pkg, err := e.svc.StartCreate(ctx, backup.KindPlatform, nil, 1)
	require.NoError(t, err)
	require.Equal(t, backup.PackageReady, pkg.Status)
	reader, err := e.packages.Open(ctx, pkg.Filename)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, reader)
	_ = reader.Close()

	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", u.ID).
		Update("nickname", "修改后").Error)
	require.NoError(t, e.db.Where("id = ?", sub.ID).Delete(&submission.Submission{}).Error)
	require.NoError(t, e.files.Delete(ctx, "problems/1/business.txt"))
	_, err = e.files.Put(ctx, "problems/1/extra.txt", strings.NewReader("extra"), 100)
	require.NoError(t, err)

	job, err := e.svc.StartRestore(ctx, backup.KindPlatform, 1, &pkg.ID, "", nil)
	require.NoError(t, err)
	require.Equal(t, backup.RestoreSuccess, job.Status, job.Error)
	assert.True(t, job.RestartRequired)

	var restored user.User
	require.NoError(t, e.db.First(&restored, u.ID).Error)
	assert.Equal(t, "原始昵称", restored.Nickname)
	var count int64
	require.NoError(t, e.db.Model(&submission.Submission{}).Where("id = ?", sub.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	reader, err = e.files.Open(ctx, "problems/1/business.txt")
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	_ = reader.Close()
	assert.Equal(t, "original", string(data))
	_, err = e.files.Open(ctx, "problems/1/extra.txt")
	assert.True(t, errors.Is(err, storage.ErrNotFound), "stale objects are removed by a platform restore")
}

func TestPlatformBackupKeepsOnlyLatestSuccessfulPackage(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	first, err := e.svc.StartCreate(ctx, backup.KindPlatform, nil, 1)
	require.NoError(t, err)
	second, err := e.svc.StartCreate(ctx, backup.KindPlatform, nil, 1)
	require.NoError(t, err)
	require.NotEqual(t, first.Filename, second.Filename)
	_, err = e.packages.Open(ctx, first.Filename)
	assert.True(t, errors.Is(err, storage.ErrNotFound), "the previous successful package is removed")
	ov, err := e.svc.Overview(ctx)
	require.NoError(t, err)
	require.NotNil(t, ov.PlatformPackage)
	assert.Equal(t, second.ID, ov.PlatformPackage.ID)
}

func TestPlatformBackupRetentionKeepsSafetySnapshots(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	_, err := e.packages.Put(ctx, "safety/old.zip", strings.NewReader("safety"), 100)
	require.NoError(t, err)
	safety := &backup.BackupPackage{
		Kind: backup.KindPlatform, Filename: "safety/old.zip", SizeBytes: 6,
		Status: backup.PackageReady, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, e.db.Create(safety).Error)

	_, err = e.svc.StartCreate(ctx, backup.KindPlatform, nil, 1)
	require.NoError(t, err)
	var count int64
	require.NoError(t, e.db.Model(&backup.BackupPackage{}).Where("id = ?", safety.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "safety package rows survive normal retention")
	reader, err := e.packages.Open(ctx, "safety/old.zip")
	require.NoError(t, err)
	_ = reader.Close()
}

func TestPlatformRestoreSchemaMismatchDoesNotTouchFiles(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	_, err := e.files.Put(ctx, "problems/1/keep.txt", strings.NewReader("original"), 100)
	require.NoError(t, err)

	zipPath := filepath.Join(t.TempDir(), "mismatch.zip")
	writer, err := backup.NewWriter(zipPath, 100)
	require.NoError(t, err)
	entry, err := writer.Create("files/problems/1/keep.txt")
	require.NoError(t, err)
	_, err = entry.Write([]byte("replacement"))
	require.NoError(t, err)
	require.NoError(t, writer.Close(backup.Manifest{
		Kind: backup.KindPlatform, CreatedAt: time.Now().UTC(),
		SchemaHash: "different-schema",
	}))

	file, err := os.Open(zipPath)
	require.NoError(t, err)
	uploadKey, err := e.svc.SaveUpload(ctx, "mismatch.zip", file)
	require.NoError(t, file.Close())
	require.NoError(t, err)
	job, err := e.svc.StartRestore(ctx, backup.KindPlatform, 1, nil, uploadKey, nil)
	require.NoError(t, err)
	require.Equal(t, backup.RestoreFailed, job.Status)
	assert.Contains(t, job.Error, "schema")

	reader, err := e.files.Open(ctx, "problems/1/keep.txt")
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	_ = reader.Close()
	assert.Equal(t, "original", string(data))
	_, err = e.packages.Open(ctx, uploadKey)
	assert.True(t, errors.Is(err, storage.ErrNotFound), "failed upload must be cleaned")
}

func TestPlatformRestoreRejectsWrongKind(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	pkg, err := e.svc.StartCreate(ctx, backup.KindAccounts, nil, 1)
	require.NoError(t, err)

	job, err := e.svc.StartRestore(ctx, backup.KindPlatform, 1, &pkg.ID, "", nil)
	require.NoError(t, err)
	require.Equal(t, backup.RestoreFailed, job.Status)
	assert.Contains(t, job.Error, "不是全平台备份包")
}
