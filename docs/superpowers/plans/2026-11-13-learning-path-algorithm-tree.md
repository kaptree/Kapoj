# 学习路线算法之树 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `/learning-paths/:id` 详情页从 `TreeTable` 改造成可交互的“算法之树”，并在移动端提供纵向树降级，同时为题目叶子增加题目级 `solved` 状态。

**Architecture:** 服务端在现有学习路线树 DTO 上增加 `LearningProblemSummary.solved`，不改变进度/锁存语义。前端新增纯函数布局工具，把节点森林排成底部生根、向上分叉的 tidy tree；桌面端用 SVG 画树干/枝干/树冠，用绝对定位的 HTML 节点与题叶叠加；移动端用纵向树干 + 枝干卡片 + 内联卷宗。详情页只负责加载、选中态与桌面/移动切换。

**Tech Stack:** Go 1.26 + Gin + GORM + testify；Vue 3 `<script setup>` + TypeScript + PrimeVue 4 + Vitest + Playwright；OpenAPI 类型由 `corepack pnpm gen:api` 生成。

**Spec:** `docs/superpowers/specs/2026-11-13-learning-path-algorithm-tree-design.md`

## Global Constraints

- 只改学习路线**详情页**；列表页、编辑页（继续用 `TreeTable`）、官方管理页不改。
- 不新增前端依赖、图表库或位图树背景；树背景由布局结果生成。
- 节点状态决定圆章/枝干/节点标签；题目 `solved` 决定题叶/题目行，两者独立。`locked` 节点里已 AC 的题叶仍显示绿色 ✓。
- 枝干颜色取**子节点**状态；根节点主干取根节点状态；不做双色渐变。
- 桌面端树舞台 + 300px 卷宗；`<900px` 使用纵向树 + 内联卷宗，不做横向滚动。
- 题叶只在**没有子节点**的叶子算法节点上展开；有子节点的节点显示题目数徽标，完整题目清单在卷宗中。
- 游客 `solved` 恒为 `false`；游客根节点 `active`、其余 `locked`。
- 所有颜色走 `tokens.css`；支持深色/浅色；`prefers-reduced-motion: reduce` 时关闭脉冲/流动动画。
- 提交只包含本任务文件，不夹带 `Export.zip`、`docs/性能测试报告.md` 等无关未跟踪文件。
- 每个任务结束前跑对应测试；最终跑后端/前端全量检查。

## Review Focus

- **深层/宽树**：深度 8 或多个兄弟节点时，节点、标题、题叶不能重叠或裁出画布；必须能滚动并自动定位选中节点。测试：Task 2 的深度/宽度布局用例，Task 7 的真实浏览器深层/宽树用例。
- **锁定节点中的已通过题**：节点保持灰色“未解锁”，但已 AC 的题叶/题目行显示绿色 ✓。测试：Task 1 的 `TestTreeProblemSolvedFlag`，Task 3/4 的锁定节点题叶与卷宗用例。
- **游客与 `solved` 泄露**：游客和其他用户看到 `solved=false`，不能借用作者/管理员的 AC 状态。测试：Task 1 的游客/其他用户断言。
- **移动端 375px**：纵向树、题叶、内联卷宗不能横向溢出，点击区域可用。测试：Task 6 的移动组件用例，Task 7 的 375px E2E。
- **路线切换残留**：从一条路线切到另一条时，旧节点、旧选中、旧滚动位置不能残留。测试：Task 5 的路由切换用例。

---

### Task 1: 服务端题目级 `solved` 字段

**Files:**
- Modify: `apps/server/internal/learning/dto.go`
- Modify: `apps/server/internal/learning/node_service.go`
- Modify: `apps/server/internal/learning/tree_test.go`
- Modify: `apps/server/internal/learning/node_service_test.go`
- Modify: `apps/server/internal/learningapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`（由 `corepack pnpm gen:api` 生成）

