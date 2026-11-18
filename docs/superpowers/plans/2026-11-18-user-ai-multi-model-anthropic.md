# 用户 AI 多模型、多 Token 与 Anthropic 协议 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用户可保存多条 AI 模型配置（独立 token、自定义接口/模型名、OpenAI/Anthropic 协议）并选择当前使用；平台模型同步支持两种协议。

**Architecture:** 新增 `user_ai_models` 多行表与 active 选择；`ChatRequest` 增加协议并由 `ProtocolClient` 分发到 OpenAI/Anthropic 客户端；`ai.Service.chatRequest` 使用当前用户模型，旧 `user_ai_configs` 自动回填并保留兼容；平台模型增加 `protocol` 并复用同一客户端。

**Tech Stack:** Go 1.26、Gin、GORM/PostgreSQL、AES-GCM KeyManager、net/http；Vue 3.5、PrimeVue 4.5、Vitest、OpenAPI TypeScript 生成。

**Spec:** `docs/superpowers/specs/2026-11-18-user-ai-multi-model-anthropic-design.md`

## Global Constraints

- `user_ai_models` 每用户可多行；`(user_id, name)` 唯一；`(user_id) WHERE is_active` 部分唯一，最多一条 active。
- `protocol` 只接受 `openai` / `anthropic`；空值按 `openai` 处理。
- token 只以 AES-GCM 密文存储；API/日志/测试结果不返回明文或密文。
- OpenAI：`{base}/chat/completions`，`Authorization: Bearer`；拒绝 base URL 以 `/chat/completions` 结尾。
- Anthropic：base 以 `/v1` 结尾用 `{base}/messages`，否则 `{base}/v1/messages`；`x-api-key`、`anthropic-version: 2023-06-01`；system 消息抽出；`max_tokens` 必填（默认 4096，测试 16）。
- 用户 AI 功能使用当前 active 模型；无 active 时回退旧 `user_ai_configs`；无配置返回 40001“尚未配置 AI 模型”。
- 旧 `/api/ai/configs` GET/POST 继续兼容；新页面使用 `/api/ai/models`。
- 密钥轮换/重置、账号备份必须覆盖 `user_ai_models`；旧表保留并兼容重加密。
- 所有新错误文案中文；OpenAPI 先改，`corepack pnpm gen:api` 生成类型。
- 后端 DB 测试用 `-p 1` 并 source 根 `.env`；前端用 Vitest。

## Review Focus

1. **Anthropic 端点/请求格式**：base URL 归一化、system 抽取、max_tokens、content 拼接、错误脱敏；测试 `TestAnthropicChat` / `TestAnthropicURL`（Task 2）。
2. **active 唯一与删除当前模型**：并发切换、active 部分唯一索引、删除 active 拒绝；测试 `TestSetActiveUserModel` / `TestDeleteActiveUserModelRejected`（Task 3）。
3. **旧配置迁移与回退**：旧 `user_ai_configs` 回填为 active、无 active 时回退、旧 `/configs` 兼容；测试 `TestBackfillUserAIModels` / `TestChatRequestFallsBackToLegacyConfig`（Task 1/3/4）。
4. **密钥轮换/重置/备份**：新表重加密、reset 清空、账号备份包含新表；测试 `TestRotateReencryptsUserModels` / `TestAccountBackupIncludesUserModels`（Task 5）。
5. **自定义接口协议不匹配**：Anthropic base 带 `/v1`/不带 `/v1`、OpenAI 带 `/chat/completions` 应拒绝；测试 Task 2/6。

---

## File Structure

**新增后端文件**

- `apps/server/internal/ai/user_model.go` — `UserModel` 模型、视图/输入/测试结果、CRUD/active/test。
- `apps/server/internal/ai/anthropic.go` — Anthropic 客户端与 URL/消息转换。
- `apps/server/internal/ai/protocol_client.go` — 协议分发客户端。
- `apps/server/internal/ai/url.go` — 共享 base URL/协议校验（可选，若 platform.go 已有函数可迁移）。
- `apps/server/internal/aiapi/user_models.go` — 用户模型 HTTP handlers。
- 测试：`ai/user_model_test.go`、`ai/anthropic_test.go`、`ai/protocol_client_test.go`、`aiapi/user_models_test.go`、`ai/backup_test.go`（或 account 测试）。

