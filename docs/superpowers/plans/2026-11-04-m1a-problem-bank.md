# M1-A：题库与测试数据 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 管理员可以创建、编辑、发布、下线题目并管理测试数据，用户可以浏览题目列表与详情（M1 三个子项目中的第一个，后两个是 B 提交与评测服务、C 测评记录与前端提交页）。

**Architecture:** 后端新增 `storage`（文件存储抽象，本地文件系统实现）、`dbtypes`（jsonb 列表类型）、`problem`（模型、仓储、服务、DTO）、`problemapi`（HTTP）。权限在路由层用 `RequireRole` 判定，可见性（未公开题目对非管理员等同不存在）在服务层判定。前端新增题库列表、详情、编辑页，题面用 md-editor-v3 编辑和渲染，渲染前统一过 HTML 净化。

**Tech Stack:** 沿用 M0 后端（Go + Gin + GORM + PostgreSQL 17 + Redis）；新增前端依赖 md-editor-v3、sanitize-html。

**Spec:** `DEVELOPMENT.md`（2.4、3.2、4.3、附录）；决策依据见 `docs/任务清单.md` M1 与本计划「已定的实现决策」。

## Global Constraints

- 后端技术栈固定为 Go + Gin + GORM + PostgreSQL + Redis，不得替换。
- 响应格式 `{ "code": 0, "message": "ok", "data": ... }`；分页参数 `page`、`page_size`（默认 20，最大 100），返回 `{ total, items }`。
- 接口 JSON 字段用 camelCase，数据库列用 snake_case；主键 bigint 自增；时间为 `timestamptz`，接口返回 RFC 3339。
- 业务接口挂在 `/api` 下，本计划路径省略前缀。
- 题目类型：`traditional`、`special`、`interactive`；数据库列名 `samples_json`、`tags_json`。
- 已有提交记录的题目不做物理删除，只能下线（`is_public=false`）。
- 题目管理权限：`problem_admin` 及以上（`super_admin` 总是放行；`contest_admin` 不能管理题目）。
- 用户内容在展示前做 XSS 处理（DEVELOPMENT.md 附录）。
- 默认请求体上限 1 MiB；上传接口单独放宽。
- 数据库相关测试用 `KAPOJ_TEST_DATABASE_URL=postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable go test -count=1 -p 1 ./...`；前端用 `corepack pnpm test:web`、`lint:web`、`build:web`。
- 日志不得记录题目的测试数据内容。

## 已定的实现决策

以下与规格不同或规格未定，需在 `DEVELOPMENT.md` 标注「实现差异」：

- **存储**：M1 用本地文件系统（`KAPOJ_STORAGE_DIR`，默认 `data`），通过 `storage.Storage` 接口抽象；MinIO 实现留到 M3/M5。评测服务（子项目 B）与后端共享同一目录。
- **题解可见性**：`solutionMd`、`solutionCode`、`checkerUploaded`、`createdBy` 只返回给题目管理员，规格要求的右侧「解题思路/解题代码」对普通用户暂不展示，子项目 C 起改为「本人已通过该题后可见」。提示 `hintMd` 对所有人可见。
- **新题默认下线**：创建时忽略 `isPublic`，一律为 false，通过更新接口发布。发布条件：至少有 1 个测试点；`special` 题必须已上传 checker。公开题目不允许删除最后一个测试点。
- **submit_count 的维护**：由提交创建时递增（子项目 B 在创建提交的同一事务中执行），`accept_count` 在判题通过时递增。「有提交记录」即 `submit_count > 0`，据此决定删除时下线还是物理删除。
- **难度**：整数 1–5（1 入门、2 普及、3 提高、4 省选、5 NOI）。
- **特判 checker**：约定为 C++ 源码（testlib 风格），上传后存储，由子项目 B 编译运行；本计划只负责上传与存储。
- **交互题**：允许创建与存储，不参与评测；提交时的拒绝逻辑在子项目 B。
- **关联比赛**：M2 之前页面显示「比赛功能将在 M2 上线」。
- **统计接口**：`GET /problems/:id/statistics` 先返回 `submitCount`、`acceptCount`、`acceptRate`，各状态分布在子项目 B 补充。

