# M1-C 测评记录、SSE 与提交页 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用户可以查看全平台测评记录并按题目/比赛/语言/状态/用户/昵称筛选，评测状态通过 SSE 实时刷新；题目页可以在 Monaco 编辑器中提交代码；本人 AC 后可以看到题解。

**Architecture:** Web 服务在 `submission` 包中增加记录查询与详情 DTO，`submissionapi` 暴露公开列表/详情/SSE；judge 在领题与写入终态后向 Redis 频道发布 submission id，Web 的 SSE handler 订阅并把事件推给浏览器。前端新增 `SubmissionsView`、`SubmitView` 与 `CodeEditor`，用 Element Plus + Monaco；题解可见性由 problemapi 在返回详情前查询 `HasAccepted`。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL、Redis Pub/Sub、Vue 3、Element Plus、Monaco Editor。

**Spec:** `DEVELOPMENT.md` 4.6、4.2、4.3；`docs/任务清单.md` M1-8、M1-9；M1-A 的“本人 AC 后可见题解”决策。

## Global Constraints

- 接口 JSON 使用 camelCase，响应仍为 `{code,message,data}`；分页参数 `page`、`page_size`。
- `GET /submissions`、`GET /submissions/:id`、SSE 允许游客访问；练习提交的代码和测评详情公开。比赛提交在 M2 的赛时保护规则下仅本人/管理员可见，M1 先用 `contestId != null` 作为受保护提交处理。
- SSE 事件只发送 `{"submissionId": id}`，不发送代码、测试数据或判题细节；前端收到后重新拉取当前列表。
- 日志不得包含代码、测试数据或 checker 内容。
- 本人 AC 后题解可见：管理员始终可见；普通用户只有在该题存在 `accepted` 提交时，题目详情才返回 `solutionMd`、`solutionCode`、`checkerUploaded`、`createdBy`。
- Monaco 只在前端加载，不访问 CDN；构建产物必须离线可用。
- 所有新增文档与界面文案使用中文。

## Review Focus

- 游客/普通用户/管理员对记录列表、记录详情、比赛提交代码的可见性边界。
- 筛选参数为空、非法、组合使用、分页越界时不能 500 或返回错误数据。
- SSE 断开、Redis 重连、浏览器取消请求时不能泄漏 goroutine 或阻塞后续请求。
- 题解可见性不能因为客户端参数或隐藏题目被绕过。
- Monaco 在无 CDN、快速切换语言、组件卸载时正确销毁，不泄漏编辑器实例。

---

### Task 1: 提交记录查询、详情与筛选