**修改的后端文件**

- `ai/model.go` — `UserModel`、`PlatformModel.Protocol`、`UserConfig` 保留。
- `ai/service.go` — `ProviderOption` 增加 baseUrl/protocol；`AIConfigView` 兼容字段。
- `ai/generate.go` — `chatRequest` 使用 active 用户模型/旧配置回退。
- `ai/client.go` — `ChatRequest.Protocol`；OpenAI base URL 校验。
- `ai/platform.go` — 平台协议字段/校验/test。
- `ai/difficulty.go` — 平台 `resolvePlatformChat` 设置 Protocol/MaxTokens。
- `ai/key_service.go` — 轮换/重置新表。
- `aiapi/handler.go` — 挂载新路由；旧 `/configs` 适配。
- `adminapi/ai_platform_handler.go` — protocol 透传。
- `backup/account.go` — `user_ai_models` 表清单。
- `db/db.go` — AutoMigrate + backfill。
- `cmd/server/main.go` — `ai.NewProtocolClient`。
- `packages/api/openapi.yaml` — 新路径/schemas。

**前端文件**

- `apps/web/src/views/AiSettingsView.vue`、`.spec.ts` — 列表/新增/编辑/当前/测试/删除。
- `apps/web/src/views/AdminAiView.vue`、`.spec.ts` — 平台模型 protocol。
- `apps/web/src/api/ai.ts`、`api/admin.ts`、`api/types.ts`。
- 生成 `apps/web/src/api/schema.d.ts`。

---

### Task 1: 用户模型表、平台 protocol 列与旧配置回填

**Files:**
- Modify: `apps/server/internal/ai/model.go`
- Modify: `apps/server/internal/db/db.go`
- Test: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Produces:
  ```go
  type UserModel struct {
      ID int64
      UserID int64
      Name string
      Protocol string
      BaseURL string
      ModelName string
      APIKeyEncrypted string
      IsActive bool
      LastTestAt *time.Time
      LastTestOK bool
      LastTestError string
      LastTestLatencyMS int
      CreatedAt, UpdatedAt time.Time
  }
  func (UserModel) TableName() string { return "user_ai_models" }
  ```
- `PlatformModel.Protocol string`（`size:20;not null;default:openai`）。
- `db.Migrate` AutoMigrate `&ai.UserModel{}`；原生 SQL 建 `uniq_user_ai_models_active` 部分唯一索引 `(user_id) WHERE is_active`。
- 回填 key `user_ai_models_backfill_v1`。

- [ ] **Step 1: 写失败测试**

```go
func TestMigrateCreatesUserAIModelsSchema(t *testing.T) {
    // open test DB, Migrate; assert table, columns, unique indexes.
}

func TestBackfillUserAIModels(t *testing.T) {
    // insert ai_providers + user_ai_configs row; delete marker; Migrate;
    // assert one user_ai_models row: protocol=openai, base_url=provider base,
    // model_name copied, api_key_encrypted copied, is_active=true.
    // run Migrate again; assert still one row.
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/db -run 'TestMigrateCreatesUserAIModels|TestBackfillUserAIModels' -v`
Expected: FAIL，表/字段不存在。

- [ ] **Step 3: 实现模型、迁移与回填**

- `UserModel` 表字段/索引按设计 §3.1；`Name`/`UserID` 联合唯一；`Protocol` 默认 `openai`。
- `PlatformModel` 加 `Protocol`。
- `db.Migrate` AutoMigrate；raw SQL 建 active 部分唯一索引（幂等 `IF NOT EXISTS`）。
- `backfillUserAIModels`：`INSERT ... SELECT` join `ai_providers`，`WHERE NOT EXISTS` 同用户；name 截断 100；`is_active=true`；写 marker。
- 测试 DB 复用：按测试用户名/ID 清理，不 truncate 全库。

- [ ] **Step 4: 运行测试确认通过**

