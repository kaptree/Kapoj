# M5-5 AI 前端设计

> 目标：把已交付的 M5-A AI 后端接入前端：提交页 AI 补全/格式化/优化、题目详情 AI 提示、用户 AI 配置页；不新增后端能力。

## 1. 背景与现状

- 后端已交付（M5-A）：
  - `GET/POST /api/ai/configs`（登录；provider/模型/脱敏 key）。
  - `POST /api/ai/complete`（code/language/cursor 字节偏移 → completion）。
  - `POST /api/ai/format`（→ 完整格式化代码）。
  - `POST /api/ai/optimize`（→ suggestions + code）。
  - `POST /api/ai/hint`（problemId → hint/cached；缓存命中不要求用户配置；隐藏题 404）。
  - 所有接口仅在配置 `KAPOJ_AI_ENCRYPTION_KEY` 时挂载；未配置时 404。
  - 错误：未配置 40001 “ai config not set”、限流 42903、上游 50200、Redis 50300。
- 前端已有：
  - `CodeEditor.vue`（Monaco，v-model，语言切换）。
  - `SubmitView.vue` 编辑器工具栏（语言 Select + 提交按钮）。
  - `ProblemDetailView.vue` 提示卡片（`hintMd` 或“暂无提示”）。
  - `UserMenu.vue` 用户菜单；路由/鉴权。
  - 生成类型 `schema.d.ts` 已有 AI schema；`api/types.ts` 尚未导出别名。

## 2. 目标与非目标

### 目标

- 提交页编辑器工具栏增加“AI 补全 / AI 格式化 / AI 优化”：
  - 补全：按当前光标 UTF-8 字节偏移调用 complete，把返回片段插入光标处。
  - 格式化：调用 format，预览后由用户确认替换代码。
  - 优化：调用 optimize，展示建议与优化代码，由用户确认应用。
  - 未配置 AI 时展示提示与 `/settings/ai` 链接；限流/上游错误中文 Toast。
- 题目详情提示卡片增加“AI 提示”按钮：
  - 登录用户调用 hint，展示内容与是否缓存；游客跳登录。
  - 未配置且无缓存时提示去配置；隐藏题/不存在 404；限流/上游错误中文。
- 新增 `/settings/ai` 配置页：
  - 选择 provider、模型名、API key（已有配置可留空保留）；展示脱敏 key；保存/校验/中文错误。
  - 用户菜单增加“AI 设置”入口；路由需登录。
- AI 路由未挂载（404）时页面显示“AI 功能未启用”，不崩溃。

### 非目标

- 不实现 M5-4 出题造数据、M5-7 证书、M5-8 头像上传。
- 不新增 provider/后端接口，不改 AI 加密/限流/缓存策略。
- 不做流式输出、对话历史、多轮聊天、模型费用统计。
- 不做代码 diff 编辑器/复杂合并；格式化/优化只提供预览 + 覆盖确认。
- 不新增 UI 依赖；不推送/不合并。

## 3. 页面与交互

### 3.1 提交页 AI 工具栏

- 在现有 `.editor-toolbar` 中，语言 Select 与提交按钮之间增加三个 `Button`（text/outlined，小尺寸）：
  - `AI 补全`（`pi pi-sparkles`）
  - `AI 格式化`
  - `AI 优化`
- 页面加载时调用 `GET /ai/configs`：
  - 成功且 `configured=false`：显示 `Message`（info）：“尚未配置 AI 模型，配置后可使用补全/格式化/优化”，附 `/settings/ai` 链接；按钮禁用。
  - 404（AI 未启用）：显示“AI 功能未启用”；按钮禁用。
  - 其他失败：静默禁用按钮（不阻塞提交）。
- 调用中：对应按钮 loading，三个 AI 按钮互斥禁用；结果/错误以 Toast/对话框展示。
- 补全：`CodeEditor` 暴露 `getCursorByteOffset()`（Monaco UTF-16 offset → UTF-8 字节）与 `insertAtCursor(text)`；调用 `cursor` 必填。返回空片段提示“没有可插入的补全”。
- 格式化：返回代码与当前代码不同才弹预览 Dialog（`pre`/只读预览），按钮“应用格式化”/“取消”；应用后替换当前语言草稿。
- 优化：Dialog 展示 `MarkdownView(suggestions)` + 优化代码预览；“应用优化代码”替换；`code` 为空/等于原代码时禁用应用。
- 语言参数使用提交语言（c/cpp/python3/java/go）；代码按 UTF-8 字节 ≤64KiB，前端提前提示超限。

### 3.2 题目详情 AI 提示

- 提示卡片标题右侧增加 `AI 提示` 按钮（sparkles）：
  - 游客：跳登录（redirect 当前路径）。
  - 登录：调用 `POST /ai/hint {problemId}`；loading 显示“AI 正在生成提示…”。
  - 成功：在官方提示下方新增“AI 提示”区块（`MarkdownView`），缓存命中显示“来自缓存”小标签。
  - 失败：
    - 40001 且无缓存：显示中文“尚未配置 AI 模型”+ `/settings/ai` 链接。
    - 42903：Toast“操作过于频繁，请稍后再试”。
    - 50200/50300：Toast“AI 服务暂时不可用”。
    - 404：Toast“题目不存在”。
- 官方提示与 AI 提示独立展示；AI 提示按题目全局缓存，切换题目/重新加载时清空本地状态。
- 不自动请求 AI 提示（避免无配置/限流页面加载即报错）。