**Interfaces:**
- Consumes: `Repository.AcceptedProblemIDs(ctx, userID) (map[int64]struct{}, error)`、`Repository.ProblemSummaries(ctx, ids) (map[int64]ProblemSummary, error)`。
- Produces:
  - `ProblemSummary` 新增 `Solved bool `json:"solved"``。
  - `buildNodeTree(nodes []LearningPathNode, summaries map[int64]ProblemSummary, fullProblemIDs bool, progress map[int64]UserLearningProgress, accepted map[int64]struct{}) []NodeItem`。
  - `newNodeItem(n, summaries, fullProblemIDs, progress, accepted) NodeItem`。
  - `Tree` 返回的每个 `Problems[]` 带当前查看者的 `solved`。

- [ ] **Step 1: 写失败测试 `TestTreeProblemSolvedFlag`**

在 `apps/server/internal/learning/tree_test.go` 增加：

```go
func TestTreeProblemSolvedFlag(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "solved-p1", true)
	p2 := e.addProblem(t, "solved-p2", true)
	path := e.createPath(t, authorView, learning.PathInput{Title: "题叶状态", IsPublic: boolPtr(true)})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p2.ID}, RequiredCount: intPtr(2),
	})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子", ParentID: &root.ID, ProblemIDs: []int64{p1.ID},
	})

	e.addAccepted(t, e.author.ID, p1.ID)

	tree, err := e.svc.Tree(ctx, authorView, path.ID)
	require.NoError(t, err)
	require.Len(t, tree.Nodes, 1)
	require.Len(t, tree.Nodes[0].Problems, 2)
	assert.True(t, tree.Nodes[0].Problems[0].Solved, "作者 AC 的题目标记 solved")
	assert.False(t, tree.Nodes[0].Problems[1].Solved)

	// 子节点仍锁定，但题目的通过态独立于节点状态。
	require.Len(t, tree.Nodes[0].Children, 1)
	assert.Equal(t, learning.ProgressLocked, tree.Nodes[0].Children[0].Status)
	assert.True(t, tree.Nodes[0].Children[0].Problems[0].Solved)

	guestTree, err := e.svc.Tree(ctx, learning.Viewer{}, path.ID)
	require.NoError(t, err)
	assert.False(t, guestTree.Nodes[0].Problems[0].Solved, "游客不继承作者 AC")

	otherTree, err := e.svc.Tree(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.False(t, otherTree.Nodes[0].Problems[0].Solved, "其他用户不继承作者 AC")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/learning -run TestTreeProblemSolvedFlag -count=1`
Expected: FAIL，`ProblemSummary` 没有 `Solved` 字段或断言失败。

- [ ] **Step 3: 实现字段与传递**

`dto.go` 的 `ProblemSummary` 增加 `Solved bool `json:"solved"``。`node_service.go`：

- `Tree` 在 `EvaluateUser` 之后、构建树之前取 accepted：
  - 登录用户：`accepted, err = s.repo.AcceptedProblemIDs(ctx, v.UserID)`；失败返回 error。
  - 游客：`accepted = map[int64]struct{}{}`。
- `buildNodeTree`/`newNodeItem` 增加 `accepted map[int64]struct{}` 参数；`newNodeItem` 在找到 summary 时设置 `summary.Solved = true`（当 `accepted` 包含该题 id）。
- `nodeItem` 调用传 `nil` accepted。
- 更新现有测试里的 `learning.ProblemSummary{...}` 精确字面量：`tree_test.go` 与 `node_service_test.go` 中作者 AC 的题 `Solved: true`，游客/其他用户/未 AC 为 `false`。

- [ ] **Step 4: 更新 handler 断言并运行**

`apps/server/internal/learningapi/handler_test.go` 中 `TestLearningPathAPINodesAndTree` 的 `summary` 长度断言从 4 改为 5，并断言 `summary["solved"] == false`（游客读取公开树）；如该测试中有作者读取树的断言，作者 AC 后应为 true。

Run:
```bash
cd apps/server
go test ./internal/learning -count=1
go test ./internal/learningapi -run TestLearningPathAPINodesAndTree -count=1
```
Expected: PASS。

- [ ] **Step 5: 更新 OpenAPI 并重新生成类型**

`packages/api/openapi.yaml` 的 `LearningProblemSummary`：

- `required` 增加 `solved`。
- `properties` 增加 `solved: { type: boolean, description: 当前查看者是否在该题目上有 AC；游客恒为 false；不影响节点 solvedCount 与完成锁存 }`。