## 新增接口与错误码

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | /problems | 游客可访问 | 列表；参数 `keyword`、`tag`、`difficulty`、`includeHidden`（仅管理员有效）、`page`、`page_size` |
| POST | /problems | 题目管理员 | 创建，返回 201 |
| GET | /problems/:id | 游客可访问 | 详情；未公开且非管理员返回 40400 |
| PUT | /problems/:id | 题目管理员 | 更新（只写可编辑字段） |
| DELETE | /problems/:id | 题目管理员 | 返回 `{ "offlined": bool }` |
| GET | /problems/:id/statistics | 游客可访问 | `{ submitCount, acceptCount, acceptRate }` |
| GET | /problems/:id/testcases | 题目管理员 | 测试点元数据 |
| POST | /problems/:id/testcases | 题目管理员 | multipart：`input`、`output` 文件，`score`、`isSample` |
| PUT | /problems/:id/testcases/:tid | 题目管理员 | `{ score?, isSample? }` |
| POST | /problems/:id/testcases/reorder | 题目管理员 | `{ ids: [int64] }`，必须恰好是该题全部测试点的排列 |
| DELETE | /problems/:id/testcases/:tid | 题目管理员 | |
| POST | /problems/:id/checker | 题目管理员 | multipart：`file`，仅 `special` 题 |

新增错误码：`41300`（HTTP 413）请求体过大；`40903`（HTTP 409）题目标识已存在。测试点不存在、题目不存在都用 `40400`。

校验规则：`title` 1–100 字符；`slug` 先 trim 并转小写后须匹配 `^[a-z0-9][a-z0-9-]{1,62}$`；`statementMd` 非空且 ≤ 100000 字符；`inputFormatMd`、`outputFormatMd` ≤ 20000 字符；`hintMd`、`solutionMd` ≤ 50000 字符；`solutionCode` ≤ 65536 字节；`source` ≤ 200 字符；标签最多 10 个，trim 后去重，每个 1–20 字符；`samples` 最多 10 组，每个 `input`/`output` ≤ 16384 字节；`difficulty` 1–5；`timeLimitMs` 100–10000（为 0 时取 1000）；`memoryLimitMb` 16–1024（为 0 时取 256）；`type` 属于枚举。测试点：`input`/`output` 各 ≤ 16 MiB，`score` 0–1000（默认 10）；checker ≤ 256 KiB。上传请求体上限 40 MiB。

## Review Focus

