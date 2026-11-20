# 题包导入大小上限 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 后台可配置题包上限 1–2048 MiB，默认 200 MiB；kapOJ/HydroOJ 导入最多支持 2 GiB，并按包上限自动推导解压上限。

**Architecture:** 新增 `problemimport` 单例设置与迁移 0004；`problem` 服务接受请求级 zip/解压上限；`problemapi` 在 multipart 前读取设置并限制请求体；`adminapi` 提供设置读写；前端在 `/admin/site` 新 Tab 配置，导入页读取 `/problems/import/limits`。

**Tech Stack:** Go 1.26、Gin、GORM/PostgreSQL、Vue 3.5、PrimeVue、Vitest、OpenAPI TypeScript。

**Spec:** `docs/superpowers/specs/2026-11-20-problem-import-size-limit-design.md`

## Global Constraints

- 默认上限 **200 MiB**；可配置范围 **1–2048 MiB**。
- 解压上限 = `max(1 GiB, 2 × 包上限)`；2 GiB 包 → 4 GiB。
- 两个导入端点都使用同一设置：`/api/problems/import/package`、`/api/problems/import/hydro`。
- 设置管理权限 `site.settings.manage`；导入页读取权限 `problem.create`。
- `imports` 组请求体上限改为 **2064 MiB**；单请求按当前设置再限制，multipart 开销 **16 MiB**。
- 现有单题/单测试点/条目数限制不变：单测试点 16 MiB、单题测试数据 200 MiB、最多 100 题、5000 条目。
- 未配置设置时保持现有 200 MiB / 1 GiB 行为。
- 设置更新审计 action `problem.import.settings`，target `problem_import_settings`；审计失败回滚。
- 前端读取失败回退 200 MiB，不阻塞导入页。
- 自建 Nginx 需 `client_max_body_size ≥ 2g`；Caddy 无需改动。
- 后端测试 `KAPOJ_TEST_DATABASE_URL` + `-p 1`；前端命令在 `apps/web`。

## Review Focus

1. **2 GiB 上限真的可达**：handler、`imports` 组、`ImportHydro`/`ImportPackage`、前端五处都使用当前设置；Task 2/3 固定。
2. **解压上限随设置放大**：声明大小和实际读取都使用推导值，2 GiB 包不被 1 GiB 旧上限拒绝。Task 1/2 固定。
3. **旧行为不回归**：未设置时仍是 200 MiB/1 GiB；现有 200 MB 提示和测试保持。Task 2/3/5 固定。
4. **请求体提前拒绝**：`Content-Length` 和 `MaxBytesReader` 按当前设置生效，不先完整落盘 2 GiB。Task 3 固定。
5. **设置权限与审计**：非 `site.settings.manage` 403；更新写审计；非法值 40001。Task 1/3 固定。

---

### Task 1: 设置表、服务与迁移

**Files:**
- Create: `apps/server/migrations/0004_problem_import_settings.up.sql`
- Create: `apps/server/migrations/0004_problem_import_settings.down.sql`
- Create: `apps/server/internal/problemimport/model.go`
- Create: `apps/server/internal/problemimport/settings.go`
- Create: `apps/server/internal/problemimport/settings_test.go`
- Modify: `apps/server/internal/db/db_test.go` (table/default assertion)

**Interfaces:**
- Produces:
  ```go
  const (
      DefaultMaxPackageMB = 200
      MinMaxPackageMB = 1
      MaxMaxPackageMB = 2048
      MiB int64 = 1 << 20
      MinUncompressedBytes int64 = 1 << 30
  )

  type SettingsView struct {
      MaxPackageMB int `json:"maxPackageMb"`
      MaxZipBytes int64 `json:"maxZipBytes"`
      MaxUncompressedBytes int64 `json:"maxUncompressedBytes"`
      UpdatedAt time.Time `json:"updatedAt"`
  }
  type SettingsInput struct { MaxPackageMB int `json:"maxPackageMb"` }
  type SettingsService struct{ db *gorm.DB }
  func NewSettingsService(db *gorm.DB) *SettingsService
  func (s *SettingsService) Get(ctx context.Context) (*SettingsView, error)
  func (s *SettingsService) Update(ctx context.Context, actorID int64, in SettingsInput, audit func(*gorm.DB) error) (*SettingsView, error)
  ```

- [ ] **Step 1: Write failing settings tests**

