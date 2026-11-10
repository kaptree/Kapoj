# 平台访问与代码提交性能优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 CodeMirror 6 替换全量 Monaco，异步加载编辑器和 Markdown 渲染器，预热开发服务器常见入口，消除首次访问/首次提交的明显卡顿并降低内容页首屏成本。

**Architecture:** 新增 `src/editor.ts` 封装 CodeMirror，`CodeEditor.vue` 只做 Vue 生命周期与 v-model；提交页和 AI 造数据页异步加载编辑器，先显示固定高度骨架。`MarkdownView.vue` 改为异步包装，现有实现移到 `MarkdownViewContent.vue`，内容页先渲染骨架。Vite 预热常见入口，AdminLayout 改为路由懒加载。

**Tech Stack:** Vue 3 + Vite 8 + PrimeVue 4 + TypeScript；CodeMirror 6 + Lezer 语言包；Vitest。

**Spec:** `docs/superpowers/specs/2026-11-10-web-performance-design.md`

## Global Constraints

- 编辑器只被 `SubmitView.vue` 和 `TestcaseGeneratorView.vue` 使用；必须保留 `CodeEditor` 的 props/emits/expose：
  - props `modelValue`、`language`、`height`、`readonly`
  - emit `update:modelValue`
  - expose `getCursorByteOffset()`、`insertAtCursor(text)`
- 提交语言保持 `c/cpp/python3/java/go`；`editorLanguage` 映射 `c/cpp→cpp`、`python3→python`、`java→java`、`go→go`。
- 编辑器始终 one-dark，字号 14px、Tab 4、无自动换行、无 minimap；浅色主题只要求容器/骨架可读。
- `src/editor.ts` 封装 CodeMirror；`CodeEditor.vue` 不直接 import CodeMirror。
- 单元测试 mock `@/editor` 的 `createCodeEditor`；真实 CodeMirror 由浏览器冒烟验证。
- MarkdownView 的 props（`source`、`theme`）、安全行为、数学/代码渲染和标题 id 行为保持不变。
- 异步组件必须有固定高度/最小高度骨架，避免布局跳动。
- Vite 预热只列公共入口和常见路由，不预加载 Monaco/Markdown 重依赖。
- 不修改后端 API、提交 payload、语言列表、草稿或 AI 行为。
- 构建产物中不得再出现 `monaco-editor` / `editor.api` / Monaco worker。
- 前端检查：`corepack pnpm --filter web test`、`lint`、`build`。
- 不提交 `dist/`、`Export.zip` 或 `.superpowers` 临时文件。

## Review Focus

- 编辑器替换后，AI 光标处插入、UTF-8 光标偏移、语言切换、外部 v-model 更新、只读模式和销毁都必须保持（Task 1、Task 2）。
- 提交页在编辑器 chunk 加载前必须已经显示题目、表单和提交按钮；编辑器不能阻塞提交（Task 2）。
- Markdown 异步化不能导致题面永久空白、布局大幅抖动、XSS 防护失效或标题 id 冲突（Task 3）。
- 提交页在 AI 优化弹窗打开前不得加载 Markdown 渲染器（Task 3）。
- 构建产物不得再包含 Monaco；CodeMirror chunk 必须独立异步，不能回到主 chunk（Task 1、Task 5）。
- Vite 预热不能破坏构建或让管理端路由无法访问（Task 4）。

---

### Task 1: CodeMirror 适配层、CodeEditor 重写与 Monaco 移除

**Files:**
- Modify: `apps/web/package.json`
- Modify: `pnpm-lock.yaml`（`pnpm install` 生成）
- Create: `apps/web/src/editor.ts`
- Create: `apps/web/src/utils/text.ts`
- Create: `apps/web/src/utils/text.spec.ts`
- Delete: `apps/web/src/monaco.ts`
- Modify: `apps/web/src/components/CodeEditor.vue`
- Modify: `apps/web/src/components/CodeEditor.spec.ts`
- Modify: `apps/web/src/utils/submission.ts`
- Modify: `apps/web/src/utils/submission.spec.ts`
- Modify: `apps/web/src/views/SubmitView.vue`
- Modify: `apps/web/src/views/TestcaseGeneratorView.vue`

**Interfaces:**
- Produces:
  - `CodeEditorHandle` / `CodeEditorOptions` / `createCodeEditor(host, opts)`
  - `utf8Offset(value: string, utf16Offset: number): number`
  - `editorLanguage(language: SubmissionLanguage): string`

