package certificate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type env struct {
	svc *certificate.Service
	db  *gorm.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := testutil.PG(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	return &env{
		svc: certificate.NewService(certificate.NewGormRepository(g), store),
		db:  g,
	}
}

func requireAppErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "应为 AppError，实际为 %T: %v", err, err)
	assert.Equal(t, code, appErr.Code, appErr.Message)
}

func boolPtr(b bool) *bool { return &b }

func TestSeedTypesOnlyWhenEmpty(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()

	require.NoError(t, certificate.SeedTypes(ctx, g))

	var names []string
	require.NoError(t, g.Model(&certificate.CertificateType{}).Order("sort_order").Pluck("name", &names).Error)
	require.Equal(t, []string{"蓝桥杯", "ACM-ICPC", "CCCC 天梯赛", "CCPC", "NOI", "NOIP", "USACO"}, names)

	var count int64
	require.NoError(t, g.Model(&certificate.CertificateType{}).Count(&count).Error)
	assert.Equal(t, int64(7), count)

	// 再次 seed 不重复。
	require.NoError(t, certificate.SeedTypes(ctx, g))
	require.NoError(t, g.Model(&certificate.CertificateType{}).Count(&count).Error)
	assert.Equal(t, int64(7), count)

	// 已有自定义类型时 seed 不覆盖管理员修改：删除一条默认类型、新增自定义类型后
	// 再次 seed 既不恢复默认也不重复插入。
	custom := &certificate.CertificateType{Name: "自定义竞赛", IsActive: true, SortOrder: 8}
	require.NoError(t, g.Create(custom).Error)
	require.NoError(t, g.Where("name = ?", "USACO").Delete(&certificate.CertificateType{}).Error)
	require.NoError(t, certificate.SeedTypes(ctx, g))

	require.NoError(t, g.Model(&certificate.CertificateType{}).Count(&count).Error)
	assert.Equal(t, int64(7), count)
	require.NoError(t, g.Model(&certificate.CertificateType{}).Order("sort_order").Pluck("name", &names).Error)
	assert.NotContains(t, names, "USACO")
	assert.Contains(t, names, "自定义竞赛")
}

func TestListActiveAndAllTypes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))

	// 停用一个类型。
	require.NoError(t, e.db.Model(&certificate.CertificateType{}).
		Where("name = ?", "NOIP").Update("is_active", false).Error)

	active, err := e.svc.ListActiveTypes(ctx)
	require.NoError(t, err)
	require.Len(t, active, 6)
	for _, v := range active {
		assert.True(t, v.IsActive)
	}
	// 公开列表按 sort_order 升序。
	assert.Equal(t, "蓝桥杯", active[0].Name)
	assert.Equal(t, "USACO", active[len(active)-1].Name)

	all, err := e.svc.ListAllTypes(ctx)
	require.NoError(t, err)
	require.Len(t, all, 7)
	foundInactive := false
	for _, v := range all {
		if !v.IsActive {
			foundInactive = true
			assert.Equal(t, "NOIP", v.Name)
		}
	}
	assert.True(t, foundInactive, "includeInactive 应返回停用类型")
}

func TestCreateUpdateDeleteType(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// 合法创建；isActive 缺省 true，字符串两侧空白被去除。
	created, err := e.svc.CreateType(ctx, certificate.TypeInput{
		Name: " 蓝桥杯 ", Description: " 算法竞赛 ", SortOrder: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "蓝桥杯", created.Name)
	assert.Equal(t, "算法竞赛", created.Description)
	assert.True(t, created.IsActive)
	assert.Equal(t, 1, created.SortOrder)
	assert.Positive(t, created.ID)

	// 重复名称 40001。
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "蓝桥杯", SortOrder: 2})
	requireAppErrorCode(t, err, 40001)

	// 字段校验：空名称、超长名称、超长描述、排序越界。
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "  ", SortOrder: 1})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: strings.Repeat("赛", 51), SortOrder: 1})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{
		Name: "X", Description: strings.Repeat("长", 201), SortOrder: 1,
	})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "X", SortOrder: -1})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "X", SortOrder: 10001})
	requireAppErrorCode(t, err, 40001)

	// 更新：isActive 缺省保留原值。
	updated, err := e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{
		Name: "蓝桥杯（改）", Description: "更新描述", SortOrder: 9,
	})
	require.NoError(t, err)
	assert.Equal(t, "蓝桥杯（改）", updated.Name)
	assert.True(t, updated.IsActive, "isActive 缺省应保留原值")

	// 更新：显式停用。
	updated, err = e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{
		Name: "蓝桥杯（改）", SortOrder: 9, IsActive: boolPtr(false),
	})
	require.NoError(t, err)
	assert.False(t, updated.IsActive)

	// 更新为已存在的名称 40001。
	other, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "另一个", SortOrder: 3})
	require.NoError(t, err)
	_, err = e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{Name: "另一个", SortOrder: 9})
	requireAppErrorCode(t, err, 40001)
	assert.Positive(t, other.ID)

	// 更新不存在的类型 404。
	_, err = e.svc.UpdateType(ctx, 999999, certificate.TypeInput{Name: "X", SortOrder: 1})
	requireAppErrorCode(t, err, 40400)

	// 删除：无引用可删，删后读取 404。
	require.NoError(t, e.svc.DeleteType(ctx, created.ID))
	_, err = e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{Name: "X", SortOrder: 1})
	requireAppErrorCode(t, err, 40400)
}

func TestDeleteTypeInUse(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	typ, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "NOI", SortOrder: 5})
	require.NoError(t, err)

	// 插入引用该类型的证书。
	require.NoError(t, e.db.Create(&certificate.Certificate{
		UserID:              1,
		CertificateTypeID:   typ.ID,
		CertificateTypeName: "NOI",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusPending,
		SubmittedAt:         time.Now(),
	}).Error)

	// 被引用时删除返回 40001 且带中文提示。
	err = e.svc.DeleteType(ctx, typ.ID)
	requireAppErrorCode(t, err, 40001)
	assert.Contains(t, err.Error(), "已有证书使用该类型，请先停用")

	// 删除不存在的类型 404。
	requireAppErrorCode(t, e.svc.DeleteType(ctx, 999999), 40400)

	// 无引用的类型可删除。
	free, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "USACO", SortOrder: 7})
	require.NoError(t, err)
	require.NoError(t, e.svc.DeleteType(ctx, free.ID))
}

func TestTypeRenameKeepsCertificateSnapshot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	typ, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "蓝桥杯", SortOrder: 1})
	require.NoError(t, err)

	cert := &certificate.Certificate{
		UserID:              1,
		CertificateTypeID:   typ.ID,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusPending,
		SubmittedAt:         time.Now(),
	}
	require.NoError(t, e.db.Create(cert).Error)

	// 类型改名后，历史证书仍使用提交时快照的名称。
	_, err = e.svc.UpdateType(ctx, typ.ID, certificate.TypeInput{Name: "蓝桥杯（改名）", SortOrder: 1})
	require.NoError(t, err)

	var got certificate.Certificate
	require.NoError(t, e.db.First(&got, cert.ID).Error)
	assert.Equal(t, "蓝桥杯", got.CertificateTypeName)
}
