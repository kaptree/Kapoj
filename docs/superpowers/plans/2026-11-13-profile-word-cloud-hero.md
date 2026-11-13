# 个人主页 Hero 词云 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把个人主页的词云移到姓名/@用户名右侧的 Hero 区域，改成不规则散点布局；移除别人的评价列表，点击词云面板弹出评价输入 Dialog。

**Architecture:** 新增纯函数词云布局工具，负责确定性螺旋散点与碰撞检测；新增 `ProfileWordCloud` 只负责展示与点击，新增 `ProfileEvaluationDialog` 负责评价表单/保存/删除；`UserProfileView` 统一加载评价数据并同时驱动 Hero 词云与 Dialog。后端评价 API 不变。

**Tech Stack:** Vue 3 `<script setup>` + TypeScript + PrimeVue 4 + Vitest + Playwright；Go 后端不改。

**Spec:** `docs/superpowers/specs/2026-11-13-profile-word-cloud-hero-design.md`

## Global Constraints

- 不改后端评价 API、OpenAPI、数据库或权限；继续使用 `GET /users/:id/evaluations` 的 `cloud/total/myEvaluation/canEvaluate`。
- 不新增前端依赖；使用 PrimeVue `Dialog`/`Textarea`/`Button`/`useConfirm`/`useToast`。
- 词云布局必须是确定性的：相同输入得到相同位置；不得出现越界或重叠。
- 词云面板整体可点击，点击打开评价 Dialog；词条本身不单独点击。
- 不再渲染别人的评价列表、头像、时间、分页、“加载更多”或删除他人评价。
- 自己的评价仍可创建、更新、删除；Dialog 内完成。
- 评价内容 trim 后 1–40 字符；换行/制表符归一化为空格；不能自评。
- 页面只请求一次评价数据，词云和 Dialog 共用；评价加载失败不影响主页其他内容。
- 保留荣誉证书卡；右侧栏只保留证书卡。
- 支持深色/浅色主题、375px 移动端、`prefers-reduced-motion`。
- 只提交本任务相关文件，不夹带无关未跟踪文件。

## Review Focus

- **长评价文本**：40 字评价在词云中不能撑破容器；必须有最大宽度、省略和字号收缩。测试：Task 1 长文本布局、Task 2 长文本渲染。
- **空/单条/24 条评价**：空态、单条居中、24 条仍不重叠且能显示总数/溢出。测试：Task 1 空数组/24 条/小容器，Task 2 空态与 `+N`。
- **游客/本人/不可评价**：Dialog 必须显示正确提示，不能出现可提交的输入框。测试：Task 3 三种状态。
- **评价加载失败**：词云显示错误与重试，主页其它模块正常。测试：Task 2 错误态、Task 4 页面降级。
- **375px 移动端**：Hero 纵向排列，词云与 Dialog 不横向溢出。测试：Task 2 响应式布局、Task 4 移动端类名/DOM、Task 5 真实浏览器。

---

### Task 1: 词云布局纯函数

**Files:**
- Create: `apps/web/src/utils/profileWordCloudLayout.ts`
- Create: `apps/web/src/utils/profileWordCloudLayout.spec.ts`

**Interfaces:**
- Consumes: `EvaluationCloudItem`（`{ text: string; count: number }`）。
- Produces:

```ts
export interface ProfileWordCloudItem {
  text: string
  count: number
}

export interface ProfileWordCloudPlacement {
  text: string
  count: number
  x: number
  y: number
  width: number
  height: number
  fontSize: number
  rotation: number
  colorIndex: number
}

export interface ProfileWordCloudLayout {
  placements: ProfileWordCloudPlacement[]
  overflow: number
}

export interface ProfileWordCloudOptions {
  maxWords?: number
  gap?: number
  maxWidthRatio?: number
  minFontSize?: number
  maxFontSize?: number
}

export function buildProfileWordCloudLayout(
  items: ProfileWordCloudItem[],
  width: number,
  height: number,
  options?: ProfileWordCloudOptions,
): ProfileWordCloudLayout
export function estimateProfileWordWidth(text: string, fontSize: number): number
export function profileWordCloudColorIndex(text: string): number
export function createProfileWordCloudRandom(seed: number): () => number
```

- [ ] **Step 1: 写失败测试**