- [ ] **Step 1: Install CodeMirror and remove Monaco**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web add codemirror@^6.0.2 @codemirror/lang-cpp@^6.0.3 @codemirror/lang-go@^6.0.1 @codemirror/lang-java@^6.0.2 @codemirror/lang-python@^6.2.1 @codemirror/theme-one-dark@^6.1.3
corepack pnpm --filter web remove monaco-editor
```

Expected: `package.json` 不再有 `monaco-editor`，lockfile 更新。

- [ ] **Step 2: Write failing tests**

`apps/web/src/utils/text.spec.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { utf8Offset } from './text'

describe('utf8Offset', () => {
  it('counts ASCII, Chinese and surrogate pairs as UTF-8 bytes', () => {
    expect(utf8Offset('abc', 2)).toBe(2)
    expect(utf8Offset('中a', 1)).toBe(3)
    expect(utf8Offset('😀a', 2)).toBe(4)
    expect(utf8Offset('a😀b', 3)).toBe(5)
  })
})
```

`apps/web/src/components/CodeEditor.spec.ts` 重写为 mock `@/editor`：

```ts
const editor = vi.hoisted(() => ({ createCodeEditor: vi.fn() }))
vi.mock('@/editor', () => ({ createCodeEditor: editor.createCodeEditor }))

it('creates the editor, syncs props and disposes it', async () => {
  const handle = {
    getValue: vi.fn(() => 'initial'),
    setValue: vi.fn(),
    setLanguage: vi.fn(),
    getCursorByteOffset: vi.fn(() => 3),
    insertAtCursor: vi.fn(),
    focus: vi.fn(),
    destroy: vi.fn(),
  }
  editor.createCodeEditor.mockReturnValue(handle)
  const wrapper = mount(CodeEditor, { props: { modelValue: 'initial', language: 'cpp' } })
  const opts = editor.createCodeEditor.mock.calls[0]![1]
  expect(opts).toMatchObject({ value: 'initial', language: 'cpp', readonly: false })
  opts.onChange('changed')
  expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['changed'])

  await wrapper.setProps({ modelValue: 'outside' })
  expect(handle.setValue).toHaveBeenCalledWith('outside')

  await wrapper.setProps({ language: 'python' })
  expect(handle.setLanguage).toHaveBeenCalledWith('python')

  const vm = wrapper.vm as unknown as { getCursorByteOffset: () => number; insertAtCursor: (t: string) => void }
  expect(vm.getCursorByteOffset()).toBe(3)
  vm.insertAtCursor('x')
  expect(handle.insertAtCursor).toHaveBeenCalledWith('x')

  wrapper.unmount()
  expect(handle.destroy).toHaveBeenCalled()
})
```

另测：

- `modelValue` 与 handle 当前值相同时不调 `setValue`。
- handle 为空时 `getCursorByteOffset` 回退到 UTF-8 字节长度。

`apps/web/src/utils/submission.spec.ts`：把 `monacoLanguage` 改为 `editorLanguage`，断言四种映射。

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/utils/text.spec.ts src/components/CodeEditor.spec.ts src/utils/submission.spec.ts
```

Expected: FAIL，`@/editor`、`utf8Offset`、`editorLanguage` 不存在。

- [ ] **Step 4: Implement adapter and component**

`apps/web/src/utils/text.ts`：

```ts
const encoder = new TextEncoder()

/** UTF-8 byte length of value.slice(0, utf16Offset). */
export function utf8Offset(value: string, utf16Offset: number): number {
  return encoder.encode(value.slice(0, utf16Offset)).length
}
```

`apps/web/src/editor.ts`：

```ts
import { basicSetup } from 'codemirror'
import { indentWithTab } from '@codemirror/commands'
import { Compartment, EditorState, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { oneDark } from '@codemirror/theme-one-dark'
import { cpp } from '@codemirror/lang-cpp'
import { python } from '@codemirror/lang-python'
import { java } from '@codemirror/lang-java'
import { go } from '@codemirror/lang-go'
import { utf8Offset } from '@/utils/text'

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
```

`languageExtension` switch 按约束映射；`createCodeEditor`：

