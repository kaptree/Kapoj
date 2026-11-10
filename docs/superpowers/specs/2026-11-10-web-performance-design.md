# 平台访问与代码提交性能优化设计

> 目标：消除首次访问和首次进入代码提交页的明显卡顿，并降低题目/比赛/任务/学习路线等内容页首屏的固定渲染成本。

## 1. 证据与根因

### 1.1 首次提交

在开发服务器（Vite dev）冷启动后首次打开 `/problems/1/submit` 实测：

- 250 个请求，约 **38MB 未压缩传输**。
- `editor.api` 单文件约 **18.9MB** 未压缩，`toggleHighContrast` 约 **8.3MB**，`md-editor-v3` 约 2.6MB，KaTeX 约 1.7MB。
- 生产构建中：
  - `editor.api` **2,735KB（gzip 706KB）**
  - `CodeEditor` **1,252KB（gzip 318KB）**
  - `sanitize`（md-editor + KaTeX + highlight.js）**654KB（gzip 226KB）**
  - 额外 SVG/worker：ts.worker 6.6MB、css.worker 1MB、html.worker 726KB、json.worker 423KB 等。

根因：

1. `SubmitView.vue` 直接 import `CodeEditor.vue`，后者直接 import 完整 Monaco；路由 chunk 解析时就带上完整编辑器、全部语言和语言服务。
2. `SubmitView.vue` 直接 import `MarkdownView.vue`，但它只用于「AI 优化建议」弹窗；弹窗未打开时也加载 md-editor/KaTeX/highlight.js。
3. 页面 shell 与重资源没有拆分，编辑器/Markdown 与表单一起阻塞首次渲染。

### 1.2 首次访问平台

- 生产构建首页：约 **147KB gzip JS + 6.5KB CSS**，本地首屏约 137ms；生产首页本身不是主要瓶颈。
- 开发服务器冷启动首次访问：约 **96 个模块请求、400ms 首屏**，其中 Vue runtime 1.2MB、PrimeVue theme 390KB 等未压缩模块占主导。
- `AdminLayout` 在 router 中被同步 import，公共首屏也会解析管理端布局代码。
- Vite 首次访问按需 transform 模块，服务器刚启动时首个用户承担较多转换耗时。

根因：

1. 开发模式冷启动没有预热常见入口，首个访问者触发大量模块转换。
2. AdminLayout 被静态引入，增加公共模块图。
3. 生产首屏尚可，但仍有可拆分的管理端布局。

### 1.3 其他内容页

题目详情、比赛详情、任务、学习路线、题解详情等页面直接 import `MarkdownView.vue`，首屏固定加载：

- `sanitize` chunk 约 **654KB（gzip 226KB）**
- Markdown CSS 约 **100KB**
- 以及 md-editor/KaTeX/highlight.js 的解析成本。

即使页面主体是普通文本，也要先承担完整 Markdown 渲染器的下载和初始化。

## 2. 目标与非目标

### 目标

- 首次进入代码提交页时，表单、题目信息、按钮先出现；编辑器以骨架屏异步加载。
- 用 CodeMirror 6 替换 Monaco，编辑器相关 chunk 从约 4MB 降到约 300–500KB（gzip 约 100–150KB）。
- 提交页不再加载 Markdown 渲染器，除非用户打开「AI 优化建议」弹窗。
- 内容页的 Markdown 渲染器改为异步组件，页面骨架先渲染，Markdown 加载时显示轻量骨架。
- 公共首屏不再同步引入 AdminLayout；开发服务器预热常见入口，降低冷启动首个访问的模块转换等待。
- 保留现有编辑器功能：语法高亮（C/C++/Python/Java/Go）、行号、括号匹配、搜索、撤销/重做、Tab 缩进、只读模式、v-model、AI 光标处插入、UTF-8 光标偏移。
- 保留现有测试覆盖与 UI 风格。

### 非目标

