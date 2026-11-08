# M5-5 AI 前端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把现有 M5-A AI 后端接入前端：提交页补全/格式化/优化、题目详情 AI 提示、用户 AI 配置页；不新增后端能力。

**Architecture:** 新增 `api/ai.ts` 复用生成类型；扩展 `CodeEditor` 暴露光标字节偏移/插入能力；`SubmitView` 增加 AI 工具栏、配置状态和格式化/优化预览 Dialog；`ProblemDetailView` 提示卡片增加 AI 提示；新增 `AiSettingsView` 与用户菜单入口。所有错误映射为中文提示，AI 路由未挂载时降级。

**Tech Stack:** Vue 3、TypeScript、PrimeVue 4、Monaco、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-11-08-m5-5-ai-frontend-design.md`

## Global Constraints

- 不新增后端接口/改动 AI 加密、限流、缓存；复用 M5-A 接口与 `schema.d.ts`。
- AI 补全光标为 UTF-8 字节偏移（0..len(code)）；代码 ≤64KiB。
- 未配置/未启用/限流/上游错误必须中文提示；AI 路由 404 时页面降级，不阻塞提交。
- 格式化/优化必须预览确认后覆盖代码；补全插入光标处。
- AI 提示不自动请求；游客跳登录；隐藏题 404；缓存命中显示。
- 配置页：首次 key 必填，已有配置可留空保留；只显示脱敏 key；provider 切换填充默认模型。
- 不新增 UI 依赖；不推送/不合并。

## Review Focus

1. **光标/插入正确性**：UTF-16→UTF-8 字节偏移（中文/emoji）、空代码、光标在末尾、插入后代码/草稿保持。
2. **AI 状态与降级**：配置未设置/路由 404/加载失败时按钮禁用与提示；不阻塞提交；配置成功后（刷新）可用。
3. **格式化/优化确认**：预览不自动覆盖；取消保留原代码；返回原代码/空代码处理；建议 Markdown 渲染。
4. **错误映射**：40001/42903/50200/50300/404/64KiB 分别中文提示；不泄露上游内容。
5. **AI 提示**：游客跳登录、成功/缓存、未配置链接、限流/上游/404、切题清空、不自动请求。
6. **配置页**：加载/404/首次 key/留空保留/脱敏/provider 默认/校验/保存刷新；用户菜单/路由 guard。
7. **测试/回归**：提交逻辑/草稿/语言切换不回归；生成类型无手写重复；lint/build。

---

### Task 1: AI API、CodeEditor 光标与提交页工具栏

**Files:**
- Create: `apps/web/src/api/ai.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/components/CodeEditor.vue`
- Modify: `apps/web/src/components/CodeEditor.spec.ts`
- Modify: `apps/web/src/views/SubmitView.vue`
- Modify: `apps/web/src/views/SubmitView.spec.ts`
- Test: 上述 spec

**Interfaces:**
- Produces:
  - `getAIConfig(): Promise<AIConfigView>`、`saveAIConfig(input: AISaveConfigInput)`、`completeCode(input: AICompleteInput)`、`formatCode(input: AIFormatInput)`、`optimizeCode(input: AIOptimizeInput)`、`getAIHint(problemId: number)`。
  - `types.ts` AI 类型别名（AIConfigView/AIProviderOption/AISaveConfigInput/AICompleteInput/AICompleteOutput/AIFormatInput/AIFormatOutput/AIOptimizeInput/AIOptimizeOutput/AIHintOutput）。
  - `CodeEditor` expose `getCursorByteOffset(): number`（UTF-16 offset → UTF-8 bytes）与 `insertAtCursor(text: string)`。
  - SubmitView：`aiConfig`/`aiAvailable`/`aiBusy`；AI 补全/格式化/优化按钮；配置提示条；格式化/优化 Dialog；错误 Toast。
- Consumes: `http` 客户端、`schema.d.ts`、`SUBMISSION_LANGUAGES`。

- [ ] **Step 1: 写失败测试**

`CodeEditor.spec.ts` 扩展 fake editor：
```ts
it('returns UTF-8 cursor offset and inserts at cursor')
// getPosition 返回位置，getModel().getOffsetAt 返回 offset；
// code '中a'、offset 1（UTF-16）→ 3 bytes；insertAtCursor 调用 executeEdits/聚焦
```

`SubmitView.spec.ts`：
```ts
it('shows config banner and disables AI buttons when not configured')
it('disables AI when GET /ai/configs returns 404')
it('completes at cursor and inserts returned fragment')
it('previews and applies formatted code only after confirm')
it('previews and applies optimized code/suggestions')
it('blocks >64KiB with Chinese toast')
it('maps 42903/50200/40001 to Chinese messages and config link')
it('keeps submit/draft behavior unchanged')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/components/CodeEditor.spec.ts src/views/SubmitView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现 API/编辑器/工具栏**