```go
func TestGetCreatesDefaultAndComputesLimits(t *testing.T) {
    g := testutil.PG(t)
    view, err := NewSettingsService(g).Get(context.Background())
    require.NoError(t, err)
    assert.Equal(t, 200, view.MaxPackageMB)
    assert.Equal(t, int64(200*MiB), view.MaxZipBytes)
    assert.Equal(t, int64(1<<30), view.MaxUncompressedBytes)
}

func TestUpdateValidationAndAuditRollback(t *testing.T) {
    // 0 / 2049 -> 40001
    // 2048 -> MaxZipBytes 2GiB, MaxUncompressedBytes 4GiB
    // audit callback error -> old value remains
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL=... go test ./internal/problemimport -count=1 -p 1`
Expected: FAIL because package/table do not exist.

- [ ] **Step 3: Implement migration and service**

Migration up:

```sql
CREATE TABLE public.problem_import_settings (
    id bigint PRIMARY KEY,
    max_package_mb integer NOT NULL DEFAULT 200,
    updated_by bigint NOT NULL DEFAULT 0,
    updated_at timestamp with time zone NOT NULL
);
INSERT INTO public.problem_import_settings (id, max_package_mb, updated_by, updated_at)
VALUES (1, 200, 0, now())
ON CONFLICT (id) DO NOTHING;
```

Down: `DROP TABLE IF EXISTS public.problem_import_settings;`

Service:
- `Settings` model with `ID`, `MaxPackageMB`, `UpdatedBy`, `UpdatedAt`; table name.
- `Get`: transaction, `INSERT ... ON CONFLICT DO NOTHING` default if missing, `First(id=1)`, return view.
- `Update`: validate 1–2048 (`httpx.ErrValidation("题包大小上限需为 1–2048 MB")`), transaction with row lock, update `max_package_mb`, `updated_by`, `updated_at`, call audit, return view.
- `view`: `MaxZipBytes = MB*MiB`; `MaxUncompressedBytes = max(1GiB, 2*MaxZipBytes)`.

- [ ] **Step 4: Add migration test and run tests**

`db_test.go` assert table and default row `max_package_mb=200`.
Run: `go test ./internal/problemimport ./internal/db -count=1 -p 1`

- [ ] **Step 5: Commit**

```bash
git add apps/server/migrations/0004_problem_import_settings.* apps/server/internal/problemimport apps/server/internal/db/db_test.go
git commit -m "feat(problemimport): add configurable import size settings"
```

---

### Task 2: 请求级 zip/解压上限

**Files:**
- Modify: `apps/server/internal/problem/import_hydro.go`
- Modify: `apps/server/internal/problem/package_import.go`
- Test: `apps/server/internal/problem/import_hydro_test.go`
- Test: `apps/server/internal/problem/package_import_test.go`

**Interfaces:**
- Consumes: existing zip helpers/tests.
- Produces: `HydroImportOptions`/`PackageImportOptions` add:
  ```go
  MaxZipBytes int64
  MaxUncompressedBytes int64
  ```

- [ ] **Step 1: Write failing limit tests**

Use the existing synthetic zip helper. Add:

```go
func TestImportHydroHonorsRequestLimits(t *testing.T) {
    // zip with a small file
    // MaxZipBytes < len(zip) -> 40001
    // MaxUncompressedBytes < declared size -> 40001
    // large limits -> succeeds
}

func TestImportPackageHonorsRequestLimits(t *testing.T) {
    // package import also rejects zip over MaxZipBytes
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/problem -run 'TestImport.*HonorsRequestLimits' -count=1 -p 1`
Expected: FAIL because options/limits are ignored.

- [ ] **Step 3: Implement**

`ImportHydro`:
```go
zipLimit := opt.MaxZipBytes
if zipLimit <= 0 { zipLimit = maxHydroZipBytes }
if size > zipLimit { return nil, httpx.ErrValidation(fmt.Sprintf("题包文件不能超过 %d MiB", zipLimit>>20)) }
uncompressedLimit := opt.MaxUncompressedBytes
if uncompressedLimit <= 0 {
    uncompressedLimit = s.hydroUncompressedLimit
    if uncompressedLimit <= 0 { uncompressedLimit = maxHydroUncompressed }
}
```
Use `uncompressedLimit` for the declared pre-scan and actual reads.

`ImportPackage`:
- same zip check;
- derive uncompressed limit;
- `newKapOJArchive(zr, uncompressedLimit)`;
- pass both fields to `ImportHydro` options.

`newKapOJArchive`: declared check `if declared > limit`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/problem -count=1 -p 1` (test DB not required for these tests if existing ones are in-memory; otherwise use DSN).

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/problem/import_hydro.go apps/server/internal/problem/package_import.go apps/server/internal/problem/*_test.go
git commit -m "feat(problem): accept per-request import limits"
```

