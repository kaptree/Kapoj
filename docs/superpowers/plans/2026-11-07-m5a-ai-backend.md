# M5-A AI 后端基础 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 M5-1～M5-3 的 AI 后端基础：多 provider 配置、用户 API key AES 加密存储、OpenAI 兼容调用、补全/格式化/优化接口、题目 AI 提示缓存与限流。

**Architecture:** 新增 `internal/ai` 领域包（模型/provider seed/加密/OpenAI 客户端/提示缓存）与 `internal/aiapi` HTTP 层。用户配置通过 `/api/ai/configs` 管理；AI 调用使用用户自己的 encrypted key，按 provider 的 base_url 发起 OpenAI 兼容 `/chat/completions` 请求；AI 提示按 `problem_id` 全局缓存。AI 路由只在配置 `KAPOJ_AI_ENCRYPTION_KEY` 时挂载。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis、OpenAI-compatible HTTP API、testify。

**Spec:** `docs/superpowers/specs/2026-11-06-admin-rbac-design.md` 不直接覆盖 M5；M5 规格见 `DEVELOPMENT.md` 3.9、5.6、6.1–6.5、7。

## Global Constraints

- AI 接口仅登录用户可用；M5-A 不新增管理员权限码。
- 用户 API key 只能 AES-GCM 加密存储；接口只返回脱敏值，不写日志、不写审计摘要。
- 平台 provider 的密钥不写入数据库；用户自己填写 key。M5-A 不支持平台代付 key。
- 题目提示缓存唯一键 `problem_id`；首个用户触发生成，后续用户复用。
- AI 调用按用户 Redis 冷却；缓存命中不消耗冷却。
- 生成/格式化/优化返回内容不能写入日志。
- AI 上游超时、HTTP 错误、无效 key 返回可读错误，不泄漏 key 或上游响应体敏感内容。
- `KAPOJ_AI_ENCRYPTION_KEY` 必须是 base64 编码的 32 字节密钥；未配置时 AI 路由不挂载，其他服务正常启动。
- 所有用户可见文案中文；不推送、不合并分支。
- M5-A 不实现前端、出题造数据、HydroOJ 导入、证书、头像上传。

## Review Focus

1. **密钥安全**：AES-GCM 随机 nonce、错误密钥失败、数据库/API/日志无明文 key；测试放 Task 1。
2. **provider 解析**：仅 active provider 可用；未知 provider、空 model、缺 key 被拒绝；测试放 Task 1。
3. **缓存与边界**：同一题目并发/重复请求只保留一份提示；缓存命中不调用模型、不消耗限流；隐藏题不能通过 ID 泄漏提示；测试放 Task 3。
4. **上游错误**：超时、非 2xx、畸形 JSON、无效 key 都映射为稳定 API 错误且不泄漏上游敏感内容；测试放 Task 2。
5. **限流**：同一用户 AI 调用冷却；配置保存不占用 AI 冷却；Redis 故障返回 503；测试放 Task 2。

---

### Task 1: AI 模型、Provider Seed、加密与用户配置 API

