# Judge Settings and Testcase Limits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 管理员可在后台“系统 → 评测设置”调整 judge 运行时输出限制和测试数据输入/输出上传限制；server 与 judge 读取同一份配置，修改后新提交/新上传立即生效，默认值保持当前行为。

**Architecture:** 新增 `judge_settings` 单例表与 `judgesettings` 服务；server 的题目上传/导入路径通过 `problem.Service` 的限制 provider 读取动态值；judge 每次评测读取同一行并计算输出上限/标准答案读取上限；管理 API + 系统页负责编辑，修改写审计。

**Tech Stack:** Go 1.26、Gin、GORM/PostgreSQL、pgx（judge）；Vue 3.5、PrimeVue 4.5、Vitest、OpenAPI TypeScript 生成。

**Spec:** `docs/superpowers/specs/2026-11-21-judge-settings-design.md`

## Global Constraints

- `judge_settings` 单例 `id=1`；默认值必须与当前常量一致：
  - 输出下限 1 MiB
  - 输出余量 1 MiB
  - 最大输出 / 标准答案读取 16 MiB
  - 单个输入文件 16 MiB
  - 单个输出文件 16 MiB
  - 批量上传总量 200 MiB
  - 单题导入总量 200 MiB
- 范围：
  - 输出下限 1 KiB–64 MiB
  - 输出余量 0–64 MiB
  - 最大输出 1–64 MiB，且 ≥ 输出下限、≥ 输出余量
  - 单个输入 1–256 MiB
  - 单个输出 1–64 MiB，且 ≤ 最大输出
  - 批量总量 1–2048 MiB
  - 单题导入总量 1–2048 MiB
- 修改只影响后续提交/上传/导入；已有测试数据不追溯修改。
- judge 读取设置失败或表不存在时回退默认值并记录 warn；不得因设置读取失败让评测失败。
- 管理 API 使用 `system.settings.manage`；PUT 写审计 `system.settings.manage` / target `judge_settings`。
- 单测试点/批量上传外层 body 硬上限调整为 2 GiB + 16 MiB；服务层仍按设置精确校验。
- 题包 zip/解压设置继续使用现有 `problem_import_settings`，不迁移。
- 所有新错误文案中文；OpenAPI 先改，`corepack pnpm gen:api`。
- 后端 DB 测试用 `-p 1` 并 source 根 `.env`；judge 测试用 `env -i` + `KAPOJ_TEST_DATABASE_URL`。

## Review Focus

1. **默认兼容**：未创建/读取设置行时行为与当前完全一致（1/1/16/16/16/200/200）。测试 Task 1/4。
2. **judge 上限公式**：`max(floor, expected+slack)` 再受 `maxOutput` 限制；大输出题目 111 不再误判。测试 Task 4。
3. **上传链路一致**：单点、批量、Hydro、题包导入全部使用同一组输入/输出/总量限制；测试 Task 2/3。
4. **动态生效与安全**：修改后下一次提交/上传生效；body 硬上限不被绕过；读取失败回退默认。测试 Task 1/2/4。
5. **审计/权限/UI**：只有 `system.settings.manage` 可改，保存后重新读取；测试 Task 1/6。

---

## File Structure

**新增后端**

- `apps/server/internal/judgesettings/model.go` — `Settings` 模型、默认值、字节/MiB 常量。
- `apps/server/internal/judgesettings/settings.go` — `SettingsView`、`SettingsInput`、`SettingsService`、校验、懒创建、`TestcaseLimits`。
- `apps/server/internal/adminapi/judge_settings_handler.go` — GET/PUT handlers。
- 测试：`judgesettings/settings_test.go`、`adminapi/judge_settings_handler_test.go`。

**修改的后端**

