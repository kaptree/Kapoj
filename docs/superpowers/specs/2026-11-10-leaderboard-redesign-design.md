# 排行榜四榜总览与自动签到设计

> 目标：把 `/leaderboard` 从「三 Tab + 手动签到」改造成「四榜 Top10 同时可见 + 同页完整排名 + 计分规则弹窗」，并把每日签到改为登录/首屏自动签到、右上角 Toast 提醒。

## 1. 背景与现状

- 现有页面 `apps/web/src/views/LeaderboardView.vue` 只有 Rating、活跃度、共享三个 Tab；数据按 Tab 懒加载，每页 20 条。
- 现有后端榜单：
  - `GET /api/leaderboard/rating`：rating 包提供，Elo 结算后的 Rating。
  - `GET /api/leaderboard/activity`：`activity_logs.points` 汇总。
  - `GET /api/leaderboard/sharing`：`20 * 比赛共享数 + 5 * 题解点赞数`。
  - 三者都只包含 `users.status = 'active'` 且分数 > 0 的用户，按分数降序、用户 ID 升序；支持 `page` / `page_size`，缓存 60 秒。
- 榜单 API 层 `activityapi` 会把装备徽章/称号装饰到每一行，前端用 `IdentityShowcase` 展示。
- 金币余额已存在 `users.coins`，由签到、成就、任务、管理员调整增加，由徽章/称号购买扣减；目前没有富豪榜接口。
- 每日签到 `POST /api/activity/signin` 已幂等：本地自然日首次返回 `awarded=true`、`activityPoints=5`、`coins=5`，重复调用返回 `awarded=false`，不重复发奖励。
- 现有签到入口在排行榜活跃度 Tab 内，是手动按钮；改版后移除该按钮。

## 2. 目标与非目标

### 目标

- `/leaderboard` 首屏同时展示四个榜单的 Top10：Rating 榜、活跃榜、富豪榜、贡献榜；桌面从左到右四列。
- 每个榜单卡片可点击/键盘激活，切换到同页下方的完整排名表格并滚动到该区域。
- 新增公开富豪榜 `GET /api/leaderboard/wealth`，按当前金币余额排序。
- 贡献榜沿用现有共享分，只把 UI 名称改为「贡献榜 / 贡献分」，接口和数据不变。
- 页面右上角提供「计分规则」按钮，弹窗说明四个榜单的计分方式。
- 登录成功或已保持登录的用户当天首次打开站点时，自动调用签到；仅当日首次成功时在右上角弹出 Toast，并刷新用户金币。
- 视觉上采用四色强调、Top3 奖牌、轻量玻璃/霓虹质感，同时保持现有深色/浅色主题和响应式布局。
- 游客可以查看全部四个榜单和计分规则；签到只对登录用户触发。

### 非目标

- 不做金币历史余额趋势、累计获得金币榜、贡献分明细流水或新的计分表。
- 不做独立的 `/leaderboard/rating` 等子路由；完整榜单仍在 `/leaderboard` 同页。
- 不在用户停留页面跨午夜时自动补签到；只覆盖登录动作和站点首屏。
- 不改变 Rating、活跃度、共享分的计算规则、缓存策略或历史数据。
- 不新增签到奖励、连续签到、补签或签到日历。
- 不把贡献榜改名为后端 `sharing`，避免接口和缓存键迁移。

### 反目标

- 四个榜单互相拖垮：某个接口失败时其他三个卡片仍可用。
- 已签到用户每次刷新都弹「今日已签到」。
- 自动签到阻塞登录、首屏渲染或路由跳转。
- 富豪榜把 banned、零余额或负余额用户排进来。
- 移动端四张卡片强行四列导致横向溢出。
- 计分规则与实际代码不一致，或把「当前余额」写成「累计获得」。

## 3. 约束