**Files:**
- Create: `apps/server/internal/ai/model.go`
- Create: `apps/server/internal/ai/catalog.go`
- Create: `apps/server/internal/ai/seed.go`
- Create: `apps/server/internal/ai/crypto.go`
- Create: `apps/server/internal/ai/service.go`
- Create: `apps/server/internal/ai/service_test.go`
- Create: `apps/server/internal/ai/crypto_test.go`
- Create: `apps/server/internal/ai/seed_test.go`
- Create: `apps/server/internal/aiapi/handler.go`
- Create: `apps/server/internal/aiapi/handler_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: `testutil.PG`、`httpx`、`rbac.Service`（仅 hint 权限后续）、`db.Migrate`。
- Produces:
  - `ai.Provider{ID,Code,Name,BaseURL,DefaultModel,IsActive,APIKeyRef,CreatedAt,UpdatedAt}`。
  - `ai.UserConfig{ID,UserID,Provider,ModelName,APIKeyEncrypted,CreatedAt,UpdatedAt}`，唯一 `user_id`。
  - `ai.Hint{ID,ProblemID,Content,CreatedBy,CreatedAt}`，唯一 `problem_id`。
  - `ai.ProviderOption{Code,Name,DefaultModel}`。
  - `ai.ConfigView{Configured,Provider,ModelName,APIKeyMasked,Providers}`。
  - `ai.SaveConfigInput{Provider,APIKey,ModelName}`。
  - `ai.NewEncryptor(base64Key string) (*Encryptor,error)`；`Encrypt/Decrypt`。
  - `ai.SeedProviders(ctx,db) error`：幂等插入 deepseek/openai/qwen/glm；不覆盖已有 provider。
  - `ai.Service`/`ai.NewService(db,enc,limiter,client)`；`ListProviders`、`GetConfig`、`SaveConfig`。
  - `aiapi.Options{RBAC *rbac.Service}`；`GET /api/ai/configs`、`POST /api/ai/configs`，均 `RequireAuth`。
  - Config：`AIEncryptionKey`、`AIRequestTimeout`、`AICooldown`；默认 timeout 45s、cooldown 2s。
  - Main：db migrate 后 `ai.SeedProviders`；有合法 encryption key 才构造 AI service；Router `Deps.AI`。
  - Provider seed 值：
    - `deepseek` / DeepSeek / `https://api.deepseek.com/v1` / `deepseek-chat`
    - `openai` / OpenAI / `https://api.openai.com/v1` / `gpt-4o-mini`
    - `qwen` / 通义千问 / `https://dashscope.aliyuncs.com/compatible-mode/v1` / `qwen-plus`
    - `glm` / 智谱 GLM / `https://open.bigmodel.cn/api/paas/v4` / `glm-4-flash`

- [ ] **Step 1: 写失败测试**

```go
func TestEncryptorRoundTripAndWrongKey(t *testing.T)
// Encrypt -> Decrypt == original; ciphertext != plaintext; wrong key fails
func TestSeedProvidersIdempotent(t *testing.T)
// seed twice -> 4 providers; custom edit not overwritten
func TestAIConfigUpsertAndMask(t *testing.T)
// first save requires key; second blank key preserves existing; GET masked
func TestAIConfigRejectsUnknownProviderAndEmptyModel(t *testing.T)
func TestAIConfigAPIRequiresAuth(t *testing.T)
// GET/POST without token -> 401; with token -> 200 / configured=false
func TestMigrateCreatesAITables(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/db/ -run 'TestEncryptor|TestSeedProviders|TestAIConfig|TestMigrateCreatesAITables' -v`
Expected: FAIL（包/类型不存在）。

- [ ] **Step 3: 实现模型、加密、seed 与配置 API**