- `apps/server/internal/db/db.go` — AutoMigrate `judgesettings.Settings`。
- `apps/server/internal/problem/service.go` — `TestcaseLimits`/provider、单测试点动态限制、上传 body。
- `apps/server/internal/problem/testcase_batch.go` — 批量动态限制。
- `apps/server/internal/problem/import_hydro.go` — Hydro 动态单文件/单题总量限制。
- `apps/server/internal/problem/package_import.go` — kapOJ 题包动态限制。
- `apps/server/internal/problemapi/handler.go` — 单测试点路由移到 `imports` 组；batch body 默认上限提高。
- `apps/server/internal/router/router.go` — 构造 `judgesettings`，注入 problem/admin；`maxImportBodyBytes` 提高。
- `apps/server/internal/adminapi/role_handler.go` — 注册 judge settings 路由/Option。
- `apps/judge/internal/model/model.go` — `JudgeSettings`。
- `apps/judge/internal/db/repository.go` — `JudgeSettings(ctx)`。
- `apps/judge/internal/pipeline/pipeline.go` — 读取设置、动态输出/答案上限。
- `apps/judge/internal/testutil/pg.go` — `judge_settings` 表/插入 helper。
- 测试：`problem/*_test.go`、`problemapi/*_test.go`、`judge/internal/{db,pipeline}/*_test.go`。

**前端**

- `apps/web/src/views/AdminJudgeView.vue`、`.spec.ts` — 系统评测设置页。
- `apps/web/src/router/index.ts`、`components/layout/AdminNav.vue`、`AdminNav.spec.ts` — 路由/导航。
- `apps/web/src/api/admin.ts`、`types.ts`、`schema.d.ts` — API/类型。
- `packages/api/openapi.yaml` — 契约。

---

### Task 1: `judge_settings` 模型、服务、迁移、管理 API

**Files:**
- Create: `apps/server/internal/judgesettings/model.go`, `settings.go`
- Create: `apps/server/internal/adminapi/judge_settings_handler.go`
- Modify: `apps/server/internal/db/db.go`, `apps/server/internal/adminapi/role_handler.go`, `apps/server/internal/router/router.go`, `apps/server/internal/rbac/audit.go`
- Test: `apps/server/internal/judgesettings/settings_test.go`, `apps/server/internal/adminapi/judge_settings_handler_test.go`

**Interfaces:**
```go
package judgesettings

const (
    DefaultOutputFloorBytes      int64 = 1 << 20
    DefaultOutputSlackBytes      int64 = 1 << 20
    DefaultMaxOutputBytes        int64 = 16 << 20
    DefaultMaxInputFileBytes     int64 = 16 << 20
    DefaultMaxOutputFileBytes    int64 = 16 << 20
    DefaultMaxBatchTotalBytes    int64 = 200 << 20
    DefaultMaxProblemTotalBytes  int64 = 200 << 20
    MiB int64 = 1 << 20
)

type Settings struct { /* id, seven byte fields, UpdatedBy, UpdatedAt */ }
func (Settings) TableName() string { return "judge_settings" }

type SettingsView struct {
    OutputFloorBytes     int64     `json:"outputFloorBytes"`
    OutputSlackBytes     int64     `json:"outputSlackBytes"`
    MaxOutputBytes       int64     `json:"maxOutputBytes"`
    MaxInputFileBytes    int64     `json:"maxInputFileBytes"`
    MaxOutputFileBytes   int64     `json:"maxOutputFileBytes"`
    MaxBatchTotalBytes   int64     `json:"maxBatchTotalBytes"`
    MaxProblemTotalBytes int64     `json:"maxProblemTotalBytes"`
    UpdatedAt            time.Time `json:"updatedAt"`
}
type SettingsInput struct { /* same seven fields without UpdatedAt */ }

type TestcaseLimits struct {
    MaxInputBytes        int64
    MaxOutputBytes       int64
    MaxBatchTotalBytes   int64
    MaxProblemTotalBytes int64
}

func NewSettingsService(db *gorm.DB) *SettingsService
func (s *SettingsService) Get(ctx context.Context) (*SettingsView, error)
func (s *SettingsService) Update(ctx context.Context, actorID int64, in SettingsInput, audit func(*gorm.DB) error) (*SettingsView, error)
func (s *SettingsService) TestcaseLimits(ctx context.Context) (TestcaseLimits, error)
```

- `db.Migrate` AutoMigrate `&judgesettings.Settings{}` after difficulty settings.
- `rbac` add `AuditTargetJudgeSettings = "judge_settings"`.
- `adminapi.Options` add `JudgeSettings *judgesettings.SettingsService`; `router.New` constructs it and passes.
- Routes:
  - `GET /api/admin/judge/settings`
  - `PUT /api/admin/judge/settings`
  - both require `system.settings.manage`.
  - PUT uses `auditActor(c)` + `rbac.WriteAuditTx` inside `Update` with action `string(rbac.PermSystemSettingsManage)` and target.