- 后端 Go + Gin + GORM + PostgreSQL；前端 Vue 3 + PrimeVue 4；OpenAPI 生成前端类型。
- 榜单接口公开，游客可访问；签到接口仍需登录。
- Top10 复用现有分页参数 `page=1&page_size=10`，不新增专用 TopN 接口。
- 富豪榜使用实时查询，不接 Redis 缓存：金币会被购买、成就、任务、管理员调整频繁修改，跨模块失效成本高于查询成本。
- 富豪榜只展示 `status='active'` 且 `coins > 0` 的用户；排序 `coins DESC, id ASC`。
- 自动签到只依赖现有幂等接口，不改后端签到逻辑。
- 自动签到失败静默，不向用户弹错误；只有 `awarded=true` 弹成功 Toast。
- 卡片和规则弹窗使用中文文案；英文小标签仅作装饰，不承载关键信息。
- 交互必须支持键盘；动画遵守 `prefers-reduced-motion`。
- 不提交 `Export.zip`。

## 4. 方案选择

### 4.1 富豪榜接口位置

- **方案 A：放在 `activity` / `activityapi`（选用）**。复用 `LeaderboardRow`、分页归一化、身份装饰和现有公开路由组；金币查询只是一条 users 表 SQL。
- 方案 B：放在 `gamification` / `gamificationapi`。语义上更贴近金币，但需要复制身份装饰逻辑，或在 activityapi 之外再挂一套榜单响应。
- 方案 C：前端聚合用户列表。无法分页排序，且会暴露不必要的用户数据。

选择 A。

### 4.2 完整榜单展示

- **方案 A：同页下方四榜 Tab + 分页表格（选用）**。顶部卡片负责总览，点击后切换下方 Tab 并滚动；不离开页面，游客路径最短。
- 方案 B：每榜独立路由。分享方便，但四榜总览与完整榜割裂，导航和测试面更大。
- 方案 C：弹窗内完整榜。移动端体验差，分页与滚动嵌套。

选择 A（用户已确认）。

### 4.3 自动签到触发

- **方案 A：全局 `App.vue` 监听当前用户（选用）**。覆盖「已登录用户打开站点」和「登录/注册后进入应用」两种情况；用用户 ID + 本地日期去重，签到接口本身幂等。
- 方案 B：只在登录表单成功回调里调用。覆盖不了保持登录的用户刷新/重新打开站点。
- 方案 C：后端登录 Hook 内签到。只覆盖密码登录，不覆盖 refresh token 恢复会话；还会把签到耦合进认证事务。

选择 A。

### 4.4 富豪榜缓存

- **方案 A：实时查询，不缓存（选用）**。购买徽章/称号后立刻反映余额；实现简单，不需要在 gamification 所有金币写路径上增加版本失效。
- 方案 B：60 秒 Redis 缓存 + 金币写入 bump version。读性能更好，但要改签到、成就、任务、购买、管理员调整等多条写路径，漏一处就长期脏数据。
- 方案 C：短 TTL 无失效。仍会显示旧余额，和「当前余额」语义冲突。

选择 A；用户规模变大后再评估缓存或物化榜单。

### 4.5 视觉方向

- **方案 A：四色强调的「星榜」卡片（选用）**。每榜一种强调色，Top3 奖牌化，卡片有轻量玻璃高光和悬停霓虹；下方完整榜延续同一套强调色。
- 方案 B：四张完全一致的中性表格卡。信息清楚但缺少「榜单」仪式感，不符合本次艺术风格要求。
- 方案 C：重赛博朋克（扫描线、强霓虹、复杂动效）。容易压过用户信息，且与现有克制的控制台风格冲突。

选择 A。

## 5. 设计

### 5.1 信息架构

