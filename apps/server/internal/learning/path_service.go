package learning

import (
	"context"
	"errors"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Service 实现学习路线的权限、软删除与恢复校验。
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// CreatePath 创建学习路线。普通用户创建自定义路线，默认私有、活跃，且
// 不能设置官方标记/上下架状态；管理员可以创建官方路线并显式设置各状态。
func (s *Service) CreatePath(ctx context.Context, v Viewer, in PathInput) (*PathItem, error) {
	if v.UserID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	if !v.CanManage && in.IsOfficial != nil && *in.IsOfficial {
		return nil, httpx.ErrForbidden
	}

	p := &LearningPath{
		Title:         in.Title,
		DescriptionMD: in.DescriptionMD,
		CoverURL:      in.CoverURL,
		SortOrder:     in.SortOrder,
		CreatedBy:     v.UserID,
		IsPublic:      false,
		IsActive:      true,
	}
	if in.IsPublic != nil {
		p.IsPublic = *in.IsPublic
	}
	if v.CanManage {
		if in.IsOfficial != nil {
			p.IsOfficial = *in.IsOfficial
		}
		if in.IsActive != nil {
			p.IsActive = *in.IsActive
		}
	}
	if p.IsOfficial {
		p.IsPublic = true
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return s.item(ctx, v.UserID, p.ID)
}

// UpdatePath 更新路线。只有作者或管理员可以编辑；inactive 路线只有管理员
// 可以访问/恢复。非管理员不能改变官方标记与上下架状态。
func (s *Service) UpdatePath(ctx context.Context, v Viewer, id int64, in PathInput) (*PathItem, error) {
	if v.UserID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if !p.IsActive && !v.CanManage {
		return nil, httpx.ErrNotFound
	}
	if !v.CanManage && p.CreatedBy != v.UserID {
		return nil, httpx.ErrNotFound
	}
	if p.IsOfficial && !v.CanManage {
		return nil, httpx.ErrForbidden
	}
	if !v.CanManage {
		if in.IsOfficial != nil && *in.IsOfficial != p.IsOfficial {
			return nil, httpx.ErrForbidden
		}
		if in.IsActive != nil && *in.IsActive != p.IsActive {
			return nil, httpx.ErrForbidden
		}
	}

	// 公开或恢复上架前必须确认真实数据库中的节点题目仍然存在且公开。
	needValidation := in.IsPublic != nil && *in.IsPublic
	if v.CanManage && in.IsOfficial != nil && *in.IsOfficial {
		needValidation = true
	}
	if v.CanManage && in.IsActive != nil && *in.IsActive && !p.IsActive {
		needValidation = true
	}

	p.Title = in.Title
	p.DescriptionMD = in.DescriptionMD
	p.CoverURL = in.CoverURL
	p.SortOrder = in.SortOrder
	if in.IsPublic != nil {
		p.IsPublic = *in.IsPublic
	}
	if v.CanManage {
		if in.IsOfficial != nil {
			p.IsOfficial = *in.IsOfficial
		}
		if in.IsActive != nil {
			p.IsActive = *in.IsActive
		}
	}
	if p.IsOfficial {
		p.IsPublic = true
	}

	if needValidation {
		if err := s.validatePathProblemsPublic(ctx, p.ID); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, mapServiceError(err)
	}
	return s.item(ctx, v.UserID, p.ID)
}

// DeletePath 软删除路线（is_active=false）。只有作者或管理员可以删除；
// 删除后作者不能再查看/恢复，只有管理员可通过 includeInactive 查看并恢复。
func (s *Service) DeletePath(ctx context.Context, v Viewer, id int64) error {
	if v.UserID <= 0 {
		return httpx.ErrUnauthorized
	}
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapServiceError(err)
	}
	if !p.IsActive && !v.CanManage {
		return httpx.ErrNotFound
	}
	if !v.CanManage && p.CreatedBy != v.UserID {
		return httpx.ErrNotFound
	}
	if p.IsOfficial && !v.CanManage {
		return httpx.ErrForbidden
	}
	if err := s.repo.SetActive(ctx, id, false); err != nil {
		return mapServiceError(err)
	}
	return nil
}

// ListPaths 返回查看者可见的路线。includeInactive 仅对管理员生效；mine
// 只返回查看者创建的路线。
func (s *Service) ListPaths(ctx context.Context, v Viewer, includeInactive, mine bool) ([]PathItem, error) {
	items, err := s.repo.List(ctx, ListFilter{
		ViewerID:        v.UserID,
		IncludeInactive: includeInactive && v.CanManage,
		Mine:            mine,
	})
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []PathItem{}
	}
	return items, nil
}

// FindPath 返回路线元信息。active 官方/公开路线所有人可见；active 私有
// 路线仅作者/管理员；inactive 路线仅管理员。
func (s *Service) FindPath(ctx context.Context, v Viewer, id int64) (*PathItem, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if !p.IsActive && !v.CanManage {
		return nil, httpx.ErrNotFound
	}
	if p.IsActive && !p.IsOfficial && !p.IsPublic && !v.CanManage && p.CreatedBy != v.UserID {
		return nil, httpx.ErrNotFound
	}
	return s.item(ctx, v.UserID, id)
}

func (s *Service) item(ctx context.Context, viewerID, id int64) (*PathItem, error) {
	item, err := s.repo.Item(ctx, viewerID, id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return item, nil
}

func (s *Service) validatePathProblemsPublic(ctx context.Context, pathID int64) error {
	count, err := s.repo.CountUnavailableNodeProblems(ctx, pathID)
	if err != nil {
		return err
	}
	if count > 0 {
		return httpx.ErrValidation("题目已下线，请先修复节点")
	}
	return nil
}

func mapServiceError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return httpx.ErrNotFound
	}
	return err
}