1. **关键字含 `%`、`_`、`\`**：应按字面匹配，不能当通配符。由 Task 3 的 `TestListKeywordEscapesWildcards` 覆盖。
2. **未公开题目对游客和普通用户等同不存在**：详情、统计、测试点等所有按 id 访问的接口都返回 40400，不能 403 泄露存在性。由 Task 5 的 `TestHiddenProblemLooksNonexistent` 覆盖。
3. **题解、checker 信息不泄露给非管理员**：列表与详情的 JSON 中不得出现这些键。由 Task 4 的 `TestDetailDTOHidesManagerFields` 与 Task 5 的 `TestDetailHidesSolutionFromUsers` 覆盖。
4. **上传格式错误或超限**：缺少文件字段、单个文件超过 16 MiB、请求体超过 40 MiB，分别返回 40001、40001、41300，且不留下孤立文件或数据库行。由 Task 4 的 `TestAddTestcaseRejectsOversizeWithoutResidue` 与 Task 5 的 `TestUploadErrors` 覆盖。
5. **并发上传测试点**：8 个并发上传得到互不重复的 `sortOrder` 且无 500。由 Task 3 的 `TestAddTestcaseConcurrentSortOrder` 覆盖。

---

### Task 1: 公共基础设施

**Files:**
- Create: `apps/server/internal/dbtypes/list.go`、`list_test.go`
- Create: `apps/server/internal/httpx/bind.go`、`bind_test.go`、`page.go`、`page_test.go`
- Create: `apps/server/internal/middleware/body.go`、`body_test.go`
- Modify: `apps/server/internal/httpx/errors.go`（新增两个错误）、`apps/server/internal/config/config.go`、`config_test.go`
- Modify: `apps/server/internal/auth/handler.go`、`apps/server/internal/userapi/handler.go`、`handler_test.go`（改用 httpx 的新函数，删除私有 `bind`、`parsePage`）
- Modify: `apps/server/internal/testutil/pg.go`、`apps/server/internal/router/router.go`、`router_test.go`
- Modify: `apps/server/cmd/server/main.go`（不在本任务接线存储，仅保证编译）

**Interfaces:**
- Produces: `dbtypes.List[T any] []T`，方法 `Value() (driver.Value, error)`（nil 写为 `[]`）、`Scan(src any) error`（接受 `[]byte`/`string`/nil，nil 得到空切片）、`GormDataType() string` 返回 `"jsonb"`。
- Produces: `httpx.BindJSON[T any](c *gin.Context) (T, bool)`：JSON 无效返回 `ErrBadRequest`，请求体超过 `MaxBytesReader` 限制返回 `ErrPayloadTooLarge`；失败时已写响应，返回 false。
- Produces: `httpx.ParsePage(rawQuery string) (page, size int)`，规则与 M0 的 `parsePage` 相同（默认 1/20，大小最大 100）。
- Produces: `httpx.ErrPayloadTooLarge`（413, 41300, "请求体过大"）、`httpx.ErrSlugTaken`（409, 40903, "题目标识已存在"）。
- Produces: `middleware.MaxBody(n int64) gin.HandlerFunc`。
- Produces: `config.Config.StorageDir string`，环境变量 `KAPOJ_STORAGE_DIR`，默认 `data`。
- Produces: `testutil.PG(t)` 改为清空 public 模式下全部表（`TRUNCATE ... RESTART IDENTITY CASCADE`），以后新表无需改它。

- [ ] **Step 1: 写失败测试**
  - `list_test.go`：`TestListValueNilIsEmptyArray`、`TestListScanBytesAndString`、`TestListScanNilIsEmpty`、`TestListScanInvalidJSON`（返回 error）。
  - `bind_test.go`：`TestBindJSONMalformed`（400/40000）、`TestBindJSONOversize`（用 `http.MaxBytesReader` 限制 10 字节，返回 413/41300）、`TestBindJSONOK`。
  - `page_test.go`：把 `userapi` 里的 `TestParsePageClamps` 原样移来。
  - `body_test.go`：`TestMaxBodyRejectsOversize`（读取返回 `*http.MaxBytesError`）、`TestMaxBodyAllowsWithinLimit`。
  - `config_test.go`：`TestLoadStorageDirDefault`（`data`）、`TestLoadStorageDirFromEnv`。
  - `router_test.go`：把 `TestRouterRejectsOversizedBody` 的期望改为 HTTP 413、`code` 41300。
- [ ] **Step 2: 运行 `go test ./...`（带数据库变量）**，Expected：编译失败（未定义符号）。
- [ ] **Step 3: 实现上述 Interfaces**；`auth`、`userapi` 改用 `httpx.BindJSON` 与 `httpx.ParsePage` 并删除私有实现；`router.go` 里私有 `limitBody` 改为调用 `middleware.MaxBody`（行为不变，仍全局 1 MiB）；`testutil.PG` 用 `SELECT tablename FROM pg_tables WHERE schemaname='public'` 拼出表名清空。
- [ ] **Step 4: 运行全部后端测试**，Expected：全部 PASS，且 `go vet ./...` 无输出。
- [ ] **Step 5: Commit** `feat(server): shared plumbing for problem bank`

### Task 2: 文件存储

**Files:**
- Create: `apps/server/internal/storage/storage.go`、`local.go`、`local_test.go`

**Interfaces:**
- Produces: 哨兵错误 `storage.ErrNotFound`、`storage.ErrTooLarge`、`storage.ErrInvalidKey`。
- Produces: `storage.Storage` 接口：`Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (int64, error)`、`Open(ctx context.Context, key string) (io.ReadCloser, error)`、`Delete(ctx context.Context, key string) error`（幂等）。
- Produces: `storage.NewLocal(root string) (*Local, error)`（目录不存在则创建），`*Local` 实现 `Storage`。键为 `/` 分隔的相对路径。

- [ ] **Step 1: 写失败测试** `local_test.go`：`TestPutOpenRoundTrip`（返回字节数等于内容长度）、`TestPutCreatesNestedDirs`、`TestPutOverwrites`、`TestPutTooLargeLeavesNothing`（超过 `maxBytes` 返回 `ErrTooLarge`，目标文件不存在，存储根目录下无残留临时文件）、`TestOpenMissing`（`ErrNotFound`）、`TestDeleteIdempotent`、`TestInvalidKeys`（表驱动：`""`、`"../x"`、`"/abs"`、`"a/../../b"`、`"a\\b"`、含 NUL；`Put`、`Open`、`Delete` 都返回 `ErrInvalidKey`，且根目录之外没有创建任何文件）。
- [ ] **Step 2: 运行 `go test ./internal/storage/...`**，Expected：编译失败。
- [ ] **Step 3: 实现 `Local`**：先写入同目录临时文件，校验大小后 `os.Rename` 到目标，保证读者不会看到半个文件；用 `io.LimitReader(r, maxBytes+1)` 检测超限。键校验用 `path.Clean` 比较加字符黑名单，并确认拼接后的路径仍在根目录内。
- [ ] **Step 4: 运行测试**，Expected：PASS。
- [ ] **Step 5: Commit** `feat(server): local file storage`

### Task 3: 题目模型与仓储

**Files:**
- Create: `apps/server/internal/problem/model.go`、`repository.go`、`repository_test.go`
- Modify: `apps/server/internal/db/db.go`（迁移加入 `Problem`、`Testcase`）

**Interfaces:**
- Consumes: `dbtypes.List`（Task 1）。
- Produces: `problem.Type`（`TypeTraditional`、`TypeSpecial`、`TypeInteractive`）、`problem.Sample{Input, Output string}`（JSON 键 `input`、`output`）。
- Produces: `problem.Problem`，字段与列名按 DEVELOPMENT.md 3.2：`ID int64`、`Title`、`Slug`（唯一索引 `uniq_problems_slug`）、`StatementMD`、`InputFormatMD`、`OutputFormatMD`、`Samples dbtypes.List[Sample]`（列 `samples_json`）、`HintMD`、`SolutionMD`、`SolutionCode`、`Type`、`Tags dbtypes.List[string]`（列 `tags_json`，GIN 索引）、`Difficulty int`、`TimeLimitMS int`、`MemoryLimitMB int`、`CheckerPath string`、`IsPublic bool`、`Source string`、`SubmitCount int`、`AcceptCount int`、`CreatedBy int64`、`CreatedAt`、`UpdatedAt`；`Testcases []Testcase`（外键 `problem_id`，`OnDelete:CASCADE`，`json:"-"`）。
- Produces: `problem.Testcase{ID, ProblemID int64; InputFile, OutputFile string; InputSize, OutputSize int64; Score int; IsSample bool; SortOrder int}`，`(problem_id, sort_order)` 唯一索引 `uniq_testcases_problem_order`。
- Produces: 哨兵错误 `problem.ErrNotFound`、`ErrSlugTaken`、`ErrTestcaseNotFound`、`ErrBadOrder`。
- Produces: `problem.Filter{Keyword, Tag string; Difficulty int; IncludeHidden bool}`、`problem.DeleteResult{Offlined bool; FileKeys []string}`。
- Produces: `problem.Repository` 接口与 `problem.NewGormRepository(*gorm.DB) Repository`：
  - `Create(ctx, *Problem) error`
  - `FindByID(ctx, id int64) (*Problem, error)`
  - `List(ctx, f Filter, offset, limit int) ([]Problem, int64, error)`（按 id 升序）
  - `Update(ctx, *Problem) error`：只写可编辑列（title、slug、三个格式/题面列、samples_json、hint_md、solution_md、solution_code、type、tags_json、difficulty、time_limit_ms、memory_limit_mb、is_public、source、updated_at），不写 checker_path、两个计数、created_by
  - `SetCheckerPath(ctx, id int64, path string) error`
  - `Delete(ctx, id int64) (DeleteResult, error)`：`submit_count = 0` 时物理删除并返回所有测试点文件键与 checker 路径，否则置 `is_public=false` 并返回 `Offlined: true`
  - `AddTestcase(ctx, *Testcase) error`：在事务内对题目行加 `FOR UPDATE`，`SortOrder` 取现有最大值加 1
  - `ListTestcases(ctx, problemID int64) ([]Testcase, error)`（按 `sort_order`）
  - `FindTestcase(ctx, problemID, id int64) (*Testcase, error)`
  - `UpdateTestcase(ctx, problemID, id int64, score *int, isSample *bool) error`
  - `DeleteTestcase(ctx, problemID, id int64) (*Testcase, error)`（返回被删记录以便清理文件）
  - `ReorderTestcases(ctx, problemID int64, ids []int64) error`：`ids` 必须恰好是该题全部测试点，否则 `ErrBadOrder`；事务内先整体平移 `sort_order`（加 1000000）再写最终值，避免唯一约束冲突
  - `CountTestcases(ctx, problemID int64) (int, error)`

- [ ] **Step 1: 写失败的集成测试** `repository_test.go`（用 `testutil.PG`）：`TestCreateAndFindRoundTrip`（标签与样例往返，空标签存为 `[]`）、`TestCreateDuplicateSlugMapsToErr`、`TestListFilters`（关键字不区分大小写且同时匹配标题与 slug、标签精确匹配 `a` 不匹配 `ab`、难度、`IncludeHidden=false` 排除未公开）、`TestListPaginates`（含 total）、`TestListKeywordEscapesWildcards`（含 `%`、`_`、`\` 的关键字只匹配字面量）、`TestUpdateKeepsProtectedColumns`（先直接改库把 checker_path、submit_count、created_by 设为非零，再用旧对象 `Update`，这三列保持不变）、`TestAddTestcaseAssignsSequentialOrder`、`TestAddTestcaseConcurrentSortOrder`（8 个 goroutine 并发添加，全部成功且 sortOrder 恰为 1..8）、`TestReorderTestcases`（交换两个位置不触发唯一约束；传入缺项、多项、重复项返回 `ErrBadOrder`）、`TestTestcaseOperationsAreScopedToProblem`（用题目 B 的 problemID 操作题目 A 的测试点得到 `ErrTestcaseNotFound`）、`TestDeleteWithoutSubmissionsCascades`（返回的 `FileKeys` 包含全部测试点输入输出文件与 checker，测试点行被级联删除）、`TestDeleteWithSubmissionsOfflines`（`submit_count>0` 时保留测试点，`is_public` 变 false）、`TestDeleteMissing`（`ErrNotFound`）。
- [ ] **Step 2: 运行 `go test -p 1 ./internal/problem/...`（带数据库变量）**，Expected：编译失败。
- [ ] **Step 3: 实现模型、迁移与仓储**：重复 slug 通过 `*pgconn.PgError` 的 `23505` 加约束名映射；关键字转义 `\`、`%`、`_` 后用 `ILIKE ? ESCAPE '\'`；标签过滤用 `tags_json @> ?::jsonb`。
- [ ] **Step 4: 运行测试**，Expected：PASS。
- [ ] **Step 5: Commit** `feat(server): problem model and repository`

