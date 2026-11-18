# 用户 AI 多模型、多 Token 与 Anthropic 协议设计

> 目标：用户 AI 设置从“每个用户一套 provider/key/model”升级为“可保存多套模型配置，每套独立 token，支持自定义接口地址与模型名，支持 OpenAI 与 Anthropic 协议，并指定当前使用哪一套”；平台级 AI 模型同步支持 OpenAI/Anthropic 两种协议。

## 1. 背景与现状

- 用户 AI 配置目前是 `user_ai_configs` 单行：`user_id` 唯一，字段为 `provider`（`ai_providers.code`）、`model_name`、`api_key_encrypted`。`ai_providers` 只是内置 provider 目录，base URL 由目录决定，用户不能自定义接口地址。
- `ai.Service.chatRequest(userID)` 读 `user_ai_configs` + `ai_providers`，返回 `ChatRequest{BaseURL,APIKey,Model}`；`OpenAIClient` 只支持 OpenAI 兼容的 `{base}/chat/completions`。
- AI 用户接口：`GET/POST /api/ai/configs`；`AiSettingsView.vue` 是单表单（provider 下拉、模型名、API Key）。
- 平台模型 `ai_platform_models` 也只有 OpenAI 兼容协议；`ai_settings.platform_model_id` 选择一个默认模型，用于题目难度初评。
- `KeyService.rotate` 重加密 `user_ai_configs` 和 `ai_platform_models`；`ResetKey` 清除用户配置和平台 token。账号备份表清单包含 `user_ai_configs`。
- `ChatRequest` 已有 `MaxTokens`（平台测试/难度评估使用）；`OpenAIClient` 请求体包含 `max_tokens`（omitempty）。

## 2. 目标与非目标

### 目标

- 新增 `user_ai_models`：一个用户可保存多条模型配置；每条是一套“接口地址 + 模型名 + token + 协议”，同一模型可保存多个 token/接口，用名称区分。
- 协议字段 `protocol`：`openai` / `anthropic`；用户与平台模型都支持。
- 用户可指定唯一“当前使用”的模型；AI 补全/格式化/优化/提示/造数据等用户级功能使用该模型。
- 支持自定义 `base_url` 与 `model_name`；内置 provider 作为可选的预填模板。
- 平台模型新增 `protocol`，管理后台可选择协议、测试连接；题目难度初评使用所选协议。
- 旧 `user_ai_configs` 自动迁移为一条 active 的用户模型；旧表保留用于回滚，不参与新写入。
- 保留旧 `/api/ai/configs` 读写兼容：读取当前模型；写入更新/创建当前模型。
- token 加密、脱敏、密钥轮换/重置、账号备份全部覆盖新表。

### 非目标

- 不做流式输出、工具调用、embedding、图片/语音模型、模型自动发现。
- 不做按功能（提示/补全/难度）分别选模型；用户只有“当前使用”一个选择。
- 不做 Anthropic 以外的非 OpenAI 兼容协议（如 Gemini 原生格式）。
- 不改变平台 AI 总开关、用户限流、提示缓存、比赛期间 AI 禁用逻辑。
- 不改变 `ai_providers` 的目录语义；自定义地址存到用户模型行，不写入目录。
- 不做用户模型用量统计/费用账单。

## 3. 数据模型

### 3.1 `user_ai_models`

```text
id                  bigserial primary key
user_id             bigint       not null
name                varchar(100) not null
protocol            varchar(20)  not null default 'openai'  -- openai / anthropic
base_url            varchar(512) not null
model_name          varchar(100) not null
api_key_encrypted   text         not null default ''
is_active           bool         not null default false
last_test_at        timestamptz  null
last_test_ok        bool         not null default false
last_test_error     varchar(500) not null default ''
last_test_latency_ms int         not null default 0
created_at          timestamptz  not null
updated_at          timestamptz  not null
```

- 唯一索引 `(user_id, name)`：同一用户的显示名称唯一，避免多个 token 难以区分；同模型可多行。
- 部分唯一索引 `(user_id) WHERE is_active`：每个用户最多一个当前使用模型。
- 索引 `(user_id, is_active)`。
- `base_url` 校验 http/https、无 userinfo、不包含 `/chat/completions` 或 `/v1/messages` 端点；允许内网/自建地址（仅用户自己的 token，风险由用户承担）。
- `protocol` 只接受 `openai` / `anthropic`。

### 3.2 `ai_platform_models` 扩展

```text
protocol varchar(20) not null default 'openai'  -- openai / anthropic
```