- Validation exact per Global Constraints; messages Chinese.
- `Get` and `TestcaseLimits` lazily create default row; `TestcaseLimits` returns four fields.
- `Update` lock row, write all fields, `updated_by`, `updated_at`.

- [ ] **Step 1: 写失败测试**

```go
func TestJudgeSettingsDefaultsAndUpdate(t *testing.T) {
    // Get creates id=1 with defaults; Update changes all fields; Get reads back.
}
func TestJudgeSettingsValidation(t *testing.T) {
    // floor > max, output file > max output, out-of-range values -> 40001.
}
func TestJudgeSettingsAuditRollback(t *testing.T) {
    // audit callback error rolls back update.
}
func TestJudgeSettingsAPI(t *testing.T) {
    // 401; forbidden without permission; GET defaults; PUT; audit row.
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/judgesettings ./internal/adminapi -run 'TestJudgeSettings' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现模型/服务/API/迁移**

- Defaults match Global Constraints.
- `ensureSettings` uses `ON CONFLICT DO NOTHING`.
- Update uses `FOR UPDATE`.
- Audit target `judge_settings`; no audit on GET.

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/judgesettings ./internal/adminapi ./internal/db ./internal/router`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/judgesettings apps/server/internal/adminapi/judge_settings_handler.go apps/server/internal/adminapi/judge_settings_handler_test.go apps/server/internal/adminapi/role_handler.go apps/server/internal/db/db.go apps/server/internal/router/router.go apps/server/internal/rbac/audit.go
git commit -m "feat(judge): add configurable judge settings"
```

---

### Task 2: 单测试点/批量上传动态限制与 body 上限

**Files:**
- Modify: `apps/server/internal/problem/service.go`, `apps/server/internal/problem/testcase_batch.go`
- Modify: `apps/server/internal/problemapi/handler.go`, `apps/server/internal/router/router.go`
- Test: `apps/server/internal/problem/testcase_limits_test.go`, `apps/server/internal/problemapi/handler_test.go`

**Interfaces:**
```go
package problem

type TestcaseLimits struct {
    MaxInputBytes        int64
    MaxOutputBytes       int64
    MaxBatchTotalBytes   int64
    MaxProblemTotalBytes int64
}
type TestcaseLimitsProvider interface {
    TestcaseLimits(ctx context.Context) (TestcaseLimits, error)
}
func WithTestcaseLimitsProvider(p TestcaseLimitsProvider) Option
func DefaultTestcaseLimits() TestcaseLimits
```

- `Service` field `testcaseLimits TestcaseLimitsProvider`; helper `limits(ctx)` returns defaults if nil/error (log warn).
- `stageTestcase`:
  - input `s.put(..., limits.MaxInputBytes, "输入文件")`
  - output `s.put(..., limits.MaxOutputBytes, "输出文件")`
- `AddTestcasesBatch`:
  - read limits once;
  - `pairBatchTestcases(files, limits)` checks per-file input/output and total `MaxBatchTotalBytes`;
  - writing uses corresponding input/output limits.
- Router:
  - `maxImportBodyBytes` from `210 << 20` to `(2 << 30) + (16 << 20)`.
  - `problemapi` single testcase route moves from `uploads.POST` to `imports.POST` (same 2 GiB+16 MiB group); batch default `MaxBatchBodyBytes` becomes `(2 << 30) + (16 << 20)`.
- `problemapi.Options.MaxBatchBodyBytes` remains for tests; default changes.
- Do not alter other upload routes (`uploads` remains 40 MiB).

- [ ] **Step 1: 写失败测试**

```go
func TestTestcaseUploadUsesConfiguredLimits(t *testing.T) {
    // provider with input 4, output 2; oversized input/output -> 40001 with MiB text.
}
func TestBatchUploadUsesConfiguredLimits(t *testing.T) {
    // per-file and batch total limits; small files accepted with raised limits.
}
func TestDefaultTestcaseLimitsMatchLegacyConstants(t *testing.T) { /* 16/16/200/200 */ }
```

`problemapi` 测试覆盖单测试点路由仍可上传、body 上限不再被旧 40 MiB 组挡住（可用小限制 + 测试注入）。

- [ ] **Step 2: 运行测试确认失败**

```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/problem ./internal/problemapi -run 'TestTestcaseUpload|TestBatchUpload|TestDefaultTestcaseLimits' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现动态限制**