### Task 4: 题目服务与 DTO

**Files:**
- Create: `apps/server/internal/problem/validate.go`、`validate_test.go`、`service.go`、`service_test.go`、`dto.go`、`dto_test.go`

**Interfaces:**
- Consumes: `problem.Repository`（Task 3）、`storage.Storage`（Task 2）、`httpx.ErrValidation`、`httpx.ErrNotFound`、`httpx.ErrSlugTaken`、`httpx.Page`。
- Produces: `problem.Viewer{CanManage bool}`（零值表示游客或普通用户）。
- Produces: `problem.Input`（JSON 键 camelCase，字段 `Title, Slug, StatementMD, InputFormatMD, OutputFormatMD, Samples []Sample, HintMD, SolutionMD, SolutionCode, Type, Tags []string, Difficulty, TimeLimitMS, MemoryLimitMB, IsPublic, Source`），`ListQuery{Keyword, Tag string; Difficulty int; IncludeHidden bool}`，`TestcaseUpload{Input, Output io.Reader; Score int; IsSample bool}`，`Stats{SubmitCount, AcceptCount int; AcceptRate float64}`。
- Produces: `problem.NewService(repo Repository, store storage.Storage) *Service`，方法：
  - `Create(ctx, createdBy int64, in Input) (*Problem, error)`
  - `Update(ctx, id int64, in Input) (*Problem, error)`
  - `Get(ctx, v Viewer, id int64) (*Problem, error)`
  - `List(ctx, v Viewer, q ListQuery, page, size int) (httpx.Page[ListItem], error)`
  - `Delete(ctx, id int64) (offlined bool, err error)`
  - `Statistics(ctx, v Viewer, id int64) (Stats, error)`
  - `AddTestcase(ctx, id int64, up TestcaseUpload) (*Testcase, error)`
  - `ListTestcases(ctx, id int64) ([]TestcaseDTO, error)`
  - `UpdateTestcase(ctx, id, tid int64, score *int, isSample *bool) error`
  - `DeleteTestcase(ctx, id, tid int64) error`
  - `ReorderTestcases(ctx, id int64, ids []int64) error`
  - `UploadChecker(ctx, id int64, r io.Reader) error`