- 不重写 Markdown 渲染器，不替换 md-editor-v3/MarkdownView 的渲染行为。
- 不改后端 API、数据库、权限或评测链路。
- 不做 SSR、CDN、HTTP/2 Server Push、图片压缩或 Service Worker。
- 不优化管理后台的重型表格/图表页面（只做 AdminLayout 懒加载）。
- 不改变提交表单、语言列表、草稿、AI 功能行为。
- 不引入在线代码编辑器 CDN。

### 反目标

- 编辑器替换后丢失语言切换、AI 插入或草稿行为。
- Markdown 异步化导致题目题面/比赛描述永久空白或布局抖动。
- 首次提交只把 Monaco 换成另一个 1MB+ 的编辑器包。
- 开发预热拖慢服务器启动到不可接受，或让 HMR 失效。
- 为了性能修改编辑器内容、提交 payload 或后端接口。

## 3. 约束

- 前端 Vue 3 + Vite 8 + PrimeVue 4 + TypeScript。
- 代码编辑器仅被 `SubmitView.vue` 和 `TestcaseGeneratorView.vue` 使用；必须保持现有 `CodeEditor` 对外 API：
  - props：`modelValue`、`language`、`height`、`readonly`
  - emits：`update:modelValue`
  - expose：`getCursorByteOffset()`、`insertAtCursor(text)`
- 提交语言仍为 `c/cpp/python3/java/go`；`editorLanguage` 映射到 CodeMirror 语言扩展。
- 不改变 `SUBMISSION_LANGUAGES` 和提交 payload。
- 主题沿用现有深色/浅色 token；编辑器始终使用 one-dark，行为与当前 Monaco 的 vs-dark 一致；浅色主题下只要求容器、骨架和文字可读，不要求编辑器跟随浅色主题。
- 编辑器行为与当前 Monaco 对齐：字号 14px、Tab 宽度 4、无自动换行、无 minimap。
- 异步组件必须有稳定高度的骨架，避免布局跳动。
- Vite 预热只列公共入口和常见路由，不预加载 Monaco/Markdown 等重依赖。
- 现有 `MarkdownView` 的 props（`source`、`theme`）和测试选择器尽量保持。
- 构建必须无新增 TypeScript/ESLint 错误；不提交 `dist/`。

## 4. 方案选择

### 4.1 编辑器

- **方案 A：CodeMirror 6 替换 Monaco（选用）**。包体小一个数量级，`basicSetup` 覆盖行号、搜索、括号匹配、历史、自动补全、折叠；Lezer 语言包覆盖五种提交语言。API 封装在现有 `CodeEditor.vue` 内，改动面可控。
- 方案 B：保留 Monaco，仅异步加载。页面骨架先出现，但首次提交仍下载约 1MB gzip 并解析 4MB 代码；没有解决“首次点击慢”的根本成本。
- 方案 C：裁剪 Monaco 语言/贡献。实测可从约 4MB 降到 2.7MB（gzip 约 695KB），但仍远大于 CodeMirror，且需要维护脆弱的自定义入口。

选择 A（用户已确认）。

### 4.2 Markdown 异步化

- **方案 A：全局 `MarkdownView` 异步包装（选用）**。把现有实现移到 `MarkdownViewContent.vue`，`MarkdownView.vue` 只做异步加载和骨架；所有调用方自动受益，不逐个改页面。
- 方案 B：只在 SubmitView 做异步。改动最小，但题目/比赛/任务/学习路线仍然首屏加载 654KB。
- 方案 C：按页面逐个 defineAsyncComponent。重复代码多，容易漏页面。

选择 A，并在 SubmitView 的弹窗内加 `v-if`，让 Markdown 只在弹窗打开时加载。

### 4.3 首次访问开发模式

