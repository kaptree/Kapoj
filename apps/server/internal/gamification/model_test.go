package gamification_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestBadgeAndTitleModelsMigrate(t *testing.T) {
	g := testutil.PG(t)

	badge := &gamification.Badge{Name: "早鸟", IsActive: false}
	require.NoError(t, g.Create(badge).Error)
	var gotBadge gamification.Badge
	require.NoError(t, g.First(&gotBadge, badge.ID).Error)
	assert.False(t, gotBadge.IsActive, "Badge.IsActive 不能因 DB 默认值变成 true")
	assert.Equal(t, "其他", gotBadge.Category, "Category 应使用默认分类")

	activeBadge := &gamification.Badge{Name: "全勤", IsActive: true}
	require.NoError(t, g.Create(activeBadge).Error)
	var gotActiveBadge gamification.Badge
	require.NoError(t, g.First(&gotActiveBadge, activeBadge.ID).Error)
	assert.True(t, gotActiveBadge.IsActive)

	title := &gamification.Title{
		Name:                "卷王",
		Color:               "#7C3AED",
		UnlockConditionType: "ac_count",
		IsActive:            false,
	}
	require.NoError(t, g.Create(title).Error)
	var gotTitle gamification.Title
	require.NoError(t, g.First(&gotTitle, title.ID).Error)
	assert.False(t, gotTitle.IsActive, "Title.IsActive 不能因 DB 默认值变成 true")

	userBadge := &gamification.UserBadge{UserID: 1, BadgeID: badge.ID, PurchasedAt: time.Now()}
	require.NoError(t, g.Create(userBadge).Error)
	err := g.Create(&gamification.UserBadge{UserID: 1, BadgeID: badge.ID, PurchasedAt: time.Now()}).Error
	assert.Error(t, err, "user_badges 应拒绝重复的 (user_id, badge_id)")

	userTitle := &gamification.UserTitle{UserID: 1, TitleID: title.ID, UnlockedAt: time.Now()}
	require.NoError(t, g.Create(userTitle).Error)
	err = g.Create(&gamification.UserTitle{UserID: 1, TitleID: title.ID, UnlockedAt: time.Now()}).Error
	assert.Error(t, err, "user_titles 应拒绝重复的 (user_id, title_id)")
}

func TestBadgeAndTitleRepositoryReads(t *testing.T) {
	g := testutil.PG(t)
	repo := gamification.NewRepository(g)
	ctx := context.Background()

	second := &gamification.Badge{Name: "后", SortOrder: 2, IsActive: true}
	first := &gamification.Badge{Name: "前", SortOrder: 1, IsActive: true}
	require.NoError(t, g.Create(second).Error)
	require.NoError(t, g.Create(first).Error)

	badges, err := repo.ListBadges(ctx)
	require.NoError(t, err)
	require.Len(t, badges, 2)
	assert.Equal(t, first.ID, badges[0].ID)
	assert.Equal(t, second.ID, badges[1].ID)

	found, err := repo.FindBadge(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, "前", found.Name)

	_, err = repo.FindBadge(ctx, first.ID+1000)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	title := &gamification.Title{
		Name:                "称号",
		Color:               "#123456",
		UnlockConditionType: "rating",
		IsActive:            true,
	}
	require.NoError(t, g.Create(title).Error)

	titles, err := repo.ListTitles(ctx)
	require.NoError(t, err)
	require.Len(t, titles, 1)
	assert.Equal(t, title.ID, titles[0].ID)

	foundTitle, err := repo.FindTitle(ctx, title.ID)
	require.NoError(t, err)
	assert.Equal(t, "称号", foundTitle.Name)

	_, err = repo.FindTitle(ctx, title.ID+1000)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestIdentityJSONFields(t *testing.T) {
	identity := gamification.Identity{
		Badges: []gamification.BadgeSummary{
			{ID: 1, Name: "早鸟", ImageURL: "/api/badges/1/image"},
		},
		Title: &gamification.TitleSummary{ID: 3, Name: "卷王", Color: "#7C3AED"},
	}
	raw, err := json.Marshal(identity)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"equippedBadges": [{"id": 1, "name": "早鸟", "imageUrl": "/api/badges/1/image"}],
		"equippedTitle": {"id": 3, "name": "卷王", "color": "#7C3AED"}
	}`, string(raw))
}

func TestServiceOptionsMaxEquippedBadges(t *testing.T) {
	assert.Equal(t, 3, gamification.NewService(nil).MaxEquippedBadges(), "NewService 默认上限应为 3")

	for _, tc := range []struct {
		name string
		in   int
		want int
	}{
		{name: "zero falls back", in: 0, want: 3},
		{name: "negative falls back", in: -2, want: 3},
		{name: "minimum", in: 1, want: 1},
		{name: "custom", in: 5, want: 5},
		{name: "maximum", in: 10, want: 10},
		{name: "above maximum clamps", in: 11, want: 10},
		{name: "far above maximum clamps", in: 99, want: 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := gamification.NewServiceWithOptions(nil, gamification.ServiceOptions{MaxEquippedBadges: tc.in})
			assert.Equal(t, tc.want, svc.MaxEquippedBadges())
		})
	}
}