- Produces: `problem.ListItem`、`problem.DetailDTO`、`problem.TestcaseDTO`（`id, sortOrder, score, isSample, inputSize, outputSize`）；`problem.ToListItem(*Problem) ListItem`、`problem.ToDetail(p *Problem, manage bool) DetailDTO`。`DetailDTO` 的 `solutionMd`、`solutionCode`、`checkerUploaded`、`createdBy` 用指针加 `omitempty`，`manage=false` 时 JSON 中不出现这些键；`samples`、`tags` 恒为数组，不为 null。
- Produces: 存储键格式 `problems/<problemID>/testcases/<n>.in|.out`（`<n>` 为测试点 ID 之外的随机十六进制串，防止覆盖）、`problems/<problemID>/checker.cpp`。

- [ ] **Step 1: 写失败测试**
  - `validate_test.go`：对上面「校验规则」逐条取边界值（slug 先转小写后通过、`A` 开头的大写 slug 被规范化、`-ab` 被拒；标签去重与 trim、第 11 个标签被拒；`samples` 第 11 组被拒；`timeLimitMs` 99 与 10001 被拒，0 取默认；`memoryLimitMb` 15 与 1025 被拒）。
  - `dto_test.go`：`TestDetailDTOHidesManagerFields`（序列化 JSON，`manage=false` 时不含 `solutionMd`、`solutionCode`、`checkerUploaded`、`createdBy`，`manage=true` 时含）、`TestDTOArraysNeverNull`。
  - `service_test.go`（用 `testutil.PG` 与 `t.TempDir()` 存储）：`TestCreateForcesHidden`（输入 `IsPublic=true` 仍为 false）、`TestCreateDuplicateSlug`（40903）、`TestGetHiddenAsNonManager`（`ErrNotFound` 对应 40400，管理员可读）、`TestListHidesNonPublicUnlessManager`（`IncludeHidden` 对非管理员无效）、`TestPublishRequiresTestcase`（先失败，加测试点后成功）、`TestPublishSpecialRequiresChecker`、`TestUpdateDoesNotTouchProtectedColumns`、`TestAddTestcaseStoresFilesAndRow`（存储里有两个文件，行里的大小正确）、`TestAddTestcaseRejectsOversizeWithoutResidue`（输入 16 MiB + 1 字节返回 40001，存储目录与数据库都无残留）、`TestAddTestcaseMissingProblemCleansFiles`（题目不存在返回 40400，存储目录无残留）、`TestAddTestcaseScoreRange`（−1 与 1001 被拒）、`TestDeleteTestcaseRemovesFiles`、`TestCannotDeleteLastTestcaseOfPublicProblem`、`TestReorderRejectsBadIDs`（40001）、`TestUploadCheckerOnlyForSpecial`、`TestUploadCheckerSizeLimit`（256 KiB + 1 字节被拒且不覆盖旧文件）、`TestDeleteWithoutSubmissionsRemovesFiles`（存储里测试点与 checker 文件都被删）、`TestDeleteWithSubmissionsKeepsFiles`、`TestStatisticsAcceptRate`（无提交时为 0，不出现 NaN）。
