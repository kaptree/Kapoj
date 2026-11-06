package learning

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	maxNodeDepth    = 8
	maxNodeProblems = 50
)

// CreateNode 在学习路线下创建节点。只有路线作者或拥有 official.manage 能力
// 的管理员可以写入；官方路线与 inactive 路线只有管理员可以写入。
func (s *Service) CreateNode(ctx context.Context, v Viewer, pathID int64, in NodeInput) (*NodeItem, error) {
	path, err := s.writablePath(ctx, v, pathID)
	if err != nil {
		return nil, err
	}
	in, err = normalizeNodeInput(in)
	if err != nil {
		return nil, err
	}
	nodes, err := s.repo.ListNodes(ctx, path.ID)
	if err != nil {
		return nil, err
	}
	if err := validateNodeHierarchy(nodes, 0, in.ParentID); err != nil {
		return nil, err
	}
	if err := s.validateNodeProblems(ctx, in.ProblemIDs); err != nil {
		return nil, err
	}
	required, err := nodeRequiredCount(in)
	if err != nil {
		return nil, err
	}

	n := &LearningPathNode{
		PathID:        path.ID,
		ParentID:      in.ParentID,
		Title:         in.Title,
		DescriptionMD: in.DescriptionMD,
		ProblemIDs:    dbtypes.List[int64](in.ProblemIDs),
		SortOrder:     in.SortOrder,
		RequiredCount: required,
	}
	if err := s.repo.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	return s.nodeItem(ctx, n)
}

// UpdateNode 更新节点。字段为整体替换；required_count 未提交时默认题目数。
func (s *Service) UpdateNode(ctx context.Context, v Viewer, pathID, nodeID int64, in NodeInput) (*NodeItem, error) {
	path, err := s.writablePath(ctx, v, pathID)
	if err != nil {
		return nil, err
	}
	n, err := s.loadPathNode(ctx, path.ID, nodeID)
	if err != nil {
		return nil, err
	}
	in, err = normalizeNodeInput(in)
	if err != nil {
		return nil, err
	}
	nodes, err := s.repo.ListNodes(ctx, path.ID)
	if err != nil {
		return nil, err
	}
	if err := validateNodeHierarchy(nodes, n.ID, in.ParentID); err != nil {
		return nil, err
	}
	if err := s.validateNodeProblems(ctx, in.ProblemIDs); err != nil {
		return nil, err
	}
	required, err := nodeRequiredCount(in)
	if err != nil {
		return nil, err
	}

	n.ParentID = in.ParentID
	n.Title = in.Title
	n.DescriptionMD = in.DescriptionMD
	n.ProblemIDs = dbtypes.List[int64](in.ProblemIDs)
	n.SortOrder = in.SortOrder
	n.RequiredCount = required
	if err := s.repo.UpdateNode(ctx, n); err != nil {
		return nil, mapServiceError(err)
	}
	return s.nodeItem(ctx, n)
}

// DeleteNode 删除叶子节点并级联删除该节点的用户进度。
func (s *Service) DeleteNode(ctx context.Context, v Viewer, pathID, nodeID int64) error {
	path, err := s.writablePath(ctx, v, pathID)
	if err != nil {
		return err
	}
	n, err := s.loadPathNode(ctx, path.ID, nodeID)
	if err != nil {
		return err
	}
	children, err := s.repo.CountChildren(ctx, n.ID)
	if err != nil {
		return err
	}
	if children > 0 {
		return invalid("存在子节点，不能删除")
	}
	return mapServiceError(s.repo.DeleteNode(ctx, n))
}

// Tree 返回路线的结构树。权限与 FindPath 一致。本阶段仅提供结构状态：
// 根节点 active、其余 locked、solvedCount 为 0，M4-2B 替换为真实进度。
func (s *Service) Tree(ctx context.Context, v Viewer, pathID int64) (*Tree, error) {
	path, err := s.FindPath(ctx, v, pathID)
	if err != nil {
		return nil, err
	}
	nodes, err := s.repo.ListNodes(ctx, path.ID)
	if err != nil {
		return nil, err
	}
	ids := uniqueProblemIDs(nodes)
	summaries, err := s.repo.ProblemSummaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	// 公开树对非作者/非管理员只暴露公开题目的 ID，避免枚举已下线或
	// 已删除题目；作者、admin.read 与 official.manage 保留完整存储 ID 供编辑。
	fullProblemIDs := v.CanManageOfficial || v.CanViewAll || (v.UserID > 0 && path.Author.ID == v.UserID)
	return &Tree{Path: *path, Nodes: buildNodeTree(nodes, summaries, fullProblemIDs)}, nil
}

// writablePath 加载可写路线并校验作者/管理员、官方与上下架权限。
// CanViewAll 不参与写授权，只有作者或 official.manage 可以写入。
func (s *Service) writablePath(ctx context.Context, v Viewer, pathID int64) (*LearningPath, error) {
	if v.UserID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	p, err := s.repo.FindByID(ctx, pathID)
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
	return p, nil
}