Run: `corepack pnpm gen:api`
Expected: `apps/web/src/api/schema.d.ts` 中 `LearningProblemSummary` 出现 `solved: boolean`。

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/learning/dto.go apps/server/internal/learning/node_service.go \
  apps/server/internal/learning/tree_test.go apps/server/internal/learning/node_service_test.go \
  apps/server/internal/learningapi/handler_test.go \
  packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(api): expose per-problem solved state in learning tree"
```

---

### Task 2: 树布局纯函数与测试

**Files:**
- Create: `apps/web/src/utils/learningTreeLayout.ts`
- Create: `apps/web/src/utils/learningTreeLayout.spec.ts`
- Modify: `apps/web/src/utils/learning.ts`
- Modify: `apps/web/src/utils/learning.spec.ts`

**Interfaces:**
- Consumes: `LearningPathNodeItem`、`LearningProblemSummary`、`LearningNodeStatus`。
- Produces:

```ts
export interface LearningTreeLayoutOptions {
  nodeSlotWidth?: number
  siblingGap?: number
  rootGap?: number
  levelGap?: number
  paddingX?: number
  paddingTop?: number
  paddingBottom?: number
  nodeSizeRoot?: number
  nodeSizeD1?: number
  nodeSizeD2?: number
  nodeSizeD3Plus?: number
}

export interface LearningTreeLayoutNode {
  node: LearningPathNodeItem
  id: number
  parentId: number | null
  depth: number
  x: number
  y: number
  size: number
  width: number
  children: LearningTreeLayoutNode[]
}

export interface LearningTreeLayoutEdge {
  parentId: number
  childId: number
  path: string
  status: LearningNodeStatus
}

export interface LearningTreeLayout {
  roots: LearningTreeLayoutNode[]
  nodes: LearningTreeLayoutNode[]
  edges: LearningTreeLayoutEdge[]
  width: number
  height: number
}

export interface LearningLeafPlacement {
  key: string
  problem: LearningProblemSummary | null
  moreCount: number
  x: number
  y: number
}

export const DEFAULT_LEARNING_TREE_LAYOUT: Required<LearningTreeLayoutOptions>
export function buildLearningTreeLayout(
  nodes: LearningPathNodeItem[],
  options?: LearningTreeLayoutOptions,
): LearningTreeLayout
export function buildLearningBranchPath(
  parentX: number,
  parentY: number,
  childX: number,
  childY: number,
): string
export function computeLearningTreeFitScale(
  canvasWidth: number,
  canvasHeight: number,
  viewportWidth: number,
  viewportHeight: number,
  minScale?: number,
): number
export function buildLearningLeafPlacements(
  layoutNode: LearningTreeLayoutNode,
  problems: LearningProblemSummary[],
  canvasWidth: number,
  canvasHeight: number,
): LearningLeafPlacement[]
export function createSeededRandom(seed: number): () => number

