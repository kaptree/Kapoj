package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
)

func TestMigrateLegacyPackagesCopiesObjectsAndRewritesPaths(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	source := t.TempDir()
	legacyPath := filepath.Join(source, "platform", "legacy.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.WriteFile(legacyPath, []byte("legacy-zip"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "ai.key"), []byte("secret"), 0o600))

	pkg := &backup.BackupPackage{
		Kind: backup.KindPlatform, Filename: legacyPath, SizeBytes: 10,
		Status: backup.PackageReady, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, e.db.Create(pkg).Error)
	job := &backup.BackupRestoreJob{
		Kind: backup.KindPlatform, UploadedFile: legacyPath,
		Status: backup.RestoreRunning, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, e.db.Create(job).Error)

	copied, updated, err := backup.MigrateLegacyPackages(ctx, e.db, source, e.packages)
	require.NoError(t, err)
	assert.Equal(t, 1, copied)
	assert.Equal(t, 2, updated)

	reader, err := e.packages.Open(ctx, "platform/legacy.zip")
	require.NoError(t, err)
	_ = reader.Close()

	var gotPkg backup.BackupPackage
	require.NoError(t, e.db.First(&gotPkg, pkg.ID).Error)
	assert.Equal(t, "platform/legacy.zip", gotPkg.Filename)
	var gotJob backup.BackupRestoreJob
	require.NoError(t, e.db.First(&gotJob, job.ID).Error)
	assert.Equal(t, "platform/legacy.zip", gotJob.UploadedFile)
}
