# 排行榜四榜总览与自动签到实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `/leaderboard` 改为 Rating/活跃/富豪/贡献四榜 Top10 同屏总览，新增公开富豪榜，并提供同页完整排名、计分规则弹窗与登录/首屏自动签到。

**Architecture:** 后端在 `activity`/`activityapi` 中复用现有榜单行与身份装饰，新增实时 `GET /api/leaderboard/wealth`；前端重写 `LeaderboardView.vue`，四路 Top10 并行加载、下方四榜分页表格懒加载；`App.vue` 监听当前用户并调用现有幂等签到接口，成功后 Toast 并刷新用户金币。

**Tech Stack:** Go + Gin + GORM + PostgreSQL；Vue 3 + PrimeVue 4 + Pinia；OpenAPI 生成前端类型；Vitest。

**Spec:** `docs/superpowers/specs/2026-11-10-leaderboard-redesign-design.md`

## Global Constraints

- 四个榜单顺序固定：Rating、活跃、富豪、贡献。
- 富豪榜只含 `status='active'` 且 `coins > 0` 的用户，按 `coins DESC, id ASC`，实时查询不缓存。
- 贡献榜沿用 `/leaderboard/sharing`，UI 文案改为「贡献榜 / 贡献分」；贡献分可因取消点赞、题解删除而下降。
- 活跃榜规则：提交 +2（日上限 40）、报名比赛 +10（日上限 50）、签到 +5（每日一次）、题解回复 +3（日上限 20）、点赞题解 +1（日上限 20，记点赞者）、关注 +2（日上限 20）。
- 自动签到只依赖现有幂等 `POST /api/activity/signin`；仅 `awarded=true` 弹右上角成功 Toast；已签到、失败均静默。
- 自动签到按 `${userId}:${本地日期}` 去重；同一用户同一天只尝试一次。
- Top10 使用 `page_size=10`；完整榜每页 20，使用 `page_size=20`。
- 卡片、Tab、表格共享同一套四色强调色；必须支持键盘与 `prefers-reduced-motion`。
- 不修改 Rating、活跃度、共享分算法；不新增签到奖励或贡献明细。
- 后端测试使用 `go test -p 1`；前端提交前跑 focused spec、全量测试、lint、build。
- 不提交 `Export.zip` 或任何 `.superpowers/` 临时文件。

## Review Focus

- 单个榜单接口失败不能拖垮其他卡片；重试只重发该卡片（Task 3）。
- 点击卡片必须切换完整榜 Tab、首次加载第 1 页并滚动，已加载的榜不重复请求（Task 3）。
- 计分规则必须与真实代码一致：题解点赞 +1 归点赞者，贡献分取消点赞会回落（Task 4）。
- 自动签到只对登录用户触发一次/天，仅首次成功 Toast，且并发 `fetchMe` 不能把签到后的金币覆盖回旧值（Task 5）。
- 富豪榜必须排除 banned 和零余额用户，同余额按 ID 升序，分页 rank 连续（Task 1）。

---

### Task 1: 后端富豪榜接口、索引与路由

**Files:**
- Modify: `apps/server/internal/user/model.go`
- Modify: `apps/server/internal/activity/leaderboard.go`
- Modify: `apps/server/internal/activity/leaderboard_test.go`
- Modify: `apps/server/internal/activityapi/handler.go`
- Modify: `apps/server/internal/activityapi/handler_test.go`

**Interfaces:**
- Consumes: 现有 `LeaderboardRow`、`normalizePage`、`activityapi.handler.leaderboard`。
- Produces:
  - `func (s *Service) WealthLeaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error)`
  - 公开路由 `GET /api/leaderboard/wealth`
  - `User.Coins` / `User.Status` 上的组合索引 `idx_users_status_coins`
  - `LeaderboardRow.Score` 在富豪榜中表示当前金币余额

- [ ] **Step 1: Write failing tests**

在 `apps/server/internal/activity/leaderboard_test.go` 添加辅助函数：

```go
func setLeaderboardCoins(t *testing.T, e *activityEnv, userID, coins int64) {
	t.Helper()
	require.NoError(t, e.db.Exec(`UPDATE users SET coins = ? WHERE id = ?`, coins, userID).Error)
}
```