```text
排行榜页
├─ 页头
│  ├─ RANKS // HALL OF FAME
│  ├─ 标题：排行榜
│  ├─ 描述：四榜同辉，点击卡片查看完整排名
│  └─ 操作：计分规则
├─ 赛季提示（仅 Rating 赛季加载成功时）
├─ 四榜 Top10 卡片网格（Rating / 活跃 / 富豪 / 贡献）
└─ 完整排名
   ├─ 四榜 Tab
   ├─ 分页表格（排名 / 用户 / 分数 / Rating 等级）
   └─ 分页器
```

### 5.2 富豪榜后端

新增公开接口：

```http
GET /api/leaderboard/wealth?page=1&page_size=10
```

响应与其他榜单一致：

```json
{
  "code": 0,
  "data": {
    "total": 42,
    "items": [
      {
        "rank": 1,
        "userId": 7,
        "username": "alice",
        "nickname": "Alice",
        "score": 2300,
        "equippedBadges": [],
        "equippedTitle": null
      }
    ]
  }
}
```

- `score` 表示当前金币余额。
- 查询：

```sql
SELECT id AS user_id, username, nickname, coins AS score
FROM users
WHERE status = 'active' AND coins > 0
ORDER BY coins DESC, id ASC
LIMIT ? OFFSET ?;
```

- 总数：

```sql
SELECT COUNT(*) FROM users WHERE status = 'active' AND coins > 0;
```

- 排名：`offset + i + 1`。
- 空页返回 `items: []` 而不是 `null`。
- 复用 `normalizePage`，`page_size` 上限 100，默认 20。
- 不经过 `leaderboard()` 缓存辅助函数。
- 在 `user.User` 上增加组合索引 `idx_users_status_coins`（`status` 优先、`coins` 次之），由 AutoMigrate 创建；如果实际数据量很小，索引也不会造成可感知负担。
- `activityapi.RegisterRoutes` 增加 `r.GET("/leaderboard/wealth", h.wealthLeaderboard)`。
- `activity.Service` 增加 `WealthLeaderboard(ctx, page, size)`。
- 身份装饰复用现有 `h.leaderboard` 逻辑；为避免复制，可把 `leaderboard` 的加载函数参数化后同时服务四个榜单。

### 5.3 榜单口径

| 榜单 | API | 分数含义 | 排序 | 入选条件 |
|------|-----|----------|------|----------|
| Rating 榜 | `/leaderboard/rating` | 当前赛季 Rating | rating desc, id asc | active |
| 活跃榜 | `/leaderboard/activity` | 累计活跃度 | score desc, id asc | active 且总分 > 0 |
| 富豪榜 | `/leaderboard/wealth`（新增） | 当前金币余额 | coins desc, id asc | active 且余额 > 0 |
| 贡献榜 | `/leaderboard/sharing`（沿用） | `20 * 比赛共享数 + 5 * 题解点赞数` | score desc, id asc | active 且贡献分 > 0 |

- 「贡献榜」只改 UI 文案，API 仍叫 sharing，缓存键与历史数据不变。
- 活跃/贡献/富豪榜的总分或余额下降时排名实时下降；Rating 榜按赛季结算更新。
- 贡献分是当前值而非历史累计：取消题解点赞会扣回 5，题解被删除会移除该题解的贡献；比赛删除不会移除已产生的 +20（现有行为）。

### 5.4 页面数据流

前端定义：

```ts
type BoardKey = 'rating' | 'activity' | 'wealth' | 'contribution'
```

- `contribution` 映射到 `listSharingLeaderboard`，其余一一对应。
- 页面维护两份状态：
  - `topStates`：四榜 Top10，`pageSize=10`，进入页面即四路并行请求。
  - `tableStates`：四榜完整表格，`pageSize=20`，Tab 首次访问时懒加载。
- 每个状态独立保存 `loading/error/rows/total/page/seq`；序号用于丢弃过期响应。
- 卡片和完整表格共用同一套行类型；Rating 行额外有 `level`。
- 卡片加载失败只显示该卡片的错误和「重试」，不影响其他卡片。
- 完整表格加载失败在表格上方显示错误和重试。
- 不在客户端合并或重新排序；服务端顺序即最终顺序。