// apps/web/src/utils/learning.ts 新增
export function flattenLearningNodesWithDepth(
  nodes: LearningPathNodeItem[],
  depth?: number,
): Array<{ node: LearningPathNodeItem; depth: number }>
export function splitLearningProblems(
  problems: LearningProblemSummary[],
  limit?: number,
): { visible: LearningProblemSummary[]; more: number }
```

- [ ] **Step 1: 写布局测试**

在 `learningTreeLayout.spec.ts` 顶部定义本地工厂 `node(overrides: Partial<LearningPathNodeItem> = {})`（默认 `status:'active'`、`problems:[]`、`children:[]`），然后覆盖：

```ts
it('places a parent at the midpoint of its children and colors edges by child status', () => {
  const layout = buildLearningTreeLayout([
    node({ id: 1, title: '根', status: 'active', children: [
      node({ id: 2, parentId: 1, title: '左', status: 'completed' }),
      node({ id: 3, parentId: 1, title: '右', status: 'locked' }),
    ]}),
  ])
  const root = layout.nodes.find((n) => n.id === 1)!
  const left = layout.nodes.find((n) => n.id === 2)!
  const right = layout.nodes.find((n) => n.id === 3)!
  expect(root.x).toBe((left.x + right.x) / 2)
  expect(left.x).toBeLessThan(right.x)
  expect(root.y).toBeGreaterThan(left.y)
  expect(layout.edges.find((e) => e.childId === 2)!.status).toBe('completed')
  expect(layout.edges.find((e) => e.childId === 3)!.status).toBe('locked')
})
```

另加：
- `builds multiple roots without overlap`：两个根子树并排，`roots.length === 2`，第一棵所有节点 x 小于第二棵。
- `handles depth 8 and assigns smaller node sizes`：链式 8 层，`height === paddingTop + 7 * levelGap + paddingBottom`，最深节点 size 为 `nodeSizeD3Plus`。
- `builds a cubic branch path`：`buildLearningBranchPath(10, 100, 30, 20)` 以 `M10 100C` 开头且包含终点 `30 20`。
- `fit scale clamps to 1 and 0.7`：小画布返回 1，超大画布返回 0.7，中间值按比例。
- `leaf placements fall back to the sides near the top edge`：`layoutNode.y` 很小时所有 placement 的 x/y 在画布内且不全在节点上方。
- `leaf placements cap at 4 + more`：7 道题返回 5 个 placement，最后一个 `moreCount === 3`、`problem === null`。
- `seeded random is stable`：相同 seed 序列相等，不同 seed 第一个值不同。
- `flattenLearningNodesWithDepth` 和 `splitLearningProblems` 的用例放在 `learning.spec.ts`。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/utils/learningTreeLayout.spec.ts src/utils/learning.spec.ts`
Expected: FAIL，模块/导出不存在。

- [ ] **Step 3: 实现布局工具**

按 spec §6.1 实现 tidy tree：先递归计算子树宽度，再递归分配 x；根在底部，`y = height - paddingBottom - depth * levelGap`；多根并排居中。常量用 `DEFAULT_LEARNING_TREE_LAYOUT`（spec 建议值）。`buildLearningLeafPlacements` 按 spec §8.1：题数 ≤5 全部展开，>5 取前 4 + `moreCount`；节点上方空间不足时回退到左右两侧；所有坐标 clamp 在画布内。`createSeededRandom` 用 mulberry32。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/utils/learningTreeLayout.spec.ts src/utils/learning.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/utils/learningTreeLayout.ts apps/web/src/utils/learningTreeLayout.spec.ts \
  apps/web/src/utils/learning.ts apps/web/src/utils/learning.spec.ts
git commit -m "feat(web): add learning tree layout utilities"
```

---

### Task 3: 桌面树舞台组件

**Files:**
- Create: `apps/web/src/components/learning/LearningTreeNode.vue`
- Create: `apps/web/src/components/learning/LearningTreeLeaves.vue`
- Create: `apps/web/src/components/learning/LearningTreeCanvas.vue`
- Create: `apps/web/src/components/learning/LearningTreeNode.spec.ts`
- Create: `apps/web/src/components/learning/LearningTreeCanvas.spec.ts`
- Modify: `apps/web/src/styles/tokens.css`

**Interfaces:**
- Consumes: Task 2 的布局函数与类型。
- Produces:
  - `LearningTreeNode` props `{ layoutNode: LearningTreeLayoutNode; selected: boolean }`，emit `select(node: LearningPathNodeItem)`。
  - `LearningTreeLeaves` props `{ layoutNode: LearningTreeLayoutNode; problems: LearningProblemSummary[]; canvasWidth: number; canvasHeight: number }`，emit `more(node: LearningPathNodeItem)`。
  - `LearningTreeCanvas` props `{ nodes: LearningPathNodeItem[]; pathId: number; selectedId: number | null }`，emit `select(node)`、`more(node)`。

- [ ] **Step 1: 写节点/树舞台失败测试**

`LearningTreeNode.spec.ts`：
- 渲染 `status-completed`/`status-active`/`status-locked` class。
- `aria-label` 包含标题、状态、`solved / required`。
- 点击触发 `select`。
- 有子节点且有题目时显示题目数徽标；无子节点不显示。

`LearningTreeCanvas.spec.ts`：
- 传入根 + 两个子节点，渲染 3 个 `.tree-node`。
- 选中一个叶子节点（有 2 道题）时渲染 2 个 `.tree-leaf` 链接，href 为 `/problems/:id`。
- 已 AC 的题叶带 `solved` class；未 AC 的不带；锁定节点中的已 AC 题叶仍带 `solved`。
- 点击节点 emit `select`。
- 有子节点的节点不渲染题叶，但显示题目徽标。
- 多个根节点都渲染。
- `aria-label="学习路线树"` 存在。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/components/learning/LearningTreeNode.spec.ts src/components/learning/LearningTreeCanvas.spec.ts`
Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现三个组件与树 token**

