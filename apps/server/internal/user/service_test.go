package user_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

var (
	pngMagic  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	jpegMagic = []byte{0xff, 0xd8, 0xff, 0xe0}
	gifMagic  = []byte("GIF89a")
	webpMagic = []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
)

type avatarEnv struct {
	svc   *user.Service
	repo  user.Repository
	store storage.Storage
	root  string
	user  *user.User
}

func newAvatarEnv(t *testing.T) *avatarEnv {
	t.Helper()
	repo := user.NewMemoryRepository()
	root := t.TempDir()
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	svc := user.NewService(repo, user.WithStorage(store))

	u := &user.User{
		Username:     "avatar-user",
		Nickname:     "头像用户",
		Email:        "avatar-user@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, repo.Create(context.Background(), u))
	return &avatarEnv{svc: svc, repo: repo, store: store, root: root, user: u}
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr, "expected AppError, got %T: %v", err, err)
	assert.Equal(t, code, appErr.Code, appErr.Message)
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	require.NoError(t, err)
	return out
}

func TestUploadAvatar(t *testing.T) {
	e := newAvatarEnv(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		data   []byte
		wantCT string
	}{
		{"png", pngMagic, "image/png"},
		{"jpeg", jpegMagic, "image/jpeg"},
		{"webp", webpMagic, "image/webp"},
		{"gif", gifMagic, "image/gif"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := e.svc.UploadAvatar(ctx, e.user.ID, bytes.NewReader(tc.data))
			require.NoError(t, err)
			assert.NotEmpty(t, u.AvatarKey)
			assert.Contains(t, u.AvatarKey, "avatars/")
			assert.Equal(t, tc.wantCT, u.AvatarContentType)
			assert.Contains(t, u.AvatarURL, "/api/users/")
			assert.Contains(t, u.AvatarURL, "/avatar?v=")

			img, err := e.svc.AvatarImage(ctx, e.user.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.data, img.Data)
			assert.Equal(t, tc.wantCT, img.ContentType)

			me := user.ToMe(u)
			assert.True(t, me.AvatarUploaded)
		})
	}
}

func TestUploadAvatarReplacesOldKey(t *testing.T) {
	e := newAvatarEnv(t)
	ctx := context.Background()

	first, err := e.svc.UploadAvatar(ctx, e.user.ID, bytes.NewReader(pngMagic))
	require.NoError(t, err)
	oldKey := first.AvatarKey
	require.NotEmpty(t, oldKey)

	second, err := e.svc.UploadAvatar(ctx, e.user.ID, bytes.NewReader(gifMagic))
	require.NoError(t, err)
	assert.NotEqual(t, oldKey, second.AvatarKey)
	assert.NotEqual(t, first.AvatarURL, second.AvatarURL, "cache-busting v must change per upload")

	_, err = e.store.Open(ctx, oldKey)
	assert.ErrorIs(t, err, storage.ErrNotFound, "old avatar file must be deleted")

	img, err := e.svc.AvatarImage(ctx, e.user.ID)
	require.NoError(t, err)
	assert.Equal(t, gifMagic, img.Data)
}

func TestUploadAvatarValidation(t *testing.T) {
	e := newAvatarEnv(t)
	ctx := context.Background()

	_, err := e.svc.UploadAvatar(ctx, e.user.ID, strings.NewReader(""))
	assertCode(t, err, 40001)
	_, err = e.svc.UploadAvatar(ctx, e.user.ID,
		io.MultiReader(bytes.NewReader(pngMagic), bytes.NewReader(make([]byte, maxAvatarSizeForTest))))
	assertCode(t, err, 40001)
	_, err = e.svc.UploadAvatar(ctx, e.user.ID, strings.NewReader("plain text"))
	assertCode(t, err, 40001)

	// 校验失败不改变用户数据。
	u, err := e.repo.FindByID(ctx, e.user.ID)
	require.NoError(t, err)
	assert.Empty(t, u.AvatarKey)
	assert.Empty(t, u.AvatarURL)
	assert.Empty(t, listFiles(t, e.root), "no file may be written on validation failure")
}

func TestUploadAvatarStoreFailurePreservesOld(t *testing.T) {
	repo := user.NewMemoryRepository()
	u := &user.User{Username: "store-fail", Nickname: "sf", Email: "store-fail@example.com", PasswordHash: "x"}
	require.NoError(t, repo.Create(context.Background(), u))
	store := failingStore{Storage: mustLocal(t), putErr: errors.New("disk full")}
	svc := user.NewService(repo, user.WithStorage(store))

	_, err := svc.UploadAvatar(context.Background(), u.ID, bytes.NewReader(pngMagic))
	require.Error(t, err)

	got, err := repo.FindByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Empty(t, got.AvatarKey)
	assert.Empty(t, got.AvatarURL)
}