**Files:**
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/submission/service.go`
- Modify: `apps/server/internal/submission/dto.go`
- Create: `apps/server/internal/submission/query.go`
- Test: `apps/server/internal/submission/repository_test.go`
- Test: `apps/server/internal/submission/service_test.go`

**Interfaces:**
- Produces:
  ```go
  type ListFilter struct {
      ProblemID int64
      ContestID *int64
      UserID    int64
      Language  Language
      Status    Status
      Nickname  string
  }
  type ListQuery struct {
      ProblemID int64
      ContestID *int64
      UserID    int64
      Language  Language
      Status    Status
      Nickname  string
  }
  type ListRow struct {
      Submission
      Username     string
      Nickname     string
      ProblemTitle string
      ProblemSlug  string
  }
  type ListItem struct {
      ID           int64      `json:"id"`
      UserID       int64      `json:"userId"`
      Username     string     `json:"username"`
      Nickname     string     `json:"nickname"`
      ProblemID    int64      `json:"problemId"`
      ProblemTitle string     `json:"problemTitle"`
      ProblemSlug  string     `json:"problemSlug"`
      ContestID    *int64     `json:"contestId,omitempty"`
      Language     Language   `json:"language"`
      Status       Status     `json:"status"`
      TimeUsedMS   int        `json:"timeUsedMs"`
      MemoryUsedKB int        `json:"memoryUsedKb"`
      Score        int        `json:"score"`
      CreatedAt    time.Time  `json:"createdAt"`
      JudgedAt     *time.Time `json:"judgedAt,omitempty"`
  }
  type Detail struct {
      ListItem
      Code         string            `json:"code,omitempty"`
      CodeHidden   bool              `json:"codeHidden"`
      ErrorMessage string            `json:"errorMessage,omitempty"`
      JudgeResult  []JudgeCaseResult `json:"judgeResult"`
  }
  type DetailRow struct {
      ListRow
      Code string
      ErrorMessage string
      JudgeResult dbtypes.List[JudgeCaseResult]
  }
  type Viewer struct{ UserID int64; CanManage bool }
  func (s *Service) List(ctx context.Context, q ListQuery, page, size int) (httpx.Page[ListItem], error)
  func (s *Service) Detail(ctx context.Context, v Viewer, id int64) (*Detail, error)
  func (s *Service) HasAccepted(ctx context.Context, userID, problemID int64) (bool, error)
  func (r *gormRepository) List(ctx, f ListFilter, offset, limit int) ([]ListRow, int64, error)
  func (r *gormRepository) FindDetail(ctx, id int64) (*DetailRow, error)
  func (r *gormRepository) HasAccepted(ctx, userID, problemID int64) (bool, error)
  ```
- `ListQuery.Normalize`：`Language` 非空时必须合法；`Status` 非空时必须属于九种状态；`Nickname` trim；`ProblemID/UserID` 为负时 40001；`ContestID` 为负数时 40001；`Nickname` 长度 ≤64。
- `Service.List` 分页限制与 `problem.Service` 相同（page 至少 1，size 最大 100），按 `submissions.id DESC` 排序。No auth.
- `Service.Detail`：不存在 → `httpx.ErrNotFound`。若 `row.ContestID != nil` 且 `!v.CanManage` 且 `v.UserID != row.UserID`，返回 `CodeHidden:true`、空 `Code`、空 `JudgeResult`；练习提交（`ContestID == nil`）始终返回代码与结果。
- Repository list 使用 `submissions s JOIN users u JOIN problems p`；昵称使用 `ILIKE %...% ESCAPE '\'`（转义 `\ % _`）；`ContestID != nil` 且 `*ContestID == 0` 表示 `s.contest_id IS NULL`。
- `HasAccepted` 使用 `EXISTS` 查询 `status='accepted'`。
- DTO 的 `JudgeResult` 永远返回数组（空为 `[]`，不是 `null`）。

- [ ] **Step 1: 写失败测试**

```go
func TestListFiltersAndOrders(t *testing.T) {
    // 建 2 个用户、2 道题、多条提交；
    // 分别按 problemId、userId、nickname（模糊）、language、status、contestId 过滤；
    // 结果按 id DESC；分页 total 正确。
}

func TestListRejectsInvalidStatusAndLanguage(t *testing.T) {
    // status=bogus / language=rust -> 40001。
}

func TestDetailHidesContestCodeFromOthers(t *testing.T) {
    // contest_id 非空：本人/管理员看到 code 和 judgeResult；
    // 其他普通用户 codeHidden=true 且 code/judgeResult 为空。
    // 练习提交（contest_id IS NULL）对所有人可见。
}

func TestHasAccepted(t *testing.T) {
    // 只有 accepted 算通过；WA/TLE 不算。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/submission/`
Expected: FAIL（类型/方法不存在）。

- [ ] **Step 3: 实现查询、DTO 与服务**

按 Interfaces 实现。`ListRow` 用 `Scan` 到包含 `Submission` 与 join 字段的临时结构；`FindDetail` 同样 join。保持 `List` 不返回 code/judge_result。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/submission
git commit -m "feat(server): submission records query and filters"
```

---

### Task 2: 记录 HTTP 接口与筛选参数

**Files:**
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `packages/api/openapi.yaml`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/submissions.ts`
- Test: `apps/server/internal/submissionapi/handler_test.go`