在 `profileWordCloudLayout.spec.ts` 覆盖：

```ts
it('places 24 short items without overlap and inside the canvas', () => {
  const items = Array.from({ length: 24 }, (_, i) => ({
    text: `评价${i + 1}`,
    count: (i % 5) + 1,
  }))
  const layout = buildProfileWordCloudLayout(items, 640, 220)
  expect(layout.placements).toHaveLength(24)
  expect(layout.overflow).toBe(0)
  for (const p of layout.placements) {
    expect(p.x - p.width / 2).toBeGreaterThanOrEqual(0)
    expect(p.x + p.width / 2).toBeLessThanOrEqual(640)
    expect(p.y - p.height / 2).toBeGreaterThanOrEqual(0)
    expect(p.y + p.height / 2).toBeLessThanOrEqual(220)
  }
  for (let i = 0; i < layout.placements.length; i += 1) {
    for (let j = i + 1; j < layout.placements.length; j += 1) {
      const a = layout.placements[i]
      const b = layout.placements[j]
      const overlapX = Math.abs(a.x - b.x) < (a.width + b.width) / 2
      const overlapY = Math.abs(a.y - b.y) < (a.height + b.height) / 2
      expect(overlapX && overlapY).toBe(false)
    }
  }
})
```

另加：
- 相同输入两次布局的 `placements` 完全相等（位置/字号/旋转）。
- 40 字长文本的 `width <= 640 * 0.45`，字号不大于短文本同 count 的字号。
- 200×120 小容器 `overflow > 0`，所有 placement 仍在容器内。
- `rotation` 在 -8–8 度。
- `maxWords: 10` 时 `placements.length <= 10` 且 `placements.length + overflow === items.length`（超出 maxWords 的条目计入 overflow）。
- 空数组返回 `{ placements: [], overflow: 0 }`。
- `profileWordCloudColorIndex` 对相同文本稳定、在 0–4。
- `createProfileWordCloudRandom` 同 seed 序列稳定。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/utils/profileWordCloudLayout.spec.ts`
Expected: FAIL，模块不存在。

- [ ] **Step 3: 实现布局**

- 字号：`maxFontSize` 默认 27px，`minFontSize` 默认 12px；按 `count` 增大，按文本长度缩小。
- 文本宽度：CJK 约 1em、ASCII 约 0.58em；单条宽度上限 `width * maxWidthRatio`（默认 0.45），高度 `fontSize * 1.35`。
- 从容器中心开始螺旋：角度步进 0.35rad，半径步进 3px；碰撞盒加 `gap`（默认 5px）；旋转只影响视觉，不参与碰撞。
- 若一次放不下，整体字号乘 0.9 重试最多 3 次；仍放不下的条目计入 `overflow`。
- 旋转/颜色用文本哈希 + 固定种子生成；位置确定、稳定。
- 所有坐标 clamp 在容器内。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/utils/profileWordCloudLayout.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/utils/profileWordCloudLayout.ts apps/web/src/utils/profileWordCloudLayout.spec.ts
git commit -m "feat(web): add profile word cloud layout"
```

---

### Task 2: `ProfileWordCloud` 组件

**Files:**
- Create: `apps/web/src/components/profile/ProfileWordCloud.vue`
- Create: `apps/web/src/components/profile/ProfileWordCloud.spec.ts`

**Interfaces:**
- Consumes: Task 1 的 `buildProfileWordCloudLayout`。
- Produces:
  - Props `{ cloud: EvaluationCloudItem[]; total: number; loading?: boolean; error?: string }`
  - Emits `open()`、`retry()`。

- [ ] **Step 1: 写失败测试**