- DB AutoMigrate 追加 `ai.Provider`、`ai.UserConfig`、`ai.Hint`；唯一索引。
- `Encryptor`：AES-GCM，nonce 12 bytes，输出 `v1:`+base64(nonce+ciphertext)；key 必须是 base64 32 bytes。
- `SeedProviders`：按 code `ON CONFLICT DO NOTHING` 插入四个 provider；不更新已有行。
- `Service.GetConfig`：按 `user_id` 读取；无配置返回 `configured=false` 且带 active providers。
- `SaveConfig`：provider 必须 active；model 为空时用 provider.DefaultModel；首次 key 必填；已有配置且 key 为空时保留原密文；加密后 upsert；返回脱敏视图。
- API key mask：长度 >8 显示前 3 + `…` + 后 4，否则 `****`。
- `aiapi` 只用 `CurrentClaims.UserID`；POST body 使用 `ai.SaveConfigInput`。
- `config.Load` 增加 AI 配置解析；`KAPOJ_AI_ENCRYPTION_KEY` 为空时 main 不挂 AI 路由；非法 base64/长度在启动时返回错误。
- `main` 调用 `ai.SeedProviders`；SQLite 不需要，使用 Postgres。
- Router `Deps.AI *ai.Service`；`aiapi.RegisterRoutes(api, d.AI, aiapi.Options{RBAC: d.RBAC})`（RBAC 选项留给 hint）。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/ai internal/aiapi internal/db internal/config internal/router cmd/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/db/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/ai apps/server/internal/aiapi apps/server/internal/db apps/server/internal/config apps/server/internal/router apps/server/cmd/server
git commit -m "feat(server): add ai providers and encrypted user config"
```

---

### Task 2: OpenAI 兼容客户端、限流与补全/格式化/优化接口

**Files:**
- Create: `apps/server/internal/ai/client.go`
- Create: `apps/server/internal/ai/limiter.go`
- Create: `apps/server/internal/ai/prompts.go`
- Create: `apps/server/internal/ai/generate.go`
- Create: `apps/server/internal/ai/client_test.go`
- Create: `apps/server/internal/ai/generate_test.go`
- Create: `apps/server/internal/ai/limiter_test.go`
- Create: `apps/server/internal/aiapi/generate_handler.go`
- Create: `apps/server/internal/aiapi/generate_handler_test.go`
- Modify: `apps/server/internal/ai/service.go`
- Modify: `apps/server/internal/aiapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 的 models/encryptor/config/USER API。
- Produces:
  - `ai.ChatMessage{Role,Content string}`；`ai.ChatRequest{BaseURL,APIKey,Model string; Messages []ChatMessage; Temperature float64}`。
  - `ai.ChatClient` 接口 `Chat(ctx, req) (string,error)`。
  - `ai.OpenAIClient`，默认 HTTP timeout 来自 `AIRequestTimeout`。
  - `ai.Limiter` 接口 `Allow(ctx,userID) (bool,error)`；`ai.NewRedisLimiter(rdb,cooldown)`。
  - `ai.Complete(ctx,userID,in)` → `{completion}`；`ai.Format` → `{code}`；`ai.Optimize` → `{suggestions,code}`。
  - `aiapi`：`POST /api/ai/complete`、`POST /api/ai/format`、`POST /api/ai/optimize`，均登录 + 限流。
  - 输入：`{code,language,cursor?}`；代码最大 64 KiB；language 非空 ≤32。
  - 上游错误映射为 `50200 AI 服务调用失败`；限流为 `42903 AI 请求过于频繁，请稍后再试`。
  - `httpx` 增加 `ErrTooManyAIRequests` 与 `ErrBadGateway`。

- [ ] **Step 1: 写失败测试**

```go
func TestOpenAIClientSendsAuthAndParsesContent(t *testing.T)
// httptest server checks Authorization / model / messages; returns choices[0].message.content
func TestOpenAIClientErrors(t *testing.T)
// non-2xx, malformed JSON, timeout -> error without leaking key
func TestAICompleteFormatOptimize(t *testing.T)
// fake client returns expected content; prompts include language/code/cursor
func TestAIRateLimit(t *testing.T)
// first allowed, second rejected; cached hint not limited (Task 3)
func TestAIGenerateRequiresConfig(t *testing.T)
// no config -> 40001; bad provider -> error; code limit -> 40001
func TestAIGenerateAPIRequiresAuth(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ -run 'TestOpenAIClient|TestAIComplete|TestAIRate|TestAIGenerate' -v`
Expected: FAIL。

- [ ] **Step 3: 实现客户端、限流与生成接口**