- `tokens.css` 深色/浅色各增加：`--kapoj-tree-trunk`、`--kapoj-tree-branch`、`--kapoj-tree-branch-done`、`--kapoj-tree-branch-active`、`--kapoj-tree-branch-locked`、`--kapoj-tree-foliage`、`--kapoj-tree-grid`、`--kapoj-tree-glow`。
- `LearningTreeNode`：`<button type="button">` 绝对定位；`--node-size` 与 `--progress` CSS 变量；圆章内显示标题首字符；完成 ✓ 徽标；锁定虚线；有子节点且有题目时显示叶形徽标 + 数字。
- `LearningTreeLeaves`：绝对定位层；用 `buildLearningLeafPlacements` 排布；SVG 虚线叶柄；`RouterLink` 题叶，`@click.stop`；`+N` 为 `<button>` emit `more`；`solved` 题叶绿色 ✓，未通过青色/灰色。
- `LearningTreeCanvas`：`ResizeObserver` 测视口，`computeLearningTreeFitScale` 计算缩放；外层滚动、内层 `transform: scale()`；一个背景 SVG（主干、枝干、树冠、微光、网格）和一个状态连线 SVG；根主干取根状态；遍历 `layout.nodes` 渲染节点；选中叶子节点渲染 `LearningTreeLeaves`；底部图例；`watch(selectedId)` + `nextTick` 把选中节点滚到视口中央（手动计算 `scrollLeft/Top`，不调用会滚动整页的 `scrollIntoView`）。
- 装饰层 `aria-hidden`、`pointer-events:none`；`prefers-reduced-motion` 关闭动画。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/components/learning/LearningTreeNode.spec.ts src/components/learning/LearningTreeCanvas.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/components/learning/LearningTreeNode.vue \
  apps/web/src/components/learning/LearningTreeLeaves.vue \
  apps/web/src/components/learning/LearningTreeCanvas.vue \
  apps/web/src/components/learning/LearningTreeNode.spec.ts \
  apps/web/src/components/learning/LearningTreeCanvas.spec.ts \
  apps/web/src/styles/tokens.css
git commit -m "feat(web): add algorithm tree canvas"
```

---

### Task 4: 节点卷宗与路线摘要组件

**Files:**
- Create: `apps/web/src/components/learning/LearningNodePanel.vue`
- Create: `apps/web/src/components/learning/LearningPathSummary.vue`
- Create: `apps/web/src/components/learning/LearningNodePanel.spec.ts`
- Create: `apps/web/src/components/learning/LearningPathSummary.spec.ts`

**Interfaces:**
- Consumes: `learningStatusLabel/learningStatusSeverity/nodeProgressPercent`、`flattenLearningNodes`、`MarkdownView`、PrimeVue `Tag`/`ProgressBar`/`Button`。
- Produces:
  - `LearningNodePanel` props `{ node: LearningPathNodeItem | null; index?: number | null; dismissible?: boolean; emptyLabel?: string }`，emit `close()`。
  - `LearningPathSummary` props `{ path: LearningPathItem; nodes: LearningPathNodeItem[]; showSolved: boolean; compact?: boolean }`。

- [ ] **Step 1: 写卷宗/摘要失败测试**

`LearningNodePanel.spec.ts`：
- `node=null` 显示默认空提示。
- 渲染标题、状态标签、`solved / required`、描述、题目行。
- 2 题已通过、1 题未通过时，“继续挑战”链接指向未通过题；全部通过时文案为“复习题目”；无题目时按钮 disabled。
- 锁定节点里已 AC 的题目行显示通过态。
- `dismissible` 时点击关闭 emit `close`。

`LearningPathSummary.spec.ts`：
- 两个节点共享同一 `problemId` 时总数只算 1 次。
- `showSolved=false`（游客）显示 `Y 题`，不显示 `0 / Y`。
- `compact` 不渲染作者和节点数，只渲染进度。
- `nodeCount=0` 时百分比为 0 且不崩溃。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/components/learning/LearningNodePanel.spec.ts src/components/learning/LearningPathSummary.spec.ts`
Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现组件**

