# AI 加密密钥管理 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 管理后台自动/手动生成并保存 `KAPOJ_AI_ENCRYPTION_KEY` 到本地密钥文件，运行时立即生效；轮换时重加密用户 AI Key；环境变量优先且后台只读。

**Architecture:** 新增 `ai.KeyManager`（env/文件加载、运行时替换、加解密、指纹）与密钥文件配置；`ai.Service` 改为持有 KeyManager，AI 路由始终挂载、无密钥返回 `keyConfigured=false`/40001；`ai.SettingsService` 扩展密钥查看/生成/轮换（事务内重加密 `user_ai_configs` + 文件备份）；`adminapi` 挂载密钥 GET/生成/手动 API；管理页新增密钥板块，用户侧区分“AI 未启用”与“管理员未配置密钥”。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、AES-256-GCM、Vue 3、PrimeVue 4、Vitest。

**Spec:** `docs/superpowers/specs/2026-11-08-ai-key-management-design.md`

## Global Constraints

- 密钥文件 `KAPOJ_AI_KEY_FILE`（默认 `data/ai.key`，0600）；环境变量 `KAPOJ_AI_ENCRYPTION_KEY` 优先且后台只读。
- 生成：`crypto/rand` 32 字节 → base64；手动：base64 32 字节校验。
- 轮换：旧密钥解密、新密钥重加密所有 `user_ai_configs`；事务失败回滚；文件写失败回滚 DB；无旧密钥但存在用户配置 → 40001，不破坏数据。
- 密钥不进 DB/日志/审计/普通 API；生成接口一次性返回完整密钥；其他接口只返回 masked/指纹。
- 无密钥：AI 路由仍挂载；`GET /ai/configs` 200 + `keyConfigured=false`；保存/complete/format/optimize 40001；hint 缓存命中仍可读。
- 管理开关 `enabled` 仍独立；关闭/开启不删除密钥/用户配置；启用要求当前 keyConfigured。
- 复用 `ai.settings.manage`（第 36 权限）与审计；不新增权限。
- 不改变 AES-GCM 密文格式、provider seed、限流、提示缓存。
- 不推送/不合并；中文文案；PrimeVue 4；不新增 UI 依赖。

## Review Focus

1. **密钥加载/优先级**：env 优先/非法 env 启动失败；文件缺失/非法/不可读；masked/指纹；运行时 Replace 并发安全。
2. **无密钥语义**：AI 路由挂载但 config 200 keyConfigured=false；保存/生成 40001；hint 缓存可用；开关启用要求 key。
3. **轮换安全**：旧解密/新加密全部用户配置；任一失败回滚；无旧密钥有配置拒绝；文件备份/恢复；审计不含密钥。
4. **文件安全**：0600/目录 0700；原子写；不写日志；env 来源只读。
5. **管理 UI**：未配置/文件/env 三态；生成一次性展示/复制；手动；轮换确认；开关与密钥联动。
6. **用户侧**：AI 未启用（404）vs 管理员未配置密钥（keyConfigured=false）vs 用户未配置模型；提示/提交/设置页文案。
7. **回归/类型**：现有 AI 测试/开关/路由更新；OpenAPI 类型；lint/build。

---

### Task 1: KeyManager、配置与 AI Service 无密钥语义