- **方案 A：Vite `server.warmup.clientFiles` 预热 + AdminLayout 懒加载（选用）**。预热常见入口和布局，减少首个用户的 transform 等待；AdminLayout 只在 `/admin` 路由加载。
- 方案 B：只做 AdminLayout 懒加载。改动小，但开发冷启动首个访问仍慢。
- 方案 C：把所有依赖加入 `optimizeDeps.include`。Monaco/md-editor 体积大，预打包会拖慢服务器启动且可能引入大 chunk，不采用。

选择 A。

## 5. 设计

### 5.1 CodeMirror 编辑器

依赖变更：

```json
"codemirror": "^6.0.2",
"@codemirror/lang-cpp": "^6.0.3",
"@codemirror/lang-go": "^6.0.1",
"@codemirror/lang-java": "^6.0.2",
"@codemirror/lang-python": "^6.2.1",
"@codemirror/theme-one-dark": "^6.1.3"
```

移除：

```json
"monaco-editor": "^0.57.0"
```

`apps/web/src/editor.ts`（新增）封装 CodeMirror，避免 Vue 组件直接依赖 DOM 测量细节，并让单元测试可稳定 mock：

```ts
export interface CodeEditorHandle {
  getValue(): string
  setValue(value: string): void
  getCursorByteOffset(): number
  insertAtCursor(text: string): void
  setLanguage(language: string): void
  focus(): void
  destroy(): void
}

export interface CodeEditorOptions {
  value: string
  language?: string
  readonly?: boolean
  onChange: (value: string) => void
}

export function createCodeEditor(host: HTMLElement, opts: CodeEditorOptions): CodeEditorHandle
```

- `editor.ts` 内部使用 `basicSetup`、`EditorView`、`EditorState`、`Compartment`、`oneDark`，负责语言扩展、one-dark 主题、Tab 宽度 4、字号 14 和文档同步。
- `CodeEditor.vue` 只负责生命周期、v-model、props/watch 和 expose；单元测试 mock `@/editor`，真实 CodeMirror 行为由浏览器冒烟覆盖。
- 语言扩展：

```ts
function languageExtension(language: string | undefined): Extension {
  switch (language) {
    case 'python': return python()
    case 'java': return java()
    case 'go': return go()
    case 'cpp':
    case 'c':
    default: return cpp()
  }
}
```

- 语言切换通过 `Compartment.reconfigure`，不重建编辑器。
- `EditorView.updateListener` 在 `docChanged` 时调用 `opts.onChange`。
- 外部 `setValue(value)` 只在文档内容不同时 dispatch 替换。
- `EditorState.readOnly.of(readonly)` 与 `EditorView.editable.of(!readonly)`。
- 高度由宿主 `.code-editor` 固定，CodeMirror `&`/`.cm-scroller` 高度 100%。
- 主题：`oneDark` + 局部主题设置字体（`var(--kapoj-font-mono)`）和背景透明/容器风格。
- `getCursorByteOffset()`：`TextEncoder` 编码 `doc.sliceString(0, selection.main.head)` 的字节长度。
- `insertAtCursor(text)`：在 `selection.main.head` 插入并移动光标，然后 `view.focus()`。
- `CodeEditor.vue` 的 `onBeforeUnmount` 调 `handle.destroy()`。
- 删除 `apps/web/src/monaco.ts`，全局不再引用 `monaco-editor`。

`utils/submission.ts`：

- `monacoLanguage` 重命名为 `editorLanguage`，返回值保持 `cpp/python/java/go`。
- 更新 SubmitView、TestcaseGeneratorView 和对应测试的引用。

### 5.2 异步编辑器

`SubmitView.vue`：

```ts
import { defineAsyncComponent } from 'vue'

const CodeEditor = defineAsyncComponent({
  loader: () => import('@/components/CodeEditor.vue'),
  loadingComponent: CodeEditorSkeleton,
  delay: 0,
})
const editorRef = ref<{
  getCursorByteOffset(): number
  insertAtCursor(text: string): void
} | null>(null)
```

