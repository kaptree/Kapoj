# M3-C 游戏化前端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 M3 的游戏化前端：成就墙、任务大厅、徽章商城、称号商店、装扮管理、游戏化管理后台，以及个人主页/测评记录/比赛榜单的装备展示。

**Architecture:** 复用 M3-B OpenAPI 生成的类型，新增 `api/gamification.ts`、`utils/gamification.ts` 和 `IdentityShowcase.vue`；用户页面用卡片/网格，管理后台用四个独立面板；购买/装备后刷新 `auth.fetchMe()`，公开身份只在展示层渲染。

**Tech Stack:** Vue 3、TypeScript、Element Plus、Pinia、Vue Router、Vitest、Vite。

**Spec:** `docs/superpowers/specs/2026-11-04-m3-gamification-design.md` 6、8、11。

## Global Constraints

- 所有页面文案、提示、文档使用中文；不访问 CDN。
- 公开页面：成就、任务、徽章商城、称号商店；登录后才能领取/购买/装备。
- 管理入口仅 `super_admin`；普通管理员不显示。
- 金币、装备、购买状态以服务端返回为准；不做本地乐观加减。
- 购买/装备/领取成功后刷新 `auth.fetchMe()` 或对应列表；失败显示服务端中文错误。
- 已拥有但下线的徽章仍可装备；已解锁但下线的称号仍可购买/装备。
- 管理端编辑徽章时，未修改库存必须省略 `stock`，避免覆盖并发库存。
- 不实现 M4 排行榜/Rating 页面。
- 不推送、不合并分支。

## Review Focus

1. **收藏/购买/领取后的余额与状态一致性**：不能显示旧余额或旧装备；测试放 Task 3/4。
2. **管理端冻结字段**：任务已有领取记录后，题目与奖励必须禁用；测试放 Task 5。
3. **公开身份在窄屏/空装备下不破版**：`IdentityShowcase` 空数组/null 不渲染多余元素；测试放 Task 1/6。
4. **权限入口**：普通用户看不到管理路由/菜单，直接访问被守卫拦截；测试放 Task 6。
5. **错误处理**：购买余额不足、库存售罄、未解锁、任务已领取等中文提示可见；测试放 Task 3/4。

---

### Task 1: API 客户端、类型、工具函数与 IdentityShowcase

**Files:**
- Create: `apps/web/src/api/gamification.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/utils/gamification.ts`
- Create: `apps/web/src/utils/gamification.spec.ts`
- Create: `apps/web/src/components/IdentityShowcase.vue`
- Create: `apps/web/src/components/IdentityShowcase.spec.ts`
- Modify: `apps/web/src/utils/format.ts`（仅当需要金币格式化）

**Interfaces:**
- Consumes: M3-B 生成的 `schema.d.ts`。
- Produces:
  - `AchievementItem/AchievementInput/AchievementConditionType/AchievementConditionConfig/AchievementConditionProgress`
  - `TaskItem/TaskInput/TaskStatus`
  - `BadgeItem/BadgeInput`
  - `TitleItem/TitleInput/TitleUnlockType/TitleUnlockConfig/TitleProgress`
  - `CoinTransactionItem/CoinTransactionPage`
  - `EquippedBadge/EquippedTitle`
  - API 函数：
    - `listAchievements(opts?: { includeInactive?: boolean }): Promise<AchievementItem[]>`
    - `createAchievement/updateAchievement/deleteAchievement`
    - `listTasks(opts?: { includeInactive?: boolean }): Promise<TaskItem[]>`
    - `claimTask(id)`, `createTask/updateTask/deleteTask`
    - `listBadges(opts?)`, `createBadge/updateBadge/deleteBadge`, `uploadBadgeImage(id, file)`, `purchaseBadge/equipBadge/unequipBadge`, `listUserBadges(userId)`
    - `listTitles(opts?)`, `createTitle/updateTitle/deleteTitle`, `purchaseTitle/equipTitle/unequipTitle`
    - `listCoinTransactions({page,pageSize})`, `adminAdjustCoins(userId,{amount,note})`
  - `IdentityShowcase.vue`：props `badges: EquippedBadge[]`、`title: EquippedTitle | null`、`size?: 'small'|'default'`；徽章图片 + tooltip，称号显示配置颜色；空状态渲染无内容。
  - 工具：
    - `achievementConditionLabel(type)`
    - `taskStatusLabel/statusTagType`
    - `titleUnlockLabel`
    - `coinReasonLabel(reason)`
    - `safeTitleColor(color)`（仅接受 `#RRGGBB`，否则 fallback）
    - `badgeInitial(name)` / `badgeImageSrc(url)`（空 URL 返回 undefined）
    - `formatCoins(n)`

