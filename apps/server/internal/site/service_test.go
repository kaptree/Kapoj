package site_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/site"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func newSite(t *testing.T) (*site.Service, site.Repository, storage.Storage) {
	t.Helper()
	g := testutil.PG(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	repo := site.NewGormRepository(g)
	return site.NewService(repo, store), repo, store
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestSiteSettingsLazyDefaults(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	view, err := svc.Public(ctx)
	require.NoError(t, err)
	assert.Equal(t, "KAPOJ // ONLINE JUDGE", view.Home.Eyebrow)
	assert.Equal(t, "欢迎来到", view.Home.Title)
	assert.Equal(t, "kapOJ", view.Home.TitleHighlight)
	assert.Len(t, view.Home.QuickLinks, 4)
	assert.True(t, view.Footer.ShowVisits)
	assert.Equal(t, int64(0), view.VisitCount)
	assert.NotZero(t, view.StartedAt)

	again, err := svc.Public(ctx)
	require.NoError(t, err)
	assert.Equal(t, view.StartedAt, again.StartedAt)
}

func TestSiteHomeValidation(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	bad := site.DefaultHomeContent()
	bad.PrimaryCTA.To = "javascript:alert(1)"
	_, err := svc.UpdateHome(ctx, 1, bad)
	assertCode(t, err, 40001)

	bad = site.DefaultHomeContent()
	bad.QuickLinks = append(bad.QuickLinks, bad.QuickLinks[0])
	_, err = svc.UpdateHome(ctx, 1, bad)
	assertCode(t, err, 40001)
}

func TestDefaultHomeLinksHaveIconPos(t *testing.T) {
	home := site.DefaultHomeContent()
	links := append([]site.Link{}, home.QuickLinks...)
	links = append(links, home.GrowthLinks...)
	links = append(links, home.PrimaryCTA, home.SecondaryCTA)
	require.NotEmpty(t, links)
	for _, link := range links {
		assert.Contains(t, []string{"left", "right"}, link.IconPos, "link %q", link.Title)
	}
}

func TestSiteHomeNormalizesEmptyIconPos(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	home := site.DefaultHomeContent()
	home.QuickLinks[0].IconPos = ""
	home.GrowthLinks[0].IconPos = ""

	view, err := svc.UpdateHome(ctx, 1, home)
	require.NoError(t, err)
	assert.Equal(t, "left", view.Home.QuickLinks[0].IconPos)
	assert.Equal(t, "left", view.Home.GrowthLinks[0].IconPos)

	pub, err := svc.Public(ctx)
	require.NoError(t, err)
	assert.Equal(t, "left", pub.Home.QuickLinks[0].IconPos)
	assert.Equal(t, "left", pub.Home.GrowthLinks[0].IconPos)
}

func TestSiteFooterValidation(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	in := site.DefaultFooterInput()
	in.PoliceURL = "http://example.com"
	_, err := svc.UpdateFooter(ctx, 1, in)
	assertCode(t, err, 40001)

	in = site.DefaultFooterInput()
	in.StartedAt = time.Now().Add(48 * time.Hour)
	_, err = svc.UpdateFooter(ctx, 1, in)
	assertCode(t, err, 40001)

	in = site.DefaultFooterInput()
	in.StartedAt = time.Now().Add(-72 * time.Hour)
	view, err := svc.UpdateFooter(ctx, 1, in)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, view.RunningDays, 2)
}

func TestSiteVisitCounter(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	n, err := svc.RecordVisit(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	n, err = svc.RecordVisit(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestSiteLogoLifecycle(t *testing.T) {
	svc, repo, store := newSite(t)
	ctx := context.Background()
	png := pngBytes(t)
	view, err := svc.UploadLogo(ctx, 1, bytes.NewReader(png))
	require.NoError(t, err)
	assert.Contains(t, view.LogoURL, "/api/site/logo?v=")
	oldURL := view.LogoURL
	settings, err := repo.Get(ctx)
	require.NoError(t, err)
	oldKey := settings.LogoKey

	img, err := svc.Logo(ctx)
	require.NoError(t, err)
	assert.Equal(t, "image/png", img.ContentType)
	assert.Equal(t, png, img.Data)

	view, err = svc.UploadLogo(ctx, 1, bytes.NewReader(png))
	require.NoError(t, err)
	assert.NotEqual(t, oldURL, view.LogoURL)
	_, err = store.Open(ctx, oldKey)
	assert.ErrorIs(t, err, storage.ErrNotFound, "old logo file must be cleaned")

	view, err = svc.DeleteLogo(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, view.LogoURL)
	_, err = svc.Logo(ctx)
	assert.ErrorIs(t, err, site.ErrNotFound)
}

func TestSiteRejectsNonImage(t *testing.T) {
	svc, _, _ := newSite(t)
	_, err := svc.UploadFavicon(context.Background(), 1, bytes.NewBufferString("<svg/>"))
	assertCode(t, err, 40001)
}