添加 `TestWealthLeaderboardOrderAndPagination`：

- 用户 2、3、4、5、6：2/3 金币 300，4 金币 120，5 金币 0，6 为 banned 金币 999。
- `WealthLeaderboard(ctx, 1, 2)`：`Total=3`，`Items` 为 2、3，rank 1/2，score 300。
- `WealthLeaderboard(ctx, 2, 2)`：rank 3，用户 4，score 120。
- `WealthLeaderboard(ctx, 3, 2)`：`Total=3`，`Items` 非 nil 且为空。
- 断言 5（零余额）与 6（banned）不出现。

把 `TestLeaderboardHugePageClamps` 扩展一行：先 `setLeaderboardCoins(t, e, testUserID, 10)`，再调用 `WealthLeaderboard(ctx, math.MaxInt, 100)`，断言 `Total=1`、`Items` 非 nil 且为空。

在 `apps/server/internal/activityapi/handler_test.go` 添加 `TestWealthLeaderboardPublicAndEquippedIdentity`：

- 创建 decorated(active)、plain(active)、banned、zero(active) 四个用户，金币分别为 500、120、999、0。
- 给 decorated 装备一个徽章和一个称号（复用现有测试的 SQL）。
- 游客 `GET /api/leaderboard/wealth` → 200；`total=2`；第一行 decorated/500，第二行 plain/120；banned 与 zero 不出现。
- 第一行 `equippedBadges` 长度 1、`equippedTitle` 非 nil；行内没有 `level`。
- `GET /api/leaderboard/wealth?page=2&page_size=1` → plain rank 2。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/activity/ ./internal/activityapi/
```

Expected: FAIL，`WealthLeaderboard` 未定义，路由 404。

- [ ] **Step 3: Implement**

`user/model.go`：

```go
Coins  int64  `gorm:"not null;default:0;check:chk_users_coins_non_negative,coins >= 0;index:idx_users_status_coins,priority:2"`
Status Status `gorm:"size:20;not null;default:active;index:idx_users_status_coins,priority:1"`
```

`activity/leaderboard.go`：

```go
func (s *Service) WealthLeaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error)
```

- `normalizePage(page, size)`。
- `COUNT(*) FROM users WHERE status = ? AND coins > 0`，参数 `"active"`。
- 列表 SQL：

```sql
SELECT id AS user_id, username, nickname, coins AS score
FROM users
WHERE status = ? AND coins > 0
ORDER BY coins DESC, id ASC
LIMIT ? OFFSET ?
```

- 空页返回 `items: []`。
- 不调用 `s.leaderboard(...)`，不写 Redis。

`activityapi/handler.go`：

```go
r.GET("/leaderboard/wealth", h.wealthLeaderboard)
```

```go
func (h *handler) wealthLeaderboard(c *gin.Context) {
	h.leaderboard(c, h.svc.WealthLeaderboard)
}
```

- 复用现有身份装饰，不新增响应结构。

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/activity/ ./internal/activityapi/ ./internal/db/
```

Expected: PASS；重复迁移不报索引错误。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/user/model.go apps/server/internal/activity/leaderboard.go apps/server/internal/activity/leaderboard_test.go apps/server/internal/activityapi/handler.go apps/server/internal/activityapi/handler_test.go
git commit -m "feat(activity): add wealth leaderboard"
```

---

### Task 2: OpenAPI 富豪榜契约、生成类型与前端客户端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/leaderboard.ts`
- Test: `apps/web/src/api/leaderboard.spec.ts`（新建）

**Interfaces:**
- Consumes: Task 1 的 `GET /api/leaderboard/wealth` 响应形状。
- Produces:
  - `WealthLeaderboardRow`（`rank/userId/username/nickname/score/equippedBadges/equippedTitle`）
  - `listWealthLeaderboard(q: PageQuery): Promise<Page<WealthLeaderboardRow>>`

- [ ] **Step 1: Write failing client test**

新建 `apps/web/src/api/leaderboard.spec.ts`：

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('./client', () => ({ http }))

import { listWealthLeaderboard } from './leaderboard'

