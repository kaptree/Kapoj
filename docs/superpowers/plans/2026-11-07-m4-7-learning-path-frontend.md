# M4-7 学习路线前端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付学习路线前端：列表、树状详情与进度、作者编辑（路线表单 + 节点树编辑器）、官方管理页，并接入主/管理导航。

**Architecture:** 新增 `api/learning.ts` 与 `utils/learning.ts`；四个视图共用 PrimeVue 4 组件、`AppPageHeader`/`AppSurface`/`AppFormField`/`MarkdownView`/`MarkdownEditor`。列表页展示官方/公开自定义路线与个人完成数；树详情页用 `TreeTable` 展示节点状态与进度；编辑页用路线表单 + `TreeTable` 节点管理 + 节点对话框（标题/描述/父节点/远程选题/requiredCount/排序）；官方管理页用 `DataTable` 管理全部路线（含下架恢复）。所有权限以 `auth.can`/后端返回为准，前端不自行推导。

**Tech Stack:** Vue 3、TypeScript、Pinia、Vue Router、PrimeVue 4（TreeTable/DataTable/Dialog/Select/InputNumber/ProgressBar/Tag）、Vitest。

**Spec:** `docs/superpowers/specs/2026-11-05-m4-learning-paths-design.md`

## Global Constraints

- 不修改后端；所有 API 路径/字段以 `apps/web/src/api/schema.d.ts` 与 OpenAPI 为准。
- PrimeVue 4，不新增 UI 依赖；中文文案。
- 列表/树公开可读；创建路线需登录（任意登录用户可创建自定义路线）；编辑/删除由后端校验作者或 `learning_path.official.manage`。
- 官方路线强制公开；`isOfficial` 仅创建时由 `learning_path.official.manage` 设置；作者编辑自定义路线不能修改 `isOfficial`/`isActive`。
- 树状态：`completed` 绿色、`active` 主色、`locked` 灰色锁；游客根 active、其余 locked；已完成节点显示完成时 `solvedCount`。
- 节点：标题 1–100、描述 ≤50,000、题目 1–50 且不重复、`requiredCount` 1–题目数、父节点同路线且不可成环、仅叶子可删除。
- 官方管理页：`learning_path.official.manage` 可写；`learning_path.admin.read` 只读；包含下架路线；下架/恢复/删除均使用现有软删除 API。
- 所有页面具备加载/错误/空态与移动端可用布局；不新增 CDN 请求。
- 不推送、不合并当前 `main`，除非用户确认。

## Review Focus

1. **权限门禁**：新建/编辑/删除/下架/恢复按钮是否严格按作者、`official.manage`、`admin.read` 区分；只读管理员不能触发写操作。
2. **树状态与进度**：completed/active/locked 渲染；`solvedCount/requiredCount`；已完成锁存值；游客状态；私有/下架路线 404。
3. **节点编辑器正确性**：父节点不可选自身/后代；题目 1–50 去重；requiredCount 范围随题目数变化；仅叶子可删；上移/下移不破坏同级顺序。
4. **路线编辑语义**：新建后能添加节点；官方强制公开；作者不能改官方/上下架；完整 PUT 不丢字段。
5. **列表与管理页数据**：`mine`/`includeInactive` 参数；`myCompletedCount` 展示；空/错/载入态；分页（列表无分页时显示全部）。
6. **移动端与可访问性**：375px 下卡片、TreeTable/DataTable 横向滚动、对话框可操作；按钮有中文标签。

---

### Task 1: API、类型、工具函数与路线列表

**Files:**
- Create: `apps/web/src/api/learning.ts`
- Create: `apps/web/src/utils/learning.ts`
- Create: `apps/web/src/utils/learning.spec.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/validation.ts`（新增 `validateCoverUrl`）
- Modify: `apps/web/src/validation.spec.ts`
- Create: `apps/web/src/views/LearningPathListView.vue`
- Create: `apps/web/src/views/LearningPathListView.spec.ts`
- Modify: `apps/web/src/router/index.ts`（仅 `/learning-paths` 列表路由）
- Modify: `apps/web/src/components/layout/AppNav.vue`
- Test: 上述 spec 文件

