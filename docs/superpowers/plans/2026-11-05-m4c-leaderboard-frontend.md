# M4-C 排行榜、签到与 Rating 历史前端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 M4 前端：三个排行榜（Rating/活跃度/共享）、每日签到入口、用户主页 Rating 历史卡片，并启用导航。

**Architecture:** 新增 `api/leaderboard.ts`、`RatingLevelName.vue`、`LeaderboardView.vue`、`RatingHistoryCard.vue`；使用现有 PrimeVue 组件/AppSurface/AppPageHeader/IdentityShowcase；签到/榜单数据全部服务端获取，操作后刷新 `auth.fetchMe()` 与榜单。

**Tech Stack:** Vue 3、TypeScript、PrimeVue 4、Pinia、Vue Router、Vitest。

**Spec:** `docs/superpowers/specs/2026-11-05-m4-leaderboard-rating-learning-design.md` 4.5、4.6、6、9、10。

## Global Constraints

- 排行榜/历史公开；签到/摘要需要登录。
- 所有数据以服务端为准，不做本地乐观加减。
- 黑红名：首字符黑色、其余红色；其他等级使用 `level.color`。
- 榜单只展示公开字段（用户名/昵称/分数/等级/装备），不展示邮箱/角色。
- 签到成功后刷新 `auth.fetchMe()`、签到摘要和活跃度榜；失败显示服务端中文错误。
- 所有页面文案中文；不访问 CDN。
- 使用 PrimeVue 4，不新增 UI 框架；不破坏现有 PrimeVue 迁移页面。
- 不实现学习路线、社交、Rating 衰减/赛季。
- 不推送、不合并分支。

## Review Focus

1. **黑红名/等级颜色**：首字符黑、其余红；普通等级颜色正确；测试放 Task 1。
2. **签到状态**：重复签到按钮禁用、金币/活跃度不重复；测试放 Task 2。
3. **三榜切换/分页**：切换 Tab 不串数据、页码/总数正确；测试放 Task 2。
4. **Rating 历史空态/私密比赛**：无记录显示空态；私密比赛不产生无效链接；测试放 Task 3。
5. **移动端表格/导航**：榜单窄屏可横向滚动，M4 导航启用；测试放 Task 4。

---

### Task 1: API 客户端、类型、工具与 RatingLevelName

**Files:**
- Create: `apps/web/src/api/leaderboard.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/utils/rating.ts`
- Create: `apps/web/src/utils/rating.spec.ts`
- Create: `apps/web/src/components/RatingLevelName.vue`
- Create: `apps/web/src/components/RatingLevelName.spec.ts`

**Interfaces:**
- Consumes: `schema.d.ts` 的 `RatingLevel/RatingLeaderboardRow/RatingLeaderboardPage/RatingHistoryItem/RatingHistoryPage/ActivityLeaderboardRow/SharingLeaderboardRow/ActivitySummary/ActivitySignInResult`。
- Produces:
  - 类型别名：`RatingLevel`、`RatingLeaderboardRow`、`RatingLeaderboardPage`、`RatingHistoryItem`、`RatingHistoryPage`、`ActivityLeaderboardRow`、`SharingLeaderboardRow`、`ActivitySummary`、`ActivitySignInResult`。
  - API：
    - `listRatingLeaderboard({page,pageSize})`
    - `listActivityLeaderboard({page,pageSize})`
    - `listSharingLeaderboard({page,pageSize})`
    - `getRatingHistory(userId,{page,pageSize})`
    - `getActivitySummary()`
    - `signInActivity()`
  - `rating.ts`：
    - `ratingLevelStyle(level): { color: string }`
    - `isBlackRed(level): boolean`
    - `splitBlackRedName(name): { first, rest }`
    - `formatDelta(delta): string`（`+16`/`-5`/`0`）
  - `RatingLevelName.vue`：props `name: string`、`level: RatingLevel`；黑红名首字符 `#000000`、其余 `#FF0000`，其他用 `level.color`；带 `title=level.name` 或 `aria-label`。

- [ ] **Step 1: 写失败工具/组件测试**

```ts
// rating.spec.ts
it('detects black-red level and splits first character');
it('formats rating deltas with sign');
it('uses regular level color for non-black-red');
// RatingLevelName.spec.ts
it('renders first char black and rest red for black_red');
it('renders regular level color');
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/utils/rating.spec.ts src/components/RatingLevelName.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现 API/类型/工具/组件**

- API 全部使用 `http`，分页参数 `page/page_size`；返回 `Page<T>` 或 schema page。
- `RatingLevelName` 使用 `Array.from(name)` 处理中文/emoji；空名回退 `?`。
- 不新增手写 DTO；全部从 `schema.d.ts` 别名。
- 颜色只使用服务端 `level.color`/`accentColor`，不做任意 CSS 注入。

- [ ] **Step 4: 运行测试、lint、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/utils/rating.spec.ts src/components/RatingLevelName.spec.ts && corepack pnpm lint:web`
Expected: PASS。