#### 5.4.1 点击卡片

```ts
function openBoard(key: BoardKey) {
  activeTab.value = key
  if (!tableStates[key].loaded) void fetchTable(key, 1)
  nextTick(() => fullRankingsRef.value?.scrollIntoView({
    behavior: prefersReducedMotion ? 'auto' : 'smooth',
    block: 'start',
  }))
}
```

- 卡片本身可点击，内部「查看全部」按钮也触发同一动作。
- 卡片使用 `role="button"` / `tabindex="0"`，Enter 和 Space 均可激活。
- 错误态的「重试」按钮用 `@click.stop`，避免同时触发打开完整榜。
- 滚动目标为完整排名区域的标题，避免被固定头部遮挡。

### 5.5 Top10 卡片设计

四张卡片从左到右：Rating、活跃、富豪、贡献。

```text
┌──────────────────────────────┐
│ [icon] Rating 榜      查看全部 → │  ← 强调色图标 + 英文小标签
│ RATING · TOP 10               │
│ ──────────────────────────── │
│ ①  Alice             2450    │  ← Top1 金色描边/皇冠
│ ②  Bob               2388    │
│ ③  Carol             2210    │
│ ④  Dave              2084    │
│ …                            │
│ ⑩  Jack              1802    │
│ 当前赛季 S3 · 下次重置 12-01   │  ← 仅 Rating 卡片
└──────────────────────────────┘
```

- 强调色：
  - Rating `#22d3ee`（青）
  - 活跃 `#34d399`（翠绿）
  - 富豪 `#fbbf24`（琥珀金）
  - 贡献 `#a78bfa`（紫）
- 卡片结构：
  - 顶部：图标圆片、榜单中文名、英文小标签、`查看全部` 箭头。
  - 行：名次、用户身份（徽章/称号）、昵称/用户名、分数。
  - 第 1–3 名使用金/银/铜名次徽章；第 1 名整行加淡强调底色和描边。
  - Rating 行使用 `RatingLevelName` 展示等级颜色；其他榜普通文字。
  - 富豪分数使用 `formatCoins`；活跃/贡献使用千分位。
- 卡片视觉：
  - 背景为 `var(--kapoj-surface)` 叠加强调色 6%–12% 的径向渐变，顶部 1px 内高光。
  - 悬停：上移 2px、边框强调色 35%、外发光；不使用大面积 `backdrop-filter`，避免低端设备卡顿。
  - 首屏四卡错峰淡入（每张延迟 40ms），`prefers-reduced-motion` 下关闭。
- 空态：卡片内显示「虚位以待」和一句引导，不整卡消失。
- 加载态：3–5 行骨架条，保持卡片高度稳定。
- 移动端卡片保留 Top10 全部条目，不折叠为 Top5；页面纵向变长是可接受的。

#### 5.5.1 响应式

- `>= 1200px`：四列。
- `768px – 1199px`：两列。
- `< 768px`：单列。
- `<= 640px`：缩小行高、隐藏英文小标签、分数右对齐，禁止横向溢出。

### 5.6 完整排名表格

- 位于卡片网格下方，外层 `AppSurface`，标题「完整排名」，副标题「四个榜单的全部上榜用户」。
- 四榜 Tab 使用 PrimeVue `Tabs`，默认 Rating；Tab 文案与卡片一致。
- 表格列：
  - 排名：Top3 使用奖牌圆片，其余数字。
  - 用户：昵称/用户名 + `IdentityShowcase`；Rating 榜用 `RatingLevelName`。
  - 分数：按榜单显示 Rating / 活跃度 / 金币 / 贡献分。
  - 等级：仅 Rating 榜显示等级徽章。