- 错误信息使用当前配置的 MiB 值，不再写死 16/200。
- 所有上传先校验 headers 再写文件；批量失败清理已写文件。
- 单测试点与批量路由使用高 body 组；服务层是最终限制。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/problem ./internal/problemapi ./internal/router`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem/service.go apps/server/internal/problem/testcase_batch.go apps/server/internal/problem/testcase_limits_test.go apps/server/internal/problemapi/handler.go apps/server/internal/problemapi/handler_test.go apps/server/internal/router/router.go
git commit -m "feat(problem): configure testcase upload limits"
```

---

### Task 3: Hydro/题包导入动态限制

**Files:**
- Modify: `apps/server/internal/problem/import_hydro.go`, `apps/server/internal/problem/package_import.go`
- Test: `apps/server/internal/problem/import_limits_test.go`（或现有 import 测试）
- Modify: `apps/server/internal/problemapi/import_hydro_handler.go`, `package_import_handler.go` only if needed for `Options`

**Interfaces:**
- `ImportHydro` 内部读取 `s.limits(ctx)`：
  - `.in` 使用 `MaxInputBytes`，`.out/.ans` 使用 `MaxOutputBytes`；
  - 单题总量使用 `MaxProblemTotalBytes`；
  - 错误信息显示当前 MiB 值。
- `importKapOJ` 使用同一 `s.limits(ctx)`；`ImportPackage` 识别 Hydro 后调用 `ImportHydro`，沿用服务 provider。
- 题包 zip/解压限制继续由 `HydroImportOptions.MaxZipBytes/MaxUncompressedBytes` 和 `problem_import_settings` 控制，不改为 judge settings。
- 不新增 API 字段，服务测试通过 `problem.WithTestcaseLimitsProvider(fake)` 注入。

- [ ] **Step 1: 写失败测试**