```bash
git add apps/web/src/api/leaderboard.ts apps/web/src/api/types.ts apps/web/src/utils/rating.ts apps/web/src/utils/rating.spec.ts apps/web/src/components/RatingLevelName.vue apps/web/src/components/RatingLevelName.spec.ts
git commit -m "feat(web): leaderboard api and rating level"
```

---

### Task 2: 排行榜页（Rating/活跃度/共享）与签到

**Files:**
- Create: `apps/web/src/views/LeaderboardView.vue`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/layout/AppNav.vue`
- Modify: `apps/web/src/layouts/DefaultLayout.vue`
- Test: build + 浏览器验证（可加组件测试）

**Interfaces:**
- Consumes: Task 1 API、`RatingLevelName`、`IdentityShowcase`、`AppPageHeader/AppSurface/AppEmptyState/AppErrorState/AppLoadingState`。
- Produces:
  - `/leaderboard` 页面，三个 Tab：Rating（默认）、活跃度、共享。
  - 活跃度 Tab 顶部签到卡：`ActivitySummary` + `POST /activity/signin`。
  - 路由 `leaderboard`，导航移除 M4 禁用；版本标签更新为 M4-C。

- [ ] **Step 1: 实现页面骨架**

- `activeTab` ref；每个 Tab 独立 `page`/`rows`/`total`/`loading`/`error`/request sequence，切换 Tab 不串数据。
- 初次加载 Rating 榜；切 Tab 时按需加载并保留各自页码。
- 表格列：排名、用户（昵称 + `IdentityShowcase`）、分数；Rating Tab 额外等级列（`RatingLevelName`）。
- 分数格式化：Rating 原样；活跃度/共享分用 `toLocaleString('zh-CN')`。
- 分页：PrimeVue `Paginator`，pageSize 20；总数来自服务端。
- 错误/加载/空态使用共享组件；表格外层 `.table-scroll` 横向滚动。

- [ ] **Step 2: 实现签到卡**

- 登录用户加载 `getActivitySummary()`；游客显示登录 CTA。
- 签到按钮：`signedInToday` 时禁用并显示“今日已签到”；点击确认/直接请求 `signInActivity()`。
- 成功 Toast：首次 `+5 活跃度 / +5 金币`；重复返回 `awarded=false` 时提示“今日已签到”。
- 成功后 `auth.fetchMe()`、刷新 summary 和活跃度榜第一页；失败 `ApiError.message`。
- 显示今日活跃度/总活跃度。

- [ ] **Step 3: 路由/导航**

- 路由 `leaderboard` 指向 `LeaderboardView.vue`，meta title“排行榜”。
- `AppNav` 移除 leaderboard 的 `milestone: 'M4'`。
- `DefaultLayout` 侧栏版本 `M4-A` → `M4-C`。

- [ ] **Step 4: 构建、浏览器验证、提交**

Browser：游客三 Tab 加载；`m2bsolver1` 签到前后状态/余额；重复签到禁用；移动端表格横向滚动；无控制台/CDN 错误。

Run: `corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`

```bash
git add apps/web/src/views/LeaderboardView.vue apps/web/src/router/index.ts apps/web/src/components/layout/AppNav.vue apps/web/src/layouts/DefaultLayout.vue
git commit -m "feat(web): leaderboard and sign-in"
```

---

### Task 3: 用户主页 Rating 历史卡片

**Files:**
- Create: `apps/web/src/components/RatingHistoryCard.vue`
- Create: `apps/web/src/components/RatingHistoryCard.spec.ts`
- Modify: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/views/UserProfileView.spec.ts`（如存在）

**Interfaces:**
- Consumes: Task 1 `getRatingHistory`、`RatingHistoryItem`、`RatingLevelName`（可选）、`formatDateTime`。
- Produces:
  - `RatingHistoryCard.vue` props `userId: number`、可选 `rating?: number`；加载最近 20 条历史。
  - 表格列：比赛（标题/私密比赛）、名次、赛前→赛后、变化（正绿负红）、时间。
  - 简单 SVG 折线：按 `ratingAfter` 绘制；少于 2 条时只显示表格/空态；坐标使用相对 viewBox，不引入图表库。
  - 私密比赛 `contestId=0` 不生成链接，标题显示“私密比赛”。

- [ ] **Step 1: 写失败组件测试**