- [ ] **Step 1: 写失败工具/组件测试**

```ts
// gamification.spec.ts
it('maps task statuses and tag types');
it('maps achievement and title condition labels');
it('rejects unsafe title colors');
it('formats coin reasons');
// IdentityShowcase.spec.ts
it('renders equipped badges and title');
it('renders nothing for empty badges and null title');
it('uses fallback color for invalid title color');
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/utils/gamification.spec.ts src/components/IdentityShowcase.spec.ts`
Expected: FAIL（模块/组件不存在）。

- [ ] **Step 3: 实现类型、API 客户端与工具**

- `api/gamification.ts` 全部使用现有 `http`，查询参数 `includeInactive: 'true'`；上传图片用 `http.postForm`，`FormData` 字段名 `file`。
- 删除/购买/装备返回 `null` 时不要尝试读 `data`。
- 工具函数独立于组件，便于测试；`safeTitleColor` 使用 `/^#[0-9A-Fa-f]{6}$/`。

- [ ] **Step 4: 实现 IdentityShowcase**

```vue
<template>
  <span v-if="title" class="title" :style="{ color: safeTitleColor(title.color) }">{{ title.name }}</span>
  <span v-for="badge in badges" :key="badge.id" class="badge">
    <el-tooltip :content="badge.name">
      <el-avatar :size="size === 'small' ? 18 : 22" :src="badgeImageSrc(badge.imageUrl)">
        {{ badgeInitial(badge.name) }}
      </el-avatar>
    </el-tooltip>
  </span>
</template>
```

- 空数组/null 时不输出空 `<span>`；窄屏可换行。
- 徽章图片加载失败时显示首字（`el-avatar` 默认行为即可）。

- [ ] **Step 5: 运行测试、lint、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/utils/gamification.spec.ts src/components/IdentityShowcase.spec.ts && corepack pnpm lint:web`
Expected: PASS。

```bash
git add apps/web/src/api/gamification.ts apps/web/src/api/types.ts apps/web/src/utils/gamification.ts apps/web/src/utils/gamification.spec.ts apps/web/src/components/IdentityShowcase.vue apps/web/src/components/IdentityShowcase.spec.ts
git commit -m "feat(web): gamification api and identity showcase"
```

---

### Task 2: 成就墙与任务大厅

**Files:**
- Create: `apps/web/src/views/AchievementsView.vue`
- Create: `apps/web/src/views/TasksView.vue`
- Modify: `apps/web/src/router/index.ts`
- Test: 通过 build + 浏览器验证（本任务不强制组件测试）

**Interfaces:**
- Consumes: Task 1 的 `listAchievements/claimTask/listTasks`、`IdentityShowcase` 不需要。
- Produces:
  - `/achievements`：卡片网格、状态筛选（全部/已解锁/未解锁）、条件进度条、奖励金币/徽章提示、未登录 CTA 登录。
  - `/tasks`：任务卡片，显示标题、描述摘要、奖励、时间、题目数、状态、进度、领取按钮；领取成功刷新列表。
  - 路由 `achievements`、`tasks`。

- [ ] **Step 1: 实现成就墙**

- `onMounted` 调 `listAchievements()`；未登录时接口仍返回 active 列表，`unlocked=false`。
- 状态筛选在前端计算：`全部 / 已解锁 / 未解锁`。
- 卡片：图标（无图标时首字/默认图标）、名称、描述、奖励、进度条（多条件逐个显示）、已解锁时间。
- 已解锁卡片使用成功色边框；未解锁显示进度。
- 错误用 `el-alert`，加载用 `v-loading`，空状态 `el-empty`。

- [ ] **Step 2: 实现任务大厅**

- `listTasks()`；状态标签映射：
  - `upcoming` 未开始、`active` 可领取、`claimed` 进行中、`completed` 已完成、`expired` 已过期、`ended` 已结束、`inactive` 已下线。
- 领取按钮仅在 `status === 'active'` 且登录时显示；游客显示“登录后领取”。
- 领取前 `ElMessageBox.confirm`；成功后 `ElMessage.success` + 重新加载；失败显示 `ApiError.message`。
- 进度显示 `progress/problemCount`；已完成显示完成时间。
- 描述使用 `MarkdownView` 或纯文本摘要（长文截断）；本任务可用纯文本摘要，详情暂不做弹窗。

- [ ] **Step 3: 路由**

```ts
{ path: 'achievements', name: 'achievements', component: () => import('@/views/AchievementsView.vue'), meta: { title: '成就' } }
{ path: 'tasks', name: 'tasks', component: () => import('@/views/TasksView.vue'), meta: { title: '任务' } }
```

- [ ] **Step 4: 构建与浏览器验证**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm build:web && corepack pnpm lint:web`
Browser: 游客打开 `/achievements`、`/tasks` 无控制台错误；登录 `m2bsolver1` 看到「首次 AC」「初次参赛」已解锁、任务「新手热身」已完成。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AchievementsView.vue apps/web/src/views/TasksView.vue apps/web/src/router/index.ts
git commit -m "feat(web): achievements and task hall"
```

---

### Task 3: 徽章商城、称号商店与装扮管理

**Files:**
- Create: `apps/web/src/views/BadgeShopView.vue`
- Create: `apps/web/src/views/TitleShopView.vue`
- Create: `apps/web/src/views/AppearanceView.vue`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/stores/auth.ts`（如需要 `refreshMe` 别名，优先复用 `fetchMe`）
- Test: 通过 build + 浏览器验证

