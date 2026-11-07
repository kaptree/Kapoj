# M5-6 HydroOJ 题包导入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 支持上传 HydroOJ zip 题包，按题目文件夹导入 title/tag/题面/测试数据，题目默认下线且创建者为上传者；题目详情公开显示创建者并可跳转主页。

**Architecture:** `problem` 域新增作者摘要查询与 `ImportHydro` 解析/导入服务；`problemapi` 新增独立 210MB body 的导入路由（权限 `problem.create`），逐题部分成功并做 best-effort 审计。前端新增 `/problems/import` 上传页、题目列表入口、详情创建者链接；OpenAPI/生成类型同步 `ProblemDetail.author` 与导入结果。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、archive/zip、go.yaml.in/yaml/v3、Vue 3、TypeScript、PrimeVue 4、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-11-08-m5-6-hydro-import-design.md`

## Global Constraints

- 每个包含 `problem.yaml` 的目录是一道题；`problem.yaml` 只取 `title`/`tag`；`problem_zh.md`（缺失/空回退 `problem.md`）作为 `statement_md`。
- 测试数据：`testdata/` 下同名 `.in`/`.out` 成对，按 basename 数字优先再字典序，score=10；空文件允许；`gen.py`/`std.cpp` 等忽略。
- slug=`hydro-`+文件夹名净化（小写、非 `[a-z0-9-]`→`-`）；冲突/非法逐题失败，不覆盖。
- 导入题目 `is_public=false`、`created_by=上传者`、`source='HydroOJ'`、difficulty=1、time=1000ms、memory=256MB、traditional；默认下线。
- 限制：zip 文件 ≤200MB；路由 body 210MB；解压 ≤1GB；条目 ≤5000；题目 ≤100；单题测试点 ≤500；单文件 ≤16MB。ZIP/解析超限 40001；body 超限 413。
- 逐题部分成功：题包可解析且发现题目文件夹时返回 200，`imported`/`failed`；整个 zip 损坏/加密/无题目文件夹 → 40001。
- 权限：导入使用 `problem.create`，不新增权限码；编辑/发布沿用现有权限；每个成功题目 best-effort 审计 `problem.create`，target=`problem`。
- 作者：`GET /problems/:id` 返回可选 `author{id,username,nickname}`；`created_by<=0`/用户不存在省略；DB 错误 500；POST/PUT 可省略。邮箱不暴露。
- 不推送/不合并；不执行题包内脚本；不导入 `additional_file`；不改现有单题上传接口。
- 所有用户文案中文；PrimeVue 4；不新增 UI 依赖。

## Review Focus

1. **zip 安全**：路径穿越、绝对路径、符号链接、加密/损坏、解压炸弹、重复条目、条目/大小/题数限制；不得执行/落盘解压脚本。
2. **映射正确性**：title/tag/题面回退、slug 净化与冲突、测试点成对/排序/score、默认下线/creator/source/limits。
3. **部分成功与清理**：一题失败不影响其他题；失败题不留下题目/测试文件；重复导入不覆盖。
4. **作者契约**：GET 详情 author 可见/可空/DB 错误；隐藏题仍 404；POST/PUT 不因 author 变更破坏。
5. **前端导入**：文件选择/大小提示/上传/成功失败展示/编辑链接权限/列表入口；生成类型无手写重复。
6. **真实题包**：根目录 `Export.zip` 的 `6760` 能导入（求和、循环/DAI、10 组测试数据、下线、creator=上传者），重复导入冲突。
7. **大 body/413**：200MB 文件校验与 210MB body 限制的边界，错误码符合契约。

---

### Task 1: 题目详情公开创建者摘要（后端）

**Files:**
- Modify: `apps/server/internal/problem/dto.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problem/dto_test.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Test: 上述测试文件

**Interfaces:**
- Produces:
  - `problem.AuthorSummary{ID int64; Username string; Nickname string}`（JSON `id/username/nickname`）。
  - `DetailDTO.Author *AuthorSummary `json:"author,omitempty"``。
  - `Repository.FindAuthor(ctx, userID) (*AuthorSummary, error)`：用户不存在返回 `(nil,nil)`；DB 错误返回 error。
  - `Service.Author(ctx, userID) (*AuthorSummary, error)`：userID≤0 返回 nil；DB 错误透传。
  - `ToDetail(p *Problem, includeSolution bool, author *AuthorSummary) DetailDTO`（或等价签名）。
  - `GET /problems/:id` 填充 author；`POST/PUT` 传 nil。