---

### Task 3: 设置 API、limits API 与请求体限制

**Files:**
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problemapi/import_hydro_handler.go`
- Modify: `apps/server/internal/problemapi/package_import_handler.go`
- Create: `apps/server/internal/problemapi/import_limits_handler.go`
- Modify: `apps/server/internal/adminapi/role_handler.go` (Options/handler/routes)
- Create: `apps/server/internal/adminapi/problem_import_settings_handler.go`
- Modify: `apps/server/internal/rbac/audit.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Test: `apps/server/internal/problemapi/import_hydro_handler_test.go`
- Test: `apps/server/internal/adminapi/*_test.go`

**Interfaces:**
- Consumes: `problemimport.SettingsService`, Task 2 options.
- Produces:
  ```go
  type ImportLimitProvider interface {
      Get(ctx context.Context) (*problemimport.SettingsView, error)
  }
  // problemapi.Options.ImportSettings ImportLimitProvider
  func (h *handler) importLimits(ctx context.Context) (int64, int64, error)
  // GET /api/problems/import/limits
  // GET/PUT /api/admin/problems/import/settings
  const AuditActionProblemImportSettings = "problem.import.settings"
  ```

- [ ] **Step 1: Write failing API tests**

`problemapi` fake provider:

```go
type fakeImportSettings struct{ view *problemimport.SettingsView }
func (f fakeImportSettings) Get(context.Context) (*problemimport.SettingsView, error) { return f.view, nil }
```

Tests:
- `GET /api/problems/import/limits` with `problem.create` returns `{maxPackageMb, maxUncompressedMb}`.
- `POST /import/hydro` with a file over `MaxZipBytes` -> 40001.
- `POST /import/package` with a file over `MaxZipBytes` -> 40001.
- `Content-Length` over `MaxZipBytes+16MiB` -> 413.
- Admin GET/PUT: super 200; plain 403; invalid 0/2049 -> 40001; audit row with counts/limit.
- `TestMigrateCreatesProblemImportSettings` maybe Task 1.

- [ ] **Step 2: Run RED**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL=... go test ./internal/problemapi ./internal/adminapi -run 'ImportLimits|ProblemImportSettings' -count=1 -p 1`
Expected: FAIL because routes/options do not exist.

- [ ] **Step 3: Implement**

- `problemapi.Options.ImportSettings ImportLimitProvider` + handler field.
- helper `importLimits(ctx)`:
  - default 200 MiB / 1 GiB; if `MaxImportFileBytes > 0` use it with derived 1 GiB/2×; if provider non-nil use its view.
- Both import handlers before `parseMultipart`:
  ```go
  maxZip, maxUncompressed, err := h.importLimits(ctx)
  if err != nil { fail; return }
  maxBody := maxZip + 16<<20
  if c.Request.ContentLength > maxBody { fail ErrPayloadTooLarge; return }
  c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
  ```
  Then parse; check `fh.Size > maxZip`; pass both options.
- `GET /problems/import/limits` route with `load` + `require(problem.create)`, returns MB values.
- Admin:
  - `adminapi.Options.ProblemImport *problemimport.SettingsService`; handler field.
  - `GET/PUT /admin/problems/import/settings` with `site.settings.manage`.
  - PUT calls `Update` with `WriteAuditTx` action `problem.import.settings`, target `problem_import_settings`, summary.
- `router.go`: `importSettings := problemimport.NewSettingsService(d.DB)`; inject; `maxImportBodyBytes = 2064 << 20`.
- OpenAPI paths/schemas; regenerate types.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/problemapi ./internal/adminapi ./internal/problem ./internal/problemimport ./internal/router -count=1 -p 1`

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/problemapi apps/server/internal/adminapi apps/server/internal/rbac apps/server/internal/router packages/api apps/web/src/api/schema.d.ts
git commit -m "feat(problemapi): expose configurable import limits"
```

---

### Task 4: 前端后台设置 Tab

**Files:**
- Modify: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/components/admin/AdminProblemImportSettings.vue`
- Create: `apps/web/src/components/admin/AdminProblemImportSettings.spec.ts`
- Modify: `apps/web/src/views/AdminSiteView.vue`
- Modify: `apps/web/src/views/AdminSiteView.spec.ts`

**Interfaces:**
- Produces:
  ```ts
  export function getAdminProblemImportSettings(): Promise<ProblemImportSettings>
  export function updateAdminProblemImportSettings(input: ProblemImportSettingsInput): Promise<ProblemImportSettings>
  ```
