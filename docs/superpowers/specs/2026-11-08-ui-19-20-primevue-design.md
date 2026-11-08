# UI-19 / UI-20 PrimeVue 迁移设计

> 目标：完成 `docs/primevue-migration.md` 最后两个页面迁移任务，把题库编辑/测试点/样例与 403/404 异常页切到 PrimeVue 4，随后移除 Element Plus 依赖与过渡样式。

## 1. 背景

- `UI-00～UI-18`、`UI-30～UI-39` 已迁移；剩余 `UI-19`（题库编辑）与 `UI-20`（异常页）。
- 当前 `feat/ui-19-20-primevue` 分支从已本地合并 M5-4 的 `main`（`c9855dc`）创建；M5-4 已给 `TestcaseManager` 增加「AI 造数据」入口（`problem-testcase-generator`），迁移必须保留。
- `ProblemEditView`、`TestcaseManager`、`SampleEditor`、`ForbiddenView`、`NotFoundView` 仍使用 Element Plus；`main.ts` 是唯一允许同时安装两套库的文件。
- UI-19 的编辑页依赖 `MarkdownEditor`（md-editor-v3，不属于本次迁移）、Monaco（仅测试点生成页使用）、现有 API 与权限。

## 2. 目标与非目标

### 目标

- `ProblemEditView` 使用 PrimeVue 4 与共享 UI 组件，保留全部字段、Markdown 编辑、样例编辑、测试点管理、保存/删除/跳转与错误处理。
- `TestcaseManager` 使用 PrimeVue 4，保留测试点列表、分数/样例修改、上下移动、删除、上传、checker 上传、AI 造数据入口与请求竞态保护。
- `SampleEditor` 使用 PrimeVue 4，保留样例增删、最多 10 组、输入/输出双向绑定。
- `ForbiddenView`/`NotFoundView` 统一为 PrimeVue 结果页视觉，保留「返回首页」行为与中文文案。
- 测试覆盖四个页面/组件的核心行为；新增 `ProblemEditView.spec.ts`、`SampleEditor.spec.ts`、`ForbiddenView.spec.ts`、`NotFoundView.spec.ts`，重写 `TestcaseManager.spec.ts` 的 PrimeVue 测试。
- 清理 Element Plus：从 `main.ts`、`package.json`、lockfile、`styles.css`/`base.css` 移除 Element Plus 与 `@element-plus/icons-vue`；legacy allowlist 清空；更新迁移文档验证记录。

### 非目标

- 不修改后端、API、OpenAPI、路由 path/name/meta（UI-20 仅可补 `title`）、权限、Pinia store、SSE、localStorage 草稿。
- 不改变测试点上传字段、16 MiB 限制、checker 256 KiB 限制、题目保存语义、发布条件。
- 不重做 MarkdownEditor（继续 md-editor-v3）、不替换 CodeEditor/Monaco、不新增 UI 依赖。
- 不新增业务功能（除保留 M5-4 AI 入口外）；不做整体视觉重设计。
- 不引入 `@primevue/forms` 或新校验库。

### 反目标

- 迁移后仍有 Element Plus import/selector/`v-loading` 残留。
- AI 造数据入口丢失，或 `ProblemEditView` 与 `TestcaseManager` 的 props/emit 契约改变。
- 测试点上传/排序/删除、题目保存/删除、样例编辑行为回归。
- 暗色/亮色主题、390px 窄屏下表格/表单溢出。

## 3. 方案与取舍

### 方案 A（采纳）：原地组合 PrimeVue + 共享组件，最后移除 Element Plus