- 已有行默认 `openai`。
- 管理 API 的 `PlatformModelInput/View` 增加 `protocol`。
- 平台模型 `name` 仍全局唯一。

### 3.3 旧数据迁移

- 一次性回填 `user_ai_models_backfill_v1`（`db_migrations` 标记）：
  - 读取 `user_ai_configs` 并 join `ai_providers`。
  - 每个用户插入一条 `user_ai_models`：`name = provider.name + ' / ' + model_name`（长度截断 100），`protocol='openai'`，`base_url=COALESCE(provider.base_url,'')`，`model_name`、`api_key_encrypted` 原样复制，`is_active=true`。
  - 若用户已有 `user_ai_models` 行则跳过（幂等）。
- `user_ai_configs` 保留不删；新代码只写 `user_ai_models`。保留是为了回滚和旧备份恢复。
- `ai_platform_models.protocol` 由 AutoMigrate 加列，默认 openai。

## 4. 协议客户端

### 4.1 `ChatRequest`

```go
type ChatRequest struct {
    Protocol    string // "", "openai", "anthropic"；空按 openai
    BaseURL     string
    APIKey      string
    Model       string
    Messages    []ChatMessage
    Temperature float64
    MaxTokens   int
}
```

### 4.2 客户端分发

- 新增 `ProtocolClient` 实现 `ChatClient`：
  - `ProtocolClient{openai *OpenAIClient, anthropic *AnthropicClient}`
  - `Chat` 根据 `req.Protocol` 分发；空/`openai` 走 `OpenAIClient`，`anthropic` 走 `AnthropicClient`，未知协议直接返回错误。
- `main.go` 用 `ai.NewProtocolClient(cfg.AIRequestTimeout)` 替代 `ai.NewOpenAIClient`；`OpenAIClient` 保留给测试/兼容。
- `ai.Service` 与 `ai.PlatformService` 使用同一个 `ChatClient`，所以用户和平台都自动支持两种协议。

### 4.3 Anthropic 客户端

- 端点：
  - base URL 以 `/v1` 结尾 → `{base}/messages`。
  - 否则 → `{base}/v1/messages`。
  - 拒绝以 `/v1/messages` 或 `/messages` 结尾的 base URL（提示只填到 `/v1` 或域名）。
- 请求头：`x-api-key: <token>`、`anthropic-version: 2023-06-01`、`content-type: application/json`。
- 请求体：
  ```json
  {
    "model": "...",
    "max_tokens": 4096,
    "system": "拼接的 system 消息",
    "messages": [{"role":"user","content":"..."}],
    "temperature": 0.2
  }
  ```
  - `max_tokens`：`ChatRequest.MaxTokens` > 0 时使用，否则默认 4096（Anthropic 必填）。
  - system 消息从 `Messages` 中抽出，多个 system 用 `\n\n` 拼接；其余消息保持 user/assistant。
- 响应：`content` 数组，拼接 `type=="text"` 的 `text` 字段；空 content / 非 2xx / JSON 非法映射为稳定的上游错误。
- 错误不包含 token、上游 body、prompt；与现有 `OpenAIClient` 的脱敏语义一致。
- `OpenAIClient` 增加 base URL 校验：拒绝以 `/chat/completions` 结尾，避免重复拼接。

## 5. 业务流