- Consumes: 现有 `Problem`/`ToDetail`/handler 的 `get/create/update`。

- [ ] **Step 1: 写失败测试**

`dto_test.go`：
```go
func TestDetailAuthorOptional(t *testing.T)
// ToDetail(..., author) 含 author；nil 时 JSON 不含 author 键
```

`problemapi/handler_test.go`：
```go
func TestProblemDetailIncludesPublicAuthor(t *testing.T)
// GET /problems/:id 游客也返回 author{id,username,nickname}，不含 email
func TestProblemDetailOmitsAuthorForMissingCreator(t *testing.T)
// created_by=0 不返回 author，状态 200（不 500）
func TestProblemDetailAuthorDBError(t *testing.T)
// 仅当查询本身失败时 500（可用 fake repo 或关闭 DB 模拟）
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ -run 'TestDetailAuthor|TestProblemDetailIncludesPublicAuthor|TestProblemDetailOmitsAuthor|TestProblemDetailAuthorDBError' -v`

Expected: FAIL。

- [ ] **Step 3: 实现作者摘要**

- `dto.go` 增加 `AuthorSummary`/`DetailDTO.Author`；`ToDetail` 增加参数并设置。
- `repository.go` 接口/实现 `FindAuthor`：`SELECT id,username,nickname FROM users WHERE id=?`；无行返回 nil,nil。
- `service.go` `Author`。
- `handler.get` 读取 `p.CreatedBy` 的作者并传入 `ToDetail`；create/update 传 nil。
- 不改 `CreatedBy` 现有可见性（仅管理/题解可见者）。
- 不实现导入/OpenAPI/前端。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/problem internal/problemapi && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ ./internal/router/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi
git commit -m "feat(problem): expose public author summary"
```

---

### Task 2: HydroOJ 解析/导入服务与 API

**Files:**
- Create: `apps/server/internal/problem/import_hydro.go`
- Create: `apps/server/internal/problem/import_hydro_test.go`
- Create: `apps/server/internal/problemapi/import_hydro_handler.go`
- Create: `apps/server/internal/problemapi/import_hydro_handler_test.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/go.mod` / `go.sum`（`go.yaml.in/yaml/v3` 转直接依赖）
- Test: 上述测试文件

**Interfaces:**
- Produces:
  - `HydroImportResult{Imported []HydroImportedProblem; Failed []HydroImportFailure}`（JSON `imported/failed`）。
  - `HydroImportedProblem{ID int64; Title,Slug string; TestcaseCount int}`。
  - `HydroImportFailure{Folder,Title,Error string}`。
  - `Service.ImportHydro(ctx context.Context, createdBy int64, r io.ReaderAt, size int64) (*HydroImportResult, error)`。
  - `POST /api/problems/import/hydro`（multipart `file`，`problem.create`，body 210MB）。
  - `problemapi.RegisterRoutes(api, problemJSON, uploads, imports gin.IRouter, ...)`。
  - `router` 新 `imports` 组：`MaxBody(210<<20)`。
- Consumes: Task 1 之前的 `Service.Create/AddTestcase/Delete`、`storage.Storage`、`problem.Input`、`maxTestcaseBytes`。

- [ ] **Step 1: 写失败测试**

`import_hydro_test.go`（用 `archive/zip` 在内存构造题包）：
```go
func TestImportHydroSamplePackage(t *testing.T)
// 6760/problem.yaml(title 求和,tag 循环/DAI)、problem_zh.md、1..10.in/out、gen.py/std.cpp
// 导入后：title/tags/statement/source/isPublic=false/createdBy；10 testcases score=10/顺序
func TestImportHydroMultipleAndPartialFailure(t *testing.T)
// 两题：一好一缺 problem_zh.md；好题成功，坏题 failed；坏题无残留
func TestImportHydroSlugConflictDoesNotOverwrite(t *testing.T)
func TestImportHydroValidation(t *testing.T)
// 无题目文件夹/损坏 zip/缺 testdata/in 不成对/非法 slug/路径穿越/符号链接/超限/伪造解压大小
func TestImportHydroRollbackOnZipLevelError(t *testing.T)
// 先成功导入一题，再触发实际解压超限 -> 40001 且首题/测试文件回滚、无残留
func TestImportHydroCleanupOnTestcaseFailure(t *testing.T)
// 测试文件读取失败后题目与已写文件被清理
```

`import_hydro_handler_test.go`：
```go
func TestImportHydroHandlerPermissions(t *testing.T)
// 游客 401、无 problem.create 403、有权限 200
func TestImportHydroHandlerResultAndAudit(t *testing.T)
// multipart 上传、imported/failed JSON、审计 problem.create target problem id
func TestImportHydroHandlerBodyLimit(t *testing.T)
// >210MB body -> 413；200MB 文件校验 -> 40001（可用小限制单测）
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ -run 'TestImportHydro' -v`

Expected: FAIL。

- [ ] **Step 3: 实现解析/导入/API**

- `import_hydro.go`：
  - 常量 `maxHydroZipBytes=200<<20`、`maxHydroUncompressed=1<<30`、`maxHydroEntries=5000`、`maxHydroProblems=100`、`maxHydroTestcases=500`、`maxHydroTestcaseBytes=16<<20`。
  - `zip.NewReader`；遍历条目做 `path.Clean`/绝对路径/`..`/symlink/加密/重复/大小校验；统计 header 解压总量，并用实际读取的共享 `io.LimitReader`/计数防止伪造 `UncompressedSize64`；YAML/题面单文件读取上限 1MB/100k 字符。
  - 找 `problem.yaml` 目录；每个目录读取 YAML（`Title string; Tags []string `yaml:"tag"``），题面回退，测试点成对排序；构建 `problem.Input`；调用 `s.Create` → `s.AddTestcase`（stream reader，score 10）→ 失败 `s.Delete` 清理。
  - slug `hydro-` + sanitize(folder)；冲突/非法记录 failed。
  - 返回结果；无题目文件夹/整体非法返回 `httpx.ErrValidation`。若整包级错误发生在已导入部分题目之后（如实际解压超限），回滚本次已导入题目及文件后再返回 40001；可通过 `Options.MaxImportUncompressedBytes` 小上限在测试中验证。
- `import_hydro_handler.go`：
  - `parseMultipart(c)`；`formFile(c,"file")`；`fh.Size > maxHydroZipBytes` → 40001；`svc.ImportHydro(ctx, actorID, f, fh.Size)`。
  - 逐题 `h.rbac.RecordAudit`（best-effort，`rbac.AuditMeta` from `CurrentAccess` + client IP/UA，action `problem.create`，target `problem`，id）。
  - 返回 `httpx.OK(result)`。
- `handler.go`：RegisterRoutes 增加 `imports` 参数；注册静态路由（在 `problemJSON`/`uploads` 之外）。
- `router.go`：`imports := r.Group("/api", Authenticate, MaxBody(maxImportBodyBytes))`，传参。
- `go.mod`：`go.yaml.in/yaml/v3` 直接依赖（`go mod tidy`）。
- 不改前端/OpenAPI/文档；不覆盖已有题目。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/problem internal/problemapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ ./internal/router/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/server/internal/router apps/server/go.mod apps/server/go.sum
git commit -m "feat(problem): import HydroOJ packages"
```