- 表格视觉：
  - 表头 sticky（在表格滚动容器内），等宽英文小标题。
  - 当前 Tab 的强调色驱动表头下划线、Top3 名次和分数颜色。
  - 行悬停使用 `--kapoj-surface-hover`，不改变文字颜色。
  - 表格外层保留横向滚动，移动端 `min-width` 约 560px。
- 分页：每页 20，底部右对齐；移动端拉通。
- 顶部卡片点击后切到对应 Tab；若该 Tab 已加载则直接滚动，不重复请求。
- 完整榜与卡片独立请求；卡片加载成功不代表完整表格已加载。

### 5.7 计分规则弹窗

- 页头操作区增加按钮：`计分规则`，图标 `pi pi-question-circle`，outlined。
- 点击打开 PrimeVue `Dialog`，标题「四榜计分规则」，宽约 760px，移动端全宽、内容可滚动。
- 内容按四色分区：

#### Rating 榜

- 初始 Rating 1500。
- 仅 Rated 比赛在结束后结算；至少 2 名参赛者。
- Elo v1：`K=32`，单场变化上限 `±100`，最低 0。
- 高分段长时间不活跃会按站点设置衰减；赛季结束按设置的百分比软重置。
- 只展示 active 用户。

#### 活跃榜

| 行为 | 活跃度 | 每日上限 |
|------|--------|----------|
| 提交一次 | +2 | 40 |
| 报名比赛 | +10 | 50 |
| 每日签到 | +5 | 每天 1 次 |
| 发表题解回复 | +3 | 20 |
| 点赞题解（点赞者获得） | +1 | 20 |
| 关注用户 | +2 | 20 |

- 同一对象只计一次；达到每日上限后不再增加。
- 题解点赞的 +1 活跃度记在点赞者；题解作者获得的是贡献榜 +5，不是活跃度。
- 榜单显示累计活跃度，不扣除。

#### 富豪榜

- 按当前金币余额排序，不是累计获得。
- 金币来源：每日签到 +5、成就奖励、任务奖励、活动/管理员调整。
- 金币支出：购买徽章、称号等会降低余额和排名。
- 只展示 active 且余额 > 0 的用户。

#### 贡献榜

- 创建或克隆比赛：每场 +20；删除比赛不会扣回（现有行为）。
- 题解被点赞：每个赞 +5，记在题解作者。
- 取消点赞：扣回对应题解的 5 分；题解被删除：移除该题解带来的贡献。
- 贡献分是当前值，可能下降，不是历史累计。
- 贡献分即原「共享分」，接口仍为 `/leaderboard/sharing`。

- 弹窗底部「知道了」关闭。
- 规则文案集中为组件内静态数组，模板按数组渲染，避免四段 JSX/HTML 重复。
- 规则数字必须与后端常量一致；后续改规则时同步更新该数组和测试。

### 5.8 自动签到

- 在 `App.vue` 增加全局自动签到，不改后端。
- 触发条件：
  - 应用启动时当前已有登录用户（持久化 token + user）。
  - 登录/注册成功后 `auth.user.id` 从无到有。
  - 同一天同一用户只尝试一次；用 `${userId}:${localDate}` 去重。
- 流程：

```text
watch(auth.user?.id, immediate)
  -> 未登录 / 无 user：跳过
  -> 已尝试过同一 userId + 本地日期：跳过
  -> POST /api/activity/signin
       awarded=true  -> Toast 成功 + auth.fetchMe()
       awarded=false -> 静默
       请求失败      -> 静默
```

