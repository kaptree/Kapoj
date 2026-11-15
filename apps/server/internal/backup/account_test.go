package backup_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestAccountBackupRestoreIsScopedToUser(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	a := &user.User{
		Username: "account-a", Nickname: "A", Email: "a@example.com", PasswordHash: "hash-a",
		Coins: 25, AvatarKey: "avatars/a.png",
	}
	b := &user.User{
		Username: "account-b", Nickname: "B", Email: "b@example.com", PasswordHash: "hash-b",
		Coins: 5,
	}
	require.NoError(t, e.db.Create(a).Error)
	require.NoError(t, e.db.Create(b).Error)
	_, err := e.files.Put(ctx, "avatars/a.png", strings.NewReader("avatar-a"), 100)
	require.NoError(t, err)

	p := &problem.Problem{
		Title: "账号题", Slug: "account-problem", StatementMD: "题干",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000,
		MemoryLimitMB: 256, IsPublic: true, CreatedBy: a.ID, Answers: dbtypes.List[string]{},
	}
	require.NoError(t, e.db.Create(p).Error)
	sub := &submission.Submission{
		UserID: a.ID, ProblemID: p.ID, Language: submission.LanguageCPP,
		Code: "int main(){}", Status: submission.StatusAccepted,
	}
	require.NoError(t, e.db.Create(sub).Error)

	pkg, err := e.svc.StartCreate(ctx, backup.KindAccount, &a.ID, 1)
	require.NoError(t, err)
	require.Equal(t, backup.PackageReady, pkg.Status)

	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", a.ID).
		Updates(map[string]any{"nickname": "changed", "coins": 99}).Error)
	require.NoError(t, e.db.Where("id = ?", sub.ID).Delete(&submission.Submission{}).Error)
	require.NoError(t, e.files.Delete(ctx, "avatars/a.png"))

	job, err := e.svc.StartRestore(ctx, backup.KindAccount, 1, &pkg.ID, "", &a.ID)
	require.NoError(t, err)
	require.Equal(t, backup.RestoreSuccess, job.Status, job.Error)
	assert.False(t, job.RestartRequired)

	var restored, other user.User
	require.NoError(t, e.db.First(&restored, a.ID).Error)
	require.NoError(t, e.db.First(&other, b.ID).Error)
	assert.Equal(t, "A", restored.Nickname)
	assert.EqualValues(t, 25, restored.Coins)
	assert.Equal(t, "hash-a", restored.PasswordHash)
	assert.Equal(t, "B", other.Nickname)
	assert.EqualValues(t, 5, other.Coins)
	var subCount int64
	require.NoError(t, e.db.Model(&submission.Submission{}).Where("id = ?", sub.ID).Count(&subCount).Error)
	assert.EqualValues(t, 1, subCount)
	reader, err := e.files.Open(ctx, "avatars/a.png")
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	_ = reader.Close()
	assert.Equal(t, "avatar-a", string(data))
}

func TestAllAccountsPackageRestoresOneUser(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	a := &user.User{Username: "all-a", Nickname: "A", Email: "all-a@example.com", PasswordHash: "a"}
	b := &user.User{Username: "all-b", Nickname: "B", Email: "all-b@example.com", PasswordHash: "b"}
	require.NoError(t, e.db.Create(a).Error)
	require.NoError(t, e.db.Create(b).Error)

	pkg, err := e.svc.StartCreate(ctx, backup.KindAccounts, nil, 1)
	require.NoError(t, err)
	require.Equal(t, backup.PackageReady, pkg.Status)

	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", b.ID).
		Update("nickname", "B changed").Error)
	job, err := e.svc.StartRestore(ctx, backup.KindAccount, 1, &pkg.ID, "", &b.ID)
	require.NoError(t, err)
	require.Equal(t, backup.RestoreSuccess, job.Status, job.Error)

	var restored user.User
	require.NoError(t, e.db.First(&restored, b.ID).Error)
	assert.Equal(t, "B", restored.Nickname)
}

func TestAccountRestoreRejectsIdentityConflict(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	a := &user.User{Username: "conflict-a", Nickname: "A", Email: "conflict-a@example.com", PasswordHash: "h"}
	require.NoError(t, e.db.Create(a).Error)
	pkg, err := e.svc.StartCreate(ctx, backup.KindAccount, &a.ID, 1)
	require.NoError(t, err)

	require.NoError(t, e.db.Model(&user.User{}).Where("id = ?", a.ID).
		Updates(map[string]any{"username": "conflict-a-new", "email": "conflict-a-new@example.com"}).Error)
	other := &user.User{Username: "conflict-a", Nickname: "Other", Email: "other@example.com", PasswordHash: "h"}
	require.NoError(t, e.db.Create(other).Error)

	job, err := e.svc.StartRestore(ctx, backup.KindAccount, 1, &pkg.ID, "", &a.ID)
	require.NoError(t, err)
	require.Equal(t, backup.RestoreFailed, job.Status)
	assert.Contains(t, job.Error, "占用")
}
