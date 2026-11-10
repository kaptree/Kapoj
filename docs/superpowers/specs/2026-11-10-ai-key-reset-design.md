# AI 加密密钥一键重置设计

> 目标：在后台 AI 设置页增加「清除旧密钥并生成新密钥」按钮。点击后删除旧的平台加密密钥、清空所有用户已保存的 AI API Key，并立即生成一个随机的新密钥，便于旧密钥丢失后的恢复。

## 1. 背景与现状

- 后台 AI 设置页已有「自动生成密钥 / 重新生成」和「手动填写密钥」。
- 现有轮换 `ai.SettingsService.rotate` 会先解密所有 `user_ai_configs`，再用新密钥重加密；旧密钥不可用时会拒绝操作，避免破坏用户密文。
- 当前数据库可能存在用户 AI 配置，但平台密钥文件/环境变量已丢失。此时「自动生成密钥」会被 `rotate` 的“存在无法解密的用户 AI 配置，请先恢复旧密钥”拦截。
- 用户需要一个明确的恢复入口：放弃旧密钥和旧密文，生成全新的平台密钥，让系统恢复可用；用户之后重新填写自己的 AI API Key。

## 2. 目标与非目标

### 目标

- 后台 AI 设置页新增危险操作按钮「清除旧密钥并生成新密钥」。
- 点击后：
  - 删除所有 `user_ai_configs`（用户已保存的 AI provider/model/API Key）。
  - 删除/替换旧的平台密钥文件。
  - 生成并保存新的 32 字节随机密钥（base64）。
  - 返回一次性完整密钥供管理员备份。
  - 更新 `ai_settings.key_fingerprint`，写审计。
  - 运行时立即切换到新密钥，AI 功能无需重启。
- 操作前必须二次确认，明确告知会删除所有用户 AI Key。
- 环境变量 `KAPOJ_AI_ENCRYPTION_KEY` 来源仍然只读；该按钮不显示/不可用。
- 清除后用户侧 AI 功能恢复为“平台已配置密钥”，但用户需要重新填写自己的 API Key 才能调用 AI。

### 非目标

- 不保留旧密钥或旧用户密文的备份/恢复入口。
- 不保留用户 AI 配置并尝试重新解密；旧密钥丢失时这些密文不可恢复。
- 不改变 AI provider 列表、AI 开关、提示缓存 `ai_hints`、题目/比赛 AI 禁用规则。
- 不自动通知用户“你的 AI Key 已被清除”；用户下次使用时自行重新配置。
- 不提供“仅清除密钥、不生成新密钥”的中间状态。
- 不把新密钥写入数据库或日志；仍只写本地密钥文件，并一次性返回。

### 反目标

- 旧密钥仍可用时误删用户 AI Key：只有明确点击「清除旧密钥并生成新密钥」并确认后才执行。
- 环境变量来源被后台覆盖。
- 操作失败后旧密钥文件或用户配置处于半删除状态。
- 新密钥泄露到日志、审计详情或数据库。
- AI 开关在没有密钥的窗口期被错误改写。

## 3. 约束

- 后端 Go + Gin + GORM + PostgreSQL；前端 Vue 3 + PrimeVue；OpenAPI 生成类型。
- 权限沿用 `ai.settings.manage`；无新增权限。
- 审计 target 使用现有 `ai_key`，summary 为「清除旧密钥并生成新密钥」。
- 密钥仍为 32 字节随机数 + base64；文件 0600、父目录 0700，沿用现有 `writeKeyFileWithBackup`。
- 清除与生成必须在同一把 `keyMu` 锁内完成，避免与现有生成/手动替换/加解密并发交错。
- 数据库删除用户配置、更新指纹、写审计必须在同一事务内；失败回滚，并恢复旧密钥文件。
- 环境变量来源返回 40001，不执行删除。
- 新接口返回 `KeyGenerateResult`，完整密钥仅响应一次。
- 按钮只在 `canManage && !isEnvSource` 时显示；所有管理操作仍由后端权限校验兜底。

## 4. 方案选择

### 4.1 清除范围

- **方案 A：删除旧密钥 + 清空所有用户 AI 配置（选用）**。适合旧密钥丢失的恢复；不会留下无法解密的密文，用户重新填写即可。
- 方案 B：只删平台密钥，保留用户密文。用户 AI 功能持续不可用，且数据库残留不可解密数据，排除。
- 方案 C：旧密钥可用时重加密、不可用时删除。行为随状态分叉，管理员难以预期；且现有「重新生成」已覆盖安全轮换。选择独立的破坏性重置入口。

### 4.2 接口形态

- **方案 A：新增 `POST /api/admin/ai/key/reset`（选用）**。语义明确、可单独审计、不与现有生成/手动替换混淆。
- 方案 B：给现有 `POST /admin/ai/key/generate` 加 `purgeUserConfigs=true`。容易误调用，破坏性参数隐藏在日常生成接口里。
- 方案 C：前端先调用一个删除接口再调用生成接口。两步可能半成功，且多一个无密钥窗口。

选择 A，后端一次完成。

### 4.3 UI 位置

- **方案 A：放在「加密密钥」卡片的操作区，现有生成/手动填写下方，使用危险色按钮（选用）**。与密钥操作同区，危险语义清楚。
- 方案 B：放进页面顶部更多菜单。可发现性差。
- 方案 C：弹窗内再放一个按钮。多一层交互。

选择 A。

## 5. 设计

### 5.1 后端服务

`ai.SettingsService` 新增：

```go
func (s *SettingsService) ResetKey(
    ctx context.Context,
    actorID int64,
    audit func(*gorm.DB) error,
) (*KeyGenerateResult, error)
```

流程：