- `EditorState.create` extensions：
  - `basicSetup`
  - `keymap.of([indentWithTab])`
  - `EditorState.tabSize.of(4)`
  - `EditorState.readOnly.of(!!opts.readonly)`
  - `EditorView.editable.of(!opts.readonly)`
  - `oneDark`
  - `EditorView.theme({ '&': { height: '100%', fontSize: '14px' }, '.cm-scroller': { fontFamily: 'var(--kapoj-font-mono)' }, '&.cm-focused': { outline: 'none' } })`
  - `languageCompartment.of(languageExtension(opts.language))`
  - `EditorView.updateListener.of((u) => { if (u.docChanged) opts.onChange(u.state.doc.toString()) })`
- `new EditorView({ state, parent: host })`
- handle methods：
  - `getValue`/`setValue`（only dispatch when different）
  - `getCursorByteOffset` = `utf8Offset(doc, selection.main.head)`
  - `insertAtCursor` dispatch `{from: head, insert: text}` + selection anchor `head + text.length` + focus
  - `setLanguage` dispatch `languageCompartment.reconfigure(...)`
  - `focus`/`destroy`
- `CodeEditor.vue` only lifecycle/watch/expose; height on host; no CodeMirror import.

`utils/submission.ts`：

```ts
/** Maps a submission language to the editor language id. */
export function editorLanguage(language: SubmissionLanguage): string { ... }
```

`SubmitView.vue` / `TestcaseGeneratorView.vue`：改用 `editorLanguage`。

删除 `apps/web/src/monaco.ts`。

- [ ] **Step 5: Run tests and typecheck**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/utils/text.spec.ts src/components/CodeEditor.spec.ts src/utils/submission.spec.ts
corepack pnpm --filter web exec vue-tsc -b --force
```

Expected: PASS / 类型检查通过。

- [ ] **Step 6: Commit**

```bash
git add apps/web/package.json pnpm-lock.yaml apps/web/src/editor.ts apps/web/src/utils/text.ts apps/web/src/utils/text.spec.ts apps/web/src/components/CodeEditor.vue apps/web/src/components/CodeEditor.spec.ts apps/web/src/utils/submission.ts apps/web/src/utils/submission.spec.ts apps/web/src/views/SubmitView.vue apps/web/src/views/TestcaseGeneratorView.vue
git rm apps/web/src/monaco.ts
git commit -m "feat(web): replace Monaco with CodeMirror"
```

---

### Task 2: 异步加载编辑器与骨架屏

**Files:**
- Create: `apps/web/src/components/CodeEditorSkeleton.vue`
- Modify: `apps/web/src/views/SubmitView.vue`
- Modify: `apps/web/src/views/SubmitView.spec.ts`
- Modify: `apps/web/src/views/TestcaseGeneratorView.vue`
- Modify: `apps/web/src/views/TestcaseGeneratorView.spec.ts`

**Interfaces:**
- Consumes: `CodeEditor.vue`
- Produces:
  - `CodeEditorSkeleton`（`height` prop 可选，默认 100%）
  - 提交页/AI 造数据页编辑器异步加载，页面表单先渲染

- [ ] **Step 1: Write failing tests**

`SubmitView.spec.ts`：

- 在现有 `vi.mock('@/components/CodeEditor.vue')` 下，确认 `await flushPromises()` 后 `wrapper.findComponent({ name: 'CodeEditor' })` 仍存在。
- 增加一个测试：mock `@/components/CodeEditor.vue` 为延迟 resolve 的组件（或 mock loader 模块），在 resolve 前断言：
  - `wrapper.find('[data-test="code-editor-skeleton"]').exists()` 为 true
  - 提交按钮存在且可点击
  - 题目标题/语言选择已渲染
- 现有 AI 插入/提交测试保持通过。

`TestcaseGeneratorView.spec.ts`：

- 增加编辑器骨架在异步加载前的断言；加载后 CodeEditor 仍出现。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/SubmitView.spec.ts src/views/TestcaseGeneratorView.spec.ts
```

Expected: FAIL，骨架不存在。

- [ ] **Step 3: Implement**

`CodeEditorSkeleton.vue`：

```vue
<script setup lang="ts">
withDefaults(defineProps<{ height?: string }>(), { height: '100%' })
</script>

<template>
  <div
    class="code-editor-skeleton"
    data-test="code-editor-skeleton"
    :style="{ height }"
    aria-busy="true"
    aria-label="代码编辑器加载中"
  >
    <span v-for="n in 6" :key="n" class="code-editor-skeleton__line" />
  </div>
</template>
```

- 暗色背景、固定高度、细 shimmer；`prefers-reduced-motion` 关闭动画。

`SubmitView.vue`：