---

### Task 3: OpenAPI、导入页与详情创建者展示（前端）

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/problems.ts`
- Create: `apps/web/src/views/ProblemImportView.vue`
- Create: `apps/web/src/views/ProblemImportView.spec.ts`
- Modify: `apps/web/src/views/ProblemListView.vue` + spec
- Modify: `apps/web/src/views/ProblemDetailView.vue` + spec
- Modify: `apps/web/src/router/index.ts`
- Test: 上述 spec

**Interfaces:**
- Consumes: Task 1 `author`、Task 2 导入 API。
- Produces:
  - OpenAPI：`ProblemDetail.author` 可选 `ProblemAuthor`；`HydroImportResult`/`HydroImportedProblem`/`HydroImportFailure`；`POST /problems/import/hydro`。
  - `api/types.ts` 类型；`api/problems.ts` `importHydroProblems(file: File): Promise<HydroImportResult>`（`http.postForm`）。
  - 路由 `problem-import` → `/problems/import`，`requiresAuth` + `permission: PERM.problem.create`。
  - `ProblemImportView`：文件选择（`.zip`，≤200MB 提示）、上传 loading、成功表（标题/slug/测试点数，`problem.update` 时编辑链接）、失败表（文件夹/错误）、继续导入/返回题库。
  - 题目列表 `canCreate` 时“导入题包”按钮，链接导入页。
  - 题目详情 meta 区“创建者 <nickname>”链接 `/users/:id`；无 author 不显示；昵称回退 username。

- [ ] **Step 1: OpenAPI + 生成类型**

- 更新 `ProblemDetail.author`、导入 path/schemas；`corepack pnpm gen:api`；检查类型形状。

- [ ] **Step 2: 写失败前端测试**

`ProblemImportView.spec.ts`：
```ts
it('uploads a zip and renders imported/failed results')
it('shows edit links only with problem.update')
it('shows upload/API errors in Chinese')
it('rejects non-zip/too-large file before upload')
```
`ProblemListView.spec.ts`：`canCreate` 显示“导入题包”链接。
`ProblemDetailView.spec.ts`：author 渲染/链接/游客可见/无 author 不显示/昵称回退。

- [ ] **Step 3: 实现前端**

- 导入页使用原生 `<input type="file" accept=".zip">` + 选择文件展示 + 上传按钮；`importHydroProblems`；结果 `DataTable`/列表；错误 `Message`。
- 列表按钮/路由；详情作者 meta；样式移动端。
- 编辑链接条件 `auth.can(PERM.problem.update)`；导入页路由 meta。
- 不修改后端；不新增依赖。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api apps/web/src/views/ProblemImportView.vue apps/web/src/views/ProblemImportView.spec.ts apps/web/src/views/ProblemListView.vue apps/web/src/views/ProblemListView.spec.ts apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add HydroOJ import and problem author"
```