`ProfileWordCloud.spec.ts` 用 `vi.mock('@/utils/profileWordCloudLayout')` 控制 placements/overflow，覆盖：
- 渲染词条文本、`total` 徽标、`点击词云，写一句评价` 提示。
- 点击面板 emit `open`；点击右上角“写评价”按钮 emit `open`。
- 空态显示“还没有评价 / 点击写下第一条评价”，点击仍 emit `open`。
- `loading` 显示加载态，不渲染词条。
- `error` 显示错误与“重试”按钮；点击重试 emit `retry`，不 emit `open`。
- 长文本 `title` 保留全文，文本节点有省略 class。
- `overflow=3` 时显示 `+3`。
- 面板有 `aria-label="评价词云，共 N 条评价"`。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/components/profile/ProfileWordCloud.spec.ts`
Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现组件**

- 根节点 `div.profile-word-cloud`，`data-test="profile-word-cloud"`，`role="group"`，`aria-label`，`@click="emit('open')"`；`cursor:pointer`。
- 用 `ref` 测量面板宽高；`ResizeObserver` 更新；初始回退 `640×220`；宽度 `<480` 时 `maxWords=16`，否则 24。
- 用 `buildProfileWordCloudLayout` 计算 placements；词条绝对定位，`left/top` 为 placement 中心，`transform: translate(-50%,-50%) rotate(...)`；字号用 `fontSize` px；颜色 class 由 `colorIndex` 决定。
- 词条 `pointer-events:none`，`title` 全文，CSS `max-width` + `text-overflow:ellipsis`。
- 右上角 `<button class="profile-word-cloud__cta" aria-label="写评价" @click.stop="emit('open')">`，显示编辑图标 + “写评价”。
- `total` 徽标；`overflow > 0` 时显示 `+N`。
- 空/加载/错误状态；错误重试按钮 `@click.stop="emit('retry')"`。
- 面板高度桌面 220px、移动端 180px；`prefers-reduced-motion` 关闭词条浮动/过渡。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/components/profile/ProfileWordCloud.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/components/profile/ProfileWordCloud.vue apps/web/src/components/profile/ProfileWordCloud.spec.ts
git commit -m "feat(web): add profile word cloud hero panel"
```

---

### Task 3: `ProfileEvaluationDialog` 组件

**Files:**
- Create: `apps/web/src/components/profile/ProfileEvaluationDialog.vue`
- Create: `apps/web/src/components/profile/ProfileEvaluationDialog.spec.ts`

**Interfaces:**
- Consumes: `saveMyEvaluation`、`deleteMyEvaluation`、`EvaluationItem`、PrimeVue Dialog/Textarea/Button/useToast/useConfirm。
- Produces:
  - Props `{ visible: boolean; targetUserId: number; targetActive?: boolean; myEvaluation: EvaluationItem | null; canEvaluate: boolean; isSelf: boolean; isLoggedIn: boolean }`
  - Emits `update:visible`、`saved`、`deleted`。

- [ ] **Step 1: 写失败测试**

`ProfileEvaluationDialog.spec.ts`（PrimeVue + `teleport: true` 或 `attachTo: document.body`；提供 router 插件或 stub `RouterLink`；mock `@/api/social`、toast、confirm）覆盖：
- `isLoggedIn=false`：显示“登录后可以给 TA 写评价”+ 登录按钮，无 textarea。
- `isSelf=true`：显示“这是你的主页，不能给自己评价”，无 textarea。
- `canEvaluate=false`（非本人）：传 `targetActive=false` 时显示“该用户当前不可评价”，无 textarea。
- `canEvaluate=true`：textarea 存在，计数器 `0/40`，提交按钮初始 disabled。
- 输入后提交：调用 `saveMyEvaluation(targetUserId, '内容')`，关闭 Dialog，emit `saved`，toast success。
- `myEvaluation` 存在：预填内容，按钮文案“更新评价”，显示“删除我的评价”。
- 删除：`confirm.require` 的 accept 调用 `deleteMyEvaluation`，关闭并 emit `deleted`。
- 输入 `a\nb\tc` 提交时调用参数为 `a b c`。
- 保存失败：Dialog 不关闭，显示错误，输入保留。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/components/profile/ProfileEvaluationDialog.spec.ts`
Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现组件**

- `dialogVisible` computed 双向绑定。
- `content` ref；`watch([visible, myEvaluation])` 在打开/数据变化时预填。
- `normalizedContent = content.replace(/[\r\n\t]+/g, ' ')`；`contentLength` 用 `Array.from`；`canSubmit` trim 后非空且不 saving。
- `title`：`myEvaluation ? '更新评价' : '写评价'`；`submitLabel` 同理。
- 模板：`Dialog`（modal，宽度 `min(92vw, 30rem)`）+ 状态分支；可评价时 Textarea（40 maxlength，autofocus，rows 3）+ 计数器 + 提交；已有评价时删除按钮。
- `submit` 调 `saveMyEvaluation`；`remove` 用 `useConfirm` 二次确认后调 `deleteMyEvaluation`；成功 toast、关闭、emit。
- 失败在 Dialog 内显示 `error`，不关闭。
- 登录按钮 `RouterLink` 到 login（或 emit `login`，本实现用 RouterLink）。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- src/components/profile/ProfileEvaluationDialog.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/components/profile/ProfileEvaluationDialog.vue apps/web/src/components/profile/ProfileEvaluationDialog.spec.ts
git commit -m "feat(web): add profile evaluation dialog"
```