```ts
import { defineAsyncComponent } from 'vue'
import CodeEditorSkeleton from '@/components/CodeEditorSkeleton.vue'

const CodeEditor = defineAsyncComponent({
  loader: () => import('@/components/CodeEditor.vue'),
  loadingComponent: CodeEditorSkeleton,
  delay: 0,
})
```

- `editorRef` 改为：

```ts
const editorRef = ref<{
  getCursorByteOffset(): number
  insertAtCursor(text: string): void
} | null>(null)
```

- `.editor-wrap { min-height: 520px; }`，骨架高度 100%。
- 提交按钮不依赖 `editorRef`，保持原逻辑。

`TestcaseGeneratorView.vue`：同样异步加载，`.editor-wrap` 或容器 `min-height: 320px`。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/SubmitView.spec.ts src/views/TestcaseGeneratorView.spec.ts
corepack pnpm --filter web test
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/CodeEditorSkeleton.vue apps/web/src/views/SubmitView.vue apps/web/src/views/SubmitView.spec.ts apps/web/src/views/TestcaseGeneratorView.vue apps/web/src/views/TestcaseGeneratorView.spec.ts
git commit -m "perf(web): lazy-load the code editor"
```

---

### Task 3: MarkdownView 异步包装与内容页骨架

**Files:**
- Create: `apps/web/src/components/MarkdownViewSkeleton.vue`
- Create: `apps/web/src/components/MarkdownViewContent.vue`
- Modify: `apps/web/src/components/MarkdownView.vue`
- Modify: `apps/web/src/components/MarkdownView.spec.ts`
- Modify: `apps/web/src/views/SubmitView.vue`
- Modify: `apps/web/src/views/SubmitView.spec.ts`

**Interfaces:**
- Consumes: 现有 `MdPreview`/`@/markdown`/`sanitizeHtml`
- Produces:
  - `MarkdownView` 异步包装，props `source`/`theme` 不变
  - `MarkdownViewContent` 为原实现
  - 提交页优化弹窗仅在打开时渲染 MarkdownView

- [ ] **Step 1: Write failing tests**

`MarkdownView.spec.ts`：

- 现有测试保持，确保异步加载后仍渲染 markdown/数学/代码/安全行为/主题类/标题 id。
- 新增：挂载后立即断言骨架 `[data-test="markdown-view-skeleton"]` 存在（在 loader resolve 前），随后 `vi.waitFor` 内容出现。
- 主题类通过 `wrapper.find('.markdown-view--dark')` 或 root classes 断言。

`SubmitView.spec.ts`：

- 初始页面不包含 `.md-stub`（优化弹窗未打开）。
- 点击 AI 优化并 mock `optimizeCode` 后，`.p-dialog` 内出现 `.md-stub` 和 suggestions。
- 现有优化预览测试保持。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/components/MarkdownView.spec.ts src/views/SubmitView.spec.ts
```

Expected: FAIL，骨架不存在；优化弹窗内容可能提前渲染。

- [ ] **Step 3: Implement**

- 把现有 `MarkdownView.vue` 内容移到 `MarkdownViewContent.vue`（包括 `MdPreview`、`@/markdown`、`sanitizeHtml`、`useId`、headingId、样式）。
- `MarkdownView.vue` 为异步包装：

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

- `MarkdownViewSkeleton.vue`：固定最小高度、3 行文本块 + 1 个块级骨架；`aria-busy="true"`；`prefers-reduced-motion` 关闭 shimmer。
- 保留 `.markdown-view` 根类在 `MarkdownViewContent`；wrapper 传主题类。
- `SubmitView.vue` 的优化 Dialog：

```vue
<MarkdownView
  v-if="optimizeDialogVisible"
  :source="optimizeSuggestions"
  :theme="isDark ? 'dark' : 'light'"
/>
```

- 确保 `optimizeSuggestions` 在设置后再打开弹窗（现有顺序已满足）。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/components/MarkdownView.spec.ts src/views/SubmitView.spec.ts
corepack pnpm --filter web test
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/MarkdownViewSkeleton.vue apps/web/src/components/MarkdownViewContent.vue apps/web/src/components/MarkdownView.vue apps/web/src/components/MarkdownView.spec.ts apps/web/src/views/SubmitView.vue apps/web/src/views/SubmitView.spec.ts
git commit -m "perf(web): lazy-load markdown rendering"
```

---

### Task 4: Vite 预热与 AdminLayout 懒加载

**Files:**
- Modify: `apps/web/vite.config.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Produces:
  - `server.warmup.clientFiles` 列表
  - `/admin` 路由使用 `() => import('@/layouts/AdminLayout.vue')`