- `OpenAIClient.Chat`：POST `{baseURL}/chat/completions`，Bearer key，JSON `model/messages/temperature`；解析 `choices[0].message.content`；非 2xx/空 choices/畸形 JSON 返回错误。
- `RedisLimiter` key `kapoj:ai:cool:<userID>`，`SetNX` cooldown；Redis 错误向上传递。
- 生成服务先加载配置并解密 key；未知/停用 provider、空 model、密钥解密失败返回 40001 或 502。
- 先校验输入，再加载配置；首次调用才消耗 limiter；`Complete` 提示模型只返回补全片段；`Format` 返回完整代码；`Optimize` 要求 JSON `{suggestions,code}`，解析失败时把原文作为 suggestions、原代码作为 code（不丢用户代码）。
- 去除模型返回的单层 Markdown code fence（complete/format）。
- `aiapi` handlers 使用 `CurrentClaims`；返回中文错误。
- `httpx` 增加稳定错误变量。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/ai internal/aiapi internal/httpx internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/ai apps/server/internal/aiapi apps/server/internal/httpx apps/server/internal/router
git commit -m "feat(server): add ai generation endpoints and rate limiting"
```

---

### Task 3: 题目 AI 提示缓存

**Files:**
- Create: `apps/server/internal/ai/hint.go`
- Create: `apps/server/internal/ai/hint_test.go`
- Create: `apps/server/internal/aiapi/hint_handler.go`
- Create: `apps/server/internal/aiapi/hint_handler_test.go`
- Modify: `apps/server/internal/ai/service.go`
- Modify: `apps/server/internal/aiapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 2 客户端/限流、`rbac.Access`、problem table。
- Produces:
  - `ai.HintInput{ProblemID int64}`；`ai.HintView{Hint string; Cached bool}`。
  - `ai.Service.Hint(ctx,userID,problemID,canViewHidden bool)`。
  - `POST /api/ai/hint`，登录 + `LoadAccess`（用于隐藏题权限）。
  - 缓存命中不调用模型、不消耗 limiter；首次生成后 `INSERT ... ON CONFLICT DO NOTHING`，返回最终缓存内容。
  - 公开题允许所有登录用户；非公开题仅 `problem.view_hidden` 或 super 可请求。
  - 题面不存在返回 404；无配置/上游错误返回对应错误。

- [ ] **Step 1: 写失败测试**

```go
func TestAIHintCachesFirstResult(t *testing.T)
// first calls fake client and stores one row; second returns cached=true, calls=1
func TestAIHintDoesNotRateLimitCached(t *testing.T)
// consume limiter after first, second cached still succeeds
func TestAIHintHiddenProblemPermission(t *testing.T)
// public user 404; view_hidden/super allowed; nonexistent 404
func TestAIHintConcurrentKeepsSingleCache(t *testing.T)
// concurrent hint calls -> one ai_hints row; response consistent
func TestAIHintRequiresAuth(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ -run 'TestAIHint' -v`
Expected: FAIL。

- [ ] **Step 3: 实现提示缓存与权限**