describe('leaderboard api', () => {
  beforeEach(() => {
    http.get.mockReset()
    http.post.mockReset()
  })

  it('requests the wealth leaderboard with page params', async () => {
    http.get.mockResolvedValue({ total: 0, items: [] })
    await expect(listWealthLeaderboard({ page: 2, pageSize: 10 })).resolves.toEqual({
      total: 0,
      items: [],
    })
    expect(http.get).toHaveBeenCalledWith('/leaderboard/wealth', {
      params: { page: 2, page_size: 10 },
    })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/leaderboard.spec.ts
```

Expected: FAIL，`listWealthLeaderboard` 未导出。

- [ ] **Step 3: Implement contract and client**

`packages/api/openapi.yaml`：

- 在 `/leaderboard/sharing` 后新增 `/leaderboard/wealth`：
  - summary「富豪排行榜（公开）」。
  - description「只包含状态为 active 且金币余额 > 0 的用户，按当前金币余额倒序、用户 ID 升序；榜单行附带已装备徽章/称号。游客可访问。」
  - `security: [{}, { bearer: [] }]`
  - `page` / `page_size` 参数与其他榜单一致。
  - 200 → `WealthLeaderboardPageResponse`。
- 在 `SharingLeaderboardRow` 附近新增 `WealthLeaderboardRow`、`WealthLeaderboardPage`、`WealthLeaderboardPageResponse`；`score` 描述为「当前金币余额」。

`apps/web/src/api/types.ts`：

```ts
export type WealthLeaderboardRow = Schemas['WealthLeaderboardRow']
```

`apps/web/src/api/leaderboard.ts`：

```ts
export const listWealthLeaderboard = (q: PageQuery) =>
  http.get<Page<WealthLeaderboardRow>>('/leaderboard/wealth', { params: pageParams(q) })
```

重新生成：

```bash
corepack pnpm gen:api
```

- [ ] **Step 4: Run test and typecheck**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/leaderboard.spec.ts
corepack pnpm --filter web exec vue-tsc -b --force
```

Expected: test PASS；类型检查通过（或仅剩 Task 3 尚未使用的既有错误，本任务不应新增）。

- [ ] **Step 5: Commit**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/leaderboard.ts apps/web/src/api/leaderboard.spec.ts
git commit -m "feat(api): add wealth leaderboard contract"
```

---

### Task 3: 四榜 Top10 卡片、同页完整排名与移除手动签到

**Files:**
- Modify: `apps/web/src/views/LeaderboardView.vue`
- Rewrite: `apps/web/src/views/LeaderboardView.spec.ts`

**Interfaces:**
- Consumes: `listRatingLeaderboard`、`listActivityLeaderboard`、`listWealthLeaderboard`、`listSharingLeaderboard`、`getRatingSeason`；`RatingLevelName`、`IdentityShowcase`、`AppSurface`、`AppPageHeader`、`AppLoadingState`、`AppErrorState`、`AppEmptyState`。
- Produces:
  - `data-board="rating|activity|wealth|contribution"` 的卡片
  - `data-test="full-rankings"` 完整榜区域
  - 卡片点击切 Tab + 首次加载 + 滚动
  - 完整榜四个 Tab、分页、独立错误/重试
  - 页面不再包含手动签到 UI

- [ ] **Step 1: Rewrite tests with failing expectations**

`LeaderboardView.spec.ts`：

- Mock：

```ts
vi.mock('@/api/leaderboard', () => ({
  listRatingLeaderboard: vi.fn(),
  listActivityLeaderboard: vi.fn(),
  listWealthLeaderboard: vi.fn(),
  listSharingLeaderboard: vi.fn(),
  getRatingSeason: vi.fn(),
}))
```

- 不再 mock `@/stores/auth`、`primevue/usetoast`；不 import `signInActivity` / `getActivitySummary`。
- 固定数据：Rating 行含 level；Activity/Wealth/Sharing 行。

测试用例：

1. `loads four top10 cards and the rating full table`
   - `listRatingLeaderboard` 调用两次：`{page:1,pageSize:10}` 与 `{page:1,pageSize:20}`。
   - 其余三榜各一次 `{page:1,pageSize:10}`。
   - `wrapper.findAll('[data-board]')` 顺序为 rating/activity/wealth/contribution。
   - 文本含四个中文榜名；Rating 卡片含 `RatingLevelName`；富豪分数显示千分位。
2. `switches to a board full table and scrolls when its card is clicked`
   - 在 `Element.prototype.scrollIntoView = vi.fn()`（jsdom 无实现）。
   - 点击 `[data-board="wealth"]`。
   - `listWealthLeaderboard` 最后一次调用 `{page:1,pageSize:20}`。
   - 完整榜区域文本包含 Wealth 行；`scrollIntoView` 被调用。
3. `does not refetch an already loaded table`
   - 连续点击贡献卡片两次；`listSharingLeaderboard` 调用参数为 10、20 各一次。
4. `keeps cards independent when one board fails`
   - `listWealthLeaderboard` 首次 reject `ApiError(500, 50000, '财富榜挂了')`，其余成功。
   - 富豪卡片显示「财富榜挂了」与重试按钮；其他三卡仍有数据。
   - 点击重试后 mock resolve；富豪卡片显示行。
5. `paginates the complete table per board`
   - `listRatingLeaderboard` 的完整榜 `total=45`；触发 Paginator page 1 → 最后一次调用 `{page:2,pageSize:20}`。
   - 切到活跃 Tab 首次调用 `{page:1,pageSize:20}`。
6. `shows season and ignores season failure`
   - 成功时显示赛季名与下次重置日期；失败时不显示错误。
7. `does not render a manual sign-in entry`
   - 文本不含 `签到 +5 金币`、`今日已签到`、`登录后签到领金币`。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/LeaderboardView.spec.ts
```

Expected: FAIL，旧页面仍是三 Tab + 签到卡。

- [ ] **Step 3: Implement the page**

`LeaderboardView.vue` 脚本：

```ts
type BoardKey = 'rating' | 'activity' | 'wealth' | 'contribution'

interface BoardDef {
  key: BoardKey
  label: string
  english: string
  icon: string
  accent: string
  unit: string
  emptyTitle: string
  emptyDescription: string
}
```

- `boardDefs` 顺序固定；强调色：rating `#22d3ee`、activity `#34d399`、wealth `#fbbf24`、contribution `#a78bfa`。
- `topStates` / `tableStates` 各自含 `rows/total/loading/loaded/error/page/seq`。
- `fetchTop(key)` 调对应 API `{page:1,pageSize:10}`。
- `fetchTable(key,page)` 调对应 API `{page,pageSize:20}`；contribution 映射 `listSharingLeaderboard`。
- `openBoard(key)`：设置 `activeTab`，未加载则 `fetchTable(key,1)`，`nextTick` 后对完整榜区域 `scrollIntoView`；用 `matchMedia('(prefers-reduced-motion: reduce)')` 选择 `auto`/`smooth`。
- 页面挂载：四路 `fetchTop` + `fetchTable('rating',1)` + `loadSeason()`。
- 保留 `formatScore`：Rating 原样，其余 `toLocaleString('zh-CN')`；富豪可用 `formatCoins`。
- 删除签到摘要、`signInActivity`、`getActivitySummary`、`useToast`、auth 依赖。

模板：

- 页头：`AppPageHeader`，标题「排行榜」，eyebrow「RANKS // HALL OF FAME」，描述四榜总览；actions 留空（Task 4 加规则按钮）。
- 赛季提示保留。
- `section.boards-grid`：四张 `article.board-card`：
  - `:data-board="board.key"`、`role="button"`、`tabindex="0"`。
  - `@click="openBoard(board.key)"`。
  - `@keydown.enter.prevent` / `@keydown.space.prevent` 同样打开。
  - 顶部：图标、中文名、英文小标签、「查看全部 →」。
  - 行：名次、昵称/用户名、`IdentityShowcase`、分数；Rating 行用 `RatingLevelName`。
  - Top3 加 `is-podium` / `is-first` 类。
  - Loading 用 5 行骨架；错误显示错误文案 + `@click.stop` 重试；空态显示 `emptyTitle`。
  - Rating 卡片底部显示赛季名/重置日期（有则显示）。
- `AppSurface data-test="full-rankings" ref="fullRankingsRef"`：
  - 标题「完整排名」；四榜 `Tabs`。
  - 每个 TabPanel 包含现有表格结构：排名/用户/分数/等级（仅 Rating）。
  - 错误 + 重试；空态；分页。
  - `scroll-margin-top: calc(var(--kapoj-header-height) + var(--kapoj-space-4))`。
- 删除签到卡片模板和样式。

样式：

- `.boards-grid`：默认 4 列，`gap: var(--kapoj-space-4)`；`@media (max-width: 1199px)` 2 列；`@media (max-width: 767px)` 1 列。
- 卡片：`--board-accent` 变量、强调色径向渐变、1px 边框、顶部内高光；hover `translateY(-2px)` + 强调色阴影；`@media (prefers-reduced-motion: reduce)` 关闭动画。
- Top3 名次：金/银/铜 CSS 渐变圆片；第一行淡强调底色。
- 移动端隐藏英文小标签，分数右对齐。
- 表格 Tab 激活色与当前榜强调色一致（通过 `--board-accent` 绑定到完整榜容器）。

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/LeaderboardView.spec.ts
corepack pnpm --filter web test
```

Expected: focused PASS；全量 PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/LeaderboardView.vue apps/web/src/views/LeaderboardView.spec.ts
git commit -m "feat(web): redesign leaderboard with four top10 boards"
```

---

### Task 4: 四榜计分规则弹窗

**Files:**
- Modify: `apps/web/src/views/LeaderboardView.vue`
- Modify: `apps/web/src/views/LeaderboardView.spec.ts`

**Interfaces:**
- Consumes: Task 3 的页头 actions 位置与四榜定义。
- Produces: `data-test="rules-button"` 的按钮、`data-test="rules-dialog"` 的 `Dialog`。

- [ ] **Step 1: Write failing tests**

在 `LeaderboardView.spec.ts` 添加：

```ts
it('opens the four-board scoring rules dialog', async () => {
  const wrapper = await mountPage()
  await wrapper.find('[data-test="rules-button"]').trigger('click')
  await flushPromises()

  const text = document.body.textContent ?? ''
  expect(text).toContain('四榜计分规则')
  expect(text).toContain('Rating 榜')
  expect(text).toContain('活跃榜')
  expect(text).toContain('富豪榜')
  expect(text).toContain('贡献榜')
  expect(text).toContain('+2')
  expect(text).toContain('+10')
  expect(text).toContain('+3')
  expect(text).toContain('+1')
  expect(text).toContain('+20')
  expect(text).toContain('取消点赞')
  expect(text).toContain('点赞者')
})
```

在 `beforeEach` / `afterEach` 清理 `document.body.innerHTML = ''`。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/LeaderboardView.spec.ts
```

Expected: FAIL，规则按钮不存在。

- [ ] **Step 3: Implement dialog**

- 页头 actions 增加：

```vue
<Button
  data-test="rules-button"
  label="计分规则"
  icon="pi pi-question-circle"
  outlined
  @click="rulesVisible = true"
/>
```

- `rulesVisible = ref(false)`。
- 静态规则数组按四榜顺序；每项含 `key/label/accent/icon/items`。
- `Dialog v-model:visible="rulesVisible" modal header="四榜计分规则" data-test="rules-dialog"`：
  - Rating：初始 1500；Rated 比赛 Elo v1，K=32，单场 ±100；赛季衰减/软重置。
  - 活跃：六条规则，明确「点赞题解 +1 记点赞者」「题解回复 +3」。
  - 富豪：当前金币余额；签到/成就/任务/活动获得；购买徽章/称号扣除；只展示 active 且 >0。
  - 贡献：创建/克隆比赛 +20；题解被点赞 +5；取消点赞扣回、题解删除移除；比赛删除不扣回；当前值非累计。
- 四段用强调色左边框/图标区分；移动端宽度 100%。
- 底部「知道了」关闭。

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/LeaderboardView.spec.ts
corepack pnpm --filter web test
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/LeaderboardView.vue apps/web/src/views/LeaderboardView.spec.ts
git commit -m "feat(web): add leaderboard scoring rules dialog"
```

---

### Task 5: 全局自动签到与 fetchMe 竞争保护

**Files:**
- Modify: `apps/web/src/App.vue`
- Create: `apps/web/src/App.spec.ts`
- Modify: `apps/web/src/stores/auth.ts`
- Modify: `apps/web/src/stores/auth.spec.ts`

**Interfaces:**
- Consumes: `signInActivity()`、`useAuthStore()`、PrimeVue `useToast()`。
- Produces:
  - `App.vue` 在登录用户出现时自动签到
  - `auth.fetchMe()` 只应用最后一次响应
  - Toast：`severity=success`、`summary=签到成功`、`detail=活跃度 +5 · 金币 +5`

- [ ] **Step 1: Write failing tests**

`apps/web/src/App.spec.ts`：

- 使用真实 Pinia auth store；mock `@/api/auth`、`@/api/users`、`@/api/site`、`@/api/leaderboard`、`primevue/usetoast`。
- Mount：

```ts
const wrapper = shallowMount(App, {
  global: {
    plugins: [createPinia(), router],
    stubs: { RouterView: true },
  },
})
```

- `site.ensureLoaded` 通过 mock `getSite` 返回一个最小站点；或在 mount 前 mock `useSiteStore`。

测试：

1. `signs in on session start and shows a success toast`
   - 预置 auth session（user id 1），`signInActivity` resolve `{awarded:true, activityPoints:5, coins:5, date:'2026-11-10'}`。
   - mount + flush；`signInActivity` 调用一次；`toast.add` 收到 success + detail；`auth.fetchMe` 被调用。
2. `does not toast when already signed in`
   - resolve `awarded:false`；`toast.add` 不调用。
3. `does not sign in for guests`
   - 无 session；`signInActivity` 不调用。
4. `silently ignores sign-in failure`
   - reject `ApiError(500, ...)`；无 toast，无未处理异常。
5. `signs in after a login transition`
   - 初始无 session，mount；`store.setSession(...)`；`await nextTick()`；`signInActivity` 调用一次。
6. `tries once per user per local day`
   - 用户 1 mount → 1 次；切到用户 2 → 2 次；切回用户 1 → 仍 2 次。

`auth.spec.ts` 添加：

```ts
it('only applies the latest fetchMe response', async () => {
  const store = useAuthStore()
  let resolveFirst!: (value: Me) => void
  let resolveSecond!: (value: Me) => void
  usersApi.getMe
    .mockImplementationOnce(() => new Promise((resolve) => (resolveFirst = resolve)))
    .mockImplementationOnce(() => new Promise((resolve) => (resolveSecond = resolve)))

  const first = store.fetchMe()
  const second = store.fetchMe()
  resolveSecond({ ...me, nickname: 'Second' })
  await second
  resolveFirst({ ...me, nickname: 'First' })
  await first

  expect(store.user?.nickname).toBe('Second')
})
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/App.spec.ts src/stores/auth.spec.ts
```

Expected: FAIL，App 未自动签到；`fetchMe` 后到的旧响应会覆盖新响应。

- [ ] **Step 3: Implement**

`stores/auth.ts`：

```ts
let fetchMeSeq = 0

async function fetchMe() {
  const seq = ++fetchMeSeq
  const next = await usersApi.getMe()
  if (seq !== fetchMeSeq) return
  setUser(next)
}
```

`App.vue`：

```ts
import { onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'

import { signInActivity } from '@/api/leaderboard'

const toast = useToast()
let signInInFlight = false
let lastSignInKey = ''

function localDateKey(): string {
  const now = new Date()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${now.getFullYear()}-${month}-${day}`
}

async function autoSignIn() {
  const userID = auth.user?.id
  if (!auth.isLoggedIn || !userID || signInInFlight) return
  const key = `${userID}:${localDateKey()}`
  if (key === lastSignInKey) return
  lastSignInKey = key
  signInInFlight = true
  try {
    const result = await signInActivity()
    if (result.awarded) {
      toast.add({
        severity: 'success',
        summary: '签到成功',
        detail: `活跃度 +${result.activityPoints} · 金币 +${result.coins}`,
        life: 4000,
      })
      await auth.fetchMe().catch(() => undefined)
    }
  } catch {
    // 后台签到失败不打扰用户；下一次登录/启动会再次尝试。
  } finally {
    signInInFlight = false
  }
}

watch(() => auth.user?.id, () => void autoSignIn(), { immediate: true })
```

- 保留现有 `site.ensureLoaded()` 与启动时 `auth.fetchMe()`。
- 不要新增「今日已签到」Toast。
- 不修改 `Toast` 位置。

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/App.spec.ts src/stores/auth.spec.ts
corepack pnpm --filter web test
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/App.vue apps/web/src/App.spec.ts apps/web/src/stores/auth.ts apps/web/src/stores/auth.spec.ts
git commit -m "feat(web): auto sign in on login and session start"
```

---

### Task 6: 全量验证、真实冒烟与任务清单同步

**Files:**
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`（当前进度一行）

- [ ] **Step 1: Backend checks**

```bash
cd /Users/admin/codespace/kapoj/apps/server
test -z "$(gofmt -l .)"
go vet ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./...
```

Expected: 全部通过。

- [ ] **Step 2: Frontend checks**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm gen:api
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过。

- [ ] **Step 3: Real API smoke**

使用隔离临时库/端口（不要把主服务当成测试库）：

1. 创建临时库 `kapoj_lb_smoke`，临时 Redis 容器端口 `6381`，临时 server `:18082`，临时 storage 目录。
2. 用 SQL 创建 4 个 active 用户（金币 300、300、120、0）和 1 个 banned 用户（金币 999）。
3. `GET /api/leaderboard/wealth` → 只返回前三个 active 正余额；顺序/并列/rank 正确。
4. `GET /api/leaderboard/wealth?page=2&page_size=1` → rank 2。
5. 用临时普通用户登录，调用 `POST /api/activity/signin`：首次 `awarded=true, coins=5`；再次 `awarded=false, coins=0`；`GET /api/users/me` 金币 +5。
6. 清理临时 server、Redis、数据库、storage；不影响主服务。

- [ ] **Step 4: Browser smoke（有桌面浏览器时）**

- `/leaderboard`：四卡同屏、Top10 行、四色强调、Top3 奖牌；点击富豪卡滚动到完整榜；规则弹窗四段齐全。
- 游客无签到入口；登录用户首次登录 Toast；刷新不弹。
- 1440/1024/375px 无横向溢出；深色/浅色主题可读。
- 若无浏览器，记录「浏览器 E2E 延后」，不要声称已验证。

- [ ] **Step 5: Update docs**

- `docs/任务清单.md`：M8 标题改为 ✅，LB-A…LB-F 全部 ✅，写交付摘要、验证结果、已知取舍。
- `docs/开发日志.md`：追加 `2026-11-10：M8 排行榜四榜总览与自动签到`，记录变更、文件、规则口径、验证与待办。
- `README.md`：当前进度加入 M8 一行。

- [ ] **Step 6: Commit**

```bash
git add docs/任务清单.md docs/开发日志.md README.md
git commit -m "docs: record leaderboard redesign delivery"
```

---

## Self-Review

- **Spec coverage:** 富豪榜接口/索引（Task 1）、契约（Task 2）、四榜 Top10 + 完整榜 + 移除手动签到（Task 3）、规则弹窗（Task 4）、自动签到 + fetchMe 竞争（Task 5）、全量验证/文档（Task 6）。
- **Type consistency:** `WealthLeaderboardRow`、`listWealthLeaderboard`、`BoardKey`、`topStates`/`tableStates`、`data-board`、`data-test="full-rankings"` 在任务间一致。
- **Review Focus mapping:** 接口隔离/重试（Task 3）、卡片点击与懒加载（Task 3）、规则真实口径（Task 4）、自动签到去重/Toast/竞争（Task 5）、富豪榜过滤与分页（Task 1）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出签名、测试断言和关键值，不逐行抄写实现。
- **Known spec edge:** 贡献榜规则明确「取消点赞会扣回」，避免把共享分写成只增不减。