---

### Task 4: 个人主页 Hero 集成与旧评价卡移除

**Files:**
- Modify: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/views/UserProfileView.spec.ts`
- Delete: `apps/web/src/components/profile/ProfileEvaluationCard.vue`
- Delete: `apps/web/src/components/profile/ProfileEvaluationCard.spec.ts`

**Interfaces:**
- Consumes: Task 2/3 组件、`listUserEvaluations`、`EvaluationPage`。
- Produces: Hero 词云 + 点击弹 Dialog 的完整页面。

- [ ] **Step 1: 重写页面测试**

`UserProfileView.spec.ts`：
- 移除 `ProfileEvaluationCard` stub；新增 `ProfileWordCloud` stub（props `cloud/total/loading/error`，emit `open`/`retry`）和 `ProfileEvaluationDialog` stub（props `visible/targetUserId/myEvaluation/canEvaluate/isSelf/isLoggedIn`）。
- mock `listUserEvaluations` 返回 `{ total: 3, items: [], cloud: [...], myEvaluation: null, canEvaluate: true }`。
- 断言 `listUserEvaluations` 调用一次 `('7', 1, 1)`。
- 断言 `.profile-hero` 在 `.profile-surface` 之前，词云 stub 在 Hero 内；简介在 surface 内（DOM 顺序）。
- 点击词云 stub 的 `open` 后 Dialog stub `visible=true`。
- Dialog `saved`/`deleted` 后再次调用 `listUserEvaluations`。
- 评价加载失败时主页仍渲染 profile/bio/stats，词云收到 `error`。
- 右侧栏只有证书 stub，没有评价卡/评价列表。
- 保留关注、统计、贡献图、Rating 历史、404/加载测试。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- src/views/UserProfileView.spec.ts`
Expected: FAIL，旧组件/DOM 断言不匹配。

- [ ] **Step 3: 实现页面**

- 删除 `AppPageHeader` 与旧 `profile-head` 头像/身份重复区；新增 `.profile-hero`：
  - 左：Avatar、`USER // PROFILE` eyebrow、`h1` 昵称、`@username`。
  - 右：`ProfileWordCloud`，绑定 `evaluationPage?.cloud`、`total`、`loading`、`error`，`@open`、`@retry`。
- `AppSurface` 保留 IdentityShowcase、bio、social row、stats。
- 评价状态：
  ```ts
  const evaluationPage = ref<EvaluationPage | null>(null)
  const evaluationLoading = ref(false)
  const evaluationError = ref('')
  const evaluationDialogOpen = ref(false)
  async function loadEvaluations(id: string, seq: number) { ... listUserEvaluations(id, 1, 1) ... }
  ```
  `loadProfile` 中 `void loadEvaluations(id, seq)`，失败只设置 `evaluationError`，不影响 profile。
- `retryEvaluations`、`reloadEvaluations`。
- Dialog：
  ```html
  <ProfileEvaluationDialog
    v-if="profile"
    v-model:visible="evaluationDialogOpen"
    :target-user-id="profile.id"
    :target-active="true"
    :my-evaluation="evaluationPage?.myEvaluation ?? null"
    :can-evaluate="evaluationPage?.canEvaluate ?? false"
    :is-self="isSelf"
    :is-logged-in="isLoggedIn"
    @saved="reloadEvaluations"
    @deleted="reloadEvaluations"
  />
  ```