Run: 同上。
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai/model.go apps/server/internal/db/db.go apps/server/internal/db/db_test.go
git commit -m "feat(ai): add user AI models schema and backfill"
```

---

### Task 2: 协议客户端（OpenAI/Anthropic）

**Files:**
- Modify: `apps/server/internal/ai/client.go`
- Create: `apps/server/internal/ai/anthropic.go`, `apps/server/internal/ai/protocol_client.go`
- Test: `apps/server/internal/ai/client_test.go`, `anthropic_test.go`, `protocol_client_test.go`

**Interfaces:**
- `ChatRequest.Protocol string`（空/`openai` → OpenAI，`anthropic` → Anthropic）。
- `type ProtocolClient struct`；`func NewProtocolClient(timeout time.Duration) *ProtocolClient`；实现 `ChatClient`。
- `type AnthropicClient struct`；`func NewAnthropicClient(timeout time.Duration) *AnthropicClient`。
- `func normalizeAIBaseURL(raw, protocol string) (string, error)`（可与 platform 共享）。
- OpenAI base URL 拒绝 `/chat/completions`；Anthropic 拒绝 `/v1/messages`/`/messages`。

- [ ] **Step 1: 写失败测试**

```go
func TestAnthropicURL(t *testing.T) {
    // https://api.anthropic.com -> https://api.anthropic.com/v1/messages
    // https://api.anthropic.com/v1 -> https://api.anthropic.com/v1/messages
    // .../v1/messages -> error
}
func TestAnthropicChat(t *testing.T) {
    // httptest server asserts x-api-key, anthropic-version, system, messages,
    // max_tokens default 4096/override, response content text concat.
}
func TestProtocolClientDispatch(t *testing.T) {
    // openai/empty hits /chat/completions; anthropic hits /v1/messages; unknown error.
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/ai -run 'TestAnthropic|TestProtocolClient' -v`
Expected: FAIL，类型不存在。

- [ ] **Step 3: 实现客户端**

- `ChatRequest.Protocol`；OpenAI 默认行为不变；base URL 校验。
- Anthropic：
  - URL 规则按设计 §4.3；拒绝端点后缀。
  - 请求头/体；system 抽取；`max_tokens` 默认 4096。
  - response `content[]` 拼接 text；空/非法/非 2xx 映射稳定错误；不泄露 token/body。
- `ProtocolClient` 分发；未知协议返回 `ErrValidation("不支持的 AI 协议")`。
- 不在本任务修改 `main.go`/`Service` 构造。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/ai -run 'TestAnthropic|TestProtocolClient|TestChat' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai/client.go apps/server/internal/ai/anthropic.go apps/server/internal/ai/protocol_client.go apps/server/internal/ai/*_test.go
git commit -m "feat(ai): add Anthropic and protocol-dispatch clients"
```

---

### Task 3: 用户模型服务、active 选择与 chatRequest

**Files:**
- Create: `apps/server/internal/ai/user_model.go`
- Modify: `apps/server/internal/ai/service.go`, `apps/server/internal/ai/generate.go`
- Test: `apps/server/internal/ai/user_model_test.go`, `generate_test.go`

**Interfaces:**
- `ProviderOption` 增加 `BaseURL string`, `Protocol string`。
- `UserModelView`/`UserModelInput`/`UserModelTestResult`/`UserModelsView` 按设计 §7。
- `Service` 方法：
  ```go
  func (s *Service) ListUserModels(ctx context.Context, userID int64) (*UserModelsView, error)
  func (s *Service) CreateUserModel(ctx context.Context, userID int64, in UserModelInput) (*UserModelView, error)
  func (s *Service) UpdateUserModel(ctx context.Context, userID, id int64, in UserModelInput) (*UserModelView, error)
  func (s *Service) DeleteUserModel(ctx context.Context, userID, id int64) error
  func (s *Service) SetActiveUserModel(ctx context.Context, userID, id int64) (*UserModelView, error)
  func (s *Service) TestUserModel(ctx context.Context, userID, id int64) (*UserModelTestResult, error)
  ```
- `chatRequest` 使用 active；无 active 回退旧 `user_ai_configs`；返回 `Protocol`、`MaxTokens:4096`。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateUserModelEncryptsAndAutoActivatesFirst(t *testing.T) { /* ... */ }
func TestSetActiveUserModelEnforcesSingleActive(t *testing.T) { /* ... */ }
func TestDeleteActiveUserModelRejected(t *testing.T) { /* ... */ }
func TestTestUserModelUpdatesLastTest(t *testing.T) { /* ... */ }
func TestChatRequestUsesActiveAndFallsBackToLegacy(t *testing.T) { /* ... */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/ai -run 'TestCreateUserModel|TestSetActive|TestDeleteActive|TestTestUserModel|TestChatRequest' -v`
Expected: FAIL。

- [ ] **Step 3: 实现服务与解析**

- 校验 name/protocol/base URL/model/token；base URL 按协议规则；token 加密；名称重复 40001。
- 创建时：首个模型或 `isActive=true` → active；否则 false。
- `SetActive` 事务清空其他 active；`Delete` active 拒绝；`Test` 用 `ChatRequest{Protocol,...MaxTokens:16}` 并更新 last_test。
- `chatRequest`：active → 旧配置回退 → 40001；`ProviderOption` 带 base URL/protocol（openai）。
- 旧 `SaveConfig`/`GetConfig` 暂不改（Task 4 兼容）。

- [ ] **Step 4: 运行测试确认通过**

Run: 同上 + `go test -p 1 ./internal/ai -run 'TestGenerate|TestHint' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai/user_model.go apps/server/internal/ai/service.go apps/server/internal/ai/generate.go apps/server/internal/ai/user_model_test.go apps/server/internal/ai/generate_test.go
git commit -m "feat(ai): add user model CRUD and active selection"
```

---

### Task 4: 用户模型 API 与旧 /configs 兼容

**Files:**
- Modify: `apps/server/internal/aiapi/handler.go`
- Create: `apps/server/internal/aiapi/user_models.go`
- Test: `apps/server/internal/aiapi/user_models_test.go`, `handler_test.go`

**Interfaces:**
- Routes：
  - `GET /api/ai/models`
  - `POST /api/ai/models`
  - `PUT /api/ai/models/:id`
  - `DELETE /api/ai/models/:id`
  - `PUT /api/ai/models/:id/active`
  - `POST /api/ai/models/:id/test`
- `GET/POST /api/ai/configs` 继续工作：GET 返回 active 模型兼容字段；POST 更新/创建 active（openai）。

- [ ] **Step 1: 写失败测试**

```go
func TestUserModelRoutes(t *testing.T) { /* 401; list; create; update; active; delete; test */ }
func TestLegacyConfigRoutesUseActiveModel(t *testing.T) { /* GET/POST /ai/configs */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/aiapi -run 'TestUserModel|TestLegacyConfig' -v`
Expected: FAIL，路由不存在。

- [ ] **Step 3: 实现 handlers 与兼容**

- 解析 current user；调用 Service；错误中文。
- 旧 `getConfig` 使用 active 模型（有 provider 匹配则填 provider，否则空）；`saveConfig` 更新 active 或创建 openai active。
- 路由挂在现有 `/ai` group（RequireAIEnabled 已覆盖）。
- 不改变 `/complete`/`/hint`。

- [ ] **Step 4: 运行测试确认通过**

Run: 同上 + `go test -p 1 ./internal/aiapi -run 'TestAI' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/aiapi
git commit -m "feat(ai): add user model API and legacy config compatibility"
```

---

### Task 5: 密钥轮换、重置与账号备份

**Files:**
- Modify: `apps/server/internal/ai/key_service.go`
- Modify: `apps/server/internal/backup/account.go`
- Test: `apps/server/internal/ai/key_service_test.go`, `apps/server/internal/backup/account_test.go`

**Interfaces:**
- keep 轮换统计/重加密 `user_ai_models`（以及旧 `user_ai_configs`）；无旧密钥但有密文 → 拒绝。
- reset 删除 `user_ai_models` + 旧表。
- 账号备份 `user_ai_models` 加入表清单/恢复清单。

- [ ] **Step 1: 写失败测试**

```go
func TestRotateReencryptsUserModels(t *testing.T) { /* ... */ }
func TestResetDeletesUserModels(t *testing.T) { /* ... */ }
func TestAccountBackupIncludesUserModels(t *testing.T) { /* ... */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/ai ./internal/backup -run 'TestRotateReencryptsUserModels|TestResetDeletesUserModels|TestAccountBackupIncludesUserModels' -v`
Expected: FAIL。

- [ ] **Step 3: 实现轮换/重置/备份**

- `rotate` 统计三类密文；`reencryptUserModels` 事务 `SELECT FOR UPDATE`；reset 删除新/旧用户配置，保留平台元数据。
- `backup/account.go` 表清单与恢复校验加 `user_ai_models`。
- 注意旧 `user_ai_configs` 仍重加密以支持回滚。

- [ ] **Step 4: 运行测试确认通过**

Run: 同上 + `go test -p 1 ./internal/ai ./internal/backup -run 'TestKey|TestRotate|TestReset|TestAccount' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai/key_service.go apps/server/internal/ai/key_service_test.go apps/server/internal/backup/account.go apps/server/internal/backup/account_test.go
git commit -m "feat(ai): rotate and back up user AI models"
```

---

### Task 6: 平台模型 protocol

**Files:**
- Modify: `apps/server/internal/ai/model.go`, `ai/platform.go`, `ai/difficulty.go`
- Modify: `apps/server/internal/adminapi/ai_platform_handler.go`
- Test: `apps/server/internal/ai/platform_test.go`, `ai/difficulty_test.go`, `adminapi/ai_platform_handler_test.go`

**Interfaces:**
- `PlatformModel.Protocol`；`PlatformModelInput/View.Protocol`；`normalizeAIProtocol`。
- `PlatformService.Create/Update` 校验协议；`Test` 用 `ChatRequest{Protocol,...MaxTokens:16}`。
- `resolvePlatformChat` 设置 `Protocol`；`AssessDifficulty` 用 `MaxTokens:512`。

- [ ] **Step 1: 写失败测试**

```go
func TestPlatformModelProtocolCRUD(t *testing.T) { /* openai/anthropic; invalid */ }
func TestPlatformAnthropicTestConnection(t *testing.T) { /* fake anthro server */ }
func TestAssessDifficultyUsesPlatformProtocol(t *testing.T) { /* captured ChatRequest */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/ai ./internal/adminapi -run 'TestPlatformModelProtocol|TestPlatformAnthropic|TestAssessDifficultyUsesPlatformProtocol' -v`
Expected: FAIL。

- [ ] **Step 3: 实现平台协议**

- model/view/input 加 Protocol；校验；test/resolve/difficulty 设置 Protocol。
- admin handler 透传。
- 不改变平台模型选择/删除/审计语义。

- [ ] **Step 4: 运行测试确认通过**

Run: 同上 + `go test -p 1 ./internal/ai ./internal/adminapi -run 'TestPlatform|TestAssessDifficulty' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai/model.go apps/server/internal/ai/platform.go apps/server/internal/ai/difficulty.go apps/server/internal/adminapi/ai_platform_handler.go apps/server/internal/ai/*_test.go apps/server/internal/adminapi/*_test.go
git commit -m "feat(ai): add protocol to platform models"
```

---

### Task 7: OpenAPI、类型与前端 API 客户端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`, `apps/web/src/api/ai.ts`, `apps/web/src/api/admin.ts`

**Interfaces:**
- 新 paths：用户 `/ai/models` CRUD/active/test；`AIUserModelsView`、`AIUserModelView/Input`、`AIModelTestResult`。
- `AIProviderOption` 增加 `baseUrl/protocol`；`PlatformModelView/Input` 增加 `protocol`。
- API functions：
  ```ts
  listAIUserModels(); createAIUserModel(input); updateAIUserModel(id,input);
  deleteAIUserModel(id); setActiveAIUserModel(id); testAIUserModel(id);
  ```
- 保留 `getAIConfig/saveAIConfig`（兼容）。

- [ ] **Step 1: 基线构建**

Run: `corepack pnpm --filter web build`
Expected: PASS。

- [ ] **Step 2: 修改 OpenAPI 并生成**

Run: `corepack pnpm gen:api`；在 `types.ts` 加别名；在 `api/ai.ts` 加函数；`api/admin.ts` 平台 input 类型自动带 protocol。

- [ ] **Step 3: 构建确认**

Run: `corepack pnpm --filter web build`
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/ai.ts apps/web/src/api/admin.ts
git commit -m "feat(api): add user AI models and protocol contracts"
```

---

### Task 8: 用户 AI 设置页多模型

**Files:**
- Modify: `apps/web/src/views/AiSettingsView.vue`, `.spec.ts`
- Modify: `apps/web/src/api/ai.ts`

**Interfaces:**
- 页面使用 `listAIUserModels/create/update/delete/setActive/test`。
- 列表/卡片显示 name、protocol、base URL、model、masked、active、last test。
- Dialog：name、protocol、base URL/model、API key、预填 provider、设为当前。
- 当前模型删除禁用；测试 Toast；空状态。

- [ ] **Step 1: 写失败测试**

```ts
it('lists saved models and marks the active one', ...)
it('creates a custom Anthropic model', ...)
it('sets a different model active', ...)
it('disables deleting the active model', ...)
it('tests a model and shows sanitized result', ...)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AiSettingsView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现页面**

- 保留 AI 未启用/密钥未配置状态。
- 列表 + 新增/编辑 Dialog + active/测试/删除；provider 预填 base URL/model/protocol=openai。
- token 只显示 masked；编辑留空保留；错误中文。
- 旧 `getAIConfig/saveAIConfig` 不再被新页面使用，但保留导出。

- [ ] **Step 4: 运行测试确认通过**

Run: 同上 + `corepack pnpm --filter web build`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AiSettingsView.vue apps/web/src/views/AiSettingsView.spec.ts apps/web/src/api/ai.ts
git commit -m "feat(web): manage multiple user AI models and tokens"
```

---

### Task 9: 管理后台平台模型 protocol

**Files:**
- Modify: `apps/web/src/views/AdminAiView.vue`, `.spec.ts`
- Modify: `apps/web/src/api/admin.ts`（若类型需要）

**Interfaces:**
- 新增/编辑平台模型 Dialog 增加协议 Select（OpenAI/Anthropic）。
- 列表显示协议 Tag；测试按钮按协议；当前选择/删除逻辑不变。
- `PlatformModelInput` 发送 `protocol`。

- [ ] **Step 1: 写失败测试**

```ts
it('creates an Anthropic platform model', ...)
it('shows the protocol in the table and sends it on update', ...)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AdminAiView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现协议 UI**

- Dialog 加 Protocol Select；表格列；测试结果不变；key-disabled 逻辑不变。
- 更新 fixtures 增加 protocol。

- [ ] **Step 4: 运行测试确认通过**

Run: 同上 + `corepack pnpm --filter web build`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminAiView.vue apps/web/src/views/AdminAiView.spec.ts
git commit -m "feat(web): support Anthropic platform models"
```

---

### Task 10: 文档、全量验证与真实冒烟

**Files:**
- Modify: `DEVELOPMENT.md`, `运行手册.md`, `README.md`（相关 AI 设置/协议段落）, `docs/任务清单.md`, `docs/开发日志.md`
- Test: all backend/web suites.

- [ ] **Step 1: 更新文档**

- 用户多模型/active/协议/自定义接口；Anthropic 端点/头/max_tokens；旧 `/configs` 兼容；迁移/密钥/备份。
- 平台模型 protocol。
- 运行手册加“用户 AI 设置”和“Anthropic 配置示例”。

- [ ] **Step 2: 全量验证**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" KAPOJ_TEST_DATABASE_URL="$KAPOJ_TEST_DATABASE_URL" go test -count=1 -p 1 ./...
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```
Expected: PASS。

- [ ] **Step 3: 真实冒烟（如可行）**

- 临时 DB/本地 server + 假 OpenAI 与 Anthropic 服务；保存两套用户模型、切换 active、分别调用 `/ai/complete`；密钥轮换后两套 token 可用；管理员平台 Anthropic 模型测试 + 题目初评。
- 清理临时数据；若未执行，报告明确说明。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md README.md 运行手册.md docs
git commit -m "docs: document user AI models and Anthropic protocol"
```

---

## Self-Review

- Spec coverage: 数据模型/迁移（T1）、协议客户端（T2）、用户 CRUD/active/chatRequest（T3）、API/旧兼容（T4）、密钥/备份（T5）、平台 protocol（T6）、OpenAPI/types（T7）、用户 UI（T8）、平台 UI（T9）、文档/验证（T10）。
- Type consistency: `UserModelView/Input/TestResult` 在 T3 定义，T4/7/8 使用；`PlatformModel.Protocol` 在 T1 定义，T6/7/9 使用；`ProtocolClient/AnthropicClient` 在 T2 定义，T3/6 使用。
- Review Focus tests: T2 Anthropic URL/格式；T3 active/delete；T1/T3/T4 迁移/回退/旧接口；T5 轮换/备份；T2/T6 协议/URL 不匹配。
- Proportion: 计划按任务给签名、测试与命令，具体实现留给执行者。