```ts
it('renders empty state when no history');
it('renders positive/negative delta colors');
it('does not link masked private contests');
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/components/RatingHistoryCard.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现卡片并接入用户主页**

- `UserProfileView` 在资料卡下渲染 `<RatingHistoryCard :user-id="profile.id" :rating="profile.rating" />`。
- 加载/错误/空态使用 AppLoadingState/AppErrorState/AppEmptyState；错误提供重试。
- SVG：`viewBox="0 0 320 80"`，点坐标按索引和 rating 归一化；无点/1 点不绘制线；`aria-label` 描述。
- 表格在窄屏横向滚动；private contest 不生成 RouterLink。

- [ ] **Step 4: 测试/构建/提交**

Run: `corepack pnpm --filter web exec vitest run src/components/RatingHistoryCard.spec.ts src/views/UserProfileView.spec.ts && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/components/RatingHistoryCard.vue apps/web/src/components/RatingHistoryCard.spec.ts apps/web/src/views/UserProfileView.vue apps/web/src/views/UserProfileView.spec.ts
git commit -m "feat(web): rating history card"
```

---

### Task 4: 首页文案、文档、全量检查与真实浏览器 E2E

**Files:**
- Modify: `apps/web/src/views/HomeView.vue`
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `docs/primevue-migration.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量检查 + 浏览器 E2E

**Interfaces:**
- Consumes: Task 1–3。
- Produces: M4-C 状态、首页入口、PrimeVue 迁移 UI-30 状态、文档与 E2E。

- [ ] **Step 1: 首页/版本文案**

- `HomeView` 增加排行榜入口（Rating/活跃度/共享），或把“排行榜前端正在路上”改为“排行榜已上线”，并更新系统状态版本 `M4-C`。
- `DefaultLayout` 版本已由 Task 2 更新；确认无残留 M4-A。

- [ ] **Step 2: 文档**

- `DEVELOPMENT.md` 5.5：M4-C 排行榜/签到/Rating 历史完成；学习路线、社交、衰减/赛季仍待办。
- `docs/任务清单.md`：M4-5 ✅（三榜后端 + 前端）；M4-7 标 🚧（排行榜前端完成，学习路线树待办）或拆分说明；M4-6/M4-7 学习部分 ⬜。
- `docs/开发日志.md`：追加 M4-C 条目（页面、签到、Rating 历史、导航、验证、复审修复）。
- `docs/primevue-migration.md`：UI-30 M4-C 排行榜前端标 done；UI-31 学习路线仍 todo；更新状态汇总/当前阶段。
- `README.md`、`运行手册.md`：当前版本 M4-C；补充排行榜三 Tab、签到、Rating 历史、导航；说明学习路线/社交/衰减赛季尚未实现。

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

- [ ] **Step 4: 真实浏览器 E2E**

1. 游客 `/leaderboard` 三 Tab 加载；Rating 等级颜色/黑红名正确；分页/空态正常。
2. `m2bsolver1` 活跃度 Tab 签到：首次 `+5 金币/+5 活跃度`，按钮变为已签到；重复点击不重复发币；余额/榜单刷新。
3. 共享榜显示比赛创建分；切换 Tab 不串数据。
4. 用户主页 Rating 历史：表格/曲线/空态；私密比赛不产生无效链接。
5. 导航“排行榜”已启用，侧栏版本 M4-C；移动端表格横向滚动、颜色/装备不破版。
6. 无 CDN/控制台错误（既有 favicon 404 除外）。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/HomeView.vue DEVELOPMENT.md docs/任务清单.md docs/开发日志.md docs/primevue-migration.md README.md 运行手册.md
git commit -m "docs: record m4-c leaderboard frontend"
```

---

## Self-Review

**Spec coverage:**

- 三榜 Tab、等级颜色、签到、Rating 历史 → Task 1–3。
- 导航/首页/版本 → Task 2/4。
- 文档/E2E → Task 4。
- 学习路线、社交、衰减/赛季 → 后续阶段。

**Type consistency:**

- 类型全部来自 `schema.d.ts` 别名。
- `RatingLevelName` 使用 `RatingLevel`；榜单行三套类型分开，避免共享错误字段。
- `RatingHistoryCard` 使用 `RatingHistoryItem`，私密比赛 `contestId=0` 不链接。

**Review Focus mapping:**

1. 黑红名/颜色 → Task 1 component tests。
2. 签到状态/重复 → Task 2 browser + E2E。
3. 三榜切换/分页 → Task 2 per-tab state + E2E。
4. Rating 历史空态/私密 → Task 3 tests。
5. 移动端/导航 → Task 4 E2E。

**Proportion:** 计划固定 API/类型/页面状态与测试点；具体 PrimeVue 布局和样式按现有页面模式完成。