- [ ] **Step 2: 运行 `go test -p 1 ./internal/problem/...`**，Expected：编译失败。
- [ ] **Step 3: 实现校验、服务与 DTO**：上传先写存储再写数据库，数据库失败则删除刚写入的文件；删除先改数据库，再尽力删文件，文件删除失败只记录日志（不含内容）。
- [ ] **Step 4: 运行测试**，Expected：PASS。
- [ ] **Step 5: Commit** `feat(server): problem service, validation and DTOs`

### Task 5: HTTP 接口、路由与 OpenAPI

**Files:**
- Create: `apps/server/internal/problemapi/handler.go`、`handler_test.go`
- Modify: `apps/server/internal/router/router.go`、`router_test.go`、`apps/server/cmd/server/main.go`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- Consumes: `problem.Service`（Task 4）、`middleware.Authenticate`、`middleware.RequireRole`、`middleware.CurrentClaims`、`middleware.MaxBody`、`httpx.BindJSON`、`httpx.ParsePage`、`storage.NewLocal`。
- Produces: `problemapi.RegisterRoutes(api gin.IRouter, uploads gin.IRouter, s *problem.Service)`：`api` 路由组已带 1 MiB 请求体限制，`uploads` 路由组带 40 MiB 限制，两者都已执行 `Authenticate`。接口见上表。
- Produces: `router.Deps` 新增字段 `Storage storage.Storage`；`router.New` 构造两个 `/api` 路由组并调用 `problemapi.RegisterRoutes`；`/auth/*` 保持在无 `Authenticate` 的组里。`main.go` 用 `storage.NewLocal(cfg.StorageDir)` 构造存储。

- [ ] **Step 1: 写失败测试** `handler_test.go`（走完整 `router.New`，PostgreSQL + miniredis + 临时存储目录；辅助函数负责注册用户、直接改库提升角色并签发令牌）：
  - `TestWriteEndpointsRequireProblemAdmin`：表驱动覆盖全部写接口，游客 401、普通用户与 `contest_admin` 403、`problem_admin` 与 `super_admin` 通过。
  - `TestCreateReturns201AndHidden`、`TestCreateValidationErrors`（40001）、`TestCreateDuplicateSlugReturns409`（`40903`）。
  - `TestListGuestSeesOnlyPublic`、`TestListIncludeHiddenOnlyForManagers`、`TestListFiltersAndPagination`（`page_size=500` 被截为 100，`total` 正确）。
  - `TestHiddenProblemLooksNonexistent`：对游客与普通用户，详情、统计返回 404/40400，测试点相关接口按权限规则返回 401/403（不泄露），非数字 id 返回 40400；管理员均可访问。
  - `TestDetailHidesSolutionFromUsers`：普通用户详情的 JSON 无四个管理字段，`hintMd` 存在；管理员详情有。
  - `TestPublishFlow`：创建 → 上传测试点 → 更新为公开 → 游客在列表与详情中可见。
  - `TestUploadTestcase`：成功返回测试点 DTO；`TestUploadErrors`：缺少 `input` 字段、缺少 `output` 字段、`score` 非数字、单个文件 16 MiB + 1 字节（40001）、整个请求体超过 40 MiB（413/41300），每种情况后存储目录与 `GET /testcases` 都无残留。
  - `TestTestcaseUpdateReorderDelete`、`TestCheckerUpload`（特判题成功，传统题 40001）。
  - `TestDeleteProblemOfflinesWhenItHasSubmissions`（直接改库设 `submit_count`）：返回 `{"offlined":true}`；无提交时返回 `{"offlined":false}` 且之后 GET 为 404。
  - `router_test.go`：`TestJSONEndpointStillLimitedTo1MiB`（对 `/api/problems` 发送超过 1 MiB 的 JSON，返回 413/41300）。