- Toast：`severity=success`，`summary=签到成功`，`detail=活跃度 +5 · 金币 +5`，`life=4000`；位置沿用 App 根部的 `Toast position="top-right"`。
- 不弹「今日已签到」提示；游客不弹。
- 成功后刷新 `auth.fetchMe()` 更新金币，不用本地加减。
- 失败不阻塞登录、路由或页面加载；不重试、不提示。
- 并发保护：同一时刻只允许一个签到请求；`fetchMe` 增加请求序号，只应用最后一次响应，避免登录自带的 `fetchMe` 与签到后的 `fetchMe` 竞争导致金币显示回退。
- 页面刷新会再次触发一次幂等签到请求，但 `awarded=false` 时无 Toast；这是可接受的代价，换取跨浏览器和跨标签页的可靠首签。
- 若用户在排行榜页首屏同时触发自动签到，活动榜可能比签到早一个请求；不做强制刷新，下一次进入页面即可看到 +5。若实现成本低，可在签到成功后用事件通知排行榜刷新活跃榜，但列为可选增强，不作为验收条件。

### 5.9 移除手动签到

- 删除 `LeaderboardView.vue` 中签到卡片、`getActivitySummary`、`signInActivity`、`summary` 状态与相关样式。
- `LeaderboardView.spec.ts` 删除手动签到断言，改为自动签到在 `App.vue` 的测试。
- `api/leaderboard.ts` 保留 `getActivitySummary` / `signInActivity`，后者供 `App.vue` 使用；若无其他调用，`getActivitySummary` 可暂留，避免无意义删除。
- 活动榜 Tab 现在只展示排名，不再嵌入签到入口。

### 5.10 OpenAPI 与类型

`packages/api/openapi.yaml`：

- 新增 `GET /leaderboard/wealth`，公开，分页参数与其他榜单一致。
- 新增 schema：
  - `WealthLeaderboardRow`：`rank/userId/username/nickname/score/equippedBadges/equippedTitle`，`score` 描述为当前金币余额。
  - `WealthLeaderboardPage`。
  - `WealthLeaderboardPageResponse`。
- 前端：
  - `api/types.ts` 增加 `WealthLeaderboardRow`。
  - `api/leaderboard.ts` 增加 `listWealthLeaderboard`。
  - 重新执行 `pnpm gen:api`，不手改 `schema.d.ts`。

### 5.11 文件影响

后端：

- `apps/server/internal/user/model.go`（组合索引）
- `apps/server/internal/activity/leaderboard.go`
- `apps/server/internal/activity/leaderboard_test.go`
- `apps/server/internal/activityapi/handler.go`
- `apps/server/internal/activityapi/handler_test.go`
- `apps/server/internal/router`（若路由测试需要）
- `packages/api/openapi.yaml`

前端：

- `apps/web/src/views/LeaderboardView.vue`
- `apps/web/src/views/LeaderboardView.spec.ts`（重写）
- `apps/web/src/App.vue`
- `apps/web/src/App.spec.ts`（新增）
- `apps/web/src/api/leaderboard.ts`
- `apps/web/src/api/types.ts`
- `apps/web/src/stores/auth.ts`（`fetchMe` 序号保护）
- `apps/web/src/stores/auth.spec.ts`（若存在；补充竞争测试）
- `apps/web/src/api/schema.d.ts`（生成）

文档：

- `docs/任务清单.md`（新增 M8）
- `docs/开发日志.md`（交付后追加）

## 6. 测试与验收

### 6.1 后端

- `WealthLeaderboard`：
  - active 用户按 `coins DESC, id ASC`。
  - 同余额按 ID 升序。
  - banned、coins=0 用户被排除。
  - 分页 rank 连续；空页 `items` 为非 nil 空数组。
  - `page` 溢出被 clamp，`page_size` 上限 100。
- 路由：
  - 游客 `GET /api/leaderboard/wealth` 返回 200。
  - 响应带 `equippedBadges` / `equippedTitle`。
  - 现有三个榜单行为不变。
- 迁移：
  - `idx_users_status_coins` 存在；测试库可重复迁移。
- OpenAPI 生成通过。

### 6.2 前端榜单页

