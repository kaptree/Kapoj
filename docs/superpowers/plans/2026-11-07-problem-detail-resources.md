# 题目资源区、官方题解与解题思路论坛 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把题目详情页改成“题面 + 提示卡片 + 右侧四个可点击资源卡片”，交付官方题解（通过免费/金币解锁/创建者编辑）、解题思路论坛（帖子+回复）、提交统计页和关联比赛页。

**Architecture:** `problem` 域新增官方题解服务与讨论服务；`problemapi` 新增资源 API；`gamification` 提供事务内金币扣费 helper；`contest` 增加按题目筛选的联合可见性。前端新增官方题解、思路列表/详情、统计、关联比赛五个页面，详情页只做资源入口。隐藏题的官方题解/思路不沿用比赛参与者 fallback；统计/关联比赛仍可看。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis、Vue 3、TypeScript、Pinia、Vue Router、PrimeVue 4、Vitest。

**Spec:** `docs/superpowers/specs/2026-11-07-problem-detail-resources-design.md`

## Global Constraints

- 官方题解：创建者/super 可编辑；通过者免费；价格 0 所有人免费；其他用户金币购买后永久解锁。
- `problems.solution_price_coins` 0–1,000,000，默认 0。
- 官方题解内容沿用 `solution_md`/`solution_code`；题目主更新不再写入这两个字段，专用官方题解 PUT 是唯一写入口。
- 解题思路：帖子列表 + 帖子详情 + 回复；所有人可读，登录可写；作者可编辑/删除自己的内容；题目创建者/`problem.update` 管理员可删除他人内容。
- 隐藏题的官方题解/讨论要求 `problem.view_hidden`/super，不沿用比赛参与者题面 fallback；统计/关联比赛仍可看。
- 关联比赛使用 `problemId` 筛选，可见性为“公开 OR 本人报名 OR view_hidden/super”。
- 金币扣费必须与 unlock 记录同事务、幂等；余额不足不写 unlock；重复购买不重复扣费。
- 所有用户文案中文；不推送、不合并分支。
- 旧 `solutionMd`/`solutionCode` 仍出现在 `ProblemDetail`；不再出现在 `ProblemInput`/`ProblemUpdateInput`。

## Review Focus

1. **官方题解访问状态**：free/accepted/purchased/creator/locked/guest 互斥且正文不在 locked/guest 返回；测试放 Task 1。
2. **购买事务与幂等**：余额不足回滚 unlock、并发购买只扣一次、重复购买不扣费；测试放 Task 1。
3. **隐藏题资源**：官方题解/讨论对无 `view_hidden`/super 的隐藏题 404，统计/关联比赛不误伤；测试放 Task 1–4。
4. **论坛权限与软删除**：作者编辑/删除、创建者/管理员删除、删除帖子后回复不可见、隐藏题 404；测试放 Task 2。
5. **关联比赛联合可见性**：公开 OR 本人报名 OR view_hidden，不沿用 `mine=true` 互斥；测试放 Task 3。
6. **详情页不丢功能**：提示卡片、四卡片导航、官方题解编辑/购买、论坛发布/回复、统计/关联比赛页面与移动端；测试放 Task 5–6/E2E。

---

### Task 1: 官方题解、价格、访问状态与金币解锁

