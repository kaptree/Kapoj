package certificate

import (
	"context"
	"errors"
	"time"

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

	CreateCertificate(ctx context.Context, c *Certificate) error
	FindCertificate(ctx context.Context, id int64) (*Certificate, error)
	UpdateCertificateImage(ctx context.Context, id int64, key, contentType string) error
	DeleteCertificate(ctx context.Context, id int64) error
	ListMine(ctx context.Context, userID int64, offset, limit int) ([]Certificate, int64, error)
	ListAdmin(ctx context.Context, filter AdminFilter, offset, limit int) ([]adminCertificateRow, int64, error)
	Review(ctx context.Context, id int64, status Status, reason string, verifyCode *string, reviewedBy int64, now time.Time) (bool, error)
	FindByVerifyCode(ctx context.Context, code string) (*Certificate, string, error)
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

// CreateCertificate 写入证书申请。
func (r *GormRepository) CreateCertificate(ctx context.Context, c *Certificate) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// FindCertificate 返回指定证书；不存在时返回 gorm.ErrRecordNotFound。
func (r *GormRepository) FindCertificate(ctx context.Context, id int64) (*Certificate, error) {
	var c Certificate
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCertificateImage 回填证书图片的 storage key 与内容类型（提交时先插入
// pending 行、再上传、再回填）。
func (r *GormRepository) UpdateCertificateImage(ctx context.Context, id int64, key, contentType string) error {
	return r.db.WithContext(ctx).Model(&Certificate{}).Where("id = ?", id).
		Updates(map[string]any{
			"image_key":          key,
			"image_content_type": contentType,
		}).Error
}

// DeleteCertificate 删除证书；用于提交失败时的清理。
func (r *GormRepository) DeleteCertificate(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Certificate{}, id).Error
}

// ListMine 返回某用户的证书分页，按提交时间倒序，并返回总数。
func (r *GormRepository) ListMine(ctx context.Context, userID int64, offset, limit int) ([]Certificate, int64, error) {
	q := r.db.WithContext(ctx).Model(&Certificate{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var certs []Certificate
	if err := q.Order("submitted_at DESC, id DESC").Offset(offset).Limit(limit).Find(&certs).Error; err != nil {
		return nil, 0, err
	}
	return certs, total, nil
}

// adminCertificateRow 是管理员列表 join users 的查询结果：证书 + 用户摘要字段。
type adminCertificateRow struct {
	Certificate
	Username string `gorm:"column:username"`
	Nickname string `gorm:"column:nickname"`
}

// ListAdmin 返回管理员审核列表，join users 取用户名/昵称。filter.Status 为空时
// 不按状态筛选；按 submitted_at 倒序、id 倒序排列，并返回总数。
func (r *GormRepository) ListAdmin(ctx context.Context, filter AdminFilter, offset, limit int) ([]adminCertificateRow, int64, error) {
	base := r.db.WithContext(ctx).Table("certificates AS c").
		Joins("JOIN users AS u ON u.id = c.user_id")
	if filter.Status != "" {
		base = base.Where("c.status = ?", filter.Status)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []adminCertificateRow
	if err := base.Select("c.*, u.username, u.nickname").
		Order("c.submitted_at DESC, c.id DESC").
		Offset(offset).Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Review 原子地把 pending 证书转换为 approved/rejected：单条 UPDATE 带
// status='pending' 守卫，避免把已审核的证书再次覆盖。返回是否真正更新了行
// （false 表示证书已不存在或已不是 pending）。verifyCode 传 nil 时写入 NULL。
func (r *GormRepository) Review(ctx context.Context, id int64, status Status, reason string, verifyCode *string, reviewedBy int64, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE certificates
		    SET status = ?, reject_reason = ?, verify_code = ?, reviewed_at = ?, reviewed_by = ?, updated_at = ?
		  WHERE id = ? AND status = ?`,
		status, reason, verifyCode, now, reviewedBy, now, id, StatusPending,
	)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// verifyRow 是证书 join 用户昵称的查询结果。
type verifyRow struct {
	Certificate
	Nickname string `gorm:"column:nickname"`
}

// FindByVerifyCode 按验证码精确匹配已通过的证书，并 join users 取持有者昵称。
// 未命中返回 gorm.ErrRecordNotFound。调用方需先规范化验证码（大写、去空格/连字符）。
func (r *GormRepository) FindByVerifyCode(ctx context.Context, code string) (*Certificate, string, error) {
	var row verifyRow
	err := r.db.WithContext(ctx).Table("certificates AS c").
		Select("c.*, u.nickname").
		Joins("JOIN users AS u ON u.id = c.user_id").
		Where("c.status = ? AND c.verify_code = ?", StatusApproved, code).
		Scan(&row).Error
	if err != nil {
		return nil, "", err
	}
	if row.ID == 0 {
		return nil, "", gorm.ErrRecordNotFound
	}
	return &row.Certificate, row.Nickname, nil
}