**Interfaces:**
- Produces:
  - `learning.ts`：
    - `listLearningPaths(q?: { mine?: boolean; includeInactive?: boolean }): Promise<Page<LearningPathItem>>`
    - `getLearningPath(id)`, `getLearningTree(id)`
    - `createLearningPath(input: LearningPathInput)`, `updateLearningPath(id, input)`, `deleteLearningPath(id)`
    - `createLearningNode(pathId, input: LearningNodeInput)`, `updateLearningNode(pathId, nodeId, input)`, `deleteLearningNode(pathId, nodeId)`
  - `types.ts` 新增 `LearningPathItem`、`LearningPathInput`、`LearningPathNodeItem`、`LearningNodeInput`、`LearningTree`、`LearningNodeStatus`、`LearningProblemSummary`、`LearningPathAuthor`。
  - `utils/learning.ts`：
    - `learningStatusLabel(status)` → `已完成`/`进行中`/`未解锁`
    - `learningStatusSeverity(status)` → `success`/`info`/`secondary`
    - `learningPathVisibilityLabel(item)` → `官方`/`公开`/`私有`
    - `nodeProgressPercent(solved, required)` → 0–100（required≤0 时 0）
    - `flattenLearningNodes(nodes)` 先序扁平化
    - `findLearningNode(nodes, id)`
    - `descendantNodeIds(node)` 返回自身及后代 id 集合
    - `siblingNodes(nodes, parentId)` 按 `sortOrder,id` 返回同级
  - `validation.ts`：`validateCoverUrl(value)`，空字符串通过，否则必须为 http/https 且 hostname 非空，长度 ≤512，错误文案 `封面地址需为 http 或 https 链接`。
  - 路由 `learning-paths` → `/learning-paths`，公开。
  - 主导航“学习路线”入口（`pi pi-sitemap`）。
- Consumes: `http` 客户端、`schema.d.ts`。

- [ ] **Step 1: 写失败测试**

`utils/learning.spec.ts` 覆盖状态标签、可见性标签、进度百分比（含 required=0）、扁平化顺序、查找、后代集合、同级排序。

`validation.spec.ts` 新增 `validateCoverUrl`：空串通过；`https://example.com/a.png` 通过；`ftp://`/`javascript:`/`not-a-url` 返回错误；超 512 字符返回错误。

`LearningPathListView.spec.ts`：
- 加载并渲染路线卡片：标题、作者、`已完成 1/3 节点`、官方/公开/私有标签。
- 筛选“官方/自定义/我创建的”：官方与自定义客户端过滤；我创建的调用 `mine=true`；游客不显示“我创建的”和“新建路线”。
- 登录用户显示“新建路线”并跳转 `/learning-paths/new`；作者/管理员在卡片上看到“编辑”入口。
- 空态、错误态、重试。
- 我的完成数来自 `myCompletedCount`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/utils/learning.spec.ts src/validation.spec.ts src/views/LearningPathListView.spec.ts`

Expected: FAIL（文件/方法不存在）。

- [ ] **Step 3: 实现 API/类型/工具/列表页/路由/导航**

- `learning.ts` 使用 `http.get/post/put/delete`；列表参数 `mine`/`includeInactive` 传 `'true'`。
- 列表页：`SelectButton` 筛选 `全部|官方|自定义|我创建的`（游客隐藏“我创建的”）；卡片网格；封面为空时用渐变占位 + `pi pi-sitemap`；`RouterLink` 到 `/learning-paths/:id`；`新建路线` 按钮仅登录可见；作者或 `official.manage` 在卡片上显示“编辑”链接。
- 加载/错误/空态复用现有组件；请求竞态用 `latest` 序号；`mine` 筛选触发重新加载。
- 路由与主导航只添加列表入口；详情/编辑路由留给后续任务。更新 `AppNav.spec.ts`，断言登录/游客均能看到“学习路线”入口。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/utils/learning.spec.ts src/validation.spec.ts src/views/LearningPathListView.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/api/learning.ts apps/web/src/api/types.ts apps/web/src/utils/learning.ts apps/web/src/utils/learning.spec.ts apps/web/src/validation.ts apps/web/src/validation.spec.ts apps/web/src/views/LearningPathListView.vue apps/web/src/views/LearningPathListView.spec.ts apps/web/src/router/index.ts apps/web/src/components/layout/AppNav.vue
git commit -m "feat(web): add learning path list"
```