**Interfaces:**
- Consumes: Task 1 的徽章/称号/金币 API、`IdentityShowcase`。
- Produces:
  - `/shop/badges`：图片墙、分类筛选、价格/库存、拥有/装备状态、购买按钮、余额提示。
  - `/shop/titles`：称号列表、颜色预览、解锁条件进度、购买/装备状态。
  - `/appearance`：金币余额 + 流水、已拥有徽章网格（装备/卸下，显示 n/上限）、已拥有称号（装备/卸下）。
  - 路由 `badge-shop`、`title-shop`、`appearance`。

- [ ] **Step 1: 徽章商城**

- `listBadges()`；按 `category` 生成筛选项（从数据聚合）。
- 卡片：图片（无图显示首字）、名称、描述、价格、库存（限量显示剩余）、拥有/装备标签。
- 购买按钮：未登录→登录链接；已拥有→“已拥有”；余额不足→禁用并提示；限量售罄→禁用。
- 购买确认后 `purchaseBadge`，成功刷新列表 + `auth.fetchMe()`；失败显示服务端错误。
- 装备/卸下可在卡片上直接操作；成功后刷新列表 + `auth.fetchMe()`。
- 显示当前余额（来自 `auth.user.coins`）。

- [ ] **Step 2: 称号商店**

- `listTitles()`；按 `unlocked/purchased/equipped` 显示状态。
- 称号名称使用 `safeTitleColor` 着色；进度显示 `current/target`。
- 未登录→登录；已解锁未购买→购买；已购买未装备→装备；已装备→卸下。
- 购买/装备成功后刷新列表 + `auth.fetchMe()`。

- [ ] **Step 3: 装扮管理**

- `auth.fetchMe()` 获取余额；`listUserBadges(auth.user.id)` 获取拥有徽章；`listTitles()` 获取已购买称号。
- 金币流水 `listCoinTransactions({page:1,pageSize:20})`，显示原因、金额、时间；分页可用“加载更多”或分页器。
- 徽章网格显示装备状态，按钮“装备/卸下”；超过上限时禁用并提示。
- 称号列表显示已购买项，按钮“装备/卸下”；装备新称号自动卸下旧称号（服务端保证）。
- 顶部 `IdentityShowcase` 预览当前装备。

- [ ] **Step 4: 路由与构建**

添加三条路由；运行 `build:web`、`lint:web`。

- [ ] **Step 5: 浏览器验证与提交**

Browser: `m2bsolver1` 购买/装备已有徽章「探索者」和称号「初出茅庐」；余额与按钮状态正确；游客看到商城但不能购买。