- 右侧栏只保留 `ProfileCertificatesCard`。
- 移动端 Hero 纵向排列，词云在身份区下方、简介上方；无横向溢出。
- 删除旧 `ProfileEvaluationCard.vue`/spec。

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
corepack pnpm --filter web test -- src/views/UserProfileView.spec.ts src/components/profile/ProfileWordCloud.spec.ts src/components/profile/ProfileEvaluationDialog.spec.ts
corepack pnpm --filter web test
```
Expected: PASS；旧评价列表测试已随组件删除。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/UserProfileView.vue apps/web/src/views/UserProfileView.spec.ts \
  apps/web/src/components/profile/ProfileWordCloud.vue apps/web/src/components/profile/ProfileWordCloud.spec.ts \
  apps/web/src/components/profile/ProfileEvaluationDialog.vue apps/web/src/components/profile/ProfileEvaluationDialog.spec.ts \
  apps/web/src/utils/profileWordCloudLayout.ts apps/web/src/utils/profileWordCloudLayout.spec.ts
git rm apps/web/src/components/profile/ProfileEvaluationCard.vue apps/web/src/components/profile/ProfileEvaluationCard.spec.ts
git commit -m "feat(web): move profile word cloud into hero with evaluation dialog"
```

---

### Task 5: 文档、全量检查与真实浏览器验收

**Files:**
- Modify: `README.md`（M9 词云描述）
- Modify: `运行手册.md`（个人主页章节）
- Modify: `docs/任务清单.md`（M9 前端/已知取舍）

**Interfaces:**
- Consumes: 前 4 个任务的页面。
- Produces: 文档同步与真实浏览器验证。

- [ ] **Step 1: 更新文档**

- README M9：词云改为“Hero 不规则词云 + 点击弹评价 Dialog”，移除“右栏词云评价卡/评价列表”。
- 运行手册个人主页：更新为 Hero 词云、点击写评价、无评论列表；说明自己的评价可更新/删除。
- 任务清单 M9：把 PROFILE-F 描述改为 Hero 词云 + Dialog；更新“词云使用 flex-wrap”的已知取舍为“确定性螺旋散点、24 项上限、长文本省略”。

- [ ] **Step 2: 全量检查**

```bash
corepack pnpm gen:api
git diff --exit-code apps/web/src/api/schema.d.ts
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过；`gen:api` 无差异。

- [ ] **Step 3: 真实浏览器验收**

Docker 环境已在 `:8080`（后端）和 `:5173`（前端）运行。用 Playwright（本机 Chrome）：
- 打开 `http://127.0.0.1:5173/users/33`（kaptree）。
- 断言 `.profile-word-cloud` 可见，位于 `.profile-surface` 之前；页面不存在 `.evaluation-item`/旧评价列表。
- 点击词云面板，断言评价 Dialog 可见；游客状态显示登录提示。
- 登录 `m2badmin` / `correct-horse` 后再次点击，断言 textarea 可用；取消关闭。
- 375px 视口：Hero 纵向排列，词云不横向溢出，Dialog 可用。
- 控制台无错误。

为看词云效果，可在 Docker Postgres 中给 kaptree（id 33）补几条演示评价（仅本地数据，不提交代码）：

```sql
insert into user_evaluations (author_user_id, target_user_id, content, created_at, updated_at)
values
  (1, 33, '讲题很清楚', now(), now()),
  (2, 33, '代码风格好', now(), now()),
  (3, 33, '思路清晰', now(), now()),
  (4, 33, '大佬太强了', now(), now()),
  (5, 33, '耐心细致', now(), now()),
  (6, 33, '一秒AC', now(), now()),
  (7, 33, '算法很强', now(), now()),
  (8, 33, '人很好', now(), now())
on conflict (author_user_id, target_user_id)
do update set content = excluded.content, updated_at = now();
```

- [ ] **Step 4: 提交文档**

```bash
git add README.md 运行手册.md docs/任务清单.md
git commit -m "docs: update profile hero word cloud"
```

---

## 交付后检查

- 桌面：词云在姓名右侧、简介上方；点击面板弹 Dialog；无评论列表；右侧栏只有证书。
- 移动端 375px：Hero 纵向、词云/对话框无横向溢出。
- 游客/本人/登录用户：Dialog 状态正确；自己的评价可更新/删除。
- 评价加载失败：词云显示错误与重试，主页其他内容正常。
- 词云 0/1/24 条：空态、居中、无重叠；长文本省略。
- 全量测试、lint、build 通过。
