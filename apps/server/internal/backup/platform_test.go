package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type backupEnv struct {
	svc        *backup.Service
	db         *gorm.DB
	storageDir string
}

func newBackupEnv(t *testing.T) *backupEnv {
	t.Helper()
	dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
	g := testutil.PG(t)
	root := t.TempDir()
	storageDir := filepath.Join(root, "data")
	svc, err := backup.NewService(g, dsn, backup.Options{
		StorageDir: storageDir, BackupDir: filepath.Join(storageDir, "backups"),
	})
	require.NoError(t, err)
	require.NoError(t, svc.Ensure(context.Background()))
	return &backupEnv{svc: svc, db: g, storageDir: storageDir}
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
	require.NoError(t, os.WriteFile(filepath.Join(e.storageDir, "business.txt"), []byte("original"), 0o644))

	pkg, err := e.svc.StartCreate(ctx, backup.KindPlatform, nil, 1)
	require.NoError(t, err)
	require.Equal(t, backup.PackageReady, pkg.Status)
	require.FileExists(t, pkg.Filename)

	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", u.ID).
		Update("nickname", "修改后").Error)
	require.NoError(t, e.db.Where("id = ?", sub.ID).Delete(&submission.Submission{}).Error)
	require.NoError(t, os.Remove(filepath.Join(e.storageDir, "business.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(e.storageDir, "extra.txt"), []byte("extra"), 0o644))

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
	data, err := os.ReadFile(filepath.Join(e.storageDir, "business.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(data))
	_, err = os.Stat(filepath.Join(e.storageDir, "extra.txt"))
	assert.True(t, os.IsNotExist(err), "stale files are removed by a platform restore")
}

func TestPlatformBackupKeepsOnlyLatestSuccessfulPackage(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	first, err := e.svc.StartCreate(ctx, backup.KindPlatform, nil, 1)
	require.NoError(t, err)
	second, err := e.svc.StartCreate(ctx, backup.KindPlatform, nil, 1)
	require.NoError(t, err)
	require.NotEqual(t, first.Filename, second.Filename)
	_, err = os.Stat(first.Filename)
	assert.True(t, os.IsNotExist(err), "the previous successful package is removed")
	ov, err := e.svc.Overview(ctx)
	require.NoError(t, err)
	require.NotNil(t, ov.PlatformPackage)
	assert.Equal(t, second.ID, ov.PlatformPackage.ID)
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