- `ProblemEditView` 用 `AppPageHeader`/`AppSurface`/`AppFormField`/`Message`/`Tabs`/`MarkdownEditor`/`SampleEditor`/`TestcaseManager` 重新组织；`TestcaseManager` 用 `DataTable`/`InputNumber`/`ToggleSwitch`/`Dialog` 确认/Toast；`SampleEditor` 用 `Textarea`/`Button`/`AppSurface` 风格。
- UI-20 用 `AppSurface` + `AppEmptyState` 组成结果页。
- 迁移完成后删除 Element Plus 引导、依赖和过渡 CSS。
- 优点：行为风险最小，复用已完成页面的模式；一次清空 allowlist；不新增依赖。
- 代价：`ProblemEditView` 文件会较大（约 500 行），但结构清晰；需要重写多个 spec。

### 方案 B（未采纳）：只迁移 TestcaseManager/SampleEditor，编辑页保留 Element Plus

- 优点：改动小；风险低。
- 缺点：UI-19 未完成，allowlist 仍保留、两套 UI 混用；与迁移目标冲突。

### 方案 C（未采纳）：借迁移重做表单布局与客户端校验

- 优点：可以顺手改善表单体验。
- 缺点：改变现有行为与测试基线，超出 UI-19 范围；没有产品需求支撑。

### UI-20 结构选择

- 采纳 `AppSurface` + `AppEmptyState`：共享空态组件已有 icon/title/description/actions，外层卡片负责页面留白；不需要新组件。
- 未采纳自定义 `AppResultState`：两个页面使用两三次，新增抽象收益低。

## 4. 详细设计

### 4.1 共享约束

- 所有新页面/组件只 import PrimeVue 与 `@/components/ui/*`；不 import Element Plus。
- 中文文案沿用现有含义；错误 Toast 显示 `ApiError.message`，无消息时用现有 fallback。
- 状态色/Tag/按钮 severity 使用现有 `--kapoj-*` token 与 PrimeVue preset。
- 目标断点：`<=640px` 单列/按钮铺满；表格用 `responsive-layout="scroll"` 或容器 `overflow-x: auto`。
- 主题：`useTheme()` 的 `isDark` 传给 `MarkdownEditor`。

### 4.2 ProblemEditView

结构：

```
AppPageHeader (eyebrow PROBLEMS // CREATE|EDIT, title 新建题目|编辑题目,
               description, actions 返回题库)
AppLoadingState (首次加载)
AppErrorState + 返回题库 (加载失败且非 404)
AppEmptyState + 返回题库 (404 题目不存在)
Message info (新建题目的默认不公开提示)
Message error (保存/删除错误)
AppSurface
  <form>
    基本信息 grid：
      AppFormField 标题 -> InputText maxlength=100
      AppFormField 题目标识 -> InputText placeholder
      AppFormField 类型 -> SelectButton 传统/特判/交互
      AppFormField 难度 -> Select DIFFICULTIES
      AppFormField 时间限制 -> InputNumber 100..10000 step 100 + 毫秒
      AppFormField 内存限制 -> InputNumber 16..1024 step 16 + MB
      AppFormField 标签 -> InputChips max=10 allow-duplicate=false add-on-blur
      AppFormField 来源 -> InputText maxlength=200
    题面内容：
      Tabs lazy
        题目描述 -> MarkdownEditor theme
        输入格式 -> MarkdownEditor theme height=240px
        输出格式 -> MarkdownEditor theme height=240px
        样例 -> SampleEditor
        提示 -> MarkdownEditor theme height=240px
    AppFormField 公开 -> ToggleSwitch (new 时 disabled) + 发布条件提示
    操作：保存/创建 (type=submit, loading), 返回题库, 删除题目 (edit)
  </form>
AppSurface (isEdit && !loadFailed)
  TestcaseManager (problemId,type,checkerUploaded,@changed=reloadFlags)
```

行为保留：

- `form` 继续是 `reactive<Required<ProblemInput>>`；`fill()` 映射 API 字段；`watch(route.params.id)` 加载。
- `save()`：edit 调 `updateProblem`，create 调 `createProblem` 后 `router.replace(problem-edit)`；成功 Toast；失败写 `error` 并恢复 `form.isPublic = published`。
- `remove()`：`useConfirm().require` 中文确认；确认后 `deleteProblem`；按 `offlined` 显示 Toast 并 `router.replace(problems)`；失败写 `error`。
- `reloadFlags()` 仅刷新 `checkerUploaded`。
- 无新增客户端校验；保留 `maxlength`、InputNumber 范围与服务器最终校验。
- `InputChips` 值 trim、去空、最多 10；若现有标签含重复项，保留但不新增重复。