- `api/ai.ts` 使用 `http`；类型别名。
- `CodeEditor` expose 方法；插入用 `editor.executeEdits` + focus；无 editor 时 no-op/append。
- SubmitView：
  - `onMounted` 加载 `getAIConfig`；404 → `aiAvailable=false`；其他错误禁用；`configured` 决定按钮。
  - 工具栏三按钮；`aiBusy` 互斥；未配置显示 `Message` + RouterLink `/settings/ai`。
  - 补全：`new TextEncoder().encode(code).length > 65536` 提前拦截；cursor = editor expose 或 code 末尾；调用 complete；空片段 Toast；插入。
  - 格式化/优化：调用后设置预览状态，Dialog 展示只读代码（`CodeEditor` readonly 或 `<pre>`）与 suggestions（MarkdownView）；应用才 `code = result.code`。
  - 错误映射：40001 配置、42903 频率、50200/50300 服务、404 未启用/题目、其他通用；Toast。
  - 不改变提交/草稿/语言逻辑。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/components/CodeEditor.spec.ts src/views/SubmitView.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/api/ai.ts apps/web/src/api/types.ts apps/web/src/components/CodeEditor.vue apps/web/src/components/CodeEditor.spec.ts apps/web/src/views/SubmitView.vue apps/web/src/views/SubmitView.spec.ts
git commit -m "feat(web): add AI code toolbar"
```

---

### Task 2: 题目详情 AI 提示

**Files:**
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.spec.ts`
- Test: `apps/web/src/views/ProblemDetailView.spec.ts`

**Interfaces:**
- Consumes: Task 1 `getAIHint`、`ApiError`、auth/router.
- Produces:
  - 提示卡片标题右侧“AI 提示”按钮；`aiHint`/`aiHintCached`/`aiHintLoading`/`aiHintError`/`aiConfigMissing`。
  - 游客按钮跳登录 redirect；登录调用 hint；成功 MarkdownView + 缓存标签；40001 显示配置链接；429/502/503/404 Toast；切题清空 AI 状态。

- [ ] **Step 1: 写失败测试**

```ts
it('shows AI hint button and sends guests to login')
it('loads and renders AI hint with cached label')
it('shows config link when AI config missing')
it('maps 429/502/404 to Chinese toast')
it('clears AI hint when the problem id changes')
it('does not request AI hint automatically')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/ProblemDetailView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现 AI 提示**

- 提示卡片 header flex：标题 + 按钮（loading/disabled）。
- `requestAIHint()`：游客 `router.push({name:'login',query:{redirect}})`；登录调用；成功设置 hint/cached；错误映射。
- 官方提示保留；AI 提示单独区块；配置缺失内联 Message + `/settings/ai` 链接。
- 切题 watch 重置 ai 状态。
- 不自动调用。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/ProblemDetailView.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts
git commit -m "feat(web): add AI problem hints"
```

---

### Task 3: AI 配置页与用户菜单入口