---

### Task 2: 学习路线树详情

**Files:**
- Create: `apps/web/src/views/LearningPathDetailView.vue`
- Create: `apps/web/src/views/LearningPathDetailView.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Test: `apps/web/src/views/LearningPathDetailView.spec.ts`

**Interfaces:**
- Consumes: Task 1 的 `getLearningTree`、`utils/learning`、`learningPathVisibilityLabel`、`nodeProgressPercent`。
- Produces:
  - 路由 `learning-path-detail` → `/learning-paths/:id(\\d+)`，公开。
  - 页面：路线标题/描述（`MarkdownView`）、作者、官方/公开/私有标签、`myCompletedCount/nodeCount`、`TreeTable` 节点树、节点详情面板。
  - 节点列：标题 + 状态标签（completed/active/locked）、`solvedCount/requiredCount` + `ProgressBar`、题目数。
  - 选择节点后展示描述（`MarkdownView`）和题目列表，题目链接 `/problems/:id`。
  - TreeTable 默认展开全部节点；`dataKey="id"`；移动端横向滚动，详情面板在树下方。

- [ ] **Step 1: 写失败测试**

```ts
it('renders path header, tree statuses and progress')
// completed/active/locked 文案、solved/required、ProgressBar value
it('selects a node and shows its problems with links')
it('renders guest locked tree from API')
it('shows not-found/forbidden error state for private path')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/LearningPathDetailView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现详情页**

- `onMounted`/watch `route.params.id` 加载 `getLearningTree`；`ApiError.code === 40400` 显示“学习路线不存在或无权查看”，其他错误通用文案。
- `expandedKeys` 初始化为所有节点 id；TreeTable `v-model:expandedKeys`（按 PrimeVue 4 实际 API 调整）。
- 行点击/“查看”按钮设置 `selectedNode`；默认选中第一个 active 节点（无则不选）。
- 状态标签使用 `Tag` + `learningStatusSeverity`；locked 加 `pi pi-lock`。
- 题目列表只展示 `node.problems`；缺失/下线题目不展示。
- 路由与页面样式；不修改列表页。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/LearningPathDetailView.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/LearningPathDetailView.vue apps/web/src/views/LearningPathDetailView.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add learning path tree detail"
```

---

### Task 3: 路线编辑与节点编辑器

**Files:**
- Create: `apps/web/src/views/LearningPathEditView.vue`
- Create: `apps/web/src/views/LearningPathEditView.spec.ts`
- Create: `apps/web/src/components/learning/LearningNodeDialog.vue`
- Create: `apps/web/src/components/learning/LearningNodeDialog.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Test: 上述 spec 文件

**Interfaces:**
- Consumes: Task 1 的 API/工具；`listProblems` 远程选题；`MarkdownEditor`/`MarkdownView`；`useConfirm`/`useToast`。
- Produces:
  - 路由 `learning-path-create` → `/learning-paths/new`，`learning-path-edit` → `/learning-paths/:id(\\d+)/edit`，均 `requiresAuth`。
  - `LearningNodeDialog` props：`visible: boolean`、`pathId: number`、`nodes: LearningPathNodeItem[]`、`editing: LearningPathNodeItem | null`、`parentId: number | null`；emits `update:visible`、`saved`。
  - 对话框校验/提交：标题 1–100、描述 ≤50,000、题目 1–50 不重复、requiredCount 1–题目数、父节点非自身/后代；调用 create/update 节点 API；中文错误。
  - 编辑页：路线表单（标题/描述/封面/排序/公开/官方仅创建时）、保存、删除路线；节点 TreeTable（添加根/子节点、编辑、删除、上移/下移）；新建保存后跳转编辑页并提示“先保存路线，再添加节点”。
  - 官方路线公开开关强制勾选且禁用；普通作者不显示官方开关；作者不能改官方/上下架。
  - 节点删除仅叶子可用；上移/下移交换同级 `sortOrder` 并刷新树。

- [ ] **Step 1: 写失败测试**

`LearningNodeDialog.spec.ts`：
- 创建节点：标题/描述/父节点/题目/requiredCount 提交正确 `LearningNodeInput`。
- 题目 0 道、>50 道、重复、requiredCount 越界显示错误且不提交。
- 编辑节点时父节点选项排除自身与后代。
- 保存失败显示后端中文错误。

