package learning

import (
	"context"
	"errors"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Service 实现学习路线的权限、软删除与恢复校验。
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// IsProblemInActivePath reports whether the problem is referenced by any node
// of an active learning path. Used by the problem service's LearningGuard.
func (s *Service) IsProblemInActivePath(ctx context.Context, problemID int64) (bool, error) {
	return s.repo.IsProblemInActivePath(ctx, problemID)
}

// IsProblemInPaidPath reports whether an active paid path references the problem.
func (s *Service) IsProblemInPaidPath(ctx context.Context, problemID int64) (bool, error) {
	return s.repo.IsProblemInPaidPath(ctx, problemID)
}

// CanAccessPaidProblem reports whether the user bought or authored a paid path
// that references the problem.
func (s *Service) CanAccessPaidProblem(ctx context.Context, userID, problemID int64) (bool, error) {
	return s.repo.CanAccessPaidProblem(ctx, userID, problemID)
}

// CreatePath 创建学习路线。普通用户创建自定义路线，默认私有、活跃，且
// 不能设置官方标记/上下架状态；拥有 official.manage 能力的管理员可以创建
// 官方路线并显式设置各状态。
func (s *Service) CreatePath(ctx context.Context, v Viewer, in PathInput) (*PathItem, error) {
	if v.UserID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	if !v.CanManageOfficial && in.IsOfficial != nil && *in.IsOfficial {
		return nil, httpx.ErrForbidden
	}

	sortOrder := in.SortOrder
	if !v.CanManageOfficial {
		sortOrder, err = s.repo.NextSortOrder(ctx)
		if err != nil {
			return nil, err
		}
	}
	p := &LearningPath{
		Title:         in.Title,
		DescriptionMD: in.DescriptionMD,
		CoverURL:      in.CoverURL,
		SortOrder:     sortOrder,
		PriceCoins:    in.PriceCoins,
		CreatedBy:     v.UserID,
		IsPublic:      false,
		IsActive:      true,
	}
	if in.IsPublic != nil {
		p.IsPublic = *in.IsPublic
	}
	if v.CanManageOfficial {
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
	return s.item(ctx, v, p.ID)
}

// UpdatePath 更新路线。只有作者或拥有 official.manage 能力的管理员可以编辑；
// inactive 路线只有管理员可以访问/恢复。非管理员不能改变官方标记与上下架状态。
// CanViewAll 不参与写授权。
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
	if !p.IsActive && !v.CanManageOfficial {
		return nil, httpx.ErrNotFound
	}
	if !v.CanManageOfficial && p.CreatedBy != v.UserID {
		return nil, httpx.ErrNotFound
	}
	if p.IsOfficial && !v.CanManageOfficial {
		return nil, httpx.ErrForbidden
	}
	if !v.CanManageOfficial {
		if in.IsOfficial != nil && *in.IsOfficial != p.IsOfficial {
			return nil, httpx.ErrForbidden
		}
		if in.IsActive != nil && *in.IsActive != p.IsActive {
			return nil, httpx.ErrForbidden
		}
	}
	// 官方标记仅用于创建时区分官方/自定义路线，创建后不可修改；管理员
	// 也不能把自定义路线提升为官方，或取消已有官方标记。
	if in.IsOfficial != nil && *in.IsOfficial != p.IsOfficial {
		return nil, httpx.ErrValidation("不能修改路线官方标记")
	}

	// 公开或恢复上架前必须确认真实数据库中的节点题目仍然存在且公开。
	needValidation := in.IsPublic != nil && *in.IsPublic
	if v.CanManageOfficial && in.IsOfficial != nil && *in.IsOfficial {
		needValidation = true
	}
	if v.CanManageOfficial && in.IsActive != nil && *in.IsActive && !p.IsActive {
		needValidation = true
	}

	p.Title = in.Title
	p.DescriptionMD = in.DescriptionMD
	p.CoverURL = in.CoverURL
	if v.CanManageOfficial {
		p.SortOrder = in.SortOrder
	}
	p.PriceCoins = in.PriceCoins
	if in.IsPublic != nil {
		p.IsPublic = *in.IsPublic
	}
	if v.CanManageOfficial {
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

	if p.PriceCoins > 0 {
		if err := s.validatePathPaidProblems(ctx, p.ID); err != nil {
			return nil, err
		}
	} else if needValidation {
		if err := s.validatePathProblemsPublic(ctx, p.ID); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, mapServiceError(err)
	}
	return s.item(ctx, v, p.ID)
}

// DeletePath 软删除路线（is_active=false）。只有作者或拥有 official.manage 能力
// 的管理员可以删除；删除后作者不能再查看/恢复，只有管理员可通过 includeInactive
// 查看并恢复。CanViewAll 不参与写授权。
func (s *Service) DeletePath(ctx context.Context, v Viewer, id int64) error {
	if v.UserID <= 0 {
		return httpx.ErrUnauthorized
	}
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapServiceError(err)
	}
	if !p.IsActive && !v.CanManageOfficial {
		return httpx.ErrNotFound
	}
	if !v.CanManageOfficial && p.CreatedBy != v.UserID {
		return httpx.ErrNotFound
	}
	if p.IsOfficial && !v.CanManageOfficial {
		return httpx.ErrForbidden
	}
	if err := s.repo.SetActive(ctx, id, false); err != nil {
		return mapServiceError(err)
	}
	return nil
}

// ListPaths 返回查看者可见的全部路线，保留给旧调用方。
func (s *Service) ListPaths(ctx context.Context, v Viewer, includeInactive, mine bool) ([]PathItem, error) {
	page, err := s.SearchPaths(ctx, v, ListQuery{
		IncludeInactive: includeInactive, Mine: mine, Page: 1, PageSize: 100,
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// SearchPaths 返回过滤后的分页路线。scope 仅接受 all/official/custom/mine；
// 普通用户始终不能通过 includeInactive 看到下架路线。
func (s *Service) SearchPaths(ctx context.Context, v Viewer, q ListQuery) (httpx.Page[PathItem], error) {
	scope, err := normalizeScope(q.Scope)
	if err != nil {
		return httpx.Page[PathItem]{}, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	// 登录用户先重算进度，保证 myCompletedCount 首次加载即为最新值。
	if v.UserID > 0 {
		if err := s.EvaluateUser(ctx, v.UserID); err != nil {
			return httpx.Page[PathItem]{}, err
		}
	}
	items, total, err := s.repo.List(ctx, ListFilter{
		ViewerID:        v.UserID,
		IncludeInactive: q.IncludeInactive && (v.CanViewAll || v.CanManageOfficial),
		Mine:            q.Mine,
		Scope:           scope,
		Keyword:         q.Keyword,
		CreatedFrom:     q.CreatedFrom,
		CreatedTo:       q.CreatedTo,
		Offset:          (q.Page - 1) * q.PageSize,
		Limit:           q.PageSize,
	})
	if err != nil {
		return httpx.Page[PathItem]{}, err
	}
	for i := range items {
		s.decoratePath(&items[i], v)
	}
	return httpx.Page[PathItem]{Total: total, Items: items}, nil
}

func normalizeScope(scope string) (string, error) {
	switch scope {
	case "", "all":
		return "all", nil
	case "official", "custom", "mine":
		return scope, nil
	default:
		return "", invalid("scope 必须是 all、official、custom 或 mine")
	}
}

func (s *Service) decoratePath(item *PathItem, v Viewer) {
	item.CanEdit = v.CanManageOfficial || (v.UserID > 0 && item.Author.ID == v.UserID)
	item.CanEditSort = v.CanManageOfficial
}

// FindPath 返回路线元信息。active 官方/公开路线所有人可见；active 私有
// 路线仅作者、admin.read 或 official.manage 可见；inactive 路线仅 admin.read
// 或 official.manage 可见。
func (s *Service) FindPath(ctx context.Context, v Viewer, id int64) (*PathItem, error) {
	if _, err := s.visiblePath(ctx, v, id); err != nil {
		return nil, err
	}
	return s.item(ctx, v, id)
}

// visiblePath loads a path and applies the metadata visibility rules shared by
// FindPath, Tree, purchase and recommendation.
func (s *Service) visiblePath(ctx context.Context, v Viewer, id int64) (*LearningPath, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if !p.IsActive && !v.CanViewAll && !v.CanManageOfficial {
		return nil, httpx.ErrNotFound
	}
	if p.IsActive && !p.IsOfficial && !p.IsPublic &&
		p.CreatedBy != v.UserID && !v.CanViewAll && !v.CanManageOfficial {
		return nil, httpx.ErrNotFound
	}
	return p, nil
}

func (s *Service) item(ctx context.Context, v Viewer, id int64) (*PathItem, error) {
	item, err := s.repo.Item(ctx, v.UserID, id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	s.decoratePath(item, v)
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

// validatePathPaidProblems enforces that a paid path never references a public
// problem. The conflict list names the offending problems so the editor can fix
// them without guessing.
func (s *Service) validatePathPaidProblems(ctx context.Context, pathID int64) error {
	nodes, err := s.repo.ListNodes(ctx, pathID)
	if err != nil {
		return err
	}
	ids := uniqueProblemIDs(nodes)
	if len(ids) == 0 {
		return nil
	}
	states, err := s.repo.ProblemStates(ctx, ids)
	if err != nil {
		return err
	}
	var public []string
	var missing []int64
	for _, id := range ids {
		state, ok := states[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		if state.IsPublic {
			public = append(public, state.Title)
		}
	}
	if len(missing) > 0 {
		return httpx.ErrValidation("付费路线引用了不存在的题目，请先修复节点")
	}
	if len(public) > 0 {
		return httpx.ErrValidation("付费路线只能引用未公开题目：" + strings.Join(public, "、"))
	}
	return nil
}

func mapServiceError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return httpx.ErrNotFound
	}
	return err
}