**Files:**
- Create: `apps/server/internal/ai/key_manager.go`
- Create: `apps/server/internal/ai/key_manager_test.go`
- Modify: `apps/server/internal/ai/service.go`
- Modify: `apps/server/internal/ai/crypto.go`（如需要）
- Modify: `apps/server/internal/aiapi/handler.go`
- Modify: `apps/server/internal/aiapi/handler_test.go`
- Modify: `apps/server/internal/ai/settings.go`
- Modify: `apps/server/internal/ai/settings_test.go`
- Modify: `apps/server/internal/adminapi/ai_handler.go`
- Modify: `apps/server/internal/adminapi/ai_handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Test: 上述测试

**Interfaces:**
- Produces:
  - `ai.KeyManager{...}`；`NewKeyManager(envKey, filePath string) (*KeyManager, error)`。
  - `Configured() bool`、`Source() string`（`env|file|none`）、`Masked() string`、`Fingerprint() string`、`Encrypt/Decrypt`、`Replace(enc *Encryptor, source string)`。
  - `ai.NewService(db, keys *KeyManager, limiter, client)`；`Service.KeyConfigured() bool`。
  - `ConfigView.KeyConfigured bool`；`GetConfig` 填充；`SaveConfig` 无密钥 40001。
  - `ai.NewSettingsService(db, keys *KeyManager, keyFile string)`（替换现有 keyConfigured bool）。
  - `config.AIKeyFile`：`KAPOJ_AI_KEY_FILE` 默认 `data/ai.key`。
  - 主程序：始终构造 KeyManager/AI Service；env 非法启动失败；文件非法记录日志→未配置。
  - `aiapi` 路由始终挂载（`d.AI` 非 nil）；`RequireAIEnabled` 仍检查开关；complete/format/optimize/hint 无密钥 40001；hint 缓存优先。
- Consumes: 现有 `Encryptor`/OpenAI client/limiter/settings。

- [ ] **Step 1: 写失败测试**

```go
func TestKeyManagerEnvPrecedenceAndFile(t *testing.T)
// env valid -> source env, file ignored; env invalid -> error
func TestKeyManagerFileMissingInvalid(t *testing.T)
// missing -> none; invalid -> none + no panic
// file valid -> source file; masked/fingerprint
func TestKeyManagerReplaceAndConcurrency(t *testing.T)
func TestAIConfigWithoutKey(t *testing.T)
// GET config 200 keyConfigured=false; SaveConfig 40001; generate 40001; hint cache works
func TestAIServiceUsesCurrentKeyAfterReplace(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/adminapi/ -run 'TestKeyManager|TestAIConfigWithoutKey|TestAIServiceUsesCurrentKey' -v`

Expected: FAIL。

- [ ] **Step 3: 实现**

- `KeyManager`：读写锁；env 优先（`NewEncryptor`）；文件不存在→none；读/解码失败→slog 警告、none；mask/fingerprint（sha256）。
- `Service` 改用 KeyManager；`chatRequest` 解密失败/无密钥→40001；`GetConfig` 加 `KeyConfigured`；`SaveConfig` 前置检查。
- `SettingsService` 构造/字段改用 KeyManager；`SettingsView.KeyConfigured` 动态；`Update` 启用检查 `keys.Configured()`。
- `config`/`main`：AIKeyFile；始终构造 AI Service/Settings；`router.Deps.AISettings`；`adminapi.Options.AI` 用 settings；`aiapi` 始终注册（d.AI 非 nil）。
- `aiapi` 的 hint：缓存命中先返回；无 key 且未命中走 chatRequest 返回 40001。
- 更新现有开关/AI 测试对 `d.AI==nil` 的假设。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/ai internal/aiapi internal/adminapi internal/router internal/config cmd/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/adminapi/ ./internal/config/ ./internal/router/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai apps/server/internal/aiapi apps/server/internal/adminapi apps/server/internal/router apps/server/internal/config apps/server/cmd/server
git commit -m "feat(ai): load encryption key from file at runtime"
```

---

### Task 2: 密钥生成/手动/轮换与重新加密

**Files:**
- Create: `apps/server/internal/ai/key_service.go`
- Create: `apps/server/internal/ai/key_service_test.go`
- Modify: `apps/server/internal/ai/settings.go`
- Modify: `apps/server/internal/ai/model.go`
- Modify: `apps/server/internal/adminapi/ai_handler.go`
- Modify: `apps/server/internal/adminapi/ai_handler_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Test: 上述测试

**Interfaces:**
- Produces:
  - `ai.KeyView{Configured,Source,Masked,Fingerprint}`。
  - `ai.KeyGenerateResult{KeyView; Key string `json:"key"`}`。
  - `ai.SettingsService.KeyView() KeyView`。
  - `GenerateKey(ctx, actorID, audit func(*gorm.DB) error) (*KeyGenerateResult,error)`。
  - `SetKey(ctx, actorID, raw string, audit func(*gorm.DB) error) (*KeyView,error)`。
  - 管理 API：`GET /api/admin/ai/key`、`POST /api/admin/ai/key/generate`、`PUT /api/admin/ai/key`；权限 `ai.settings.manage`；审计 target `ai_key`。
  - `ai_settings.key_fingerprint` 列。
  - 密钥文件写入/备份/回滚。
- Consumes: Task 1 KeyManager/SettingsService.

- [ ] **Step 1: 写失败测试**

```go
func TestKeyGenerateAndFilePermissions(t *testing.T)
// generate -> 32-byte base64, file 0600, key manager configured, key returned once
func TestKeyManualValidation(t *testing.T)
// valid base64/32 -> ok; invalid -> 40001; env source -> 40001
func TestKeyRotationReencryptsUserConfigs(t *testing.T)
// old key encrypts configs; rotate; new key decrypts; old key fails; file backup removed
func TestKeyRotationRollback(t *testing.T)
// decrypt/DB/file failure -> old key/file/configs intact
func TestKeyRotationWithoutOldKeyRejects(t *testing.T)
// user configs exist + no key -> 40001, no file write
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/adminapi/ -run 'TestKeyGenerate|TestKeyManual|TestKeyRotation' -v`

Expected: FAIL。

- [ ] **Step 3: 实现**

- 模型 `KeyFingerprint` 列；迁移/db 测试。
- `KeyView`/`GenerateKey`/`SetKey`：env 只读；生成 crypto/rand；手动校验；密钥文件原子写 0600、备份 `.bak`。
- 轮换：
  - 无旧 key + 存在 user_ai_configs → 40001。
  - 有旧 key：事务 SELECT FOR UPDATE 所有配置，旧解密/新加密；任一失败回滚。
  - 写文件（备份）→ DB 事务（配置+指纹+审计）→ 成功 remove backup；失败恢复备份/回滚。
  - `keys.Replace(newEnc,"file")`。
- 管理 handler：GET/生成/手动；PUT body `{key}`；审计 action/target/summary（不含密钥）；env 只读错误。
- 现有 `SettingsService.Update` 保持；密钥操作与开关同一权限。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/ai internal/adminapi internal/db && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/adminapi/ ./internal/db/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai apps/server/internal/adminapi apps/server/internal/db
git commit -m "feat(ai): add key generation and rotation"
```

---

### Task 3: OpenAPI 与生成类型

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/ai.ts`
- Test: `gen:api` + typecheck

**Interfaces:**
- Produces:
  - `AIConfigView.keyConfigured`。
  - `AIKeyView`、`AIKeyGenerateResult`、`AIKeyInput`。
  - `GET/POST/PUT /admin/ai/key` paths.
  - `api/admin.ts` key methods；`api/ai.ts` config keyConfigured type.

- [ ] **Step 1: 更新 OpenAPI + 生成**

- 按后端 DTO 精确添加；`corepack pnpm gen:api`；`vue-tsc --noEmit`/build。

- [ ] **Step 2: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/admin.ts apps/web/src/api/ai.ts
git commit -m "feat(api): document AI key management"
```

---

### Task 4: 管理页密钥板块与用户侧 keyConfigured 状态

**Files:**
- Modify: `apps/web/src/views/AdminAiView.vue` + spec
- Modify: `apps/web/src/views/AiSettingsView.vue` + spec
- Modify: `apps/web/src/views/SubmitView.vue` + spec
- Modify: `apps/web/src/views/ProblemDetailView.vue` + spec
- Test: 上述 spec

**Interfaces:**
- Consumes: Task 3 types/API.
- Produces:
  - AdminAiView 密钥板块：状态 Tag（未配置/已配置文件/env）、masked/fingerprint、自动生成（一次性显示/复制/警告）、手动填写、轮换确认、错误/Toast、env 只读。
  - `AiSettingsView`：`keyConfigured=false` 时“管理员尚未配置 AI 密钥”，表单禁用；404 仍“AI 功能未启用”。
  - `SubmitView`：banner 区分 404/开关关闭、keyConfigured=false、用户未配置；按钮逻辑。
  - `ProblemDetailView`：hint 40001 时区分“管理员未配置密钥”与用户未配置；缓存命中仍展示。
  - 测试覆盖三态/生成/手动/轮换/复制/错误。

- [ ] **Step 1: 写失败测试**

```ts
// AdminAiView: key status/generate one-time/manual/rotate confirm/env readonly
// AiSettingsView: keyConfigured false vs 404 vs configured
// SubmitView: banner precedence (404, key missing, user missing)
// ProblemDetail: key missing vs user config missing; cached hint
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AdminAiView.spec.ts src/views/AiSettingsView.spec.ts src/views/SubmitView.spec.ts src/views/ProblemDetailView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现**

- 管理页密钥 `AppSurface`：状态/生成/手动/轮换/一次性 key 展示（只读 input + 复制按钮 + 警告）；env 只读。
- 用户页使用 `keyConfigured`；错误文案区分。
- 提交/提示 banner 优先级；不自动请求 hint。
- 不新增依赖。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminAiView.vue apps/web/src/views/AdminAiView.spec.ts apps/web/src/views/AiSettingsView.vue apps/web/src/views/AiSettingsView.spec.ts apps/web/src/views/SubmitView.vue apps/web/src/views/SubmitView.spec.ts apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts
git commit -m "feat(web): add AI key management UI"
```

---

### Task 5: 文档、全量检查与真实冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`
- Test: 全量后端、评测、前端、真实密钥

**Interfaces:**
- Consumes: Task 1–4。
- Produces: 密钥文件/后台生成/轮换/无密钥语义文档与冒烟记录。

- [ ] **Step 1: 文档**

- `DEVELOPMENT.md`：密钥文件/env 优先级、KeyManager、无密钥语义、轮换/重加密、用户侧状态；权限 36 不变。
- `docs/任务清单.md`：AI 管理开关/密钥板块完成；M5-5 补充。
- `docs/开发日志.md`：2026-11-08 密钥管理条目。
- `README.md`/`运行手册.md`：`KAPOJ_AI_KEY_FILE`/env 优先、后台生成/手动/轮换/备份、无密钥错误。
- `docs/primevue-migration.md`：UI-38/AdminAiView 密钥板块。

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

- 无 env/无文件：启动服务；AI 路由 200 `keyConfigured=false`；用户保存 40001；后台自动生成 → 文件 0600、AI 可用；用户配置/补全正常。
- 手动合法/非法；env 来源后台只读；开关关闭 → 404；重新开启恢复。
- 用户配置存在时轮换：新密钥可解密；旧密钥失败；备份清理；无旧密钥+配置 → 40001。
- 清理临时密钥文件/用户配置/审计；恢复原 env/密钥；不删除既有数据。
- 浏览器如可用：管理页密钥板块/一次性 key/轮换确认、用户页三态；否则 API + 单测并记录。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md
git commit -m "docs: record AI key management"
```

---

## Self-Review

**Spec coverage:**

- KeyManager/配置/无密钥语义 → Task 1。
- 生成/手动/轮换/重加密/文件 → Task 2。
- OpenAPI/类型 → Task 3。
- 管理页/用户侧 → Task 4。
- 文档/检查/冒烟 → Task 5。

**Type consistency:**

- `NewKeyManager`/`KeyManager` Task 1；Task 2 使用。
- `KeyView/KeyGenerateResult` Task 2；Task 3/4 使用。
- `AIConfigView.keyConfigured` Task 1/3；Task 4 使用。
- `NewSettingsService(db, keys, keyFile)` Task 1；Task 2 extends.

**Review Focus mapping:**

1. 加载/优先级 → Task 1 tests。
2. 无密钥语义 → Task 1 tests。
3. 轮换安全 → Task 2 tests + Task 5 smoke。
4. 文件安全 → Task 2 tests。
5. 管理 UI → Task 4 tests。
6. 用户侧 → Task 4 tests。
7. 回归/类型 → Task 1–5.