```go
func TestHydroImportUsesConfiguredTestcaseLimits(t *testing.T) {
    // fake provider input=4/output=2/problemTotal=6; package with oversized .in/.out/total rejected.
}
func TestPackageImportUsesConfiguredTestcaseLimits(t *testing.T) {
    // kapOJ/Hydro package path uses same limits; zip limit unchanged.
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/problem -run 'Test.*UsesConfiguredTestcaseLimits' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现导入限制**

- 保留 `MaxHydroTestcaseBytes`/`MaxHydroProblemTestdataBytes` 作为默认回退常量，不再直接用于校验。
- 区分输入/输出错误文案。
- 确认 `.ans`/`.out` 都按输出限制。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/problem ./internal/problemapi`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem/import_hydro.go apps/server/internal/problem/package_import.go apps/server/internal/problem/import_limits_test.go apps/server/internal/problemapi/import_hydro_handler.go apps/server/internal/problemapi/package_import_handler.go
git commit -m "feat(problem): apply judge limits to package imports"
```

---

### Task 4: judge 读取设置并应用输出/答案上限

**Files:**
- Modify: `apps/judge/internal/model/model.go`, `apps/judge/internal/db/repository.go`, `apps/judge/internal/pipeline/pipeline.go`
- Modify: `apps/judge/internal/testutil/pg.go`
- Test: `apps/judge/internal/db/repository_test.go`, `apps/judge/internal/pipeline/pipeline_test.go`, `apps/judge/internal/pipeline/limit_test.go`

**Interfaces:**
```go
type JudgeSettings struct {
    OutputFloorBytes     int64
    OutputSlackBytes     int64
    MaxOutputBytes       int64
    MaxInputFileBytes    int64
    MaxOutputFileBytes   int64
    MaxBatchTotalBytes   int64
    MaxProblemTotalBytes int64
}
func (r *Repository) JudgeSettings(ctx context.Context) (model.JudgeSettings, error)
```
- Defaults in judge package mirror server constants.
- `JudgeSettings` query `SELECT ... FROM judge_settings WHERE id=1`; `pgx.ErrNoRows` -> defaults, other DB errors -> defaults + log by caller.
- `Pipeline.Run` loads once; `outputLimitForCases(cases, settings)`:
  ```text
  limit = floor
  for each case:
     need = max(floor, OutputSize + slack)  // if OutputSize <= 0 use floor
     limit = max(limit, min(need, maxOutput))
  ```
  If `maxOutput < floor` (bad DB data), clamp to floor.
- `judgeRun(..., maxAnswerBytes int64)` uses `settings.MaxOutputBytes` for expected answer read; remove fixed `maxAnswerBytes` or use as fallback default.
- `testutil.PG` creates `judge_settings` table and truncates it; add `InsertJudgeSettings` helper for tests.
- Do not enforce `MaxInputFileBytes` on existing testcases; it only governs uploads.

- [ ] **Step 1: 写失败测试**

```go
func TestJudgeSettingsDefaultsAndRead(t *testing.T) { /* no row -> defaults; custom row -> values */ }
func TestOutputLimitUsesSettings(t *testing.T) {
    // floor 2MiB, slack 1MiB, max 8MiB; expected 3MiB -> 4MiB; expected 9MiB -> 8MiB.
}
func TestPipelineUsesConfiguredOutputLimit(t *testing.T) {
    // insert settings + testcase output_size; fake sandbox captures RunBatch.OutputLimitBytes.
}
func TestPipelineAnswerReadUsesConfiguredMaxOutput(t *testing.T) { /* oversized expected answer -> error, not silent */ }
```

- [ ] **Step 2: 运行测试确认失败**

```bash
set -a; . "$PWD/.env"; set +a; test_db="$KAPOJ_TEST_DATABASE_URL"; cd apps/judge && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" KAPOJ_TEST_DATABASE_URL="$test_db" go test -p 1 ./internal/db ./internal/pipeline -run 'TestJudgeSettings|TestOutputLimit|TestPipelineUsesConfigured' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现 judge 读取与上限**

- 设置读取失败不影响评测；记录 warn 并使用默认值。
- 不改变 objective/special checker 路径；checker 仍用 4 KiB 输出限制。
- 保留 16 MiB 默认，避免缺少表时行为变化。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./...`（judge），Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/judge/internal/model/model.go apps/judge/internal/db/repository.go apps/judge/internal/db/repository_test.go apps/judge/internal/pipeline/pipeline.go apps/judge/internal/pipeline/pipeline_test.go apps/judge/internal/pipeline/limit_test.go apps/judge/internal/testutil/pg.go
git commit -m "feat(judge): read limits from judge settings"
```

---

### Task 5: OpenAPI、类型与前端 API 客户端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`, `apps/web/src/api/admin.ts`

**Interfaces:**
- Paths:
  - `GET /admin/judge/settings`
  - `PUT /admin/judge/settings`
- Schemas: `JudgeSettingsView`, `JudgeSettingsInput`.
- Functions:
  ```ts
  getAdminJudgeSettings(); updateAdminJudgeSettings(input);
  ```
- All byte fields `integer format:int64`; `updatedAt` string date-time.

- [ ] **Step 1: 基线构建**