### 5.1 用户模型 CRUD

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/ai/models` | `{keyConfigured, activeModelId, models:[...], providers:[...]}` |
| POST | `/api/ai/models` | 新增；首个模型自动设为当前；`isActive=true` 可显式设为当前 |
| PUT | `/api/ai/models/:id` | 更新；API Key 为空保留原密文 |
| DELETE | `/api/ai/models/:id` | 当前使用模型拒绝删除，提示先切换 |
| PUT | `/api/ai/models/:id/active` | 设为当前（事务内清空其他 active） |
| POST | `/api/ai/models/:id/test` | 轻量 chat 测试，更新 `last_test_*`，返回 `{ok,latencyMs,message}` |

- `UserModelView`：`id/name/protocol/baseUrl/modelName/apiKeyMasked/isActive/lastTest*`。
- `UserModelInput`：`name/protocol/baseUrl/modelName/apiKey?`。
- 名称校验 1–100；base URL 1–512；model 1–100；token 1–4096；`protocol` 白名单。
- 创建/更新需要 `KeyManager.Configured()`，否则 40002；更新空白 token 是元数据修改，不要求密钥。
- `providers` 仍返回内置目录（OpenAI 兼容），用于预填；不再要求用户必须从目录选择。

### 5.2 当前模型解析

- `ai.Service.chatRequest(userID)`：
  1. 查询 `user_ai_models WHERE user_id=? AND is_active=true`。
  2. 无 active 行 → 尝试旧 `user_ai_configs`（回退兼容），仍无 → 40001“尚未配置 AI 模型”。
  3. 解密 token；返回 `ChatRequest{Protocol,BaseURL,APIKey,Model,Temperature,MaxTokens:4096}`。
  4. 协议未知/base URL 空 → 40001。
- `generate` / `hint` / `testcase_script` 无需改动调用方式，自动使用当前模型。
- AI 补全、格式化、优化、提示、AI 造数据都使用当前用户模型；平台任务继续使用平台模型。

### 5.3 旧接口兼容

- `GET /api/ai/configs`：
  - 返回当前 active 模型的 `provider`（若有 provider 映射，否则空）、`modelName`、`apiKeyMasked`、`configured`、`keyConfigured`、`providers`。
- `POST /api/ai/configs`：
  - 旧语义：provider + model + key。
  - 若当前 active 模型存在，更新该行（provider 的 base URL 映射到 base_url；协议 openai）。
  - 若无 active 模型，创建一条 `name=provider/model` 的 openai 模型并设为 active。
  - 旧客户端继续可用；新前端改用 `/api/ai/models`。

### 5.4 平台模型

- `PlatformModel` 增加 `Protocol`；`PlatformModelInput/View` 增加 `protocol`。
- `PlatformService.Create/Update` 校验协议；`Test` 用 `ChatRequest{Protocol,...MaxTokens:16}`。
- `ai.resolvePlatformChat` 设置 `Protocol`；`AssessDifficulty` 使用平台协议的 `MaxTokens:512`。
- 管理页新增/编辑弹窗增加协议 Select；列表显示协议；测试沿用现有按钮。
- 平台模型 base URL 校验按协议：OpenAI 拒绝 `/chat/completions`；Anthropic 按 §4.3 规则。

## 6. 密钥、备份与安全

- `KeyService.rotate`：
  - 统计 `user_ai_configs`、`user_ai_models`、`ai_platform_models` 的非空密文。
  - keep 模式：无旧密钥但有任一密文 → 拒绝。
  - 事务内 `SELECT FOR UPDATE` 重加密 `user_ai_configs`（回滚兼容）、`user_ai_models`、`ai_platform_models`。
- `ResetKey`：删除 `user_ai_configs`、`user_ai_models`，清空平台 token/选择；保留平台模型元数据。
- 账号备份表清单加入 `user_ai_models`；平台备份自动包含新表。
- token 只返回 masked；`last_test_error` 只存脱敏分类；日志不含 token/prompt/上游 body。
- 用户自定义 base URL 允许内网/本地地址；仅用户自己的 token，风险由用户承担；前端提示“请确认接口地址可信”。

## 7. API 与 DTO

新增用户接口（均在 `/api/ai` 下，需登录 + AI 开关）：

- `GET /api/ai/models` → `AIUserModelsView`
- `POST /api/ai/models` → 201 `AIUserModelView`
- `PUT /api/ai/models/:id` → 200 `AIUserModelView`
- `DELETE /api/ai/models/:id` → 200
- `PUT /api/ai/models/:id/active` → 200 `AIUserModelView`
- `POST /api/ai/models/:id/test` → 200 `AIModelTestResult`

`AIUserModelsView`：

```json
{
  "keyConfigured": true,
  "activeModelId": 3,
  "models": [
    {
      "id": 3,
      "name": "DeepSeek 主账号",
      "protocol": "openai",
      "baseUrl": "https://api.deepseek.com/v1",
      "modelName": "deepseek-chat",
      "apiKeyMasked": "sk-…abcd",
      "isActive": true,
      "lastTestAt": null,
      "lastTestOk": false,
      "lastTestError": "",
      "lastTestLatencyMs": 0
    }
  ],
  "providers": [
    {"code":"deepseek","name":"DeepSeek","defaultModel":"deepseek-chat","baseUrl":"https://api.deepseek.com/v1","protocol":"openai"}
  ]
}
```

- `ProviderOption` 增加 `baseUrl` 与 `protocol`（均为 openai），用于预填。
- 平台模型 DTO 增加 `protocol`。

## 8. 前端

### 8.1 用户 AI 设置页 `/settings/ai`

- 保留 AI 不可用/密钥未配置提示。
- 主视图改为“已保存模型”列表：
  - 卡片/表格：名称、协议 Tag（OpenAI/Anthropic）、接口地址、模型名、token masked、当前使用标记、最近测试、操作（设为当前/编辑/测试/删除）。
  - 当前使用模型用 `Tag` + 单选/“设为当前”按钮；切换调用 active API。
  - “新增模型”按钮打开 Dialog：名称、协议 Select、接口地址、模型名、API Key（Password）、可选“设为当前”；内置 provider 作为快捷预填（选择后填 base URL/model/protocol）。
  - 编辑 Dialog：API Key 留空保留；协议/地址/模型/名称可改。
  - 删除当前使用模型按钮禁用并提示先切换。
  - 测试按钮显示 loading，结果 Toast/Tag；失败脱敏。
- 无模型时显示空状态 + “新增模型”引导。
- 旧接口不再被新页面调用；`api/ai.ts` 增加 models CRUD。

### 8.2 管理后台平台模型

- `AdminAiView` 平台模型新增/编辑 Dialog 增加协议 Select（OpenAI/Anthropic）。
- 列表显示协议 Tag；测试按钮按协议调用；当前模型/密钥/删除逻辑不变。
- 管理员仍只能选一个平台默认模型；协议不影响选择。

### 8.3 类型/契约

- OpenAPI 新增 `AIUserModelView/Input/List/TestResult`、`AIModelProtocol`、`AIUserModelsView`；`AIProviderOption` 增加 `baseUrl/protocol`；`PlatformModelView/Input` 增加 `protocol`。
- 运行 `corepack pnpm gen:api`；`types.ts` 增加别名。

## 9. 迁移与测试

### 9.1 迁移

- AutoMigrate `&ai.UserModel{}`；`ai_platform_models` 加 `protocol` 列。
- 回填 `user_ai_models_backfill_v1`；`user_ai_configs` 保留。
- 密钥轮换/重置、账号备份覆盖新表。
- 旧 `/api/ai/configs` 兼容。

### 9.2 后端测试

- Anthropic 客户端：URL 归一化、x-api-key/anthropic-version、system 抽取、max_tokens 默认/显式、content 拼接、非 2xx/空 content/非法 JSON 脱敏、base URL 拒绝端点。
- OpenAI 回归：现有测试全部通过；`Protocol` 空/`openai` 行为不变。
- 用户模型：CRUD、名称唯一、协议校验、base URL 校验、token 加密/masked、空 token 更新保留、首个自动 active、active 唯一、删除 active 拒绝、test 成功/失败。
- `chatRequest`：active 行、无 active 回退旧配置、无配置 40001、未知协议/空地址 40001、MaxTokens 4096。
- 迁移：旧单配置 → active 行；幂等；provider 缺失不丢行。
- 密钥轮换/重置：新表重加密/清空；旧表兼容。
- 备份：账号备份包含 `user_ai_models`，恢复后可用。
- 平台模型：protocol 创建/更新/测试/难度评估；Anthropic fake 响应。
- 前端：列表/新增/编辑/设为当前/删除/测试/空状态/密钥缺失。
- OpenAPI 类型生成、lint、build。

### 9.3 真实冒烟

- 临时 DB + 假 OpenAI/Anthropic 服务；用户登录后保存两套模型（各一个 token），切换 active，分别调用 `/ai/complete` 验证请求头/格式；删除 active 被拒；密钥轮换后两套 token 仍可用；账号备份恢复。
- 管理员平台模型新增 Anthropic 模型，测试连接，设置默认，创建题目验证 AI 初评走 Anthropic。
- 清理临时数据。

## 10. 交付拆分

- **UA-A**：`user_ai_models` 模型/迁移/回填、`ChatRequest.Protocol`、Anthropic 客户端、ProtocolClient。
- **UA-B**：用户模型 service/API、`chatRequest` active 解析、旧 `/configs` 兼容、密钥轮换/备份。
- **UA-C**：平台模型 protocol、admin API/难度评估。
- **UA-D**：OpenAPI/types、用户 AI 设置页、平台模型管理页。
- **UA-E**：测试、文档、真实冒烟。

## 11. 留给实施计划的决定

- `ProtocolClient` 与 `AnthropicClient` 的文件拆分/构造函数。
- `user_ai_models` 名称唯一索引是否在迁移前已有重复（旧迁移生成名可能重复）；回填时加 `#id` 后缀避免冲突。
- 旧 `/api/ai/configs` 的 `provider` 字段如何映射到自定义模型（建议：有匹配 provider code 则返回，否则空字符串）。
- 用户模型测试的连接超时（建议 10 秒）和 `max_tokens` 16。
- 前端列表用卡片还是 DataTable（建议卡片，移动端友好）。
- 是否在用户页显示“最近测试”时间（建议显示）。