**Files:**
- Create: `apps/web/src/views/AiSettingsView.vue`
- Create: `apps/web/src/views/AiSettingsView.spec.ts`
- Modify: `apps/web/src/components/UserMenu.vue`
- Modify: `apps/web/src/components/UserMenu.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Test: 上述 spec

**Interfaces:**
- Consumes: Task 1 `getAIConfig`/`saveAIConfig`.
- Produces:
  - 路由 `/settings/ai` name `ai-settings`，requiresAuth。
  - 页面：加载/404 未启用/配置表单/provider 默认模型/脱敏 key/留空保留/校验/保存 Toast。
  - UserMenu “AI 设置”菜单项。

- [ ] **Step 1: 写失败测试**

```ts
// AiSettingsView: loads config; 404 disabled; first save requires key;
// existing key blank preserves; provider change fills default model; save toast; validation
// UserMenu: logged-in shows AI 设置 and navigates; guests no menu
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AiSettingsView.spec.ts src/components/UserMenu.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现配置页/菜单**

- `AiSettingsView`：AppPageHeader/AppSurface/AppFormField/Select/InputText/Password/Button/Message；`aiAvailable`；provider options；model placeholder default；`apiKey` 留空保留；保存/错误中文；成功 Toast。
- 路由/用户菜单。
- 不修改后端。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AiSettingsView.spec.ts src/components/UserMenu.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AiSettingsView.vue apps/web/src/views/AiSettingsView.spec.ts apps/web/src/components/UserMenu.vue apps/web/src/components/UserMenu.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add AI settings page"
```

---

### Task 4: 文档、全量检查与假 provider 冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`（UI-34）
- Test: 全量后端、评测、前端；假 provider

**Interfaces:**
- Consumes: Task 1–3。
- Produces: M5-5 ✅、UI-34 ✅、AI 前端冒烟记录。

- [ ] **Step 1: 文档**

- `DEVELOPMENT.md` §6.5：AI 前端页面/交互/错误/配置。
- `docs/任务清单.md`：M5-5 ✅；M5 进度更新。
- `docs/开发日志.md`：2026-11-08 M5-5 条目。
- `README.md`：AI 前端入口/使用。
- `运行手册.md`：AI 配置/补全/格式化/优化/提示验证。
- `docs/primevue-migration.md`：UI-34 done（计数更新）。

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

- [ ] **Step 3: 假 provider 真实冒烟**

- 复用 M5-A 方式：临时 server（如 `:18081`）配置 `KAPOJ_AI_ENCRYPTION_KEY`，启动假 OpenAI 兼容服务（如 `:18082`），临时把 deepseek base_url 指向假服务；或复用已配置环境。
- 配置用户 provider/key：提交页补全（cursor 字节）、格式化、优化；题目 AI 提示首次生成、第二次缓存；未配置用户提示配置；限流 42903；隐藏题 404；AI 路由未挂载时页面“未启用”。
- 浏览器如可用：`/settings/ai`、提交页 AI 工具栏/预览、题目 AI 提示、375px；否则 API + 组件测试并记录。
- 清理临时配置/提示/用户/假服务/server；恢复 base_url；不删除既有数据。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md
git commit -m "docs: record AI frontend"
```

---

## Self-Review

**Spec coverage:**

- API/types/CodeEditor/SubmitView → Task 1。
- ProblemDetail AI hint → Task 2。
- AiSettingsView/UserMenu/route → Task 3。
- Docs/full checks/fake provider → Task 4。

**Type consistency:**

- `api/ai.ts`/types 在 Task 1 定义，Task 2/3 复用。
- `getAIHint` 在 Task 1 定义，Task 2 使用。
- `getAIConfig`/`saveAIConfig` 在 Task 1 定义，Task 3 使用。
- 路由 `ai-settings` 在 Task 3 定义，Task 1/2 的配置链接复用。

**Review Focus mapping:**

1. 光标/插入 → Task 1 CodeEditor/SubmitView tests。
2. AI 状态/降级 → Task 1 tests。
3. 格式化/优化确认 → Task 1 tests。
4. 错误映射 → Task 1/2 tests。
5. AI 提示 → Task 2 tests。
6. 配置页 → Task 3 tests。
7. 回归/生成类型 → Task 1/2/3 + Task 4 full checks。