Run: `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 2: 修改 OpenAPI 并生成**

Run: `corepack pnpm gen:api`；在 `types.ts` 加别名；在 `api/admin.ts` 加函数。

- [ ] **Step 3: 构建确认**

Run: `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/admin.ts
git commit -m "feat(api): add judge settings contracts"
```

---

### Task 6: 管理后台“系统 → 评测设置”页面

**Files:**
- Create: `apps/web/src/views/AdminJudgeView.vue`, `AdminJudgeView.spec.ts`
- Modify: `apps/web/src/router/index.ts`, `apps/web/src/components/layout/AdminNav.vue`, `AdminNav.spec.ts`

**Interfaces / behavior:**
- Route `/admin/judge` name `admin-judge`, `permission: PERM.system.settings.manage`, title “评测设置”。
- AdminNav 系统组新增“评测设置”，仅 `system.settings.manage` 可见。
- 页面两组字段（API 字节 ↔ UI MiB）：
  - **评测运行**：输出下限、输出余量、最大输出。
  - **测试数据**：单个输入文件、单个输出文件、批量上传总量、单题导入总量。
- MiB 输入允许 0.5 步进，保存时转字节并四舍五入；后端范围错误原样显示。
- 提示：只影响后续提交/上传；已有数据不修改；降低输出上限可能截断已有大输出。
- 加载/保存/空状态/错误中文；保存成功后刷新表单。
- 不修改现有站点设置页的题包导入设置。

- [ ] **Step 1: 写失败测试**

```ts
it('loads defaults in MiB and saves converted bytes', ...)
it('shows validation for floor > max output', ...)
it('shows server error message', ...)
it('shows the judge settings entry only with system.settings.manage', ...)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AdminJudgeView.spec.ts src/components/layout/AdminNav.spec.ts`，Expected: FAIL。

- [ ] **Step 3: 实现页面/路由/导航**

- 复用现有 PrimeVue/AppFormField/Message/Toast 风格。
- 不新增权限码；`system.settings.manage` 已存在。
- 保存后重新读取服务端值，避免前端与后端舍入不一致。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminJudgeView.vue apps/web/src/views/AdminJudgeView.spec.ts apps/web/src/router/index.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts
git commit -m "feat(web): add judge settings page"
```

---

### Task 7: 文档、全量验证与冒烟

**Files:**
- Modify: `DEVELOPMENT.md`, `运行手册.md`, `README.md`, `docs/任务清单.md`, `docs/开发日志.md`

**Docs:**
- 新增系统 → 评测设置：字段、范围、默认值、只影响后续提交/上传。
- judge 输出公式与标准答案读取；大输出题目行为。
- 测试数据上传/导入限制；题包 zip/解压设置仍在站点设置。
- 管理权限 `system.settings.manage` 与审计。
- 常见错误：输出超过限制/答案读取过大/上传超过限制。

- [ ] **Step 1: 更新文档**

- 保持现有章节结构，在 judge、题目测试数据、管理后台相关章节补充。
- 运行手册加“修改后何时生效”和“已有数据不追溯”。

- [ ] **Step 2: 全量验证**

```bash
set -a; . "$PWD/.env"; set +a; test_db="$KAPOJ_TEST_DATABASE_URL"
cd apps/server && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" KAPOJ_TEST_DATABASE_URL="$test_db" go test -count=1 -p 1 ./...
cd apps/judge && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" KAPOJ_TEST_DATABASE_URL="$test_db" go test -count=1 -p 1 ./...
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```
Expected: PASS。

- [ ] **Step 3: 真实冒烟**

临时 DB + 临时 server（或测试库直接调用服务）：
1. GET 默认设置等于 1/1/16/16/16/200/200 MiB。
2. PUT 将输出下限/余量/最大输出调整为 2/1/4 MiB；用题目 111 的测试点 2 期望输出 1.52 MiB 验证 judge 计算出的上限为 4 MiB（或 max 限制）。
3. 将单个输入文件调为 1 MiB，上传 2 MiB 输入被拒；恢复后成功。
4. 将批量总量调小，批量上传被拒；恢复。
5. 修改后无需重启 judge，新提交使用新值。
6. 审计记录包含 `system.settings.manage` / `judge_settings`。
清理临时数据。若某项无法执行，报告明确说明。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md 运行手册.md README.md docs/任务清单.md docs/开发日志.md
git commit -m "docs: document judge settings"
```

---

## Self-Review

- Spec coverage: 设置表/服务/API（T1）、单点/批量上传（T2）、Hydro/题包导入（T3）、judge 运行时（T4）、契约/客户端（T5）、后台页面（T6）、文档/验证（T7）。
- Type consistency: `SettingsView/Input` 在 T1 定义，T5/6 使用；`TestcaseLimits` 在 T2 定义，T3 复用；`JudgeSettings` 在 T4 定义；`system.settings.manage` 贯穿 T1/T6。
- Defaults match legacy constants in T1/T2/T4.
- No change to package zip settings, checker limits, testcase counts, time/memory limits.