- 新增轻量 `CodeEditorSkeleton.vue`：固定高度、暗色背景、1–2 行 shimmer；`aria-busy="true"`。
- 编辑器加载期间表单、语言选择、AI 按钮、提交按钮立即可用；提交按钮不依赖编辑器实例。
- `editorRef` 使用结构化接口，避免 async component 的实例类型问题。
- `TestcaseGeneratorView.vue` 同样异步加载，骨架高度 320px。

不在题目详情/比赛详情做主动预取，避免为不提交的用户下载编辑器。

### 5.3 Markdown 异步包装

文件拆分：

- `MarkdownViewContent.vue`：现有 `MarkdownView.vue` 的实现（`MdPreview`、`@/markdown`、`sanitizeHtml`、样式）。
- `MarkdownView.vue`：异步包装：

```vue
<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import MarkdownViewSkeleton from './MarkdownViewSkeleton.vue'

const Content = defineAsyncComponent({
  loader: () => import('./MarkdownViewContent.vue'),
  loadingComponent: MarkdownViewSkeleton,
  delay: 0,
})

withDefaults(defineProps<{ source: string; theme?: 'light' | 'dark' }>(), {
  theme: 'light',
})
</script>

<template>
  <Content
    :source="source"
    :theme="theme"
    :class="theme === 'dark' ? 'markdown-view--dark' : 'markdown-view--light'"
  />
</template>
```

- `MarkdownViewSkeleton.vue`：固定最小高度（如 3 行文本 + 1 个块），带轻微 shimmer；`aria-busy="true"`。
- `MarkdownViewContent` 根节点保留 `.markdown-view` 类；主题类由 wrapper 传入。
- 现有页面不需要改 import；异步加载时页面骨架先出现。
- `SubmitView.vue` 的「优化建议」弹窗：
  - `MarkdownView` 外层加 `v-if="optimizeDialogVisible"`，只在弹窗打开后渲染/加载。
  - 弹窗内先显示 `MarkdownView` 骨架，加载完成后显示建议。

### 5.4 开发服务器预热与路由拆分

`vite.config.ts`：

```ts
server: {
  proxy: { ... },
  warmup: {
    clientFiles: [
      './src/main.ts',
      './src/App.vue',
      './src/router/index.ts',
      './src/layouts/DefaultLayout.vue',
      './src/layouts/AdminLayout.vue',
      './src/views/HomeView.vue',
      './src/views/LoginView.vue',
      './src/views/ProblemListView.vue',
      './src/views/ProblemDetailView.vue',
      './src/views/SubmitView.vue',
      './src/views/SubmissionsView.vue',
      './src/views/ContestListView.vue',
      './src/views/ContestDetailView.vue',
      './src/views/LeaderboardView.vue',
    ],
  },
}
```

- 预热只覆盖公共入口和常见页面，不包含 Monaco/Markdown/管理后台重表格。
- `router/index.ts` 的 `AdminLayout` 改为：

```ts
{ path: '/admin', component: () => import('@/layouts/AdminLayout.vue'), ... }
```

### 5.5 其他页面

- 题目、比赛、任务、学习路线、题解等通过全局 `MarkdownView` 异步包装自动受益，无需逐页修改。
- 管理员 `ProblemEditView`/`MarkdownEditor` 保持现状；编辑器页面不是本次主路径，MarkdownEditor 已单独 chunk。
- 不引入虚拟滚动；如果后续题目/提交记录长列表仍慢，再单独评估 DataTable 虚拟化。

## 6. 测试与验收

### 6.1 单元测试

- `CodeEditor.spec.ts` 重写（mock `@/editor` 的 `createCodeEditor`，真实 CodeMirror 由浏览器冒烟覆盖）：
  - 创建/销毁 editor handle，并传入 value/language/readonly/onChange。
  - 外部 `modelValue` 更新只在内容不同时调 `setValue`。
  - 语言变化调 `setLanguage`。
  - `getCursorByteOffset` 委托给 handle，并覆盖空 handle 的回退。
  - `insertAtCursor` 委托给 handle 并 focus。