`LearningPathEditView.spec.ts`：
- `/new` 空表单；普通用户不显示官方开关；管理员显示并可创建官方（强制公开）。
- 保存新建后调用 `createLearningPath` 并跳转 `/learning-paths/:id/edit`。
- 编辑加载 `getLearningTree` 填充表单与节点树；保存调用 `updateLearningPath`。
- 添加子节点带上 `parentId`；删除有子节点禁用；删除叶子调用 API；上移/下移调用两次更新并刷新。
- 删除路线确认后调用 `deleteLearningPath` 并返回列表。
- 404/无权限显示错误。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/components/learning/LearningNodeDialog.spec.ts src/views/LearningPathEditView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现编辑页与节点对话框**

- 对话框：`Select` 选父节点（含“无（根节点）”），`Select` 远程搜索题目并添加到 DataTable 行；行支持删除；`InputNumber` requiredCount/sortOrder；表单校验与错误 `Message`；保存中禁用按钮。
- 编辑页：`route.name`/`route.params.id` 判定新建/编辑；`watch` id 重新加载；表单校验（标题/封面）；保存后刷新；删除确认。
- 节点 TreeTable：列标题（状态标签）、题目数、requiredCount、操作；操作包括添加子节点/编辑/删除/上移/下移；`expandedKeys` 全展开。
- 上移/下移：按同级 `sortOrder,id` 排序并交换相邻位置，随后按新顺序重排同级 `sortOrder`（1..n）；只对值变化的节点调用 `updateLearningNode`（完整字段），完成后 `loadTree`。
- 仅当 `pathId` 存在时显示节点编辑区；新建页显示提示。
- 路由注册。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/components/learning/LearningNodeDialog.spec.ts src/views/LearningPathEditView.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/LearningPathEditView.vue apps/web/src/views/LearningPathEditView.spec.ts apps/web/src/components/learning apps/web/src/router/index.ts
git commit -m "feat(web): add learning path editor"
```

---

### Task 4: 官方学习路线管理页与导航

**Files:**
- Create: `apps/web/src/views/AdminLearningPathsView.vue`
- Create: `apps/web/src/views/AdminLearningPathsView.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Test: `apps/web/src/views/AdminLearningPathsView.spec.ts`

**Interfaces:**
- Consumes: Task 1 API；`PERM.learningPath.official.manage`/`admin.read`；`auth.can`/`canAny`。
- Produces:
  - 路由 `admin-learning-paths` → `/admin/learning-paths`，`requiresAuth`，`anyPermission: [PERM.learningPath.official.manage, PERM.learningPath.admin.read]`。
  - AdminNav 内容组“学习路线”（同权限）。
  - 页面：`listLearningPaths({ includeInactive: true })`；DataTable 列：标题、类型（官方/自定义）、可见性（公开/私有）、状态（上架/已下架）、作者、节点数、我的完成、更新时间、操作。
  - `official.manage`：新建官方路线（`/learning-paths/new?official=true`）、编辑、下架（DELETE，确认）、恢复（PUT `isActive:true`，带完整字段）。
  - `admin.read`：只读，不显示写按钮。
  - 类型筛选 `全部/官方/自定义`，状态筛选 `全部/上架/已下架`；空/错/载入态。

- [ ] **Step 1: 写失败测试**

```ts
it('loads all paths with includeInactive and renders row actions')
it('official manager can create official, down and restore')
it('admin.read sees rows but no write actions')
it('type/status filters work client-side')
it('empty/error states')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AdminLearningPathsView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现管理页与导航**

- DataTable + `SelectButton`/`Select` 筛选；状态 Tag（`上架`/`已下架`）；官方/私有 Tag。
- 下架确认后 `deleteLearningPath`；恢复确认后 `updateLearningPath` 发送 `title/descriptionMd/coverUrl/isPublic/sortOrder/isActive:true`；成功后重新加载。
- 新建官方按钮仅 `official.manage`；编辑链接作者或管理员；`admin.read` 只读。
- Router/AdminNav 接入；更新 `AdminNav.spec.ts` 断言 `official.manage`/`admin.read` 可见、无权限不可见；不动主路由权限。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AdminLearningPathsView.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminLearningPathsView.vue apps/web/src/views/AdminLearningPathsView.spec.ts apps/web/src/router/index.ts apps/web/src/components/layout/AdminNav.vue
git commit -m "feat(web): add admin learning paths page"
```