func TestUploadAvatarDBFailureCleansNewKey(t *testing.T) {
	repo := user.NewMemoryRepository()
	u := &user.User{Username: "db-fail", Nickname: "df", Email: "db-fail@example.com", PasswordHash: "x"}
	require.NoError(t, repo.Create(context.Background(), u))
	root := t.TempDir()
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	failing := &failingUpdateRepo{Repository: repo, err: errors.New("db down")}
	svc := user.NewService(failing, user.WithStorage(store))

	_, err = svc.UploadAvatar(context.Background(), u.ID, bytes.NewReader(pngMagic))
	require.Error(t, err)
	assert.Empty(t, listFiles(t, root), "newly stored file must be cleaned up on DB failure")

	got, err := repo.FindByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Empty(t, got.AvatarKey, "old state preserved")
}

func TestDeleteAvatar(t *testing.T) {
	e := newAvatarEnv(t)
	ctx := context.Background()

	u, err := e.svc.UploadAvatar(ctx, e.user.ID, bytes.NewReader(pngMagic))
	require.NoError(t, err)
	key := u.AvatarKey
	require.NotEmpty(t, key)

	del, err := e.svc.DeleteAvatar(ctx, e.user.ID)
	require.NoError(t, err)
	assert.Empty(t, del.AvatarURL)
	assert.Empty(t, del.AvatarKey)
	assert.Empty(t, del.AvatarContentType)

	_, err = e.store.Open(ctx, key)
	assert.ErrorIs(t, err, storage.ErrNotFound, "file must be deleted")

	// 幂等：无上传时再次删除成功。
	again, err := e.svc.DeleteAvatar(ctx, e.user.ID)
	require.NoError(t, err)
	assert.Empty(t, again.AvatarURL)
	assert.Empty(t, again.AvatarKey)

	_, err = e.svc.AvatarImage(ctx, e.user.ID)
	assert.ErrorIs(t, err, httpx.ErrNotFound)
}

func TestUpdateMeAvatarSources(t *testing.T) {
	t.Run("non-empty external replaces upload", func(t *testing.T) {
		e := newAvatarEnv(t)
		ctx := context.Background()
		u, err := e.svc.UploadAvatar(ctx, e.user.ID, bytes.NewReader(pngMagic))
		require.NoError(t, err)
		oldKey := u.AvatarKey

		external := "https://cdn.example.com/a.png"
		got, err := e.svc.UpdateMe(ctx, e.user.ID, user.UpdateMeInput{AvatarURL: &external})
		require.NoError(t, err)
		assert.Equal(t, external, got.AvatarURL)
		assert.Empty(t, got.AvatarKey)
		assert.Empty(t, got.AvatarContentType)

		_, err = e.store.Open(ctx, oldKey)
		assert.ErrorIs(t, err, storage.ErrNotFound, "old file must be deleted")
		_, err = e.svc.AvatarImage(ctx, e.user.ID)
		assert.ErrorIs(t, err, httpx.ErrNotFound)
	})

	t.Run("empty with upload preserves", func(t *testing.T) {
		e := newAvatarEnv(t)
		ctx := context.Background()
		u, err := e.svc.UploadAvatar(ctx, e.user.ID, bytes.NewReader(pngMagic))
		require.NoError(t, err)
		key := u.AvatarKey
		internal := u.AvatarURL

		empty := ""
		got, err := e.svc.UpdateMe(ctx, e.user.ID, user.UpdateMeInput{AvatarURL: &empty})
		require.NoError(t, err)
		assert.Equal(t, internal, got.AvatarURL, "internal URL preserved")
		assert.Equal(t, key, got.AvatarKey)

		img, err := e.svc.AvatarImage(ctx, e.user.ID)
		require.NoError(t, err)
		assert.Equal(t, pngMagic, img.Data)
	})

	t.Run("empty with no upload clears URL", func(t *testing.T) {
		e := newAvatarEnv(t)
		ctx := context.Background()
		external := "https://cdn.example.com/a.png"
		_, err := e.svc.UpdateMe(ctx, e.user.ID, user.UpdateMeInput{AvatarURL: &external})
		require.NoError(t, err)

		empty := ""
		got, err := e.svc.UpdateMe(ctx, e.user.ID, user.UpdateMeInput{AvatarURL: &empty})
		require.NoError(t, err)
		assert.Empty(t, got.AvatarURL)
		assert.Empty(t, got.AvatarKey)
	})
}

func TestToMeAndProfileAvatarUploaded(t *testing.T) {
	withKey := &user.User{AvatarKey: "avatars/1/abc"}
	withoutKey := &user.User{}

	assert.True(t, user.ToMe(withKey).AvatarUploaded)
	assert.False(t, user.ToMe(withoutKey).AvatarUploaded)
	assert.True(t, user.ToProfile(withKey).AvatarUploaded)
	assert.False(t, user.ToProfile(withoutKey).AvatarUploaded)
}

const maxAvatarSizeForTest = 2 << 20

func mustLocal(t *testing.T) storage.Storage {
	t.Helper()
	s, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	return s
}

// failingStore overrides Put to simulate storage failures.
type failingStore struct {
	storage.Storage
	putErr error
}

func (f failingStore) Put(context.Context, string, io.Reader, int64) (int64, error) {
	return 0, f.putErr
}

// failingUpdateRepo overrides UpdateProfile to simulate database failures.
type failingUpdateRepo struct {
	user.Repository
	err error
}

func (f *failingUpdateRepo) UpdateProfile(context.Context, int64, user.ProfileChanges) error {
	return f.err
}