- `LearningNodePanel`：按 spec §9 渲染；`nextProblem` 取第一个 `!solved`，没有则第一题；`allSolved` 决定“复习题目”；`index` 渲染 `NODE // 01`；题目行整行 `RouterLink`；底部 `Button as="router-link"` 或 `router.push`；无题目 disabled。
- `LearningPathSummary`：按 spec §5 去重统计 `problemId`；`showSolved` 控制 `已完成 X / Y 题` 或 `Y 题`；桌面显示进度环，`compact` 显示线性 `ProgressBar` 与 `已完成 X / Y 节点`；作者显示 `nickname || username`。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/components/learning/LearningNodePanel.spec.ts src/components/learning/LearningPathSummary.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/components/learning/LearningNodePanel.vue \
  apps/web/src/components/learning/LearningPathSummary.vue \
  apps/web/src/components/learning/LearningNodePanel.spec.ts \
  apps/web/src/components/learning/LearningPathSummary.spec.ts
git commit -m "feat(web): add learning node panel and path summary"
```

---

### Task 5: 详情页桌面端集成

**Files:**
- Modify: `apps/web/src/views/LearningPathDetailView.vue`
- Modify: `apps/web/src/views/LearningPathDetailView.spec.ts`
- Modify: `apps/web/src/components/learning/LearningTreeCanvas.vue`（仅在集成需要时调整 props/样式）

**Interfaces:**
- Consumes: Task 3/4 的组件、`getLearningTree`、`flattenLearningNodes`、`findLearningNode`、`useAuthStore`。
- Produces: 详情页桌面端完整可用；`selectedId`、`selectedNode`、`selectedIndex` 状态供 Task 6 复用。

- [ ] **Step 1: 重写详情页测试**

保留并改写 `LearningPathDetailView.spec.ts`：
- 加载后显示路线标题、作者、官方标签、`已完成 1 / 3 节点`、摘要条题目进度。
- 默认选中第一个 `active` 节点；卷宗显示其描述与题目链接。
- 点击另一个 `.tree-node` 更新卷宗。
- 叶子节点显示题叶链接；有子节点不显示题叶。
- `path.descriptionMd` 存在时显示折叠简介，点击“展开”后完整内容可见。
- 游客根 active、其余 locked；摘要条只显示总题数。
- 404 与通用错误保留。
- 路由 id 变化时旧路线内容消失、加载新路线；旧选中不残留。
- 空节点显示 `AppEmptyState` 文案“该路线还没有节点”。
- 选中节点变化时，视觉隐藏的 `aria-live="polite"` 区域包含“已选择 {标题}”。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/views/LearningPathDetailView.spec.ts`
Expected: FAIL，旧断言找不到 `TreeTable`/新组件未接入。

- [ ] **Step 3: 实现详情页桌面端**

