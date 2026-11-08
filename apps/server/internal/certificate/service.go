package certificate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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

	maxLevelLength = 50
	maxImageBytes  = 5 << 20 // 证书图片上限 5 MiB
)

// Service 是证书领域服务。store 用于保存证书图片。
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

// Submit 提交证书认证申请。校验类型启用、level 1–50 字符、图片 ≤5 MiB 且 magic
// bytes 为 PNG/JPEG/WebP；成功后插入 pending 行并上传图片；上传或回填失败时
// 删除行与对象，避免残留。
func (s *Service) Submit(ctx context.Context, userID, typeID int64, level string, image io.Reader) (*CertificateView, error) {
	level = strings.TrimSpace(level)
	if n := utf8.RuneCountInString(level); n < 1 {
		return nil, httpx.ErrValidation("证书等级不能为空")
	} else if n > maxLevelLength {
		return nil, httpx.ErrValidation("证书等级不能超过 50 个字符")
	}

	typ, err := s.findType(ctx, typeID)
	if err != nil {
		return nil, err
	}
	if !typ.IsActive {
		return nil, httpx.ErrValidation("该证书类型已停用")
	}

	data, err := io.ReadAll(io.LimitReader(image, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, httpx.ErrValidation("证书图片不能为空")
	}
	if len(data) > maxImageBytes {
		return nil, httpx.ErrValidation("证书图片不能超过 5 MiB")
	}
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return nil, httpx.ErrValidation("仅支持 PNG/JPEG/WebP 图片")
	}

	cert := &Certificate{
		UserID:              userID,
		CertificateTypeID:   typeID,
		CertificateTypeName: typ.Name,
		Level:               level,
		Status:              StatusPending,
		SubmittedAt:         time.Now(),
	}
	if err := s.repo.CreateCertificate(ctx, cert); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("certificates/%d", cert.ID)
	if _, err := s.store.Put(ctx, key, bytes.NewReader(data), maxImageBytes); err != nil {
		_ = s.repo.DeleteCertificate(ctx, cert.ID)
		_ = s.store.Delete(ctx, key)
		return nil, err
	}
	if err := s.repo.UpdateCertificateImage(ctx, cert.ID, key, contentType); err != nil {
		_ = s.repo.DeleteCertificate(ctx, cert.ID)
		_ = s.store.Delete(ctx, key)
		return nil, err
	}
	v := toCertificateView(*cert)
	return &v, nil
}

// ListMine 返回用户的证书分页，按提交时间倒序。
func (s *Service) ListMine(ctx context.Context, userID int64, page, size int) (*httpx.Page[CertificateView], error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	offset := (page - 1) * size
	certs, total, err := s.repo.ListMine(ctx, userID, offset, size)
	if err != nil {
		return nil, err
	}
	items := make([]CertificateView, 0, len(certs))
	for _, c := range certs {
		items = append(items, toCertificateView(c))
	}
	return &httpx.Page[CertificateView]{Total: total, Items: items}, nil
}

// Image 返回证书图片；仅本人或管理员（canManage）可读，其余一律 404 以不暴露
// 存在性。内容类型取自数据库，对象缺失返回 404。
func (s *Service) Image(ctx context.Context, id, viewerID int64, canManage bool) (*Image, error) {
	cert, err := s.findCertificate(ctx, id)
	if err != nil {
		return nil, err
	}
	if cert.UserID != viewerID && !canManage {
		return nil, httpx.ErrNotFound
	}
	rc, err := s.store.Open(ctx, cert.ImageKey)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return &Image{Data: data, ContentType: cert.ImageContentType}, nil
}

// Verify 公开核验验证码。规范化后仅匹配 approved 证书；未知/未通过统一返回
// {valid:false}，绝不提供存在性 oracle。
func (s *Service) Verify(ctx context.Context, code string) (*PublicCertificateView, error) {
	normalized, ok := normalizeVerifyCode(code)
	if !ok {
		return &PublicCertificateView{Valid: false}, nil
	}
	cert, nickname, err := s.repo.FindByVerifyCode(ctx, normalized)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &PublicCertificateView{Valid: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &PublicCertificateView{
		Valid:      true,
		Type:       cert.CertificateTypeName,
		Level:      cert.Level,
		Nickname:   nickname,
		ApprovedAt: cert.ReviewedAt,
	}, nil
}

func (s *Service) findCertificate(ctx context.Context, id int64) (*Certificate, error) {
	if id <= 0 {
		return nil, httpx.ErrNotFound
	}
	c, err := s.repo.FindCertificate(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// normalizeVerifyCode 规范化验证码：转大写、去空格与连字符、去掉 KAPOJ 前缀，
// 剩余 8 位字符时补回 `KAPOJ-XXXX-XXXX`；格式不符返回 ok=false。
func normalizeVerifyCode(code string) (string, bool) {
	s := strings.ToUpper(code)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimPrefix(s, "KAPOJ")
	if len(s) != 8 {
		return "", false
	}
	return "KAPOJ-" + s[:4] + "-" + s[4:], true
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
