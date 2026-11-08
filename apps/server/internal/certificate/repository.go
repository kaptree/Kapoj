package certificate

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// ErrTypeInUse 表示证书类型仍被证书引用，不可删除。
var ErrTypeInUse = errors.New("certificate: type is in use")

// Repository 提供证书模块的数据访问。
type Repository interface {
	ListTypes(ctx context.Context, includeInactive bool) ([]CertificateType, error)
	FindType(ctx context.Context, id int64) (*CertificateType, error)
	CreateType(ctx context.Context, t *CertificateType) error
	UpdateType(ctx context.Context, t *CertificateType) error
	DeleteType(ctx context.Context, id int64) error
}

// GormRepository 是 Repository 的 GORM 实现。
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository 创建基于 GORM 的证书仓储。
func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

// ListTypes 返回按 sort_order、id 升序排列的类型；includeInactive 为 false 时
// 仅返回启用中的类型。
func (r *GormRepository) ListTypes(ctx context.Context, includeInactive bool) ([]CertificateType, error) {
	q := r.db.WithContext(ctx).Model(&CertificateType{}).Order("sort_order, id")
	if !includeInactive {
		q = q.Where("is_active = ?", true)
	}
	var types []CertificateType
	if err := q.Find(&types).Error; err != nil {
		return nil, err
	}
	return types, nil
}

// FindType 返回指定类型；不存在时返回 gorm.ErrRecordNotFound。
func (r *GormRepository) FindType(ctx context.Context, id int64) (*CertificateType, error) {
	var t CertificateType
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateType 写入新类型。
func (r *GormRepository) CreateType(ctx context.Context, t *CertificateType) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// UpdateType 保存类型变更。
func (r *GormRepository) UpdateType(ctx context.Context, t *CertificateType) error {
	return r.db.WithContext(ctx).Save(t).Error
}

// DeleteType 删除未被任何证书引用的类型。类型不存在返回 httpx.ErrNotFound，
// 被引用返回 ErrTypeInUse；两者通过先查存在性再执行带 NOT EXISTS 的 DELETE 区分，
// 避免并发引用下误删。
func (r *GormRepository) DeleteType(ctx context.Context, id int64) error {
	if _, err := r.FindType(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		return err
	}
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM certificate_types
		 WHERE id = ? AND NOT EXISTS (
		     SELECT 1 FROM certificates WHERE certificate_type_id = ?
		 )`,
		id, id,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTypeInUse
	}
	return nil
}