1. `keyMu.Lock()`。
2. 若 `s.keys.Source() == "env"` → 40001「AI 加密密钥由环境变量管理，只读」。
3. 生成 32 字节随机密钥 `raw`。
4. 校验 `s.keyFile` 非空。
5. 调 `writeKeyFileWithBackup` 写入新密钥（旧文件备份为 `.bak`）。
6. 在 DB 事务内：
   - `ensureSettings`。
   - `DELETE FROM user_ai_configs`。
   - 更新 `ai_settings.key_fingerprint = fingerprintOf(raw)`、`updated_by`、`updated_at`。
   - 执行审计回调。
7. 任一失败：`restoreKeyFile` 恢复旧文件/删除新文件；返回错误。
8. 成功：`removeKeyBackup`、`s.keys.Replace(newEnc, "file")`。
9. 返回 `KeyGenerateResult{KeyView, Key: raw}`。

- 不清空 `ai_hints`。
- 不修改 `ai_settings.enabled`。
- 若旧密钥不存在、无用户配置，也正常生成。
- 不复用 `rotate` 的 `cfgCount > 0 && !hasOld` 拒绝逻辑；重置模式明确放弃旧密文。

### 5.2 API 与审计

新增：

```http
POST /api/admin/ai/key/reset
```

- 权限：`ai.settings.manage`。
- 成功：`AIKeyGenerateResultResponse`（含一次性 `key`）。
- 环境变量来源：40001。
- 失败：401/403/500 沿用现有错误。
- 审计：action `ai.settings.manage`，target `ai_key`，summary「清除旧密钥并生成新密钥」。
- 路由注册在现有 `/admin/ai/key/generate` 旁。
- handler 新增 `resetAIKey`，调用 `ResetKey`；审计回调与现有 `generateAIKey` 相同。

### 5.3 前端

`apps/web/src/views/AdminAiView.vue`：

- 密钥操作区在现有「重新生成/自动生成密钥」旁新增：

```text
[重新生成] [清除旧密钥并生成新密钥]
```

- 新按钮：
  - `severity="danger"`、`outlined`、`icon="pi pi-trash"`。
  - 仅 `canManage && !isEnvSource` 时渲染。
  - `:loading="keySaving"`，与生成/手动保存共用互斥状态。
- 点击先 `confirm.require`：
  - header：「清除旧密钥并生成新密钥」
  - message：「将删除所有用户已保存的 AI API Key，并生成新的平台加密密钥。此操作不可撤销，用户需要重新填写自己的 API Key。」
  - icon：`pi pi-exclamation-triangle`
  - acceptLabel：「清除并生成」
  - rejectLabel：「取消」
- 确认后调用 `resetAdminAIKey()`：
  - `key.value = result`
  - `generatedKey.value = result.key`
  - `manualKey.value = ''`
  - `refreshSettings()`
  - Toast：`summary: '旧密钥已清除'`，`detail: '新密钥已生成；请立即备份，用户需重新填写 AI API Key'`。
- 失败 Toast：`summary: '重置失败'`，`detail` 使用 `ApiError.message`。
- 新一次性密钥继续复用现有生成密钥展示区、复制/隐藏按钮和警告。
- 环境变量来源仍只读，不显示该按钮。

### 5.4 OpenAPI 与类型

`packages/api/openapi.yaml`：

- 新增 `POST /admin/ai/key/reset`，描述明确「删除所有用户 AI 配置并生成新密钥」。
- 返回 `AIKeyGenerateResultResponse`。
- 400/401/403/500 响应与其他 key 接口一致。

前端：

- `api/admin.ts` 增加：

```ts
export const resetAdminAIKey = () =>
  http.post<AIKeyGenerateResult>('/admin/ai/key/reset')
```

- 重新生成 `schema.d.ts`；`types.ts` 已有 `AIKeyGenerateResult`，无需新增别名。

### 5.5 测试

后端：

- `ResetKey` 在旧密钥存在时：
  - 删除所有 `user_ai_configs`。
  - 旧文件被替换，`.bak` 不存在。
  - 新密钥 32 字节 base64，内存指纹与 DB 指纹一致。
  - AI 设置 `enabled` 不变。
- 旧密钥不存在但存在用户配置时仍成功（恢复场景）。
- env 来源返回 40001，用户配置和旧文件不变。
- 审计回调失败：事务回滚，用户配置仍存在，旧密钥文件恢复，内存密钥仍为旧密钥。
- handler：
  - 权限/路由；
  - 审计 summary；
  - 响应包含一次性 key；
  - 不包含日志/数据库明文。
- OpenAPI 生成通过。

前端：

- 非 env 且 canManage 时显示新按钮。
- env 来源不显示。
- 点击先弹确认；取消不调用 API。
- 确认后调用 `resetAdminAIKey`，展示一次性密钥，Toast 正确，刷新设置。
- 失败显示错误 Toast。
- 现有自动生成、手动填写、复制/隐藏测试不回归。

### 5.6 文档

- `docs/任务清单.md` 增加「AI 加密密钥一键重置」小节，标 ✅。
- `docs/开发日志.md` 追加交付记录。
- README 当前进度可选补一句。

## 6. 已知取舍

- 这是破坏性恢复操作：所有用户 AI Key 被删除，不可撤销。
- 旧密钥仍可用时，应优先使用现有「重新生成」以保留用户配置；新按钮用于旧密钥丢失或明确要放弃旧密文的场景。
- 不保留旧密文备份，避免数据库继续保存不可解密数据。
- 用户不会收到自动通知；需在 UI 确认文案和 Toast 中明确说明。
- 新密钥仍只保存在本地密钥文件；多实例部署需确保共享同一密钥文件或环境变量。