**Interfaces:**
- `RegisterRoutes` 增加：
  - `GET /submissions` → `submission.PageResponse`（公开）
  - `GET /submissions/:id` → `submission.Detail`（公开，可选 bearer）
- Query 解析：
  - `problemId`、`contestId`、`userId` 用 `strconv.ParseInt`；非法或负数 → 40001。
  - `language`、`status` 直接交给 `ListQuery.Normalize`。
  - `nickname` 原样传入。
  - `page`/`page_size` 用 `httpx.ParsePage`。
- `viewer(c)`：`UserID` 来自 claims（游客 0）；`CanManage` 当角色为 `problem_admin`、`contest_admin` 或 `super_admin`。
- OpenAPI 新增：
  - `GET /submissions`：参数、`SubmissionPageResponse`。
  - `GET /submissions/{id}`：`SubmissionDetailResponse`。
  - `SubmissionListItem`、`SubmissionDetail`、`JudgeCaseResult`、`SubmissionPageResponse`、`SubmissionDetailResponse` 组件。
- 前端类型新增 `SubmissionListItem`、`SubmissionDetail`、`SubmissionPage`、`SubmissionJudgeCase`；`submissions.ts` 增加 `listSubmissions(query)`、`getSubmission(id)`。

- [ ] **Step 1: 写失败测试**

```go
func TestListSubmissionsEndpoint(t *testing.T) {
    // 游客 GET /api/submissions?status=accepted&language=cpp&nickname=ali
    // -> 200，items 只含匹配记录，total 正确。
}

func TestListSubmissionsValidation(t *testing.T) {
    // problemId=abc / status=bogus / language=rust -> 40001。
}

func TestSubmissionDetailEndpoint(t *testing.T) {
    // 练习提交游客可见 code；不存在 -> 404。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/submissionapi/`
Expected: FAIL。

- [ ] **Step 3: 实现接口并更新 OpenAPI/类型**

按 Interfaces 实现。`list` 调用 `svc.List`；`detail` 调用 `svc.Detail`。生成类型：

```bash
corepack pnpm gen:api
```

- [ ] **Step 4: 运行测试与前端构建**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/submissionapi/ ./internal/router/`
Run: `cd ../.. && corepack pnpm build:web`
Expected: PASS / build 成功。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/submissionapi apps/server/internal/router packages/api/openapi.yaml apps/web/src/api
git commit -m "feat(server): submission records api"
```

---

### Task 3: SSE 实时刷新与 Redis 发布