- [ ] **Step 2: 运行测试**，Expected：编译失败。
- [ ] **Step 3: 实现 handler 与路由**；`Viewer.CanManage` 由 `claims.Role` 是否为 `problem_admin` 或 `super_admin` 得出；`includeHidden` 的解析在 handler，生效与否由服务按 `Viewer` 决定；multipart 解析用 `c.Request.MultipartForm` 的流式文件而不是整体读入内存，文件上限由服务通过 `storage.Put` 的 `maxBytes` 执行。
- [ ] **Step 4: 更新 `openapi.yaml`**：新增上表全部接口与 `ProblemInput`、`ProblemListItem`、`ProblemDetail`、`Testcase`、`Stats` 等 schema，并在错误码说明中补充 41300、40903；运行 `corepack pnpm gen:api` 确认能生成类型。
- [ ] **Step 5: 运行全部后端测试与 `go vet ./...`、`gofmt -l .`**，Expected：全部通过且无输出。
- [ ] **Step 6: Commit** `feat(server): problem and testcase http api`

### Task 6: 前端基础设施

**Files:**
- Modify: `apps/web/package.json`（依赖 `md-editor-v3`、`sanitize-html`、`@types/sanitize-html`）
- Create: `apps/web/src/utils/sanitize.ts`、`sanitize.spec.ts`、`apps/web/src/utils/difficulty.ts`、`difficulty.spec.ts`、`apps/web/src/utils/problemQuery.ts`、`problemQuery.spec.ts`
- Create: `apps/web/src/api/problems.ts`、`apps/web/src/views/ForbiddenView.vue`
- Modify: `apps/web/src/api/types.ts`、`router/guard.ts`、`guard.spec.ts`、`router/index.ts`、`layouts/DefaultLayout.vue`

**Interfaces:**
- Consumes: 由 Task 5 生成的 `api/schema.d.ts`。
- Produces: `sanitizeHtml(html: string): string`：去掉 `script`、`iframe`、`object`、事件属性（如 `onerror`）、`javascript:` 链接；保留常见排版标签、表格、`code`/`pre`/`span` 的 `class`、`img` 的 http(s) 地址；`a` 标签强制加 `target="_blank"` 与 `rel="noopener noreferrer"`。
- Produces: `difficultyLabel(d: number): string`（1 入门、2 普及、3 提高、4 省选、5 NOI，其余「未知」）。
- Produces: `parseProblemQuery(q: Record<string, unknown>): ProblemQuery`、`toRouteQuery(q: ProblemQuery): Record<string, string>`，`ProblemQuery{keyword: string; tag: string; difficulty: number | null; page: number}`；非法值回退默认，`page` 至少为 1，往返一致且默认值不写入查询串。
- Produces: `hasRole(role: Role | null, allowed: Role[]): boolean`（`super_admin` 总是通过，与服务端一致），`authGuard(to, state: { isLoggedIn: boolean; role: Role | null }): true | RouteLocationRaw`（原有两条规则不变；`meta.roles` 存在且 `hasRole` 为假时跳转 `{ name: 'forbidden' }`，未登录仍优先跳登录页）。
- Produces: `api/problems.ts`：`listProblems(q)`、`getProblem(id)`、`createProblem(input)`、`updateProblem(id, input)`、`deleteProblem(id)`、`getStatistics(id)`、`listTestcases(id)`、`uploadTestcase(id, form)`、`updateTestcase(...)`、`reorderTestcases(id, ids)`、`deleteTestcase(id, tid)`、`uploadChecker(id, file)`，类型取自 schema。

- [ ] **Step 1: 写失败测试**
  - `sanitize.spec.ts`：`removes script tags`、`removes event handler attributes`、`removes javascript urls`、`removes iframes`、`keeps code classes`、`adds rel noopener to links`、`drops non-http image sources`。
  - `difficulty.spec.ts`、`problemQuery.spec.ts`（往返、非法值、`page=0`、`difficulty=9`）。
  - `guard.spec.ts`：改用新签名，并新增 `redirects users lacking the role to forbidden`、`lets super_admin through any role route`、`login redirect wins over role check`。
- [ ] **Step 2: 运行 `corepack pnpm test:web`**，Expected：失败（模块不存在）。
- [ ] **Step 3: 安装依赖并实现**；`router/index.ts` 的 `beforeEach` 改传新状态，`RouteMeta` 增加 `roles?: Role[]`，新增 `forbidden` 路由；`DefaultLayout` 去掉「题库」的 `milestone`，使其可点击；`api/http.ts` 的 `Http` 增加 `delete<T>` 与 `postForm<T>`（`multipart/form-data`），并为其补一条单元测试 `sends FormData without a JSON content type`。
- [ ] **Step 4: 运行 `corepack pnpm test:web`、`lint:web`、`build:web`**，Expected：全部通过。
- [ ] **Step 5: Commit** `feat(web): problem api client, sanitizer and role-aware guard`