**Files:**
- Modify: `apps/server/internal/problem/model.go`
- Modify: `apps/server/internal/problem/dto.go`
- Modify: `apps/server/internal/problem/validate.go`
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/repository.go`
- Create: `apps/server/internal/problem/official_solution.go`
- Create: `apps/server/internal/problem/official_solution_test.go`
- Modify: `apps/server/internal/gamification/coin_repository.go`
- Modify: `apps/server/internal/gamification/coin_repository_test.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Create: `apps/server/internal/problemapi/official_solution_handler.go`
- Create: `apps/server/internal/problemapi/official_solution_handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: `problem.Repository`、`gamification.SpendCoinsTx`、`subprocess` status列 `submissions.status='accepted'`、`httpx`。
- Produces:
  - `problem.Problem.SolutionPriceCoins int64`。
  - `problem.DetailDTO.SolutionPriceCoins int64`。
  - `problem.OfficialSolutionInput{SolutionMD string; SolutionCode string; PriceCoins int64}`。
  - `problem.OfficialSolutionView{PriceCoins int64; Access string; CanEdit bool; CanPurchase bool; SolutionMD string; SolutionCode string}`。
  - 访问状态：`free`、`accepted`、`purchased`、`creator`、`locked`、`guest`。
  - `problem.OfficialSolutionService{db,problems}`；`Get(ctx,userID,problemID,viewer,isSuper)`、`Update(...)`、`Purchase(...)`。
  - `gamification.SpendCoinsTx(tx *gorm.DB, userID, amount int64, reason, refType string, refID int64) error`。
  - `problemapi.Options.Solutions *problem.OfficialSolutionService`。
  - `GET /api/problems/:id/official-solution`（公开 + LoadAccess）
  - `PUT /api/problems/:id/official-solution`（RequireAuth + LoadAccess）
  - `POST /api/problems/:id/official-solution/purchase`（RequireAuth + LoadAccess）

- [ ] **Step 1: 写失败测试**

```go
func TestOfficialSolutionAccessStates(t *testing.T)
// free/accepted/purchased/creator/locked/guest; locked/guest omit content
func TestOfficialSolutionHiddenProblem(t *testing.T)
// hidden without view_hidden -> 404; view_hidden/super -> 200
func TestOfficialSolutionUpdateOnlyCreatorOrSuper(t *testing.T)
// creator/super update; other user 403; price bounds; content limits
func TestOfficialSolutionPurchase(t *testing.T)
// price 0/accepted/creator/unlocked no charge; normal purchase deducts and unlock row; idempotent
func TestOfficialSolutionPurchaseInsufficientCoinsRollsBack(t *testing.T)
func TestProblemUpdateDoesNotWriteSolution(t *testing.T)
// PUT /problems/:id ignores/removes solution fields; solution values survive
func TestProblemInputNoSolutionFields(t *testing.T)
// API schema/bytes no solutionMd/solutionCode input
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ ./internal/gamification/ -run 'TestOfficialSolution|TestProblemUpdateDoesNotWriteSolution|TestProblemInputNoSolution' -v`
Expected: FAIL。

- [ ] **Step 3: 实现官方题解**

- 模型新增 `solution_price_coins`；`db.Migrate` 追加。
- 从 `ProblemInput` 删除 `SolutionMD`/`SolutionCode`，从 `Normalize` 删除对应校验，从 `apply` 删除赋值，从 `editableColumns` 删除列；`ProblemDetail` 仍从 Problem 读出。
- `OfficialSolutionService`：
  - `Get` 先加载 problem；隐藏题无 `viewer.CanViewHidden` 且非 super -> 404。
  - 计算访问：creator/super -> creator；accepted -> accepted；unlock -> purchased；price==0 -> free；guest -> guest；否则 locked。creator 状态可编辑。
  - content 只在 creator/accepted/purchased/free 返回。
  - `Update` 事务/行锁加载 problem；creator/super 才允许；校验 price 0..1e6、MD≤50k、Code≤65536 bytes；更新 `solution_md/solution_code/solution_price_coins/updated_at`；返回 Get。
  - `Purchase` 事务：行锁 problem，校验可见性；price 0/creator/super/accepted/already unlock -> 返回 view 不扣费；插入 `problem_solution_unlocks`（唯一 user_id+problem_id，OnConflict DoNothing）；若已存在直接返回；调用 `gamification.SpendCoinsTx`；失败回滚；成功返回 view。
- `gamification.SpendCoinsTx`：抽出 `SpendCoins` 的事务核心，供 problem 事务调用；保留 `SpendCoins` 行为。
- Handler 用 `viewer(c)`/`CurrentAccess` 获得 `CanViewHidden`、`IsSuper`；GET 公开；PUT/POST auth + LoadAccess；错误中文。
- Router 构造 `OfficialSolutionService` 并传 `problemapi.Options{Solutions: ...}`。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/problem internal/problemapi internal/gamification internal/router internal/db && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ ./internal/gamification/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/server/internal/gamification apps/server/internal/router apps/server/internal/db
git commit -m "feat(server): add official solutions and coin unlocks"
```