- `editor.spec.ts`：对纯函数 `utf8Offset(value, utf16Offset)` 测试中文、emoji（代理对）和普通 ASCII 的 UTF-8 字节偏移；不启动真实 CodeMirror。
- `utils/submission.spec.ts`：`editorLanguage` 映射。
- `MarkdownView.spec.ts`：
  - 异步加载后仍渲染 markdown/数学/代码。
  - 主题类正确。
  - 安全测试（script/onerror/javascript link）保持。
  - 加载期间显示骨架且不抛错。
- `SubmitView.spec.ts`：
  - 提交页在编辑器异步加载前已显示表单/提交按钮。
  - 编辑器 stub 仍能被操作（AI 插入、提交代码）。
  - 优化弹窗打开前不渲染 MarkdownView；打开后渲染。
- `TestcaseGeneratorView.spec.ts`：编辑器仍能加载/使用。
- `AdminLayout` 路由懒加载不影响 `/admin` 访问（router 测试或现有布局测试）。

### 6.2 构建验收

```bash
corepack pnpm --filter web build
```

验收：

- 构建产物中不再出现 `monaco-editor`、`editor.api`、`editor.worker`、`ts.worker`、`css.worker`、`html.worker`、`json.worker` 等 Monaco 资源。
- 编辑器相关 chunk 总大小目标 **< 600KB minified / < 180KB gzip**。
- `SubmitView` 主 chunk 不包含 Markdown/Monaco/CodeMirror；CodeMirror 为独立异步 chunk。
- Markdown (`sanitize`/`md-editor`) 仍独立 chunk，只在 MarkdownView 实际渲染时加载。

### 6.3 浏览器验收

开发模式 / 生产预览分别测量：

1. 首次提交页：
   - 页面标题、题目信息、语言选择、提交按钮在编辑器加载前可见。
   - 网络面板不再出现 `editor.api` 18MB/2.7MB 或 Monaco worker。
   - 编辑器加载完成后可输入、切换语言、显示行号/高亮、Ctrl+F 搜索、Tab 缩进。
   - AI 插入（模拟）落到光标处，UTF-8 光标偏移正确。
2. 内容页（题目详情/比赛详情/任务/学习路线）：
   - 页面骨架先出现，Markdown 异步加载时显示骨架，无布局大幅抖动。
   - 数学公式、代码块、标题锚点、防 XSS 行为保持。
3. 首次访问平台：
   - 生产首页首屏不明显变慢；开发冷启动首个访问请求数/等待时间下降。
4. 响应式/主题：
   - 375px/1440px 编辑器与 Markdown 无横向溢出。
   - 深色/浅色主题下编辑器与骨架可读。

记录优化前后数据（请求数、传输量、编辑器 chunk、首屏/编辑器可见时间）。

### 6.4 全量检查

- `corepack pnpm --filter web test`
- `corepack pnpm --filter web lint`
- `corepack pnpm --filter web build`
- 后端不需要改动；不跑后端测试，除非连带修改 API/类型。

## 7. 已知取舍

- 用 CodeMirror 替换 Monaco，编辑体验会变化：语言智能提示、命令面板、部分高级编辑功能不如 Monaco；提交所需功能保留。
- Markdown 异步化会让题面/描述比页面骨架晚一小段时间出现；用固定高度骨架减少布局跳动。
- Vite 预热会增加开发服务器启动时间，换首个访问更快；只预热常见入口，范围有限。
- 不做编辑器预取，未提交的用户不会下载编辑器 chunk；首次提交仍需下载 CodeMirror（但远小于 Monaco）。
- 不重写 Markdown 渲染器；`md-editor-v3`/KaTeX/highlight.js 仍是一个较大 chunk，只是不再阻塞首屏和提交页。
- 生产环境首页已经较快，本次主要收益在提交页与内容页；如果部署网络/CDN 本身慢，仍需基础设施层优化。