### Task 7: 前端页面

**Files:**
- Create: `apps/web/src/views/ProblemListView.vue`、`ProblemDetailView.vue`、`ProblemEditView.vue`
- Create: `apps/web/src/components/MarkdownView.vue`、`TestcaseManager.vue`、`SampleEditor.vue`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Consumes: Task 6 的 `api/problems.ts`、`sanitizeHtml`、`difficultyLabel`、`parseProblemQuery`、`toRouteQuery`、`hasRole`。
- Produces: 路由 `problems`（`/problems`）、`problem-detail`（`/problems/:id`）、`problem-create`（`/problems/new`，`requiresAuth` 且 `roles: ['problem_admin']`）、`problem-edit`（`/problems/:id/edit`，同样的限制）。`/problems/new` 必须排在 `/problems/:id` 之前。
- Produces: `MarkdownView`（属性 `source: string`）：用 md-editor-v3 的预览组件渲染，并把 `sanitizeHtml` 作为净化函数传入。

页面行为：
- 列表页：搜索框（关键字）、标签输入、难度下拉；筛选条件与页码同步到 URL 查询串（`parseProblemQuery`/`toRouteQuery`），刷新页面保持状态；表格列为编号、标题、难度、标签、通过数/提交数；管理员额外看到「新建题目」按钮和「显示未公开」开关；未公开题目带「未公开」标记。
- 详情页：左侧题面（题面、输入格式、输出格式、样例，样例带复制按钮），右侧标签页：提示、解题思路与解题代码（仅接口返回这些字段时显示）、提交统计、关联比赛（文案「比赛功能将在 M2 上线」）；未公开题目对管理员显示横幅；404 显示「题目不存在」；管理员可见「编辑」按钮。「提交代码」入口留到子项目 C，本页暂不放。
- 编辑页：题目基本信息表单、md-editor-v3 编辑题面等 Markdown 字段、样例编辑、标签（可新建）、难度、限制、类型、公开开关、保存；特判题显示 checker 上传；编辑已有题目时显示 `TestcaseManager`（上传输入输出文件对、可编辑分数与样例标记、上下移动调整顺序、删除）；服务端校验错误显示在表单顶部；发布失败（缺测试点或 checker）显示服务端原文。

- [ ] **Step 1:** 实现 `MarkdownView` 并写组件测试 `MarkdownView.spec.ts`：`does not render script tags from markdown`、`does not render onerror handlers`（用 `@vue/test-utils` 挂载，断言 DOM 中无 `script` 元素和 `onerror` 属性）。运行并确认先失败再通过。
- [ ] **Step 2:** 实现列表页、详情页、编辑页与组件，修改路由。
- [ ] **Step 3: 运行 `corepack pnpm test:web`、`lint:web`、`build:web`**，Expected：全部通过。
- [ ] **Step 4: 手工联调**：启动后端（空闲端口）与前端，用管理员账号创建题目、上传测试点、发布，再用游客账号浏览；记录结果。如果浏览器工具可用，加载 `browser` skill 后截图确认列表、详情、编辑页没有布局错误。
- [ ] **Step 5: Commit** `feat(web): problem list, detail and edit pages`

### Task 8: 文档与端到端验收

**Files:**
- Modify: `DEVELOPMENT.md`（3.2、4.3、2.4 错误码表）、`docs/任务清单.md`、`docs/开发日志.md`、`README.md`、`运行手册.md`、`.env.example`

- [ ] **Step 1: 端到端验收**（真实后端 + PostgreSQL + Redis + 临时存储目录，空闲端口）：用 curl 依次：注册用户并在库里提升为 `problem_admin` 后登录；创建题目（确认默认下线）；未上传测试点就发布（被拒）；上传两个测试点；发布成功；游客列表与详情可见且没有题解字段；下线后游客访问返回 404；`special` 题未上传 checker 发布被拒；`submit_count` 置 1 后删除只下线；清零后删除并确认存储目录里的文件消失。
- [ ] **Step 2: 按 `skills/dev-docs-sync` 更新文档**：`DEVELOPMENT.md` 3.2 与 4.3 补「实现差异」与实现状态（含上面「已定的实现决策」全部条目），2.4 错误码表加 41300、40903；`docs/任务清单.md` 把 M1 拆为子项目 A/B/C，完成 M1-1（改为本地文件系统）至 M1-4；`docs/开发日志.md` 追加记录；`README.md`、`运行手册.md`、`.env.example` 补 `KAPOJ_STORAGE_DIR` 与上传相关说明。
- [ ] **Step 3: 全量检查**：后端全部测试、`go vet`、`gofmt -l`、golangci-lint；前端测试、lint、build。
- [ ] **Step 4: Commit** `docs: m1-a progress and implementation notes`