---

### Task 2: 解题思路帖子与回复

**Files:**
- Create: `apps/server/internal/problem/discussion_model.go`
- Create: `apps/server/internal/problem/discussion.go`
- Create: `apps/server/internal/problem/discussion_test.go`
- Create: `apps/server/internal/problemapi/discussion_handler.go`
- Create: `apps/server/internal/problemapi/discussion_handler_test.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: `problem.Viewer`、posts/replies tables、`users` 表摘要。
- Produces:
  - `problem.SolutionPostInput{Title,ContentMD string}`。
  - `problem.SolutionReplyInput{ContentMD string}`。
  - `problem.SolutionPostItem{ID,ProblemID,UserID,Title,Excerpt,Author,ReplyCount,CreatedAt,UpdatedAt,CanEdit,CanDelete}`。
  - `problem.SolutionPostDetail{SolutionPostItem; ContentMD; Replies []SolutionReplyItem}`。
  - `problem.SolutionReplyItem{ID,PostID,UserID,ContentMD,Author,CreatedAt,UpdatedAt,CanEdit,CanDelete}`。
  - `problem.DiscussionService{db,problems}`；`ListPosts`、`CreatePost`、`GetPost`、`UpdatePost`、`DeletePost`、`CreateReply`、`UpdateReply`、`DeleteReply`。
  - `problemapi.Options.Discussions *problem.DiscussionService`。
  - API：
    - GET `/api/problems/:id/solutions`（公开 + LoadAccess）
    - POST `/api/problems/:id/solutions`（登录）
    - GET `/api/problems/:id/solutions/:postId`（公开 + LoadAccess）
    - PUT/DELETE `/api/problems/:id/solutions/:postId`（登录）
    - POST `/api/problems/:id/solutions/:postId/replies`（登录）
    - PUT/DELETE `/api/problems/:id/solutions/:postId/replies/:replyId`（登录）
  - 隐藏题无 `view_hidden`/super -> 404；不沿用比赛 fallback。
  - 帖子列表分页 20；回复最多 500 条返回；所有删除软删除。
  - 作者可编辑/删除；创建者/`problem.update` 可删除他人；`canEdit`/`canDelete` 由后端按当前 viewer 计算。

- [ ] **Step 1: 写失败测试**

```go
func TestDiscussionPostCRUDAndPagination(t *testing.T)
func TestDiscussionRepliesAndDeleteCascadeVisibility(t *testing.T)
func TestDiscussionPermissions(t *testing.T)
// author edit/delete; other user 403/404; creator/moderator delete
func TestDiscussionHiddenProblem(t *testing.T)
// hidden problem 404 for ordinary/guest; view_hidden/super 200
func TestDiscussionValidationAndAuth(t *testing.T)
func TestDiscussionAPIRoutes(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ -run 'TestDiscussion' -v`
Expected: FAIL。

- [ ] **Step 3: 实现讨论服务/API**

- 新模型 `SolutionPost`/`SolutionReply` 使用 `gorm.DeletedAt`；DB migrate。
- `DiscussionService` 加载可见题目（与官方题解相同隐藏规则），查询/写入 posts/replies；作者摘要批量查 users。
- 列表用 `created_at DESC,id DESC`，回复用 `created_at ASC,id ASC`；回复数子查询；帖子摘录截断 200 字符。
- 校验标题 1–100、帖子正文 1–50000、回复 1–10000；回复上限 500。
- 删除：作者或创建者/mod 可 soft delete；删除帖子后不返回回复；删除回复后详情不显示。
- Handler：读路由 `LoadAccess`；写路由 `RequireAuth + LoadAccess`；`isModerator = access.Has(problem.update)`；路径 ID 校验；错误中文。
- Router 构造 `DiscussionService` 并传 Options。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/problem internal/problemapi internal/router internal/db && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/server/internal/router apps/server/internal/db
git commit -m "feat(server): add problem solution discussions"
```

---

### Task 3: 关联比赛 problemId 筛选

**Files:**
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/contest/service.go`
- Modify: `apps/server/internal/contest/repository_test.go`
- Modify: `apps/server/internal/contest/service_test.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `apps/server/internal/contestapi/handler_test.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: 现有 `contest.Viewer`/列表 API。
- Produces:
  - `contest.ListQuery.ProblemID int64`；`ListFilter.ProblemID int64`。
  - `GET /api/contests?problemId=<id>`；带该参数时可见性为公开 OR 本人已报名 OR `CanViewHidden`/super，不再使用 `mine=true` 的互斥语义。
  - 无 `problemId` 时保持现有行为。

- [ ] **Step 1: 写失败测试**

```go
func TestContestListByProblemVisibility(t *testing.T)
// public visible; private registered visible; private unregistered hidden; view_hidden sees all
func TestContestListByProblemKeepsExistingListBehavior(t *testing.T)
func TestContestProblemIDValidation(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/contest/ ./internal/contestapi/ -run 'TestContestListByProblem' -v`
Expected: FAIL。

- [ ] **Step 3: 实现筛选**

- `ListQuery`/`ListFilter` 增加 `ProblemID`；`contestapi.list` 解析正整数 `problemId`，非法 40001。
- Repository：有 problemId 时追加 `EXISTS contest_problems`；非 IncludeHidden 时按 ViewerID 使用 `is_public OR participant`；IncludeHidden 时全部；无 problemId 保持原逻辑。
- `registered` 字段仍按 ViewerID 计算。
- 不修改比赛详情/报名/榜单语义。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/contest internal/contestapi && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/contest/ ./internal/contestapi/ -v`
Expected: PASS。

```bash
git add apps/server/internal/contest apps/server/internal/contestapi
git commit -m "feat(server): filter contests by problem"
```

---

### Task 4: OpenAPI 契约与生成类型

**Files:**

**Interfaces:**
- Consumes: Task 1–3 的官方题解、讨论、关联比赛 API。
- Produces:
  - `ProblemDetail.solutionPriceCoins`；`ProblemInput`/`ProblemUpdateInput` 移除 solution 字段。
  - `OfficialSolutionView`、`OfficialSolutionInput`、`SolutionPostInput`、`SolutionReplyInput`、`SolutionPostItem`、`SolutionPostDetail`、`SolutionReplyItem`、分页响应。
  - 官方题解 GET/PUT/purchase、讨论帖子/回复、`/contests?problemId=` 的 OpenAPI paths。
  - 生成后的前端类型。

- [ ] **Step 1: 写契约测试/检查**

- 用 `pnpm gen:api` 验证 schema 可生成。
- 检查 `ProblemInput`/`ProblemUpdateInput` 不再含 `solutionMd`/`solutionCode`。
- 检查 `ProblemDetail` 含 `solutionPriceCoins` 且仍含只读 solution 字段。
- 检查 contest list `problemId` 参数与官方题解/讨论路径。

- [ ] **Step 2: 更新 OpenAPI + 生成**

- 添加 8 个 schema 与 6 组 path；错误码覆盖 400/401/403/404/429/502/503（按实际）。
- 官方题解 PUT 说明创建者/super；purchase 说明幂等/余额不足。
- 讨论路由描述公开读、登录写、作者/管理员删除、隐藏题 404。
- `corepack pnpm gen:api`。

- [ ] **Step 3: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(api): document problem resources and solutions"
```

---

### Task 5: 题目详情资源布局、官方题解页、统计页、关联比赛页

**Files:**
- Modify: `apps/web/src/views/ProblemDetailView.vue` + spec
- Create: `apps/web/src/views/ProblemOfficialSolutionView.vue` + spec
- Create: `apps/web/src/views/ProblemStatisticsView.vue` + spec
- Create: `apps/web/src/views/ProblemContestsView.vue` + spec
- Modify: `apps/web/src/views/ProblemEditView.vue`（移除题解字段）
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/api/problems.ts`
- Modify: `apps/web/src/api/contests.ts`
- Modify: `apps/web/src/api/types.ts`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 official endpoints、Task 3 contest problemId、现有 `getStatistics`。
- Produces:
  - 详情页左侧题面 + 独立提示卡片；右侧官方题解/解题思路/提交统计/关联比赛四张 `RouterLink` 卡片；隐藏题无 view_hidden 时只隐藏官方题解/解题思路。
  - 路由：
    - `/problems/:id/official-solution` → `problem-official-solution`
    - `/problems/:id/statistics` → `problem-statistics`
    - `/problems/:id/contests` → `problem-contests`
  - `api/problems.ts`：`getOfficialSolution`、`updateOfficialSolution`、`purchaseOfficialSolution`。
  - `api/contests.ts` 的 `ListContestsQuery.problemId`。
  - `ProblemOfficialSolutionView`：free/accepted/purchased/creator/locked/guest 状态；creator 内联编辑表单（MarkdownEditor/Textarea/InputNumber）；purchase 确认后刷新并 `auth.fetchMe()`。
  - `ProblemStatisticsView`：提交数/通过数/通过率/进度条 + 提交记录链接。
  - `ProblemContestsView`：按 problemId 分页列出关联比赛。
- ProblemEditView：删除 solutionMd/solutionCode 表单字段；保存不再提交题解字段。

- [ ] **Step 1: 写失败测试**

```ts
// ProblemDetailView.spec.ts
it('renders hint as card below statement and four resource links')
it('hides official/discussion links for hidden problem without view_hidden')
// ProblemOfficialSolutionView.spec.ts
it('renders free/accepted/purchased/creator/locked/guest states')
it('creator edits md/code/price and saves')
it('purchases with confirm and refreshes auth')
// ProblemStatisticsView.spec.ts
it('renders stats and submissions link')
// ProblemContestsView.spec.ts
it('loads contests with problemId and renders page')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/ProblemDetailView.spec.ts src/views/ProblemOfficialSolutionView.spec.ts src/views/ProblemStatisticsView.spec.ts src/views/ProblemContestsView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现页面**

- 详情页移除 Tabs，提示卡片放题面后；四个资源卡片使用 `RouterLink` + `AppSurface interactive`；显示官方题解价格（免费/金币 N）；隐藏题资源规则。
- 官方题解页使用 `MarkdownView`/`MarkdownEditor`/`Textarea`/`InputNumber`/`Button`/`Message`/`AppPageHeader`/`AppSurface`；creator 编辑；purchase 二次确认。
- 统计页复用 `getStatistics`；关联比赛页使用 `listContests({problemId})`。
- ProblemEditView 移除题解表单字段；保留其他编辑行为。
- 路由注册、API types/wrappers、错误/加载/空态、移动端。

- [ ] **Step 4: 运行测试、lint、build、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts apps/web/src/views/ProblemOfficialSolutionView.vue apps/web/src/views/ProblemOfficialSolutionView.spec.ts apps/web/src/views/ProblemStatisticsView.vue apps/web/src/views/ProblemStatisticsView.spec.ts apps/web/src/views/ProblemContestsView.vue apps/web/src/views/ProblemContestsView.spec.ts apps/web/src/views/ProblemEditView.vue apps/web/src/router/index.ts apps/web/src/api/problems.ts apps/web/src/api/contests.ts apps/web/src/api/types.ts
git commit -m "feat(web): add problem resource pages"
```

---

### Task 6: 解题思路列表、详情与回复页

**Files:**
- Create: `apps/web/src/views/ProblemSolutionsView.vue` + spec
- Create: `apps/web/src/views/ProblemSolutionDetailView.vue` + spec
- Create: `apps/web/src/api/discussions.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/router/index.ts`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 2 discussion endpoints。
- Produces:
  - 路由 `/problems/:id/solutions` → `problem-solutions`；`/problems/:id/solutions/:postId` → `problem-solution-detail`。
  - `api/discussions.ts`：list/create/get/update/delete posts；create/update/delete replies。
  - 列表页：分页、帖子摘要/作者/回复数、发布对话框（标题 + MarkdownEditor）。
  - 详情页：Markdown 正文、回复列表、回复框、作者编辑/删除；`canEdit`/`canDelete` 来自后端。
  - 游客可读；发布/回复按钮跳登录。
- 移动端卡片/正文/回复可用。

- [ ] **Step 1: 写失败测试**

```ts
it('lists posts and paginates with author/reply count')
it('creates a post via dialog')
it('renders detail, replies, and reply composer')
it('edits/deletes own post and deletes reply when allowed')
it('shows login actions to guests')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/ProblemSolutionsView.spec.ts src/views/ProblemSolutionDetailView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现论坛页面**

- 列表页用 `AppPageHeader` + `AppSurface` 卡片 + `Paginator`；发布 Dialog；错误/空态。
- 详情页展示帖子/回复 Markdown、回复表单；编辑/删除用 Dialog/Confirm/Toast；权限按钮按后端 canEdit/canDelete。
- 游客发帖/回复点击跳登录；不隐藏阅读。
- API/types/route wiring。

- [ ] **Step 4: 运行测试、lint、build、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/views/ProblemSolutionsView.vue apps/web/src/views/ProblemSolutionsView.spec.ts apps/web/src/views/ProblemSolutionDetailView.vue apps/web/src/views/ProblemSolutionDetailView.spec.ts apps/web/src/api/discussions.ts apps/web/src/api/types.ts apps/web/src/router/index.ts
git commit -m "feat(web): add problem solution discussions"
```

---

### Task 7: 文档、全量检查与真实冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端

**Interfaces:**
- Consumes: Task 1–5。
- Produces: 官方题解/讨论/关联比赛契约、前端类型、状态文档和冒烟记录。

- [ ] **Step 1: 文档**

- `DEVELOPMENT.md`：题目资源区、官方题解价格/访问、讨论区模型/权限；更新 M5/RBAC 相关状态。
- `docs/任务清单.md`：新增/更新题目资源任务为 ✅ 或当前状态；链接设计和计划。
- `docs/开发日志.md`：追加资源区/官方题解/讨论/关联比赛/测试记录。
- `README.md`、`运行手册.md`：题目详情资源、官方题解购买、讨论区使用说明。
- 不修改 AI/RBAC 已完成语义。

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

- [ ] **Step 3: 真实 API/浏览器冒烟**

- 用测试用户验证官方题解：price 0 免费；price>0 未通过 locked；通过后 accepted；购买扣金币并解锁；重复购买不扣费；创建者可编辑价格/内容。
- 验证帖子/回复/编辑/删除/管理员删除；隐藏题 404。
- 验证关联比赛 problemId 筛选（公开/本人报名/隐藏权限）。
- 详情页提示卡片和四资源卡片跳转；移动端布局。
- 清理临时题解价格/帖子/回复/unlock/金币/审计；报告临时变更与恢复。不要删除既有数据。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record problem resources and solutions"
```

---

## Self-Review

**Spec coverage:**

- 提示卡片 + 四资源卡片 → Task 5。
- 官方题解/价格/访问/购买 → Task 1 + Task 5。
- 解题思路帖子/回复/权限/隐藏题 → Task 2 + Task 6。
- 关联比赛 → Task 3 + Task 5。
- 统计页 → Task 5（现有 API）。
- OpenAPI/生成类型 → Task 4；文档/检查/冒烟 → Task 7。

**Type consistency:**

- `ProblemInput`/`ProblemUpdateInput` 不含 solution 字段；`ProblemDetail` 保留 solutionMd/solutionCode + solutionPriceCoins。
- `OfficialSolutionView.Access` 六态；`CanEdit`/`CanPurchase` 后端计算。
- 帖子/回复 `CanEdit`/`CanDelete` 后端按 viewer 计算。
- contest `problemId` 是正整数；无参数保持原行为。
- 金币 `SpendCoinsTx` 与 `problem_solution_unlocks` 同事务。

**Review Focus mapping:**

1. 访问状态 → Task 1 tests。
2. 购买事务/幂等 → Task 1 tests。
3. 隐藏题 → Task 1/2/5 tests + E2E。
4. 论坛权限/软删除 → Task 2 + Task 6 tests。
5. 关联比赛联合可见性 → Task 3 tests。
6. 详情页/移动端 → Task 5/6 + E2E。

**Proportion:** 计划固定 API、访问/购买/权限/可见性语义和测试；具体组件样式与查询实现按现有代码风格完成。