```bash
git add apps/web/src/views/BadgeShopView.vue apps/web/src/views/TitleShopView.vue apps/web/src/views/AppearanceView.vue apps/web/src/router/index.ts apps/web/src/stores/auth.ts
git commit -m "feat(web): badge and title shops"
```

---

### Task 4: 游戏化管理后台（成就 + 任务）

**Files:**
- Create: `apps/web/src/views/GamificationAdminView.vue`
- Create: `apps/web/src/components/admin/AchievementAdminPanel.vue`
- Create: `apps/web/src/components/admin/TaskAdminPanel.vue`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/server/internal/gamification/task.go`
- Modify: `apps/server/internal/gamification/task_repository.go`
- Modify: `apps/server/internal/gamification/dto.go`（若 TaskItem 在此）
- Modify: `apps/server/internal/gamificationapi/handler.go`
- Modify: `apps/server/internal/gamification/task_test.go`
- Modify: `apps/server/internal/gamificationapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Test: 后端任务测试 + build + 浏览器验证

**Interfaces:**
- Consumes: Task 1 API、MarkdownEditor、MarkdownView。
- Produces:
  - `/admin/gamification` 页面，`el-tabs`：成就、任务、徽章、称号（本任务只实现前两个 Tab；后两个在 Task 5 接入空占位）。
  - `TaskItem.hasRecords: boolean`（JSON `hasRecords`）：管理员列表返回该任务是否已有领取记录；用于冻结题目/奖励。
  - 成就表单：code/name/description/iconUrl/conditionType/conditionConfig/rewardCoins/rewardBadgeId/sortOrder/isActive；条件构造器支持单条件与 `all_of`（2–8 行，不嵌套）。
  - 任务表单：title/descriptionMd/problemIds/rewardCoins/startTime/endTime/isActive；题目远程搜索；`hasRecords=true` 时禁用 problemIds/rewardCoins 并显示“已有领取记录，题目与奖励已冻结”。

- [ ] **Step 1: 后端补 `TaskItem.hasRecords`**

- `TaskItem` 增加 `HasRecords bool json:"hasRecords"`。
- `ListTasks` 对 `Viewer.IsAdmin` 的任务列表用一条 `SELECT DISTINCT task_id FROM user_task_records` 批量填充 `hasRecords`；普通用户固定 false。
- `CreateTask` 返回 false；`UpdateTask` 返回当前 `hasRecords`。
- OpenAPI `TaskItem.required` 加 `hasRecords`；`gen:api`。
- 测试：管理员列表创建任务并领取后 `hasRecords=true`；普通用户列表字段为 false（或按实现文档写清）。

- [ ] **Step 2: 写成就管理面板**

- 表格列：名称/code/条件/奖励/排序/状态/操作。
- 新建/编辑弹窗：
  - `code` 编辑时禁用（创建必填）。
  - 条件类型选择 `ac_count/contest_count/login_streak/created_contest/all_of`。
  - 单条件显示 threshold 输入；`all_of` 显示 2–8 行子条件（type + threshold），禁止选 `all_of`。
  - 奖励徽章：从 `listBadges({includeInactive:true})` 中筛 `isActive=true`，显示名称；编辑时若原奖励徽章已下线，仍把它加入选项并标记“已下线”，避免表单把原值丢掉。可空。
  - 奖励金币 0–1,000,000。
  - 保存后刷新列表。
- 删除二次确认；失败显示服务端错误。

- [ ] **Step 3: 写任务管理面板**

- 表格列：标题/时间/奖励/题目数/状态/领取记录/操作。
- 表单：
  - 题目远程搜索，最多 20 个，显示题目标题；编辑时对已有 ID 调 `getProblem(id)` 补全标题。
  - MarkdownEditor 描述。
  - `hasRecords` 时禁用 problemIds/rewardCoins。
  - 时间用 `el-date-picker type="datetimerange"`。
- 删除二次确认。

- [ ] **Step 4: 管理页容器与路由**

- `GamificationAdminView.vue` 检查 `hasRole(auth.user?.role, ['super_admin'])`（路由 meta 已限制，页面再兜底）。
- Tabs：`成就`、`任务`、`徽章`（Task 5 占位）、`称号`（Task 5 占位）。
- 路由 `/admin/gamification`，meta `requiresAuth: true, roles: ['super_admin']`。
- 导航入口仅 super_admin 显示。

