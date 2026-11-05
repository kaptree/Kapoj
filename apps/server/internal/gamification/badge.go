package gamification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	maxBadgeImageBytes  = 2 << 20
	maxBadgePriceCoins  = 1_000_000
	badgeImageKeyPrefix = "badges/"
)

var badgeInternalImagePattern = regexp.MustCompile(`^/api/badges/[1-9][0-9]*/image$`)

// ListBadges 返回徽章商城。游客只看到上架徽章；登录用户额外看到自己已拥有的
// 下线徽章；管理员 includeInactive=true 时看到全部。每项带上当前用户的拥有状态。
func (s *Service) ListBadges(ctx context.Context, v Viewer, includeInactive bool) ([]BadgeItem, error) {
	if includeInactive && !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	badges, err := s.repo.ListBadges(ctx)
	if err != nil {
		return nil, err
	}
	owned := map[int64]UserBadge{}
	if v.UserID > 0 {
		owned, err = s.repo.UserBadgesForUser(ctx, v.UserID)
		if err != nil {
			return nil, err
		}
	}
	items := make([]BadgeItem, 0, len(badges))
	for _, badge := range badges {
		userBadge, isOwned := owned[badge.ID]
		if !includeInactive && !badge.IsActive && !isOwned {
			continue
		}
		var ownership *UserBadge
		if isOwned {
			ownership = &userBadge
		}
		items = append(items, toBadgeItem(badge, ownership))
	}
	return items, nil
}

// ListUserBadges 公开返回用户拥有的全部徽章（包括已下线）及装备状态。
func (s *Service) ListUserBadges(ctx context.Context, userID int64) ([]BadgeItem, error) {
	if userID <= 0 {
		return nil, httpx.ErrNotFound
	}
	exists, err := s.repo.UserExists(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, httpx.ErrNotFound
	}
	badges, err := s.repo.ListBadges(ctx)
	if err != nil {
		return nil, err
	}
	owned, err := s.repo.UserBadgesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]BadgeItem, 0, len(owned))
	for _, badge := range badges {
		userBadge, ok := owned[badge.ID]
		if !ok {
			continue
		}
		items = append(items, toBadgeItem(badge, &userBadge))
	}
	return items, nil
}