### 4.3 TestcaseManager

结构：

```
header: h2 测试点 + Button AI 造数据 (traditional only)
Message error (load failure, 带重试)
DataTable loading emptyMessage="还没有测试点" responsive-layout="scroll"
  Column 序号 / 大小（输入 / 输出） / 分数 InputNumber / 样例 ToggleSwitch / 操作
操作: Button 上移/下移/删除 (text, size small; boundary disabled)
AppSurface/区域 上传新测试点:
  隐藏 input[type=file] (input/output) + Button 选择文件 + 文件名
  InputNumber 分数 0..1000
  ToggleSwitch 作为样例
  Button 上传测试点 (disabled until both files, loading=busy)
  hint 输入/输出各不超过 16 MiB
special:
  h3 特判 checker + Tag 已上传/未上传
  隐藏 input[type=file] + Button 选择 checker + 文件名
  Button 上传 checker
```

行为保留：

- props 仍为 `{ problemId:number; type:ProblemType; checkerUploaded:boolean }`，emit `changed` 不变。
- `latest` 请求序号防止切题旧响应覆盖；`busy` 阻止并发操作。
- 上传成功清空文件/input，Toast，`load()`，`emit('changed')`。
- 分数 `value-change` 调 `updateTestcase`，失败回滚重载；样例同理。
- 上移/下移本地交换后调 `reorderTestcases`，失败重载。
- 删除用 `useConfirm`，中文「删除测试点/取消/删除」；成功 Toast + 重载 + emit。
- checker 选择校验、上传、Toast、emit。
- AI 按钮 `router.push({name:'problem-testcase-generator',params:{id:problemId}})`，仅 traditional 显示。

### 4.4 SampleEditor

结构：

- 每个样例一个小节（`<section>`/`AppSurface` 可选，避免嵌套过多）：标题「样例 N」+ 删除 Button。
- 两列 grid：输入/输出 `Textarea` rows=4，mono class；`<=640px` 单列。
- 空态：`还没有样例。`
- 添加 Button，disabled at 10；提示「最多 10 组样例」。

行为保留：

- `defineModel<Sample[]>({required:true})` 不变；add/remove 返回新数组（保持不可变更新）。
- 不改变样例字段名与 API 输入。

### 4.5 UI-20 异常页

`ForbiddenView` / `NotFoundView`：

```
AppSurface (result-surface, 居中, max-width)
  <p class="result-code">403 / 404</p>
  AppEmptyState
    icon: pi-lock / pi-compass
    title: 没有权限 / 页面不存在
    description: 你的账号无法访问这个页面。 / 请检查地址是否正确。
    actions: Button as router-link to="/" label="返回首页" icon="pi pi-home"
```

- 保留现有标题/副标题；不新增「返回上一页」等行为。
- `not-found` 路由补 `meta: { title: '页面不存在' }`，让顶栏标题正确；不改 path/name。
- 不使用 `AppErrorState`（它不是结果页语义，且没有标题插槽）。

### 4.6 最终 Element Plus 清理