- `ProblemImportSettings` / `ProblemImportSettingsInput` from generated schema.

- [ ] **Step 1: Write failing component tests**

- loads 200 and displays it.
- saves 2048, calls API, shows success.
- rejects 0/2049 client-side or shows API 40001.
- shows effective uncompressed limit (e.g., 4096 MB).

- [ ] **Step 2: Run RED**

Run: `cd apps/web && npm test -- src/components/admin/AdminProblemImportSettings.spec.ts`
Expected: FAIL because component/API do not exist.

- [ ] **Step 3: Implement component and tab**

- `AdminProblemImportSettings.vue`: `onMounted` load; `InputNumber` 1–2048; `AppFormField`; display `maxUncompressedMb`; save button; toast; error.
- Add `AdminProblemImportSettings` to `AdminSiteView` new Tab `problem-import` with label 「题库导入」.
- Add API types/functions.
- Tests for tab presence.

- [ ] **Step 4: Run tests**

Run: `cd apps/web && npm test -- src/components/admin/AdminProblemImportSettings.spec.ts src/views/AdminSiteView.spec.ts`

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/api apps/web/src/components/admin/AdminProblemImportSettings.vue apps/web/src/components/admin/AdminProblemImportSettings.spec.ts apps/web/src/views/AdminSiteView.vue apps/web/src/views/AdminSiteView.spec.ts
git commit -m "feat(admin): add problem import size settings"
```

---

### Task 5: 导入页动态上限

**Files:**
- Modify: `apps/web/src/api/problems.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/views/ProblemImportView.vue`
- Modify: `apps/web/src/views/ProblemImportView.spec.ts`

**Interfaces:**
- Produces: `getProblemImportLimits(): Promise<{maxPackageMb:number;maxUncompressedMb:number}>`.

- [ ] **Step 1: Write failing tests**

- mock `getProblemImportLimits` default 200; existing tests pass.
- 2048: page shows `2048 MB` and a 200MB+ file is accepted (API mock called).
- reject limits request: fallback 200 MB, still usable.

- [ ] **Step 2: Run RED**

Run: `cd apps/web && npm test -- src/views/ProblemImportView.spec.ts`
Expected: FAIL because the API/limits state is missing.

- [ ] **Step 3: Implement**

- Add `getProblemImportLimits`; `maxFileMb` ref 200; `maxFileBytes` computed; `onMounted` fetch and fallback; use in `pickFile`, error, template.
- Update test mock to include the new function and default resolved.

- [ ] **Step 4: Run tests**

Run: `cd apps/web && npm test -- src/views/ProblemImportView.spec.ts`

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/api/problems.ts apps/web/src/api/types.ts apps/web/src/views/ProblemImportView.vue apps/web/src/views/ProblemImportView.spec.ts
git commit -m "feat(web): use server problem import limits"
```

---

### Task 6: 文档与全量验证

**Files:**
- Modify: `运行手册.md`
- Modify: `docs/开发日志.md`
- Modify: `docs/任务清单.md`（可选）

- [ ] **Step 1: Update docs**

- `/admin/site` → 「题库导入」配置 1–2048 MiB，默认 200。
- 2 GiB 上传需要临时盘 ≥2 GiB；Nginx `client_max_body_size ≥2g`，`proxy_read_timeout` 足够。
- 解压上限自动为 max(1GiB, 2×包上限)。
- 内部单测试点/单题限制不变。

- [ ] **Step 2: Full verification**

```bash
cd apps/server
KAPOJ_TEST_DATABASE_URL=... go test -count=1 -p 1 ./...
cd ../web
npm run build
npm test
npm run lint
```

- [ ] **Step 3: Migration up/down smoke**

On a temp DB: `migrate up` creates `problem_import_settings` default 200; down drops it; re-up works.

- [ ] **Step 4: API smoke**

Use a temp server and admin token:
- `GET/PUT /api/admin/problems/import/settings` with 2048 -> `maxZipBytes=2GiB`, `maxUncompressedBytes=4GiB`.
- `GET /api/problems/import/limits` returns 2048/4096.
- Upload a small zip to both import endpoints -> 200.
- Set limit 1 MiB, upload a 2 MiB file -> 40001/413.
- Restore 200 MB.

- [ ] **Step 5: Commit**

```bash
git add 运行手册.md docs/开发日志.md docs/任务清单.md
git commit -m "docs(problemimport): document import size limits"
```