// CreateBadge 创建徽章；isActive 缺省为上架。
func (s *Service) CreateBadge(ctx context.Context, v Viewer, in BadgeInput) (*Badge, error) {
	if !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	in, err := normalizeBadgeInput(in, 0)
	if err != nil {
		return nil, err
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	badge := &Badge{
		Name:        in.Name,
		Description: in.Description,
		ImageURL:    in.ImageURL,
		PriceCoins:  in.PriceCoins,
		Category:    in.Category,
		IsLimited:   in.IsLimited,
		Stock:       in.Stock,
		SortOrder:   in.SortOrder,
		IsActive:    active,
	}
	if err := s.repo.CreateBadge(ctx, badge); err != nil {
		return nil, err
	}
	return badge, nil
}

// UpdateBadge 更新徽章；isActive 缺省时保留原值。
func (s *Service) UpdateBadge(ctx context.Context, v Viewer, id int64, in BadgeInput) (*Badge, error) {
	if !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	existing, err := s.findBadge(ctx, id)
	if err != nil {
		return nil, err
	}
	in, err = normalizeBadgeInput(in, id)
	if err != nil {
		return nil, err
	}
	existing.Name = in.Name
	existing.Description = in.Description
	existing.ImageURL = in.ImageURL
	existing.PriceCoins = in.PriceCoins
	existing.Category = in.Category
	existing.IsLimited = in.IsLimited
	existing.Stock = in.Stock
	existing.SortOrder = in.SortOrder
	if in.IsActive != nil {
		existing.IsActive = *in.IsActive
	}
	if err := s.repo.UpdateBadge(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteBadge 软删除徽章，保留已购买记录与公开展示。
func (s *Service) DeleteBadge(ctx context.Context, v Viewer, id int64) error {
	if !v.IsAdmin {
		return httpx.ErrForbidden
	}
	if _, err := s.findBadge(ctx, id); err != nil {
		return err
	}
	return s.repo.DeactivateBadge(ctx, id)
}

// UploadBadgeImage 校验管理员上传的图片并以固定 key badges/<id> 保存，
// 同时把 image_url 指向公开图片接口。
func (s *Service) UploadBadgeImage(ctx context.Context, v Viewer, id int64, r io.Reader) error {
	if !v.IsAdmin {
		return httpx.ErrForbidden
	}
	badge, err := s.findBadge(ctx, id)
	if err != nil {
		return err
	}
	if s.store == nil {
		return httpx.ErrInternal
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBadgeImageBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxBadgeImageBytes {
		return httpx.ErrValidation("图片不能超过 2 MiB")
	}
	contentType := http.DetectContentType(data)
	if !allowedBadgeImageType(contentType) {
		return httpx.ErrValidation("仅支持 PNG、JPEG、WebP 或 GIF 图片")
	}
	key := badgeImageKeyPrefix + strconv.FormatInt(badge.ID, 10)
	if _, err := s.store.Put(ctx, key, bytes.NewReader(data), maxBadgeImageBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return httpx.ErrValidation("图片不能超过 2 MiB")
		}
		return err
	}
	return s.repo.UpdateBadgeImageURL(ctx, badge.ID, fmt.Sprintf("/api/badges/%d/image", badge.ID))
}

// BadgeImage 读取上传的徽章图片；外链图片与缺失文件都返回 404。
func (s *Service) BadgeImage(ctx context.Context, id int64) (*BadgeImage, error) {
	badge, err := s.findBadge(ctx, id)
	if err != nil {
		return nil, err
	}
	if badge.ImageURL != fmt.Sprintf("/api/badges/%d/image", badge.ID) || s.store == nil {
		return nil, httpx.ErrNotFound
	}
	f, err := s.store.Open(ctx, badgeImageKeyPrefix+strconv.FormatInt(badge.ID, 10))
	if errors.Is(err, storage.ErrNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBadgeImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBadgeImageBytes {
		return nil, httpx.ErrNotFound
	}
	contentType := http.DetectContentType(data)
	if !allowedBadgeImageType(contentType) {
		return nil, httpx.ErrNotFound
	}
	return &BadgeImage{Data: data, ContentType: contentType}, nil
}

// PurchaseBadge 购买徽章；库存、余额、流水与拥有记录在同一事务内完成。
func (s *Service) PurchaseBadge(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return httpx.ErrUnauthorized
	}
	return s.repo.PurchaseBadge(ctx, userID, id)
}

// EquipBadge 装备徽章，已装备时幂等成功；并发装备由用户行锁串行化。
func (s *Service) EquipBadge(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return httpx.ErrUnauthorized
	}
	return s.repo.EquipBadge(ctx, userID, id, s.maxEquippedBadges)
}

// UnequipBadge 卸下徽章，未装备时幂等成功。
func (s *Service) UnequipBadge(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return httpx.ErrUnauthorized
	}
	return s.repo.UnequipBadge(ctx, userID, id)
}

func (s *Service) findBadge(ctx context.Context, id int64) (*Badge, error) {
	if id <= 0 {
		return nil, httpx.ErrNotFound
	}
	badge, err := s.repo.FindBadge(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return badge, nil
}

func normalizeBadgeInput(in BadgeInput, id int64) (BadgeInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.ImageURL = strings.TrimSpace(in.ImageURL)
	in.Category = strings.TrimSpace(in.Category)

	if err := validateText("徽章名称", in.Name, 1, 100); err != nil {
		return in, err
	}
	if err := validateText("徽章描述", in.Description, 0, 500); err != nil {
		return in, err
	}
	if err := validateText("徽章分类", in.Category, 1, 20); err != nil {
		return in, err
	}
	if err := validateText("徽章图片地址", in.ImageURL, 0, 512); err != nil {
		return in, err
	}
	if err := validateBadgeImageURL(in.ImageURL, id); err != nil {
		return in, err
	}
	if in.PriceCoins < 0 {
		return in, httpx.ErrValidation("徽章价格不能为负数")
	}
	if in.PriceCoins > maxBadgePriceCoins {
		return in, httpx.ErrValidation("徽章价格不能超过 " + strconv.Itoa(maxBadgePriceCoins))
	}
	if in.IsLimited && in.Stock < 0 {
		return in, httpx.ErrValidation("限量库存不能为负数")
	}
	if !in.IsLimited {
		in.Stock = 0
	}
	return in, nil
}

func validateBadgeImageURL(raw string, id int64) error {
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "/") {
		if id > 0 && raw == fmt.Sprintf("/api/badges/%d/image", id) {
			return nil
		}
		if id == 0 && badgeInternalImagePattern.MatchString(raw) {
			return nil
		}
		return httpx.ErrValidation("徽章图片地址必须是 http(s) 外链或 /api/badges/<id>/image")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return httpx.ErrValidation("徽章图片地址必须是 http(s) 外链或 /api/badges/<id>/image")
	}
	return nil
}

func allowedBadgeImageType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}

func toBadgeItem(badge Badge, userBadge *UserBadge) BadgeItem {
	item := BadgeItem{
		ID:          badge.ID,
		Name:        badge.Name,
		Description: badge.Description,
		ImageURL:    badge.ImageURL,
		Category:    badge.Category,
		PriceCoins:  badge.PriceCoins,
		IsLimited:   badge.IsLimited,
		Stock:       badge.Stock,
		SortOrder:   badge.SortOrder,
		IsActive:    badge.IsActive,
	}
	if userBadge != nil {
		purchasedAt := userBadge.PurchasedAt.UTC()
		item.Owned = true
		item.Equipped = userBadge.IsEquipped
		item.PurchasedAt = &purchasedAt
	}
	return item
}

// BadgeItemFromModel 将存储层徽章转换为 API 表示；用于创建/更新响应，
// 此时没有当前用户的拥有状态。
func BadgeItemFromModel(badge Badge) BadgeItem {
	return toBadgeItem(badge, nil)
}