func (s *Service) loadPathNode(ctx context.Context, pathID, nodeID int64) (*LearningPathNode, error) {
	n, err := s.repo.FindNode(ctx, nodeID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if n.PathID != pathID {
		return nil, httpx.ErrNotFound
	}
	return n, nil
}

func normalizeNodeInput(in NodeInput) (NodeInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > maxTitleRunes {
		return in, invalid("标题长度需为 1–100 个字符")
	}
	if utf8.RuneCountInString(in.DescriptionMD) > maxDescriptionRunes {
		return in, invalid("描述不能超过 50000 个字符")
	}
	if n := len(in.ProblemIDs); n < 1 || n > maxNodeProblems {
		return in, invalid("题目数量需为 1–50 道")
	}
	seen := make(map[int64]struct{}, len(in.ProblemIDs))
	for _, id := range in.ProblemIDs {
		if _, ok := seen[id]; ok {
			return in, invalid("题目不能重复")
		}
		seen[id] = struct{}{}
	}
	return in, nil
}

func nodeRequiredCount(in NodeInput) (int, error) {
	n := len(in.ProblemIDs)
	if in.RequiredCount == nil {
		return n, nil
	}
	if *in.RequiredCount < 1 || *in.RequiredCount > n {
		return 0, invalid("必做题目数需为 1–%d 道", n)
	}
	return *in.RequiredCount, nil
}

func (s *Service) validateNodeProblems(ctx context.Context, ids []int64) error {
	summaries, err := s.repo.ProblemSummaries(ctx, ids)
	if err != nil {
		return err
	}
	if len(summaries) != len(ids) {
		return invalid("题目不存在或未公开")
	}
	return nil
}

// validateNodeHierarchy 校验父节点属于同一路线、无自环/成环且深度不超过
// 8 层。更新节点时还会把被移动子树的深度计入上限。
func validateNodeHierarchy(nodes []LearningPathNode, nodeID int64, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	byID := make(map[int64]LearningPathNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	parent, ok := byID[*parentID]
	if !ok {
		return invalid("父节点必须属于同一路线")
	}
	if nodeID != 0 && parent.ID == nodeID {
		return invalid("父节点不能是自身")
	}

	depth := 1
	cur := parent
	seen := map[int64]bool{cur.ID: true}
	for cur.ParentID != nil {
		next, ok := byID[*cur.ParentID]
		if !ok {
			return invalid("父节点必须属于同一路线")
		}
		if nodeID != 0 && next.ID == nodeID {
			return invalid("节点层级不能形成环")
		}
		if seen[next.ID] {
			return invalid("节点层级不能形成环")
		}
		seen[next.ID] = true
		depth++
		cur = next
	}
	if depth+1 > maxNodeDepth {
		return invalid("节点层级不能超过 %d 层", maxNodeDepth)
	}
	if nodeID != 0 && depth+nodeSubtreeHeight(byID, nodeID) > maxNodeDepth {
		return invalid("节点层级不能超过 %d 层", maxNodeDepth)
	}
	return nil
}

// nodeSubtreeHeight 返回以 rootID 为根的子树高度（根自身记为 1）。
func nodeSubtreeHeight(byID map[int64]LearningPathNode, rootID int64) int {
	children := make(map[int64][]int64, len(byID))
	for _, n := range byID {
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	type entry struct {
		id    int64
		depth int
	}
	stack := []entry{{rootID, 1}}
	seen := make(map[int64]bool)
	maxDepth := 0
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[e.id] {
			continue
		}
		seen[e.id] = true
		if e.depth > maxDepth {
			maxDepth = e.depth
		}
		for _, childID := range children[e.id] {
			stack = append(stack, entry{childID, e.depth + 1})
		}
	}
	return maxDepth
}

func uniqueProblemIDs(nodes []LearningPathNode) []int64 {
	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for _, n := range nodes {
		for _, id := range n.ProblemIDs {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

func buildNodeTree(nodes []LearningPathNode, summaries map[int64]ProblemSummary, fullProblemIDs bool) []NodeItem {
	byID := make(map[int64]LearningPathNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	children := make(map[int64][]int64, len(nodes))
	roots := make([]int64, 0, len(nodes))
	for _, n := range nodes {
		if n.ParentID == nil {
			roots = append(roots, n.ID)
			continue
		}
		if _, ok := byID[*n.ParentID]; !ok {
			continue // 防御性忽略孤儿节点，避免破坏整棵树。
		}
		children[*n.ParentID] = append(children[*n.ParentID], n.ID)
	}

	var build func(int64, map[int64]bool) NodeItem
	build = func(id int64, seen map[int64]bool) NodeItem {
		n := byID[id]
		item := newNodeItem(&n, summaries, fullProblemIDs)
		if seen[id] {
			return item
		}
		seen[id] = true
		for _, childID := range children[id] {
			item.Children = append(item.Children, build(childID, seen))
		}
		return item
	}
	items := make([]NodeItem, 0, len(roots))
	for _, rootID := range roots {
		items = append(items, build(rootID, map[int64]bool{}))
	}
	return items
}

func newNodeItem(n *LearningPathNode, summaries map[int64]ProblemSummary, fullProblemIDs bool) NodeItem {
	item := NodeItem{
		ID:            n.ID,
		ParentID:      n.ParentID,
		Title:         n.Title,
		DescriptionMD: n.DescriptionMD,
		ProblemIDs:    make([]int64, 0, len(n.ProblemIDs)),
		Problems:      make([]ProblemSummary, 0, len(n.ProblemIDs)),
		SortOrder:     n.SortOrder,
		RequiredCount: n.RequiredCount,
		SolvedCount:   0,
		Status:        ProgressActive,
		Children:      []NodeItem{},
	}
	if n.ParentID != nil {
		item.Status = ProgressLocked
	}
	for _, id := range n.ProblemIDs {
		summary, ok := summaries[id]
		if !ok && !fullProblemIDs {
			continue
		}
		item.ProblemIDs = append(item.ProblemIDs, id)
		if ok {
			item.Problems = append(item.Problems, summary)
		}
	}
	return item
}

func (s *Service) nodeItem(ctx context.Context, n *LearningPathNode) (*NodeItem, error) {
	summaries, err := s.repo.ProblemSummaries(ctx, n.ProblemIDs)
	if err != nil {
		return nil, err
	}
	// 节点写接口仅作者/管理员可调用，返回完整存储 ID 供编辑。
	item := newNodeItem(n, summaries, true)
	return &item, nil
}