- 删除 `TreeTable`/`Column`/`TreeNode` 导入与旧树逻辑；保留 `AppPageHeader`、`AppSurface`、`AppErrorState`、`AppLoadingState`、`MarkdownView`。
- 增加 `LearningPathSummary`、`LearningTreeCanvas`、`LearningNodePanel`、`AppEmptyState`。
- 加载后 `selectedId = firstActive?.id ?? nodes[0]?.id ?? null`；`selectedNode` 用 `findLearningNode`；`selectedIndex` 用扁平数组 index+1。
- 模板：页头 → 摘要条 → 可折叠路线简介 → `AppSurface` → 错误/加载/空态 → 桌面 `.desktop-layout`（树 + 卷宗）。
- `@more` 处理：选中对应节点；桌面端在 `nextTick` 后把卷宗题目列表滚动到可见区域（给题目列表一个 ref），移动端内联卷宗已展开，无需额外处理。
- 卷宗宽度 300px；`<1100px` 收到 280px；树舞台高度用 `clamp(560px, 68vh, 680px)`。
- 路线简介默认两行 clamp，`descriptionExpanded` 控制展开/收起。
- 在页面内放一个 `class="sr-only" aria-live="polite"` 的段落，内容为 `selectedNode ? \`已选择 ${selectedNode.title}\` : ''`，用于键盘/读屏播报。
- 路由 id 变化时重置 `path/nodes/selectedId/descriptionExpanded`。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/views/LearningPathDetailView.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/LearningPathDetailView.vue apps/web/src/views/LearningPathDetailView.spec.ts
git commit -m "feat(web): render learning path detail as algorithm tree"
```

---

### Task 6: 移动端纵向树与响应式切换

**Files:**
- Create: `apps/web/src/composables/useMediaQuery.ts`
- Create: `apps/web/src/composables/useMediaQuery.spec.ts`
- Create: `apps/web/src/components/learning/LearningMobileTree.vue`
- Create: `apps/web/src/components/learning/LearningMobileTree.spec.ts`
- Modify: `apps/web/src/views/LearningPathDetailView.vue`
- Modify: `apps/web/src/views/LearningPathDetailView.spec.ts`

**Interfaces:**
- Consumes: Task 2 的 `flattenLearningNodesWithDepth`/`splitLearningProblems`，Task 4 的 `LearningNodePanel`。
- Produces:
  - `useMediaQuery(query: string): Ref<boolean>`。
  - `LearningMobileTree` props `{ nodes: LearningPathNodeItem[]; selectedId: number | null }`，emit `select(node)`。

- [ ] **Step 1: 写移动端失败测试**

`useMediaQuery.spec.ts`：
- mock `window.matchMedia` 返回 `matches=true`，`onMounted` 后 ref 为 true；`change` 事件更新；卸载时 `removeEventListener` 被调用。

`LearningMobileTree.spec.ts`：
- 渲染 `.mobile-node` 行，顺序为先序，`data-depth` 正确。
- 根 active、子 locked；状态标签与 `solved / required` 可见。
- 点击卡片 emit `select`。
- 选中叶子节点后显示题叶链接；7 道题显示 4 个链接 + `+3`。
- 选中节点后内联卷宗出现（标题、题目行、继续挑战）。
- 有子节点的节点不展开题叶，显示题目徽标。
- `role="tree"`、`treeitem`、`aria-level`、`aria-selected` 存在。

`LearningPathDetailView.spec.ts` 增加：
- mock `matchMedia('(max-width: 899px)')` 为 true，详情页渲染 `.mobile-tree` 而不是 `.tree-viewport`。
- 375px 宽度下无横向溢出（断言 `.mobile-tree` 存在且不渲染桌面树）。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/composables/useMediaQuery.spec.ts src/components/learning/LearningMobileTree.spec.ts src/views/LearningPathDetailView.spec.ts`
Expected: FAIL，组件/composable 不存在。

- [ ] **Step 3: 实现移动端**

- `useMediaQuery`：`onMounted` 创建 `matchMedia`，监听 `change`；`onBeforeUnmount` 移除监听；SSR/无 `window` 时返回 false。
- `LearningMobileTree`：按 spec §10.3 渲染 `<ol role="tree">`；每行 `<li role="treeitem" :aria-level="depth+1" :aria-selected>`；卡片 `button` 含状态圆点、标题、状态标签、`solved/required`、进度条；深度缩进 `22px`；叶子节点选中时展开 4 + `+N` 题叶；选中节点下方渲染 `LearningNodePanel` 内联卷宗；主干/枝干线用 CSS 伪元素或绝对定位线。
- `LearningPathDetailView`：`const isMobile = useMediaQuery('(max-width: 899px)')`；移动端渲染 `LearningMobileTree`，桌面端保留 `.desktop-layout`；移动端摘要条用 `compact`；页头标题/标签/返回按钮保留。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/composables/useMediaQuery.spec.ts src/components/learning/LearningMobileTree.spec.ts src/views/LearningPathDetailView.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/composables/useMediaQuery.ts apps/web/src/composables/useMediaQuery.spec.ts \
  apps/web/src/components/learning/LearningMobileTree.vue \
  apps/web/src/components/learning/LearningMobileTree.spec.ts \
  apps/web/src/views/LearningPathDetailView.vue apps/web/src/views/LearningPathDetailView.spec.ts