---

### Task 5: 文档、全量检查与浏览器 E2E

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`
- Create: `apps/web/e2e/learning-paths.spec.ts`
- Test: 全量后端、评测、前端；Playwright 真实浏览器

**Interfaces:**
- Consumes: Task 1–4。
- Produces: M4-7 状态文档、UI-31 状态、真实 E2E 记录。

- [ ] **Step 1: 文档**

- `DEVELOPMENT.md`：补学习路线前端路由/页面/权限/状态语义。
- `docs/任务清单.md`：M4-7 标 ✅；UI-31 标 ✅；更新 M4 进度说明。
- `docs/开发日志.md`：追加 2026-11-07 M4-7 条目（页面、验证、E2E）。
- `README.md`：当前进度与学习路线入口。
- `运行手册.md`：学习路线页面使用/验证说明。
- `docs/primevue-migration.md`：UI-31 标 ✅（学习路线列表/树/编辑/管理页）。

- [ ] **Step 2: 全量检查**

```bash
cd /Users/admin/codespace/kapoj/apps/server
gofmt -l .
go vet ./...
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run

cd /Users/admin/codespace/kapoj/apps/judge
gofmt -l .
go vet ./...
go test -count=1 ./...

cd /Users/admin/codespace/kapoj
corepack pnpm test:web
corepack pnpm lint:web
corepack pnpm build:web
git diff --check
```

- [ ] **Step 3: 真实浏览器 E2E（Playwright）**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test:e2e -- learning-paths.spec.ts`

- 使用现有 `E2E_ADMIN_USER`/`E2E_ADMIN_PASSWORD`（默认 `m2badmin`/`correct-horse`）与普通用户；通过 API 建临时路线/节点/提交，浏览器验证页面。
- 管理员（`learning_path.official.manage`）创建官方路线：根节点 required 2 + 两个子节点；普通用户看到根 active、子 locked。
- 普通用户 AC 根节点所需题目（临时插入 accepted 提交或真实提交），刷新树：根 completed、子 active；完成子节点后 completed；完成数 `myCompletedCount` 更新。
- 已完成节点改题/提高 required 后刷新，已完成用户不回退；另一用户按新配置重算。
- 普通用户创建私有路线、添加节点/题目：仅自己可见；切换公开后游客可见；管理员下架/恢复；作者列表不再显示下架路线。
- 节点编辑：添加子节点、编辑、上移/下移、删除叶子；有子节点删除禁用；题目 1–50/required 校验。
- 375px（或 690px 窄屏）列表卡片、TreeTable/DataTable 横向滚动、对话框可操作；无控制台/CDN 错误。
- 清理临时用户/路线/节点/进度/提交/审计；不删除既有数据。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md apps/web/e2e/learning-paths.spec.ts
git commit -m "docs: record learning path frontend"
```

---

## Self-Review

**Spec coverage:**

- 列表页 → Task 1。
- 树详情/进度/状态 → Task 2。
- 编辑页/节点编辑器/官方创建 → Task 3。
- 管理页/下架恢复/导航 → Task 4。
- 文档/全量检查/E2E → Task 5。

**Type consistency:**

- `LearningPathItem`/`LearningPathInput`/`LearningPathNodeItem`/`LearningNodeInput`/`LearningTree` 来自 `schema.d.ts`，Task 1 定义后各任务复用。
- `getLearningTree` 在 Task 2/3 使用；`listProblems` 在 Task 3 复用现有 API。
- `learningStatusLabel`/`learningStatusSeverity`/`nodeProgressPercent` 在 Task 2/3/4 复用。
- `validateCoverUrl` 在 Task 3 使用。

**Review Focus mapping:**

1. 权限门禁 → Task 3/4 tests + Task 5 E2E。
2. 树状态/进度 → Task 2 tests + Task 5 E2E。
3. 节点编辑器 → Task 3 tests + Task 5 E2E。
4. 路线编辑语义 → Task 3 tests + Task 5 E2E。
5. 列表/管理页数据 → Task 1/4 tests + Task 5 E2E。
6. 移动端/可访问性 → Task 5 E2E。