- 首屏并行请求四个 Top10，均使用 `pageSize=10`。
- 卡片顺序为 Rating、活跃、富豪、贡献；文案正确。
- Top3 有奖牌样式；Rating 卡片显示等级名和赛季信息。
- 点击富豪卡片：
  - 下方 Tab 切到富豪。
  - 首次点击请求完整榜 `page=1&page_size=20`。
  - `scrollIntoView` 被调用。
- 已加载的 Tab 再次点击不重复请求。
- 单个卡片失败只显示该卡片错误；重试只重发该卡片请求。
- 完整榜分页正确，各榜互不串数据。
- 计分规则按钮打开 Dialog，包含四个榜单标题和关键数字（+2/+10/+5/+3/+1/+2/+20/+5）；贡献榜文案明确「取消点赞会扣回」。
- 游客不显示任何签到按钮或签到摘要。
- 旧的手动签到入口完全移除。

### 6.3 前端自动签到

- 已登录用户挂载 `App.vue`：调用 `signInActivity` 一次。
- `awarded=true`：出现成功 Toast，调用 `auth.fetchMe()`。
- `awarded=false`：不出现 Toast。
- 请求失败：不出现 Toast，不抛未处理异常。
- 未登录：不调用签到。
- 同一用户同一天重复触发：只调用一次。
- 登录后从无用户变为有用户：触发签到。
- 两个并发的 `fetchMe`：较早响应不能覆盖较晚响应。

### 6.4 视觉与响应式验收

- 桌面 1440px：四列卡片同一行，卡片底部对齐。
- 1024px：两列。
- 375px：单列，无横向页面溢出；卡片和表格内容可读。
- 深色/浅色主题下强调色、文字和边框对比度足够。
- 键盘 Tab 可聚焦卡片；Enter/Space 打开；Dialog 可 Esc 关闭。
- `prefers-reduced-motion` 下无淡入/滚动平滑动画。
- Top3 奖牌、Top1 高亮、悬停发光生效，但不遮挡文字。

### 6.5 真实冒烟

- 临时数据库/端口启动后端，准备：
  - 多个 active 用户，金币分别为 300、300、120、0；
  - 一个 banned 用户金币 999；
  - 活跃度与共享分数据各若干。
- `GET /api/leaderboard/wealth` 返回 300/300/120，banned 和 0 不出现；第二页 rank 正确。
- 前端 `/leaderboard`：
  - 四张卡片均显示 Top10；
  - 点击「富豪榜」滚动到完整榜并显示正确分页；
  - 规则弹窗四段规则齐全；
  - 游客无签到入口。
- 登录测试用户：
  - 首次登录右上角 Toast「签到成功」，金币 +5；
  - 刷新页面不再弹窗；
  - 当天第二次登录不再弹窗。
- 若桌面浏览器不可用，记录为延后项，不以截图冒充已验证。

## 7. 已知取舍

- 富豪榜不缓存，读多时直接压 PostgreSQL；换来购买/奖励后立即准确。
- 自动签到按「每次应用启动/登录」尝试，刷新会多一次幂等请求；不做本地「今天已签」短路，避免多标签/多设备状态不一致。
- 自动签到失败静默，用户不会知道签到没成功；这是为了避免后台请求打扰。
- 活跃榜在首屏自动签到完成前可能少 5 分，刷新后恢复；不为此增加跨组件事件。
- 完整榜不做 URL 参数同步，刷新后回到默认 Rating Tab；保持实现简单。
- 卡片点击是页内滚动，不是新路由；浏览器「后退」不会回到之前的 Tab。
- 贡献榜只是改名，不新增贡献明细；用户无法在榜单里看到「哪场比赛 +20」。
- 贡献分随取消点赞和题解删除回落，榜单反映当前贡献而非历史累计；规则弹窗按这一真实行为说明。
- 富豪榜只展示正余额，欠费/零余额用户不占榜单位置。
- 规则弹窗文案是静态配置，规则常量变更时需要人工同步；测试只覆盖关键数字出现，不校验后端常量自动一致。