- 查询 `problems` 的 `title/statement_md/input_format_md/output_format_md/hint_md/is_public`；不存在 404；非公开且无权限 404（不泄露存在性）。
- 先查 `ai_hints`：命中直接返回 `cached=true`。
- 未命中：要求用户配置，limiter，调用模型；提示词要求只返回中文提示、不返回完整题解代码。
- `INSERT INTO ai_hints ... ON CONFLICT (problem_id) DO NOTHING`；再读取最终行；返回 `cached=false`（若并发写入者已存在，返回其内容，仍可标记 `cached=true`）。
- Handler 通过 `middleware.CurrentAccess` 判断 `problem.view_hidden`/super；无 Access 时 false。
- `aiapi.RegisterRoutes` 给 hint 路由加 `middleware.LoadAccess`。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/ai internal/aiapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/ai/ ./internal/aiapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/ai apps/server/internal/aiapi apps/server/internal/router
git commit -m "feat(server): add cached ai problem hints"
```

---

### Task 4: OpenAPI、文档、全量检查与真实冒烟

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `.env.example`
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端

**Interfaces:**
- Consumes: Task 1–3。
- Produces: AI 配置/生成/提示 OpenAPI 契约、前端类型、启用说明与验证记录。

- [ ] **Step 1: OpenAPI**

- Schemas：`AIConfigView`、`AIProviderOption`、`AISaveConfigInput`、`AICompleteInput/Output`、`AIFormatInput/Output`、`AIOptimizeInput/Output`、`AIHintInput/Output`。
- Paths `/ai/configs` GET/POST、`/ai/complete`、`/ai/format`、`/ai/optimize`、`/ai/hint`；401/400/403/404/429/502/503 描述。
- `corepack pnpm gen:api`。

- [ ] **Step 2: 文档**

- `.env.example`：`KAPOJ_AI_ENCRYPTION_KEY`（base64 32 bytes 生成方式）、`KAPOJ_AI_REQUEST_TIMEOUT`、`KAPOJ_AI_COOLDOWN`。
- `DEVELOPMENT.md`：M5-1～M5-3 完成；AI 关闭/启用语义；provider/加密/缓存/限流；M5-4～M5-8 待办。
- `docs/任务清单.md`：M5-1～M5-3 ✅，M5-4～M5-8 ⬜；计划链接。
- `docs/开发日志.md`：M5-A 条目（模型、加密、provider、客户端、限流、提示缓存、验证）。
- `README.md`、`运行手册.md`：AI 功能启用方式、用户如何配置 key、无 key 时 AI 路由不挂载；不要把真实 key 写进文档。
- 不修改 RBAC。

- [ ] **Step 3: 全量检查**

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

- [ ] **Step 4: 真实 API 冒烟**

- 生成临时 32-byte base64 AI key；启动 server 时设置 `KAPOJ_AI_ENCRYPTION_KEY`。
- 启动一个假的 OpenAI 兼容 HTTP 服务（本地 `httptest` 或临时 Python/Go server），临时把 provider base_url 改为该地址；不要调用真实外部 AI。
- 登录测试用户，保存 AI provider/key，GET 配置确认脱敏；调用 complete/format/optimize/hint，确认 fake provider 被调用、hint 第二次命中缓存且不再调用 provider。
- 验证无配置用户调用 AI 返回 40001；隐藏题 hint 对普通用户 404，对 super 可用。
- 清理临时 provider base_url 修改、AI 配置、提示和测试数据；不要删除既有数据。
- 记录所有临时变更与恢复结果。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts .env.example DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record m5-a ai backend"
```

---

## Self-Review

**Spec coverage:**

- provider/密钥加密/用户配置 → Task 1。
- OpenAI 客户端/补全/格式化/优化/限流 → Task 2。
- AI 题目提示缓存/隐藏题权限 → Task 3。
- OpenAPI/文档/全量检查/冒烟 → Task 4。
- AI 前端（M5-5）、出题造数据（M5-4）、HydroOJ（M5-6）、证书（M5-7）、头像（M5-8）不在本计划。

**Type consistency:**

- `UserConfig.Provider` 使用 provider code；`ProviderOption.DefaultModel` 与 `UserConfig.ModelName` 一致。
- `Hint.ProblemID` 唯一；`HintView.Cached` 语义固定。
- 所有 AI 路由登录；hint 额外 LoadAccess；M5-A 不新增权限码。
- `ErrTooManyAIRequests` 42903、`ErrBadGateway` 50200 在 API 与 OpenAPI 一致。

**Review Focus mapping:**

1. 密钥安全 → Task 1 crypto tests + API masking。
2. provider 解析 → Task 1 config tests。
3. 缓存/隐藏题 → Task 3 tests。
4. 上游错误 → Task 2 client tests。
5. 限流/Redis → Task 2 limiter tests。

**Proportion:** 计划固定 API、加密/provider/缓存/限流语义与测试；具体 prompt 文案和 HTTP 客户端细节留给实现者。