- [ ] **Step 1: Implement**

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
},
```

`router/index.ts`：

```ts
{
  path: '/admin',
  component: () => import('@/layouts/AdminLayout.vue'),
  ...
}
```

- [ ] **Step 2: Verify build and typecheck**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vue-tsc -b --force
corepack pnpm --filter web build
```

Expected: 通过；`vite.config.ts` 类型无错误。

- [ ] **Step 3: Test dev server warmup**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vite --port 5177 --strictPort --force
```

- 打开 `http://localhost:5177/`，确认首页正常、无 warmup 路径报错。
- 确认 `/admin` 在管理员登录下仍能加载。
- 停止临时 dev server。

- [ ] **Step 4: Run full web test**

```bash
corepack pnpm --filter web test
```

Expected: PASS（AdminLayout 路由懒加载不破坏现有测试）。

- [ ] **Step 5: Commit**

```bash
git add apps/web/vite.config.ts apps/web/src/router/index.ts
git commit -m "perf(web): warm dev routes and lazy-load admin layout"
```

---

### Task 5: 全量验证、性能对比与文档

**Files:**
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`（当前进度一行，可选）

- [ ] **Step 1: Frontend checks**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过。

- [ ] **Step 2: Build artifact checks**

```bash
cd /Users/admin/codespace/kapoj/apps/web/dist
find assets -iname '*monaco*' -o -iname '*editor.api*' -o -iname '*worker*'
ls -lh assets | sort -k5 -h | tail -20
```

验收：

- 无 Monaco/editor.api/Monaco worker 资源。
- CodeMirror chunk 总大小目标 `< 600KB minified / < 180KB gzip`。
- SubmitView 主 chunk 不包含编辑器/Markdown。
- Markdown chunk 仍独立，不进入首页初始资源。

- [ ] **Step 3: Browser performance comparison**

使用隔离/新端口，记录优化前后：

1. 开发服务器 `vite --force` 冷启动后首次打开 `/problems/1/submit`：
   - 请求数、总传输量、表单可见时间、编辑器可见时间。
   - 对比优化前基线：250 请求 / 约 38MB / 首次编辑器延迟。
2. 生产预览 `/problems/1/submit`：
   - 不再出现 Monaco 资源；CodeMirror chunk 大小；编辑器可见时间。
3. 内容页 `/problems/1`、`/contests/:id`：
   - 页面骨架先出现；Markdown 异步加载；无布局大幅抖动。
4. 首次访问 `/`：
   - 生产首页首屏不明显变慢；开发冷启动请求数/等待时间下降。
5. 交互：
   - 编辑器输入、语言切换、行号、Ctrl+F、Tab 缩进、只读模式。
   - AI 插入落到光标处。
   - Markdown 数学公式、代码块、XSS 防护、标题 id 不变。
   - 375px/1440px 无横向溢出；深色/浅色主题可读。

若浏览器不可用，记录延后，不声称已验证。

- [ ] **Step 4: Update docs**

- `docs/任务清单.md`：新增「跨里程碑：Web 访问与编辑器性能优化 ✅」，记录交付摘要、前后数据、测试结果。
- `docs/开发日志.md`：追加 `2026-11-10：Web 访问与代码提交性能优化`，记录根因、方案、文件、前后指标、已知取舍。
- `README.md`：当前进度补一行（可选）。

- [ ] **Step 5: Commit**

```bash
git add docs/任务清单.md docs/开发日志.md README.md
git commit -m "docs: record web performance optimization"
```

---

## Self-Review

- **Spec coverage:** CodeMirror 替换（Task 1）、异步编辑器/骨架（Task 2）、Markdown 异步包装与提交页弹窗（Task 3）、Vite 预热/AdminLayout（Task 4）、验证/性能对比/文档（Task 5）。
- **Type consistency:** `CodeEditorHandle`、`createCodeEditor`、`utf8Offset`、`editorLanguage`、`MarkdownViewContent`、`data-test` 在选择器/测试间一致。
- **Review Focus mapping:** 编辑器行为保持（Task 1/2）、提交页不阻塞（Task 2）、Markdown 安全/布局（Task 3）、无 Monaco 产物/独立 chunk（Task 1/5）、预热不破坏路由（Task 4）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出接口、关键行为、测试断言和性能阈值，不逐行抄写实现。