- [ ] **Step 5: 后端/前端检查、浏览器验证、提交**

Run:
```bash
cd /Users/admin/codespace/kapoj/apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/gamificationapi/ -v
cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm build:web && corepack pnpm lint:web
```
Browser: `m2badmin` 打开管理页，创建/编辑/删除成就与任务；已有领取记录的任务题目与奖励禁用。

```bash
git add apps/web/src/views/GamificationAdminView.vue apps/web/src/components/admin/AchievementAdminPanel.vue apps/web/src/components/admin/TaskAdminPanel.vue apps/web/src/router/index.ts apps/server/internal/gamification apps/server/internal/gamificationapi packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(web): gamification achievement and task admin"
```

---

### Task 5: 游戏化管理后台（徽章 + 称号）

**Files:**
- Create: `apps/web/src/components/admin/BadgeAdminPanel.vue`
- Create: `apps/web/src/components/admin/TitleAdminPanel.vue`
- Modify: `apps/web/src/views/GamificationAdminView.vue`
- Test: build + 浏览器验证

**Interfaces:**
- Consumes: Task 1 API、Task 4 管理页容器。
- Produces:
  - 徽章面板：表格、CRUD 弹窗、图片上传、价格/分类/限量库存/排序/上下架。
  - 称号面板：表格、CRUD 弹窗、颜色选择、解锁条件（rating/ac_count/achievement）、价格/排序/上下架。
  - 徽章编辑时库存字段默认不改；只有用户主动修改才提交 `stock`（nil 保留）。

- [ ] **Step 1: 徽章管理面板**

- 表格列：图片/名称/分类/价格/库存/排序/状态/操作。
- 新建：名称、描述、分类、价格、限量/库存、排序、上下架、图片上传（创建后上传，或先创建再上传）。
- 编辑：回填现有值；库存默认不提交，勾选“调整库存”后才提交 `stock`；图片不选新文件时省略 `imageUrl` 以保留。
- 上传：`el-upload` 使用 `:http-request` 调 `uploadBadgeImage`，成功后刷新。
- 删除二次确认。

- [ ] **Step 2: 称号管理面板**

- 表格列：名称（彩色）/解锁条件/价格/排序/状态/操作。
- 表单：名称、颜色（`el-color-picker`）、解锁类型、条件值、价格、排序、上下架。
- `achievement` 类型用成就下拉（`listAchievements({includeInactive:true})`，显示 code + 名称）。
- 保存/删除后刷新。

- [ ] **Step 3: 接入管理页 Tabs、构建、浏览器验证**

- 替换 Task 4 的占位 Tab。
- Browser：创建徽章并上传图片；创建称号并着色；编辑徽章名称不改变库存；删除后的下架状态正确。
- Run: `corepack pnpm build:web && corepack pnpm lint:web`

- [ ] **Step 4: 提交**

```bash
git add apps/web/src/components/admin/BadgeAdminPanel.vue apps/web/src/components/admin/TitleAdminPanel.vue apps/web/src/views/GamificationAdminView.vue
git commit -m "feat(web): gamification badge and title admin"
```

---

### Task 6: 导航、公开身份接入、文档与端到端验收

