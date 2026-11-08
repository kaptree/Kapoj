package certificate

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	maxTypeNameLength        = 50
	maxTypeDescriptionLength = 200
	maxTypeSortOrder         = 10000
)

// Service 是证书领域服务。store 在类型接口阶段尚未使用，供后续图片上传复用。
type Service struct {
	repo  Repository
	store storage.Storage
}

// NewService 创建证书服务。
func NewService(repo Repository, store storage.Storage) *Service {
	return &Service{repo: repo, store: store}
}

// ListActiveTypes 返回启用中的类型，按 sort_order 升序。
func (s *Service) ListActiveTypes(ctx context.Context) ([]TypeView, error) {
	return s.listTypes(ctx, false)
}

// ListAllTypes 返回全部类型（含停用），按 sort_order 升序。
func (s *Service) ListAllTypes(ctx context.Context) ([]TypeView, error) {
	return s.listTypes(ctx, true)
}

func (s *Service) listTypes(ctx context.Context, includeInactive bool) ([]TypeView, error) {
	types, err := s.repo.ListTypes(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	items := make([]TypeView, 0, len(types))
	for _, t := range types {
		items = append(items, toTypeView(t))
	}
	return items, nil
}

// CreateType 创建证书类型；isActive 缺省为启用。
func (s *Service) CreateType(ctx context.Context, in TypeInput) (*TypeView, error) {
	in, err := normalizeTypeInput(in)
	if err != nil {
		return nil, err
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	t := &CertificateType{
		Name:        in.Name,
		Description: in.Description,
		IsActive:    active,
		SortOrder:   in.SortOrder,
	}
	if err := s.repo.CreateType(ctx, t); err != nil {
		if isTypeNameConflict(err) {
			return nil, httpx.ErrValidation("证书类型名称已存在")
		}
		return nil, err
	}
	v := toTypeView(*t)
	return &v, nil
}

// UpdateType 更新证书类型；isActive 缺省保留原值。
func (s *Service) UpdateType(ctx context.Context, id int64, in TypeInput) (*TypeView, error) {
	existing, err := s.findType(ctx, id)
	if err != nil {
		return nil, err
	}
	in, err = normalizeTypeInput(in)
	if err != nil {
		return nil, err
	}
	existing.Name = in.Name
	existing.Description = in.Description
	existing.SortOrder = in.SortOrder
	if in.IsActive != nil {
		existing.IsActive = *in.IsActive
	}
	if err := s.repo.UpdateType(ctx, existing); err != nil {
		if isTypeNameConflict(err) {
			return nil, httpx.ErrValidation("证书类型名称已存在")
		}
		return nil, err
	}
	v := toTypeView(*existing)
	return &v, nil
}

// DeleteType 删除未被引用的类型；被证书引用返回 40001，不存在返回 404。
func (s *Service) DeleteType(ctx context.Context, id int64) error {
	if id <= 0 {
		return httpx.ErrNotFound
	}
	if err := s.repo.DeleteType(ctx, id); err != nil {
		if errors.Is(err, ErrTypeInUse) {
			return httpx.ErrValidation("已有证书使用该类型，请先停用")
		}
		return err
	}
	return nil
}

func (s *Service) findType(ctx context.Context, id int64) (*CertificateType, error) {
	if id <= 0 {
		return nil, httpx.ErrNotFound
	}
	t, err := s.repo.FindType(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func normalizeTypeInput(in TypeInput) (TypeInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)

	if n := utf8.RuneCountInString(in.Name); n < 1 {
		return in, httpx.ErrValidation("证书类型名称不能为空")
	} else if n > maxTypeNameLength {
		return in, httpx.ErrValidation("证书类型名称不能超过 50 个字符")
	}
	if n := utf8.RuneCountInString(in.Description); n > maxTypeDescriptionLength {
		return in, httpx.ErrValidation("证书类型描述不能超过 200 个字符")
	}
	if in.SortOrder < 0 || in.SortOrder > maxTypeSortOrder {
		return in, httpx.ErrValidation("排序值需在 0–10000 之间")
	}
	return in, nil
}

func isTypeNameConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_certificate_types_name"
}

// defaultTypeSeeds 是首次启动时写入的内置证书类型，sort_order 为 1..7。
var defaultTypeSeeds = []CertificateType{
	{Name: "蓝桥杯", SortOrder: 1},
	{Name: "ACM-ICPC", SortOrder: 2},
	{Name: "CCCC 天梯赛", SortOrder: 3},
	{Name: "CCPC", SortOrder: 4},
	{Name: "NOI", SortOrder: 5},
	{Name: "NOIP", SortOrder: 6},
	{Name: "USACO", SortOrder: 7},
}

// SeedTypes 仅在 certificate_types 表为空时写入默认类型；已有数据（包括管理员
// 自定义类型）时不覆盖或补齐，避免重启恢复管理员删除的类型。
func SeedTypes(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&CertificateType{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	types := make([]CertificateType, len(defaultTypeSeeds))
	for i, seed := range defaultTypeSeeds {
		types[i] = CertificateType{
			Name:      seed.Name,
			IsActive:  true,
			SortOrder: seed.SortOrder,
		}
	}
	return db.WithContext(ctx).Create(&types).Error
}