### 3.3 AI 配置页 `/settings/ai`

- 路由 `ai-settings`，`requiresAuth`；用户菜单增加“AI 设置”（`pi pi-sparkles`）。
- 页面加载 `GET /ai/configs`：
  - 404 → “AI 功能未启用，请联系管理员”；禁用表单。
  - 成功：provider Select（`providers`，默认 provider/defaultModel）、模型 InputText（占位默认模型）、API key `Password`（占位“留空保留现有密钥”）；已配置显示“当前密钥：masked”。
  - 首次配置 key 必填；保存调用 `POST /ai/configs`；成功后 Toast 并刷新视图。
  - 校验：provider 必选、模型为空时用 provider 默认（前端显示默认）、首次 key 非空；错误中文。
- 说明文案：密钥仅加密存储、不会回显明文；AI 调用使用用户自己的 key。
- 移动端表单单列。

## 4. API 客户端与类型

`apps/web/src/api/ai.ts`：

```ts
getAIConfig(): Promise<AIConfigView>
saveAIConfig(input: AISaveConfigInput): Promise<AIConfigView>
completeCode(input: AICompleteInput): Promise<AICompleteOutput>
formatCode(input: AIFormatInput): Promise<AIFormatOutput>
optimizeCode(input: AIOptimizeInput): Promise<AIOptimizeOutput>
getAIHint(problemId: number): Promise<AIHintOutput>
```

- 类型来自 `schema.d.ts`；在 `types.ts` 增加 `AIConfigView/AISaveConfigInput/AICompleteInput/AICompleteOutput/AIFormatInput/AIFormatOutput/AIOptimizeInput/AIOptimizeOutput/AIHintOutput/AIProviderOption` 别名。
- 不新增 OpenAPI/生成改动（M5-A 已生成）。

## 5. 组件改动

- `CodeEditor.vue`：`defineExpose({ getCursorByteOffset, insertAtCursor })`；保持现有 v-model/语言/readonly 行为。
  - `getCursorByteOffset`：`monaco.editor.getModel().getOffsetAt(position)` 得到 UTF-16 offset，再 `new TextEncoder().encode(code.slice(0, offset)).length`。
  - `insertAtCursor(text)`：在当前光标处插入文本并聚焦。
- `SubmitView.vue`：AI 配置状态、三个按钮、预览 Dialog、错误 Toast；不改变提交逻辑/草稿保存。
- `ProblemDetailView.vue`：AI hint 按钮/状态/展示/错误；保持官方提示卡片布局与资源卡片。
- 新 `AiSettingsView.vue`：配置表单。
- `UserMenu.vue`：新增“AI 设置”菜单项（登录用户）。
- 路由：`/settings/ai` name `ai-settings`，requiresAuth。

## 6. 错误与边界

| 场景 | 前端行为 |
|---|---|
| AI 路由 404 | Submit/设置页显示“AI 功能未启用”；按钮/表单禁用 |
| 未配置（40001） | 提交页提示去配置；Hint 无缓存时提示配置；配置页正常保存 |
| 限流 42903 | Toast“操作过于频繁，请稍后再试” |
| 上游 50200 / Redis 50300 | Toast“AI 服务暂时不可用” |
| 代码 >64KiB | 前端提示“代码超过 64 KiB，无法使用 AI”；不发请求 |
| 空补全 | Toast“没有可插入的补全” |
| 格式化/优化返回原代码 | 仍展示建议；格式化无变化提示“代码已是格式化状态” |
| 游客 | 补全/格式化/优化/提示跳登录；配置页路由 guard |
| 隐藏题 | Hint 404；详情页资源规则不变 |

## 7. 测试与验收

### 7.1 前端

- `CodeEditor` 光标字节偏移（ASCII/中文）与插入。
- SubmitView：未配置提示/按钮禁用；404 未启用；补全插入光标；格式化预览/应用/取消；优化建议/应用；64KiB/限流/上游错误 Toast；提交逻辑不回归。
- ProblemDetailView：AI 提示按钮游客跳登录；成功展示/缓存标签；未配置链接；429/502/404 Toast；切题清空。
- AISettingsView：加载/404/保存/校验/脱敏 key/留空保留/成功 Toast。
- UserMenu AI 设置入口；路由 guard。
- 生成类型/lint/build。

### 7.2 真实冒烟

- 临时 server 配置 `KAPOJ_AI_ENCRYPTION_KEY` + 假 OpenAI 兼容服务（复用 M5-A 方式）。
- 用户配置 provider/key；提交页补全/格式化/优化；题目 AI 提示首次生成、第二次缓存；未配置用户提示配置；限流；隐藏题 404；AI 路由未挂载时页面提示未启用。
- 浏览器如可用走真实 UI；否则 API + 组件测试并记录。
- 清理临时 user_ai_configs/ai_hints/配置/假服务/server。

## 8. 交付拆分

- M5-5A：API 客户端/类型 + CodeEditor 光标能力 + SubmitView 工具栏 + 测试。
- M5-5B：ProblemDetailView AI 提示 + 测试。
- M5-5C：AISettingsView + UserMenu/路由 + 测试。
- M5-5D：文档、全量检查、假 provider 真实冒烟。

## 9. 留给实施计划的决定

- 格式化/优化预览对话框的具体组件尺寸与只读代码渲染方式。
- AI 配置页 provider 切换时是否自动填充默认模型（建议是）。
- AI 工具栏按钮的图标/顺序/移动端换行。