**Files:**
- Modify: `apps/web/src/layouts/DefaultLayout.vue`
- Modify: `apps/web/src/components/UserMenu.vue`
- Modify: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/views/SubmissionsView.vue`
- Modify: `apps/web/src/components/ContestRanklist.vue`
- Modify: `apps/web/src/views/HomeView.vue`（可选：成长入口卡片）
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量检查 + 真实浏览器 E2E

**Interfaces:**
- Consumes: Task 1–5 的页面/组件/API。
- Produces:
  - 侧边导航“成长”子菜单：成就、任务、徽章商城、称号商店、装扮管理；super_admin 额外“游戏化管理”。
  - 用户菜单显示金币余额和装扮管理入口。
  - 个人主页头部显示 `IdentityShowcase`（`profile.equippedBadges/equippedTitle`）。
  - 测评记录“提交者”列显示昵称 + `IdentityShowcase`。
  - 比赛榜单“选手”列显示昵称 + `IdentityShowcase`。
  - 文档更新。

- [ ] **Step 1: 导航与用户菜单**

- `DefaultLayout.vue` 的 `NavItem` 支持 children；新增“成长” `el-sub-menu`。
- 普通用户显示成就/任务/徽章商城/称号商店/装扮管理；super_admin 显示游戏化管理。
- `UserMenu.vue` 增加金币显示（`auth.user.coins`）和“装扮管理”菜单项。

- [ ] **Step 2: 公开身份展示**

- `UserProfileView`：在昵称旁渲染 `IdentityShowcase :badges="profile.equippedBadges" :title="profile.equippedTitle"`。
- `SubmissionsView`：提交者单元格昵称下方渲染小尺寸 `IdentityShowcase`。
- `ContestRanklist`：选手单元格同样渲染。
- 空数组/null 时不显示占位；窄屏换行。

- [ ] **Step 3: 文档**

- `DEVELOPMENT.md`：4.1 个人中心、4.4 比赛榜单、4.6 测评记录装备展示、5.1–5.4 实现状态、路线图 M3 ✅。
- `docs/任务清单.md`：M3-8 ✅、M3 标题 ✅；加 M3-C 计划链接。
- `docs/开发日志.md`：追加 M3-C 条目（页面、管理后台、公开身份、验证、复审修复）。
- `README.md`、`运行手册.md`：当前版本 M3 完成；补充成就/任务/商城/装扮/管理后台操作说明、`KAPOJ_MAX_EQUIPPED_BADGES`、徽章图片存储。
- 修正 M3-B 遗留文档：OpenAPI 徽章更新摘要措辞、开发日志“称号排序缺少测试”已过时。

- [ ] **Step 4: 全量检查**

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

- [ ] **Step 5: 真实浏览器 E2E**

1. `m2badmin` 创建成就（AC 2 题）、任务（1 题）、徽章（限量 + 图片）、称号（AC 1 题）。
2. `m2bsolver1` 领取任务、完成任务、看到成就进度/金币变化。
3. 购买/装备徽章与称号；金币不足/库存售罄/未解锁提示正确。
4. 个人主页、测评记录、比赛榜单显示装备；刷新后仍正确。
5. 普通用户看不到管理入口，直接访问 `/admin/gamification` 到 403。
6. 移动端窄屏：卡片、表格、弹窗可读可操作；无控制台错误、无 CDN 请求。

- [ ] **Step 6: 提交**

```bash
git add apps/web/src/layouts/DefaultLayout.vue apps/web/src/components/UserMenu.vue apps/web/src/views/UserProfileView.vue apps/web/src/views/SubmissionsView.vue apps/web/src/components/ContestRanklist.vue apps/web/src/views/HomeView.vue DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(web): gamification pages and public identity"
```

---

## Self-Review

**Spec coverage:**

- 成就页/任务大厅/徽章商城/称号商店/装扮管理/管理后台 → Task 2–5。
- 公开身份（个人主页/测评记录/比赛榜单）→ Task 6。
- 管理端 CRUD、图片上传、任务冻结 → Task 4/5（含 `hasRecords` 后端小改动）。
- 导航、权限、金币显示、错误提示 → Task 1/3/6。
- 文档与 E2E → Task 6。
- M3-C 不实现 M4 排行榜/Rating。

**Type consistency:**

- 所有 API 类型来自 `schema.d.ts`，`api/types.ts` 只做别名。
- `IdentityShowcase` 的 props 使用 `EquippedBadge[]`/`EquippedTitle|null`。
- 徽章库存编辑用“是否调整库存”状态控制 `stock` 是否提交，避免并发覆盖。
- `TaskItem.hasRecords` 从后端到 OpenAPI 到前端统一。

**Review Focus mapping:**

1. 余额/状态一致性 → Task 3/4 + `auth.fetchMe()`。
2. 任务冻结 → Task 4 `hasRecords`。
3. 空装备/窄屏 → Task 1 组件测试 + Task 6 浏览器。
4. 权限入口 → Task 6 路由/菜单/E2E。
5. 错误处理 → Task 3/4 `ApiError.message` + E2E。

**Proportion:** 计划固定页面结构、API 名称、状态与测试；具体组件样式和表单布局留给执行者按现有 Element Plus 风格完成。