**Files:**
- Create: `apps/server/internal/submissionevents/streamer.go`
- Create: `apps/server/internal/submissionevents/streamer_test.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Create: `apps/judge/internal/events/publisher.go`
- Modify: `apps/judge/internal/pipeline/pipeline.go`
- Modify: `apps/judge/internal/pipeline/pipeline_test.go`
- Modify: `apps/judge/internal/worker/worker_test.go`
- Modify: `apps/judge/cmd/judge/main.go`
- Modify: `packages/api/openapi.yaml`
- Test: `apps/server/internal/submissionapi/handler_test.go`

**Interfaces:**
- Redis 频道常量 `kapoj:submissions`（server 与 judge 各一份，字符串必须一致）。
- Server：
  ```go
  type Event struct { SubmissionID int64 `json:"submissionId"` }
  type Streamer interface { Subscribe(ctx context.Context) (<-chan Event, error) }
  type RedisStreamer struct{ rdb *redis.Client }
  func NewRedisStreamer(rdb *redis.Client) *RedisStreamer
  func (s *RedisStreamer) Subscribe(ctx context.Context) (<-chan Event, error)
  ```
- `RedisStreamer.Subscribe`：`rdb.Subscribe(ctx, Channel)`，返回 channel；goroutine 读取消息并 JSON 解码，错误/关闭时 close 输出 channel；ctx 取消时 `sub.Close()`。
- `submissionapi` handler `stream`：设置 `Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`、`X-Accel-Buffering: no`，每 15 秒写 `: keep-alive\n\n`；事件格式 `data: {"submissionId":123}\n\n`；请求取消时退出。
- Router 构造 `submissionevents.NewRedisStreamer(d.Redis)` 传给 `submissionapi.RegisterRoutes`。
- Judge：
  ```go
  type Publisher interface { Publish(ctx context.Context, submissionID int64) }
  type RedisPublisher struct { rdb *redis.Client }
  func NewRedisPublisher(addr string) *RedisPublisher
  func (p *RedisPublisher) Publish(ctx context.Context, submissionID int64)
  func (p *RedisPublisher) Close() error
  ```
  `Publish` 失败只记录 warning，不影响判题。
- Pipeline 增加 `publisher` 字段；`New(repo, root, sb, publisher)`。在 `Claim` 成功后发布一次（judging），在 CE/终态 `Finish` 成功后发布一次，在 `MarkSystemError` 成功后发布一次。publisher 可为 nil（测试）。
- Main 创建 publisher，`defer Close`，传给 pipeline。
- OpenAPI 增加 `GET /submissions/stream`，`text/event-stream`，说明事件只有 `submissionId`。

- [ ] **Step 1: 写失败测试**

```go
func TestRedisStreamerDeliversPublishedEvents(t *testing.T) {
    // miniredis；发布 {"submissionId":42}；Subscribe 收到 42。
}

func TestSSEHandlerWritesEventAndKeepAlive(t *testing.T) {
    // fake Streamer；httptest server 读取到 "data: {...}\n\n"；
    // 关闭连接后 handler 退出。
}