- `apps/web/src/main.ts`：移除 `ElementPlus`、`zhCn`、`element-plus/dist/index.css` import 与 `app.use(ElementPlus, ...)`。
- `apps/web/package.json`：移除 `element-plus`、`@element-plus/icons-vue`；运行 `corepack pnpm install` 更新 lockfile。
- `apps/web/src/styles.css`：删除 `.el-result__title` / `.el-result__subtitle` 过渡覆盖。
- `apps/web/src/styles/base.css`：链接颜色从 `var(--el-color-primary, var(--kapoj-primary))` 改为 `var(--kapoj-primary)`；删除 legacy 注释。
- 全局 `grep -R "element-plus|ElMessage|<el-|v-loading|--el-" apps/web/src` 必须只剩说明性文档（无代码命中）。
- `docs/primevue-migration.md`：UI-19/UI-20 标记 done，legacy allowlist 清空，更新验证记录与最终清理说明。
- `docs/任务清单.md`/`docs/开发日志.md`/`DEVELOPMENT.md`（如涉及）同步 UI-19/UI-20 完成。

## 5. 测试与验收

### 单元/组件测试

- `ProblemEditView.spec.ts`：
  - 加载并填充字段（标题/类型/难度/标签/样例/公开开关）。
  - 404 显示「题目不存在」与返回题库；其他加载失败显示重试。
  - 保存 edit 调 `updateProblem` + Toast；create 调 `createProblem` + `router.replace(problem-edit)`。
  - 删除确认 accept 调 `deleteProblem` + 按 `offlined` Toast + `router.replace(problems)`；取消不调用。
  - 保存失败恢复 `isPublic` 并显示错误。
- `TestcaseManager.spec.ts`：
  - 加载/切题竞态；AI 按钮 traditional/special 显隐与跳转。
  - 上移/下移调 reorder（边界禁用）；分数/样例调 updateTestcase；失败重载。
  - 删除确认 accept/cancel；checker 上传；上传缺文件提示。
- `SampleEditor.spec.ts`：添加/删除/最多 10/双向绑定；空态。
- `ForbiddenView.spec.ts` / `NotFoundView.spec.ts`：文案、403/404 标识、返回首页链接、无 Element Plus。
- 现有 `ProblemDetailView`/`ProblemListView`/`SubmitView` 等迁移测试保持通过。

### 命令

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm test:web
corepack pnpm lint:web
corepack pnpm build:web
git diff --check
```

额外检查：

```bash
grep -RIn "element-plus\|ElMessage\|<el-\|v-loading\|--el-" apps/web/src \
  --include='*.vue' --include='*.ts' --include='*.css'
```

Expected: 无代码命中。

### 手工/浏览器（如可用）

- `/problems/new` 与 `/problems/:id/edit`：暗/亮主题、390px 单列、标签增删、样例增删、测试点上传/排序/删除、AI 入口、checker（special）。
- `/forbidden`、不存在路径：403/404 视觉、返回首页、顶栏标题。
- 控制台无 error；无 Element Plus 样式加载。

## 6. 风险与缓解

- **InputChips 与旧自由标签语义**：旧 `el-select allow-create` 允许任意字符串；InputChips 也自由输入，但需 Enter/失焦添加。保留「输入后回车添加」提示；trim/去空/最多 10；不改变后端 tag 规则。
- **File input 测试**：使用原生 hidden input，测试通过 `Object.defineProperty(files)` + `trigger('change')`，避免 PrimeVue FileUpload 的自动上传语义。
- **`MarkdownEditor` 主题**：迁移后必须传 `theme`，否则亮色编辑器在暗色页面中突兀。
- **Element Plus 移除顺序**：先迁移页面并跑测试，再删依赖；删除后重跑全量，避免提前破坏未迁移文件。
- **M5-4 入口**：TestcaseManager 重写时保留 AI 按钮与路由名；M5-4 已在 `main`，无跨分支冲突。
- **既有行为回归**：保留 props/emit、请求竞态、Toast 文案、发布/删除语义；测试覆盖关键路径。

## 7. 交给计划的事项

- 具体组件 imports、grid 断点、文件 input 的 DOM 结构与测试 helper。
- `ProblemEditView` 拆分为单文件还是拆子组件（默认单文件，保持现有路由懒加载）。
- `TestcaseManager` 是否把上传区拆成子组件（默认不拆，保持 props/emit 简单）。
- 手工浏览器验证是否可用；不可用则记录单测覆盖。