---

### Task 4: 文档、全量检查与真实 Export.zip 冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`
- Test: 全量后端、评测、前端；真实 `Export.zip`

**Interfaces:**
- Consumes: Task 1–3。
- Produces: M5-6 状态文档、UI-35 状态、真实导入冒烟记录。

- [ ] **Step 1: 文档**

- `DEVELOPMENT.md` §7：实现状态、格式/映射/限制/部分成功/作者摘要。
- `docs/任务清单.md`：M5-6 ✅；M5 进度说明（M5-4/5/7/8 仍未完成）。
- `docs/开发日志.md`：2026-11-08 M5-6 条目（实现、验证、Export.zip 冒烟）。
- `README.md`：当前进度 + 题包导入说明。
- `运行手册.md`：导入入口/权限/限制/常见错误。
- `docs/primevue-migration.md`：UI-35 标 ✅（题目导入页，无新依赖）。

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

- [ ] **Step 3: 真实 Export.zip 冒烟**

- 重建/重启本地后端为当前分支；使用有 `problem.create` 的临时角色/账号（或 super）登录。
- 通过 API 上传根目录 `Export.zip`：断言 imported 含 `hydro-6760`、title=求和、tags=循环/DAI、testcaseCount=10；数据库/API 确认 is_public=false、created_by=上传者、source=HydroOJ、10 组测试数据 score=10、题面 Markdown。
- 重复上传同一 zip：slug 冲突进入 failed，不覆盖。
- GET `/problems/:id` 返回 author{id,username,nickname}；题目详情页显示创建者链接；游客看一个既有公开题也能看到作者链接。
- 用题目管理员发布/编辑导入题目（确认测试数据可被 judge 读取/提交可选），然后清理导入题目/测试文件/审计；不删除既有数据。
- 浏览器如可用：`/problems/import` 上传 `Export.zip`（文件选择）、结果表、详情作者链接、移动端；否则记录 API 冒烟并说明浏览器延后。
- 清理临时账号/角色/题目/测试文件/审计/缓存；确认基线。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md
git commit -m "docs: record HydroOJ problem import"
```

---

## Self-Review

**Spec coverage:**

- 作者摘要/详情展示 → Task 1 + Task 3。
- zip 解析/限制/部分成功/导入服务/API → Task 2。
- OpenAPI/导入页/列表入口/创建者链接 → Task 3。
- 文档/全量检查/真实 Export.zip → Task 4。

**Type consistency:**

- `AuthorSummary`/`DetailDTO.Author` 在 Task 1 定义，Task 3 OpenAPI/frontend 复用。
- `HydroImportResult`/`Service.ImportHydro` 在 Task 2 定义，Task 3 前端类型/API 复用。
- `imports` 路由组在 Task 2 定义，router 传入。
- `problem-import` 路由名在 Task 3 定义，列表链接复用。

**Review Focus mapping:**

1. zip 安全 → Task 2 tests。
2. 映射正确性 → Task 2 tests + Task 4 Export.zip。
3. 部分成功/清理 → Task 2 tests。
4. 作者契约 → Task 1 tests + Task 3 tests。
5. 前端导入 → Task 3 tests。
6. 真实题包 → Task 4 smoke。
7. 大 body/413 → Task 2 handler tests。