git commit -m "feat(web): add mobile vertical learning tree"
```

---

### Task 7: E2E、文档与全量检查

**Files:**
- Modify: `apps/web/e2e/learning-paths.spec.ts`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/任务清单.md`
- Modify: `DEVELOPMENT.md`（如涉及前端页面说明）

**Interfaces:**
- Consumes: 前 6 个任务的页面与组件。
- Produces: 真实浏览器验收与文档同步。

- [ ] **Step 1: 更新 E2E 详情页选择器**

在 `apps/web/e2e/learning-paths.spec.ts`：

- 保留编辑页的 `.p-treetable-tbody` 选择器与用例。
- 详情页用：
  ```ts
  function treeNode(page: Page, title: string) {
    return page.locator('.tree-node').filter({ hasText: title }).first()
  }
  function mobileTreeNode(page: Page, title: string) {
    return page.locator('.mobile-node').filter({ hasText: title }).first()
  }
  ```
- 把详情页 `treeRow(...)` 断言改为 `treeNode(...)`：
  - active：`toContainText('0 / 2')`，并断言 class 含 `status-active`。
  - locked：`toContainText('未解锁')`。
  - completed：`toContainText('2 / 2')`，class 含 `status-completed`。
- 点击叶子节点后断言 `.tree-leaf` 链接出现，点击第一个题叶后 URL 为 `/problems/:id`。
- 375px 用例：断言 `.mobile-tree` 可见、`.tree-viewport` 不存在；页面 `document.documentElement.scrollWidth <= clientWidth`；选中节点后内联卷宗可见。
- 增加浅色主题用例：切换主题后 `.tree-node` 与 `.tree-leaf` 仍可见。
- 增加深层/宽树用例：通过 API 创建至少 5 层、每层 2–3 个兄弟节点的临时路线，打开详情页断言所有 `.tree-node` 渲染、选中节点滚动后在视口内、无控制台错误；结束后清理临时数据。
- 增加 reduced-motion 用例：`page.emulateMedia({ reducedMotion: 'reduce' })` 后选中节点，读取 `.tree-node.selected .node-core` 的 `::after` 计算样式 `animationName`，期望为 `none`。

- [ ] **Step 2: 运行 E2E**

前置：后端 `:18080`、PostgreSQL/Redis 已启动，前端 dev server 可启动。

Run: `corepack pnpm test:e2e -- e2e/learning-paths.spec.ts`
Expected: PASS；失败时保留 trace/截图并修复。

- [ ] **Step 3: 更新文档**

- `README.md` 学习路线详情页描述：从 `TreeTable` 改为“算法之树 + 节点卷宗 + 移动端纵向树”，补充题目级 `solved` 字段。
- `运行手册.md` 学习路线章节：更新页面说明、`solved` 字段、桌面/移动验收步骤。
- `docs/任务清单.md`：记录本次 UI 改版完成。
- `DEVELOPMENT.md`：如 4.5/前端页面说明提到 TreeTable，改为算法之树。

- [ ] **Step 4: 全量检查**

```bash
cd apps/server
gofmt -l .
go vet ./...
golangci-lint run
go test -p 1 -count=1 ./...

cd ../..
corepack pnpm gen:api
git diff --exit-code apps/web/src/api/schema.d.ts
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过；`gen:api` 无差异；无新增 lint/build 错误。

- [ ] **Step 5: 提交**

```bash
git add apps/web/e2e/learning-paths.spec.ts README.md 运行手册.md docs/任务清单.md DEVELOPMENT.md
git commit -m "docs: update learning path algorithm tree delivery"
```

---

## 交付后检查

- 桌面端：树、节点状态、题叶、卷宗、摘要条、路线简介均可见；点击节点/题叶行为正确。
- 移动端 375px：纵向树、题叶、内联卷宗可用，无横向滚动。
- 游客：根 active、其余 locked；摘要条只显示总题数；`solved` 全为 false。
- 锁定节点：节点灰色，已 AC 题叶绿色 ✓，未 AC 题叶灰色。
- 深层/宽树：缩放/滚动、自动定位选中节点、节点不重叠。
- 深色/浅色主题、`prefers-reduced-motion` 均正常。
- 编辑页仍为 `TreeTable`，未受影响。
