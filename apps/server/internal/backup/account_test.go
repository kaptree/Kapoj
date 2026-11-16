package backup_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/companion"
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

func TestAccountBackupIncludesCompanionRows(t *testing.T) {
	e := newBackupEnv(t)
	ctx := context.Background()
	a := &user.User{
		Username: "companion-a", Nickname: "A", Email: "companion-a@example.com", PasswordHash: "hash-a",
	}
	b := &user.User{
		Username: "companion-b", Nickname: "B", Email: "companion-b@example.com", PasswordHash: "hash-b",
	}
	require.NoError(t, e.db.Create(a).Error)
	require.NoError(t, e.db.Create(b).Error)

	aState := &companion.State{
		UserID: a.ID, Bond: 120, Level: 3, DailyGain: 4, DailyDate: "2026-11-17",
		ActivityLevel: "lively", DockEnabled: true, PublicProfile: true,
		EquippedSkin: "fox", UnlockedSkins: dbtypes.List[string]{"fox", "cat"},
		LastPetDate: "2026-11-17",
	}
	require.NoError(t, e.db.Create(aState).Error)
	aEvent := &companion.Event{
		UserID: a.ID, Key: "signin:2026-11-17", Type: "signin", Bond: 3,
		EventDate: "2026-11-17", CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, e.db.Create(aEvent).Error)
	bState := &companion.State{UserID: b.ID, Bond: 7, Level: 0, DailyGain: 1, DailyDate: "2026-11-17"}
	require.NoError(t, e.db.Create(bState).Error)
	bEvent := &companion.Event{
		UserID: b.ID, Key: "pet:2026-11-17", Type: "pet", Bond: 1,
		EventDate: "2026-11-17", CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, e.db.Create(bEvent).Error)

	pkg, err := e.svc.StartCreate(ctx, backup.KindAccount, &a.ID, 1)
	require.NoError(t, err)
	require.Equal(t, backup.PackageReady, pkg.Status)

	rc, err := e.packages.Open(ctx, pkg.Filename)
	require.NoError(t, err)
	zipPath := filepath.Join(t.TempDir(), "account.zip")
	file, err := os.Create(zipPath)
	require.NoError(t, err)
	_, err = io.Copy(file, rc)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.NoError(t, rc.Close())
	reader, err := backup.OpenReader(zipPath, 0, 0)
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()
	assert.True(t, reader.Has("account/companion_states.csv"), "账号备份应包含伙伴状态 CSV")
	assert.True(t, reader.Has("account/companion_events.csv"), "账号备份应包含伙伴事件 CSV")
	assert.Contains(t, reader.Manifest.Tables, "companion_states")
	assert.Contains(t, reader.Manifest.Tables, "companion_events")

	// Replace the exported rows with stale ones. The stale event must disappear on
	// restore, which only happens when the restore delete list covers the table.
	require.NoError(t, e.db.Model(&companion.State{}).Where("user_id = ?", a.ID).
		Updates(map[string]any{"bond": 999, "equipped_skin": "stale"}).Error)
	require.NoError(t, e.db.Where("user_id = ?", a.ID).Delete(&companion.Event{}).Error)
	stale := &companion.Event{
		UserID: a.ID, Key: "stale:2026-11-17", Type: "task", Bond: 1,
		EventDate: "2026-11-17", CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, e.db.Create(stale).Error)

	job, err := e.svc.StartRestore(ctx, backup.KindAccount, 1, &pkg.ID, "", &a.ID)
	require.NoError(t, err)
	require.Equal(t, backup.RestoreSuccess, job.Status, job.Error)

	var restored companion.State
	require.NoError(t, e.db.First(&restored, "user_id = ?", a.ID).Error)
	assert.Equal(t, 120, restored.Bond)
	assert.Equal(t, 3, restored.Level)
	assert.Equal(t, 4, restored.DailyGain)
	assert.Equal(t, "lively", restored.ActivityLevel)
	assert.Equal(t, "fox", restored.EquippedSkin)
	assert.Equal(t, dbtypes.List[string]{"fox", "cat"}, restored.UnlockedSkins)
	assert.Equal(t, "2026-11-17", restored.LastPetDate)

	var events []companion.Event
	require.NoError(t, e.db.Where("user_id = ?", a.ID).Order("id").Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, "signin:2026-11-17", events[0].Key)
	assert.Equal(t, "signin", events[0].Type)
	assert.Equal(t, 3, events[0].Bond)
	assert.NotEqual(t, stale.ID, events[0].ID, "恢复前的陈旧事件应先被删除")

	// The other user's companion rows must survive untouched.
	var otherState companion.State
	require.NoError(t, e.db.First(&otherState, "user_id = ?", b.ID).Error)
	assert.Equal(t, 7, otherState.Bond)
	var otherEvents []companion.Event
	require.NoError(t, e.db.Where("user_id = ?", b.ID).Find(&otherEvents).Error)
	require.Len(t, otherEvents, 1)
	assert.Equal(t, bEvent.ID, otherEvents[0].ID)
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
