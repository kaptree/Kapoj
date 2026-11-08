# AI 管理开关与权限 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在管理后台增加 AI 启用/关闭开关，新增独立权限 `ai.settings.manage`；关闭后所有 `/api/ai/*` 统一 404，默认启用保留现有行为。

**Architecture:** `ai` 包新增单例 `ai_settings` 模型与 `SettingsService`（懒创建、默认启用、keyConfigured 状态）；`aiapi` 所有路由加 `RequireAIEnabled` 中间件；`adminapi` 挂载 GET/PUT `/admin/ai/settings`（独立权限 + 事务内审计）；前端新增 `/admin/ai` 管理页与导航。密钥仍由部署环境提供。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Vue 3、TypeScript、PrimeVue 4、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-11-08-ai-admin-toggle-design.md`

## Global Constraints

- `ai_settings` 单例 `id=1`，默认 `enabled=true`；首次读取懒创建，重复读取幂等。
- 新权限 `ai.settings.manage`（第 36 个，分组“AI”），默认只给 super_admin；content_admin 不自动获得。
- 关闭后所有 `/api/ai/*` 返回 404；管理 API `/api/admin/ai/settings` 不受开关影响。
- 未配置 `KAPOJ_AI_ENCRYPTION_KEY`（`d.AI==nil`）时 AI 路由不挂载；管理页 `keyConfigured=false`，启用请求 40001，关闭可保存。
- 设置更新与审计同事务（`rbac.WriteAuditTx`）；action `ai.settings.manage`，target `ai_settings`，targetID 1。
- 开关即时生效，无需重启；不改变 M5-A 用户配置/限流/提示缓存/provider seed。
- 密钥仍由部署环境配置，不在管理后台保存/回显；不新增环境变量。
- 用户文案中文；PrimeVue 4；不新增 UI 依赖；不推送/不合并。

## Review Focus

1. **默认/懒创建**：空库默认 enabled=true；重复读取不重复插入；DB 错误传播。
2. **开关语义**：enabled=true 正常；false 全部 AI 路由 404；管理 API 始终可用；即时生效；密钥缺失不能启用、可关闭。
3. **权限/审计**：第 36 权限、分组“AI”、默认仅 super；401/403/200；PUT 审计 action/target/summary；审计失败回滚。
4. **前端**：管理页加载/开关/密钥缺失/保存/Toast/错误；AdminNav/route 权限；用户侧 404 降级。
5. **兼容**：默认启用不改变现有 AI 行为；关闭不删除用户配置/提示；重新启用后恢复。
6. **测试/生成类型**：后端设置/中间件/API/迁移；前端页面/导航；OpenAPI 类型一致。

---

### Task 1: AI 设置模型、权限、路由中间件与管理 API

**Files:**
- Create: `apps/server/internal/ai/settings.go`
- Create: `apps/server/internal/ai/settings_test.go`
- Modify: `apps/server/internal/aiapi/handler.go`
- Modify: `apps/server/internal/aiapi/handler_test.go`
- Modify: `apps/server/internal/adminapi/role_handler.go`
- Modify: `apps/server/internal/adminapi/coin_handler.go`（`Options.AI`）
- Create: `apps/server/internal/adminapi/ai_handler.go`
- Create: `apps/server/internal/adminapi/ai_handler_test.go`
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/audit.go`
- Modify: `apps/server/internal/rbac/seed_test.go`
- Modify: `apps/server/internal/rbac/roles_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Test: 上述测试文件

**Interfaces:**
- Produces:
  - `ai.Settings`（`ai_settings`）模型：`ID int16`, `Enabled bool`, `UpdatedBy int64`, `UpdatedAt time.Time`。
  - `ai.SettingsService{db,keyConfigured}`；`NewSettingsService(db,keyConfigured bool)`。
  - `ai.SettingsView{Enabled bool; KeyConfigured bool; UpdatedAt time.Time}`。
  - `Service.Get(ctx) (*SettingsView,error)`、`Update(ctx, actorID int64, enabled bool, audit func(*gorm.DB) error) (*SettingsView,error)`、`Enabled(ctx) (bool,error)`。
  - `ai.SettingsInput{Enabled bool}`（admin 请求体）。
  - `rbac.PermAISettingsManage = "ai.settings.manage"`、`PermissionGroupAI = "AI"`；审计常量。
  - `adminapi.Options.AI *ai.SettingsService`；`GET/PUT /api/admin/ai/settings`。
  - `aiapi.Options.Settings *ai.SettingsService`；`aiapi.RequireAIEnabled(settings)` 中间件。
  - Router 构造 settings（即使 `d.AI==nil`），adminapi 始终传；`d.AI!=nil` 时 aiapi 传 settings。
- Consumes: `httpx`、`middleware`、`rbac.WriteAuditTx`、`db.Migrate`。

- [ ] **Step 1: 写失败测试**

`ai/settings_test.go`：
```go
func TestSettingsDefaultLazyCreate(t *testing.T)
// 空库 Get -> enabled=true,keyConfigured=true, updatedAt set; repeated Get no duplicate
func TestSettingsUpdateAndKeyMissing(t *testing.T)
// Update(false) ok; Update(true) keyConfigured=false -> 40001; audit callback invoked; DB error
func TestSettingsEnabled(t *testing.T)
// default true; false after update; missing row default true
```

`aiapi/handler_test.go`：
```go
func TestAIRoutesDisabledReturn404(t *testing.T)
// settings enabled=false -> all /ai routes 404
func TestAIRoutesEnabledPass(t *testing.T)
```

`adminapi/ai_handler_test.go`：
```go
func TestAdminAISettingsPermission(t *testing.T)
// 401/403/200; role with ai.settings.manage; super
func TestAdminAISettingsUpdateAndAudit(t *testing.T)
// PUT enabled false/true; audit action/target/summary; key missing enable 40001; close allowed
```

`rbac` 测试：36 权限、`ai.settings.manage`、分组“AI”。
`db_test`：`ai_settings` 表。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/adminapi/ ./internal/rbac/ ./internal/db/ -run 'TestSettings|TestAIRoutes|TestAdminAISettings|TestPermissionCatalog|TestMigrateCreatesAI' -v`

Expected: FAIL。

- [ ] **Step 3: 实现**

- `settings.go`：懒创建默认 `enabled=true`（`ON CONFLICT DO NOTHING`）；`Get/Update/Enabled`；`Update` 事务更新 + audit 回调，启用且 `!keyConfigured` → `httpx.ErrValidation("未配置 AI 加密密钥，无法启用")`。
- `aiapi`：`Options.Settings`；`RequireAIEnabled` 读 `Enabled`，false → `httpx.ErrNotFound`，DB 错误 500；所有 `/ai/*` 路由挂载该中间件（settings 非 nil 时）。
- `adminapi`：`Options.AI`；`GET/PUT /admin/ai/settings`，`RequirePermission(PermAISettingsManage)`；PUT bind `{enabled}`；`Update` 传 `rbac.WriteAuditTx` with actor/action/target/summary。
- `rbac`：权限/分组/审计常量；测试计数 35→36。
- `router`：`aiSettings := ai.NewSettingsService(d.DB, d.AI != nil)`；adminapi 传 AI；aiapi 传 Settings。
- `db.Migrate` 注册 `ai.Settings`；db 测试。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/ai internal/aiapi internal/adminapi internal/rbac internal/router internal/db && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/adminapi/ ./internal/rbac/ ./internal/router/ ./internal/db/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai apps/server/internal/aiapi apps/server/internal/adminapi apps/server/internal/rbac apps/server/internal/router apps/server/internal/db
git commit -m "feat(ai): add admin AI switch and permission"
```

---

### Task 2: OpenAPI 与生成类型

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/admin.ts`
- Test: `pnpm gen:api` + typecheck

**Interfaces:**
- Produces:
  - `AdminAISettingsView{enabled,keyConfigured,updatedAt}`、`AdminAISettingsInput{enabled}`。
  - `GET/PUT /admin/ai/settings` paths/security/errors。
  - `api/admin.ts` `getAdminAISettings`/`updateAdminAISettings`；types aliases。

- [ ] **Step 1: 更新 OpenAPI + 生成**

- 按后端 DTO/路由精确添加；错误 401/403/400/500。
- `corepack pnpm gen:api`；`vue-tsc --noEmit`/build。

- [ ] **Step 2: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/admin.ts
git commit -m "feat(api): document AI admin settings"
```

---

### Task 3: 管理后台 AI 设置页与导航

**Files:**
- Create: `apps/web/src/views/AdminAiView.vue`
- Create: `apps/web/src/views/AdminAiView.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/components/layout/AdminNav.spec.ts`
- Modify: `apps/web/src/permissions.ts`
- Modify: `apps/web/src/views/AdminRolesView.spec.ts`/`apps/web/e2e/admin-rbac.spec.ts`（35→36 if hardcoded）
- Test: 上述 spec

**Interfaces:**
- Consumes: Task 2 types/API.
- Produces:
  - `PERM.ai.settings.manage` + `ADMIN_ACCESS_PERMISSIONS`。
  - 路由 `admin-ai` → `/admin/ai`，requiresAuth + permission。
  - AdminNav 内容组/AI 组“AI 设置”。
  - `AdminAiView`：加载/状态 Tag/`ToggleSwitch`/保存/`keyConfigured=false` 警告与禁用/Toast/错误/移动端。

- [ ] **Step 1: 写失败测试**

```ts
it('loads settings and renders enabled status')
it('toggles and saves enabled/disabled with toast')
it('disables enable when keyConfigured is false')
it('shows error state and retry')
it('nav/route only with ai.settings.manage')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AdminAiView.spec.ts src/components/layout/AdminNav.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现**

- AdminAiView：AppPageHeader/AppSurface/AppFormField/ToggleSwitch/Button/Message/Tag/Toast；加载 GET；保存 PUT；成功更新状态/Toast；keyConfigured false 时开关禁用并提示；错误横幅/重试。
- 路由/权限/nav；permissions.ts。
- 用户侧无需改动（404 自动降级）。
- 不新增依赖。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminAiView.vue apps/web/src/views/AdminAiView.spec.ts apps/web/src/router/index.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/permissions.ts apps/web/src/views/AdminRolesView.spec.ts apps/web/e2e/admin-rbac.spec.ts
git commit -m "feat(web): add AI admin switch page"
```

---

### Task 4: 文档、全量检查与真实冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`
- Test: 全量后端、评测、前端、真实开关

**Interfaces:**
- Consumes: Task 1–3。
- Produces: 权限 36、AI 开关文档/冒烟。

- [ ] **Step 1: 文档**

- `DEVELOPMENT.md` §4.7 权限 36 + AI 分组；§6.5 管理开关/关闭 404/密钥缺失语义。
- `docs/任务清单.md`：M5-5 补充管理开关；权限计数更新。
- `docs/开发日志.md`：2026-11-08 条目。
- `README.md`/`运行手册.md`：管理页路径、权限、启用/关闭与密钥要求。
- `docs/primevue-migration.md`：UI-34 或新增行记录管理开关页。

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

- [ ] **Step 3: 真实冒烟**

- 配置密钥的临时/本地 server：管理员关闭 AI → `/api/ai/configs` 404、前端 AI 设置/提交/提示显示未启用；开启 → 恢复；审计 `ai.settings.manage`/`ai_settings`；无权限账号 403；普通用户无影响。
- 无密钥 server：管理页 `keyConfigured=false`，启用 40001，关闭可保存；AI 路由仍 404。
- 关闭不删除用户 `user_ai_configs`/`ai_hints`；重新开启后用户配置/缓存仍在。
- 清理临时设置/用户/审计/密钥；恢复默认 enabled=true；不删除既有数据。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md
git commit -m "docs: record AI admin switch"
```

---

## Self-Review

**Spec coverage:**

- 设置/权限/中间件/管理 API → Task 1。
- OpenAPI/类型 → Task 2。
- 管理页/导航 → Task 3。
- 文档/检查/冒烟 → Task 4。

**Type consistency:**

- `SettingsView`/`SettingsInput` 在 Task 1；Task 2 OpenAPI；Task 3 API/types。
- `PermAISettingsManage` Task 1；Task 3 `PERM.ai.settings.manage`。
- `RequireAIEnabled` Task 1；router 传 settings。

**Review Focus mapping:**

1. 默认/懒创建 → Task 1 tests。
2. 开关语义 → Task 1 tests + Task 4 smoke。
3. 权限/审计 → Task 1 tests + Task 4 smoke。
4. 前端 → Task 3 tests。
5. 兼容 → Task 1/4 tests + smoke。
6. 测试/类型 → Task 1/2/3 + Task 4 full checks。