func TestPipelinePublishesJudgingAndTerminalEvents(t *testing.T) {
    // fake publisher 记录 2 次：claim 后、finish 后；CE 也发布 terminal。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test -count=1 ./internal/submissionevents/ ./internal/submissionapi/`
Run: `cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/pipeline/`
Expected: FAIL。

- [ ] **Step 3: 实现 Streamer、Publisher、SSE handler 与装配**

按 Interfaces 实现。`go get github.com/redis/go-redis/v9` in `apps/judge`。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected PASS。
Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/submissionevents apps/server/internal/submissionapi apps/server/internal/router apps/judge packages/api/openapi.yaml
git commit -m "feat: submission sse stream and judge publisher"
```

---

### Task 4: 本人 AC 后可见题解

**Files:**
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/submission/repository_test.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `packages/api/openapi.yaml`
- Test: `apps/server/internal/problemapi/handler_test.go`

**Interfaces:**
- `problemapi.RegisterRoutes(api, problemJSON, uploads, svc, accepted AcceptedChecker)`：
  ```go
  type AcceptedChecker interface {
      HasAccepted(ctx context.Context, userID, problemID int64) (bool, error)
  }
  ```
- `handler.get`：先 `svc.Get`；`includeSolution := v.CanManage`；若否且 claims 存在，`includeSolution, _ = h.accepted.HasAccepted(ctx, claims.UserID, id)`；`httpx.OK(c, problem.ToDetail(p, includeSolution))`。
- `problem.ToDetail` 参数语义改为 `includeSolution`（列表/create/update 传 true）。
- OpenAPI 的 `solutionMd/solutionCode/checkerUploaded/createdBy` 描述改为“管理员或本人已通过该题”。
- Router 传入 `submission.NewGormRepository(d.DB)`（与提交服务复用）。

- [ ] **Step 1: 写失败测试**

```go
func TestDetailShowsSolutionAfterAccepted(t *testing.T) {
    // 普通用户未 AC：无 solution 字段；
    // 插入该用户 accepted 提交后：详情含 solutionMd/solutionCode；
    // WA 提交不算。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problemapi/`
Expected: FAIL。

- [ ] **Step 3: 实现 AC 检查与 DTO 语义**

按 Interfaces 实现。`HasAccepted` 已在 Task 1 加入。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/submission apps/server/internal/problemapi apps/server/internal/router packages/api/openapi.yaml
git commit -m "feat(server): unlock problem solutions after acceptance"
```

---

### Task 5: 前端测评记录页、SSE、Monaco 提交页

**Files:**
- Create: `apps/web/src/utils/submission.ts`
- Create: `apps/web/src/utils/submission.spec.ts`
- Create: `apps/web/src/monaco.ts`
- Create: `apps/web/src/components/CodeEditor.vue`
- Create: `apps/web/src/components/CodeEditor.spec.ts`
- Create: `apps/web/src/views/SubmissionsView.vue`
- Create: `apps/web/src/views/SubmitView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/layouts/DefaultLayout.vue`
- Modify: `apps/web/src/components/CodeEditor.vue`
- Modify: `apps/web/src/api/submissions.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/package.json`（新增 `monaco-editor`）
- Test: `apps/web/src/utils/submission.spec.ts`
- Test: `apps/web/src/components/CodeEditor.spec.ts`

**Interfaces:**
- `utils/submission.ts`：
  ```ts
  export const SUBMISSION_STATUSES: { value: SubmissionStatus; label: string }[]
  export const SUBMISSION_LANGUAGES: { value: SubmissionLanguage; label: string }[]
  export function statusLabel(s: SubmissionStatus): string
  export function statusTagType(s: SubmissionStatus): 'success'|'warning'|'danger'|'info'|'primary'
  export function languageLabel(l: SubmissionLanguage): string
  ```
- `monaco.ts`：配置 `self.MonacoEnvironment` 使用本地 editor worker；导出 `monaco`。
- `CodeEditor.vue`：props `modelValue: string`、`language: string`、`height?: string`；emit `update:modelValue`；onMounted create，watch 同步 value/language，onBeforeUnmount dispose。
- `SubmissionsView.vue`：筛选栏（problemId、contestId、language、status、userId、nickname）、表格、分页、详情抽屉、SSE 自动刷新（300ms 防抖）。
- `SubmitView.vue`：`/problems/:id/submit`，需要登录；加载题目、语言选择、Monaco、提交后跳转 `/submissions?problemId=...`；按题目+语言把代码存 localStorage。
- `ProblemDetailView.vue`：头部增加“提交代码”按钮，链接到 `problem-submit`；题解 Tab 继续由接口字段控制。
- 路由：
  - `/submissions` → `SubmissionsView`，标题“测评记录”。
  - `/problems/:id(\\d+)/submit` → `SubmitView`，`requiresAuth: true`，标题“提交代码”。
- 导航去掉 `/submissions` 的 milestone 禁用标记。
- `CodeEditor.vue` 通过 `@/monaco` 引入本地 worker；提交页路由懒加载 Monaco，不增加首屏体积。

- [ ] **Step 1: 写失败测试**

```ts
// utils/submission.spec.ts
it('maps statuses to labels and tag types', () => {
  expect(statusLabel('accepted')).toBe('通过')
  expect(statusTagType('wrong_answer')).toBe('danger')
  expect(languageLabel('cpp')).toBe('C++')
})

// CodeEditor.spec.ts（mock @/monaco）
it('creates, updates and disposes the editor', async () => {
  // mount -> create called; setProps modelValue -> setValue;
  // emit update:modelValue on change; unmount -> dispose.
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/utils/submission.spec.ts src/components/CodeEditor.spec.ts`
Expected: FAIL（文件不存在）。

- [ ] **Step 3: 实现前端**

```bash
corepack pnpm --filter web add monaco-editor
```
按 Interfaces 实现。SSE 用 `new EventSource('/api/submissions/stream')`；`onmessage` 只触发 300ms 节流刷新，`onopen` 重连后补拉一次，30 秒定时兜底，`onBeforeUnmount` 关闭。详情抽屉用 `getSubmission(id)` 并用请求序号丢弃过期响应，`codeHidden` 时显示提示。草稿在代码改动 500ms 后保存，localStorage 异常不影响提交。

- [ ] **Step 4: 运行测试、lint、构建**

Run: `corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: 全部通过，构建产物无外链脚本。

- [ ] **Step 5: 提交**

```bash
git add apps/web
git commit -m "feat(web): submission records, sse and monaco submit page"
```

---

### Task 6: 文档与端到端验收

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: `docs/superpowers/plans/2026-11-04-m1c-records-submit.md`（验收记录写入开发日志）
- Create: `private e2e-m1c.sh`（临时脚本，不入库）

**Interfaces:**
- 文档更新：
  - DEVELOPMENT 4.6 实现状态；4.3 题解可见性；4.2 提交页；2.4 无新错误码；9 节 M1 仍 🚧（M1-C 完成后可标 ✅）。
  - 任务清单 M1-8/M1-9 ✅，M1 标记 ✅。
  - 开发日志追加 M1-C。
  - README/运行手册补测评记录页、SSE、提交页与 Monaco 说明。
- E2E（真实 server + judge + PostgreSQL + Redis + Docker）：
  1. 复用 M1-B 的数据，GET `/api/submissions` 按 problemId/status/language/userId/nickname 筛选，校验 total/排序。
  2. GET `/api/submissions/:id` 返回 code 与 judgeResult。
  3. 用 curl 保持 SSE 连接，提交一条新代码，1 秒内收到 `data: {"submissionId":...}`。
  4. 普通用户 AC 后 GET 题目详情包含 `solutionMd`/`solutionCode`；未 AC 用户不包含。
  5. 前端 `pnpm build:web` 成功；启动 dev server，用真实浏览器验证：/submissions 列表、筛选、SSE 自动更新、详情抽屉、/problems/:id/submit Monaco 提交并跳转。
  6. 无 CDN 请求、无控制台错误、无残留容器。

- [ ] **Step 1: 启动真实环境并写验收脚本**

复用 M1-B 的 compose 数据库/Redis/镜像；server/judge 使用同一 `KAPOJ_STORAGE_DIR` 绝对路径；SSE 用 `curl -N` + 超时读取。

- [ ] **Step 2: 运行 API 与 SSE 验收**

Expected: 所有筛选维度正确；SSE 收到事件；题解可见性正确。

- [ ] **Step 3: 运行前端验收与浏览器检查**

Run: `corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS；浏览器中列表/筛选/SSE/详情/提交页正常，Monaco 无外链。

- [ ] **Step 4: 全量检查**

```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./... && go vet ./... && gofmt -l .
cd ../judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' KAPOJ_DOCKER_TESTS=1 go test -count=1 -p 1 ./... && go vet ./... && gofmt -l .
cd ../.. && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web
```
Expected: 全绿。

- [ ] **Step 5: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: m1-c records, sse and submit page"
```

---

## Self-Review

**Spec coverage:** 4.6 的列表、筛选、单条记录、SSE 由 Task 1–3、Task 5 覆盖；4.3 的本人 AC 后题解由 Task 4 覆盖；M1-9 的 Monaco 提交页与记录页由 Task 5 覆盖；文档与验收由 Task 6 覆盖。

**Type consistency:** 服务端 `SubmissionListItem`/`SubmissionDetail`/`JudgeCaseResult` 与 OpenAPI 组件同名；SSE 事件字段 `submissionId` 在 judge publisher、server streamer、前端解析三处一致；Monaco 语言映射 `c/cpp/python3/java/go` 与提交语言一致。

**Review Focus mapping:** 可见性 → Task 1/2/4；筛选边界 → Task 1/2；SSE 生命周期 → Task 3/5；题解绕过 → Task 4；Monaco 销毁 → Task 5。
