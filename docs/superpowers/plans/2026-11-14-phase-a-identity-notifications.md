# Phase A：身份展示、通知与快速修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付第一批低风险、高收益改动：修复称号颜色保存、贡献图颜色、通知角标即时刷新，并统一称号展示与徽章大小配置。

**Architecture:** 先修两个独立 Bug（R09、R08），再用 Pinia 共享通知状态（R10），随后在站点设置中加入徽章大小（R05），最后用统一的 `UserIdentityName` 组件替换各页面“昵称 + IdentityShowcase”的手工拼装（R02）。所有改动保持现有 API 兼容，新增接口同步 OpenAPI。

**Tech Stack:** Go + Gin + GORM + PostgreSQL、Vue 3 + PrimeVue 4 + Pinia + Vitest、Playwright（关键路径）。

**Spec:** [`docs/持续开发需求.md`](../../持续开发需求.md)

**Status:** ✅ Tasks 1–5 complete (2026-11-14). Verification: backend site/siteapi/gamification tests pass; frontend 115 test files / 900 tests pass; lint and build pass. R02 covers the core public identity pages; actor summaries (notifications/followers/solution author) remain for Phase B.

## Global Constraints

- 不引入 CDN 资源；代码高亮、图表、编辑器继续本地依赖。
- 所有新增接口先更新 `packages/api/openapi.yaml`，再生成前端类型。
- 文案使用中文；错误信息可定位。
- 所有页面改动覆盖 375px 宽度与键盘操作。
- 不修改无关模块；用户工作区已有未提交改动，提交时只 stage 本计划相关文件。
- 每个任务遵循 TDD：先写失败测试，再实现，再跑测试。

## Review Focus

- 称号颜色输入可能是 `#RRGGBB` 或 `RRGGBB`，预览、校验、后端保存都必须一致，不能只修一个入口。
- 深色主题贡献图必须“提交越多颜色越深”，且最深色在深色背景上仍可辨识。
- 通知角标只能在标记已读成功后减一/归零；失败要回滚，不能出现负数或重复减。
- 排行榜 Rating 名字颜色不能被称号颜色覆盖；称号在昵称前、徽章在昵称后。
- 徽章大小必须由后端范围校验，前端滑块/输入不能产生 0 或超大值；`small` 变体不能低于 14px。

---

### Task 1: R09 修复称号颜色保存

**Files:**
- Modify: `apps/server/internal/gamification/title.go:29,286-317`
- Modify: `apps/server/internal/gamification/title_test.go:497-547`
- Modify: `apps/web/src/components/admin/TitleAdminPanel.vue:53,351-362`
- Modify: `apps/web/src/components/admin/TitleAdminPanel.spec.ts:155-188`
- Modify: `packages/api/openapi.yaml:2272,4325`

**Interfaces:**
- Consumes: 无
- Produces: `normalizeTitleColor(raw string) string`（内部函数）与前端 `onColorChange(value: string)`，最终保存值始终为 `#RRGGBB`。

- [ ] **Step 1: 写后端失败测试**

在 `title_test.go` 的 `TestTitleCRUDValidation` 中：
- 从无效 cases 中删除 `{"颜色缺少井号", ...}`。
- 新增 `TestTitleColorNormalizes`：用 `titleInput("颜色规范化", "7C3AED", ...)` 创建，断言 `created.Color == "#7C3AED"`；再用 `"ABCDEF"` 更新，断言 `updated.Color == "#ABCDEF"`。
- 保留 `#7C3AE`、`#GGGGGG` 为 40001。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/gamification -run 'TestTitle(ColorNormalizes|CRUDValidation)' -count=1`
Expected: `TestTitleColorNormalizes` FAIL（缺少 `#` 被拒绝）。

- [ ] **Step 3: 实现后端规范化**

在 `title.go`：
```go
func normalizeTitleColor(raw string) string {
    color := strings.TrimSpace(raw)
    if len(color) == 6 {
        color = "#" + color
    }
    return color
}
```
`normalizeTitleInput` 中先 `in.Color = normalizeTitleColor(in.Color)`，再走 `titleColorPattern` 校验。

- [ ] **Step 4: 写前端失败测试**

在 `TitleAdminPanel.spec.ts` 的 `creates a rating title` 中，把 ColorPicker emit 改为 `'112233'`（不带 `#`），断言 `createTitle` 收到 `color: '#112233'`。再补一个用例：编辑时 emit `'abcdef'`，保存值应为 `#abcdef`。

- [ ] **Step 5: 运行前端测试确认失败**

Run: `corepack pnpm --filter web test src/components/admin/TitleAdminPanel.spec.ts`
Expected: FAIL，`createTitle` 收到 `112233` 或校验提示“称号颜色必须为 #RRGGBB 格式”。

- [ ] **Step 6: 实现前端规范化**

在 `TitleAdminPanel.vue`：
```ts
function onColorChange(value: string) {
  const raw = value.trim()
  form.value.color = raw.startsWith('#') ? raw : `#${raw}`
}
```
模板：
```vue
<ColorPicker
  :model-value="form.color"
  format="hex"
  @update:model-value="onColorChange"
/>
```
保留 `COLOR_RE` 严格校验；预览继续使用 `safeTitleColor`。

- [ ] **Step 7: 更新 OpenAPI**

`TitleInput.color` 改为 `{ type: string, pattern: '^#?[0-9A-Fa-f]{6}$', description: 称号颜色；接受 #RRGGBB 或 RRGGBB，保存为 #RRGGBB }`；创建/更新描述同步。

- [ ] **Step 8: 运行测试与生成类型**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/gamification -run 'TestTitle(ColorNormalizes|CRUDValidation)' -count=1
corepack pnpm --filter web test src/components/admin/TitleAdminPanel.spec.ts
corepack pnpm gen:api
```
Expected: 后端、前端 PASS；类型生成无错误。

- [ ] **Step 9: 提交**

```bash
git add apps/server/internal/gamification/title.go apps/server/internal/gamification/title_test.go \
  apps/web/src/components/admin/TitleAdminPanel.vue apps/web/src/components/admin/TitleAdminPanel.spec.ts \
  packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "fix: normalize title colors before saving"
```

---

### Task 2: R08 修复贡献图深色主题色阶

**Files:**
- Modify: `apps/web/src/components/profile/ContributionGrid.vue:194-214`
- Test: `apps/web/src/components/profile/ContributionGrid.spec.ts`（补充静态断言或 E2E）

**Interfaces:**
- Consumes: 无
- Produces: 深色主题 `--contrib-1` 最浅、`--contrib-4` 最深；浅色主题保持现状。

- [ ] **Step 1: 写失败测试**

在 `ContributionGrid.spec.ts` 中新增一个测试，读取 `ContributionGrid.vue?raw` 的样式片段，断言深色主题 `--contrib-1` 的颜色亮度高于 `--contrib-4`（用简单 hex 亮度函数）。若团队不接受源码断言，则改为 Playwright 在深色主题下比较 `getComputedStyle`；二选一，必须能在修复前失败。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test src/components/profile/ContributionGrid.spec.ts`
Expected: FAIL，当前深色 `level-1` 比 `level-4` 暗。

- [ ] **Step 3: 修复深色色阶**

将深色主题变量改为：
```css
--contrib-1: #39d353; /* 最少：最浅 */
--contrib-2: #26a641;
--contrib-3: #006d32;
--contrib-4: #0e4429; /* 最多：最深 */
```
浅色主题覆盖不变。若 `level-4` 在深色背景辨识不足，增加细边框或改用 `#14532d`。

- [ ] **Step 4: 运行测试与视觉检查**

Run: `corepack pnpm --filter web test src/components/profile/ContributionGrid.spec.ts`
Expected: PASS。再在浅色/深色主题下人工检查 5 档色块和图例方向。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/components/profile/ContributionGrid.vue apps/web/src/components/profile/ContributionGrid.spec.ts
git commit -m "fix: make contribution heatmap darker with more submissions"
```

---

### Task 3: R10 通知共享状态与角标即时刷新

**Files:**
- Create: `apps/web/src/stores/notifications.ts`
- Modify: `apps/web/src/components/layout/NotificationBell.vue`
- Modify: `apps/web/src/views/NotificationsView.vue`
- Modify: `apps/web/src/components/layout/NotificationBell.spec.ts`
- Modify: `apps/web/src/views/NotificationsView.spec.ts`

**Interfaces:**
- Consumes: `listNotifications`、`unreadCount`、`markRead`、`markAllRead`（`@/api/social`）
- Produces:
  - `useNotificationsStore()`：`unreadCount: Ref<number>`、`refreshUnread(): Promise<void>`、`markRead(id): Promise<void>`、`markAllRead(): Promise<void>`
  - `NotificationBell` 和 `NotificationsView` 共用同一 store。

- [ ] **Step 1: 写 store 失败测试**

新增 `apps/web/src/stores/notifications.spec.ts`：
- 初始 `refreshUnread()` 调用 `unreadCount()`，写入 count。
- `markRead(id)` 成功后 count 减一；失败时回滚。
- `markAllRead()` 成功后 count 归零。
- 已读通知再次 mark 不重复减。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test src/stores/notifications.spec.ts`
Expected: FAIL，模块不存在。

- [ ] **Step 3: 实现 store**

创建 Pinia store，使用 `defineStore('notifications', ...)`；所有失败回滚用 `try/catch` + 恢复旧值；不要在 store 中直接操作组件。

- [ ] **Step 4: 改造 NotificationBell**

- 删除本地 `count`/`unreadCount` 调用，改用 store 的 `unreadCount` 与 `refreshUnread()`。
- 保留 60 秒 `setInterval` 兜底。
- 保留登录判断、`99+` 显示、卸载清理。

- [ ] **Step 5: 改造 NotificationsView**

- 使用 store 的 `markRead`/`markAllRead`；成功后本地 `readAt` 更新。
- “全部已读”按钮显示条件改为 `store.unreadCount > 0`，不再只看当前页。
- 点击单条未读通知后，store 角标立即减一。

- [ ] **Step 6: 更新组件测试**

- `NotificationBell.spec.ts` mock store，断言 store count 变化即时反映到 badge。
- `NotificationsView.spec.ts` mock store，断言 markRead/markAllRead 调用和角标变化；保留分页、空态、错误态测试。

- [ ] **Step 7: 运行前端测试**

Run:
```bash
corepack pnpm --filter web test src/stores/notifications.spec.ts
corepack pnpm --filter web test src/components/layout/NotificationBell.spec.ts
corepack pnpm --filter web test src/views/NotificationsView.spec.ts
```
Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add apps/web/src/stores/notifications.ts apps/web/src/stores/notifications.spec.ts \
  apps/web/src/components/layout/NotificationBell.vue apps/web/src/components/layout/NotificationBell.spec.ts \
  apps/web/src/views/NotificationsView.vue apps/web/src/views/NotificationsView.spec.ts
git commit -m "feat: sync notification unread count across bell and page"
```

---

### Task 4: R05 后台配置徽章大小

**Files:**
- Modify: `apps/server/internal/site/model.go`
- Modify: `apps/server/internal/site/dto.go`
- Modify: `apps/server/internal/site/service.go`
- Modify: `apps/server/internal/site/service_test.go`
- Modify: `apps/server/internal/siteapi/handler.go`
- Modify: `apps/server/internal/siteapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`
- Modify: `apps/web/src/api/site.ts`
- Modify: `apps/web/src/api/types.ts`（生成）
- Modify: `apps/web/src/stores/site.ts`
- Modify: `apps/web/src/views/AdminSiteView.vue`
- Modify: `apps/web/src/views/AdminSiteView.spec.ts`
- Modify: `apps/web/src/components/IdentityShowcase.vue`

**Interfaces:**
- Consumes: 无
- Produces:
  - `site.Settings.BadgeSize int`（DB `badge_size`，默认 22）
  - `site.Appearance { BadgeSize int }`，`PublicView.Appearance`
  - `PUT /api/admin/site/appearance` body `{badgeSize}`
  - 前端 `useSiteStore().badgeSize` 和 CSS 变量 `--kapoj-badge-size`。

- [ ] **Step 1: 写后端失败测试**

在 `site/service_test.go` 中：
- 默认 `Public()` 返回 `Appearance.BadgeSize == 22`。
- `UpdateAppearance(ctx, actor, 40)` 后返回 40。
- `10`、`49` 返回 40001。
在 `siteapi/handler_test.go` 中补权限与接口测试：无 `site.settings.manage` 返回 403，正常保存返回新 view。

- [ ] **Step 2: 运行后端测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/site ./internal/siteapi -run 'Appearance|BadgeSize' -count=1`
Expected: FAIL，字段/接口不存在。

- [ ] **Step 3: 实现后端**

- `model.go` 增加 `BadgeSize int gorm:"column:badge_size;not null;default:22"`。
- `repository.Update` 的字段 map 增加 `badge_size`。
- `dto.go` 增加 `Appearance`、`AppearanceInput`；`PublicView` 增加 `Appearance`。
- `service.go` 增加 `UpdateAppearance(ctx, actorID, size)`，范围 16–48；`view()` 填默认 22。
- `siteapi/handler.go` 注册 `PUT /admin/site/appearance`，写审计 target=`site_settings`。
- OpenAPI 补 `Appearance` schema 与接口。

- [ ] **Step 4: 写前端失败测试**

`AdminSiteView.spec.ts`：
- 渲染“外观”Tab，输入 40，保存，断言 `updateAdminSiteAppearance({ badgeSize: 40 })`。
- 输入 10/49 显示校验错误，不调用 API。
`IdentityShowcase.spec.ts`：
- 通过 site store/CSS 变量设置 `badgeSize=40`，断言 avatar style 为 40px；`small` 为 36px；下限 14px。

- [ ] **Step 5: 运行前端测试确认失败**

Run:
```bash
corepack pnpm --filter web test src/views/AdminSiteView.spec.ts
corepack pnpm --filter web test src/components/IdentityShowcase.spec.ts
```
Expected: FAIL。

- [ ] **Step 6: 实现前端**

- `api/site.ts` 增加 `getAdminSiteAppearance`（如需要）与 `updateAdminSiteAppearance(input)`。
- `stores/site.ts` 增加 `badgeSize` computed，默认 22；`setConfig` 后更新。
- `AdminSiteView.vue` 增加“外观”Tab：`InputNumber` + 预览 + 保存；错误提示；保存成功后 `site.setConfig(view)`。
- `IdentityShowcase.vue`：`avatarSize` 从 store 的 `badgeSize` 读取；`small` 用 `Math.max(14, badgeSize - 4)`；保留 size prop 覆盖能力。

- [ ] **Step 7: 运行测试**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/site ./internal/siteapi -count=1
corepack pnpm --filter web test src/views/AdminSiteView.spec.ts
corepack pnpm --filter web test src/components/IdentityShowcase.spec.ts
corepack pnpm gen:api
```
Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add apps/server/internal/site apps/server/internal/siteapi packages/api/openapi.yaml \
  apps/web/src/api/site.ts apps/web/src/api/schema.d.ts apps/web/src/stores/site.ts \
  apps/web/src/views/AdminSiteView.vue apps/web/src/views/AdminSiteView.spec.ts \
  apps/web/src/components/IdentityShowcase.vue apps/web/src/components/IdentityShowcase.spec.ts
git commit -m "feat: make badge size configurable in site settings"
```

---

### Task 5: R02 统一“【称号】昵称”展示

**Files:**
- Create: `apps/web/src/components/UserIdentityName.vue`
- Create: `apps/web/src/components/UserIdentityName.spec.ts`
- Modify: `apps/web/src/components/IdentityShowcase.vue`（拆出徽章渲染或保留兼容）
- Modify: `apps/web/src/views/SubmissionsView.vue`
- Modify: `apps/web/src/views/LeaderboardView.vue`
- Modify: `apps/web/src/components/ContestRanklist.vue`
- Modify: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/views/AppearanceView.vue`
- Modify: `apps/web/src/components/RatingLevelName.vue`（支持 title）
- Modify: `apps/web/src/components/RatingLevelName.spec.ts`
- Modify: `apps/web/src/components/IdentityShowcase.spec.ts`
- Modify: `apps/web/src/views/SubmissionsView.spec.ts`
- Modify: `apps/web/src/views/LeaderboardView.spec.ts`
- Modify: `apps/web/src/components/ContestRanklist.spec.ts`
- Modify: `apps/web/src/views/UserProfileView.spec.ts`
- Modify: `apps/web/src/views/AppearanceView.spec.ts`

**Interfaces:**
- Consumes: `equippedTitle: {id,name,color}`、`equippedBadges`、可选 `ratingLevel`
- Produces:
  - `UserIdentityName` props：`nickname: string`、`username?: string`、`title: EquippedTitle | null`、`badges: EquippedBadge[]`、`size?: 'small' | 'default'`、`ratingLevel?: RatingLevel | null`、`linkTo?: RouteLocationRaw`。
  - 渲染顺序：`【称号】昵称` + 徽章；无称号不显示括号；称号用 `safeTitleColor`。
  - `RatingLevelName` 增加可选 `title` prop，称号在 Rating 名字之前且使用称号颜色。

- [ ] **Step 1: 写组件失败测试**

`UserIdentityName.spec.ts`：
- 有称号：文本以 `【卷王】Alice` 开头，称号颜色正确，徽章在昵称后。
- 无称号：不出现 `【】`。
- 传 `ratingLevel`：名字仍使用 Rating 颜色，称号使用自身颜色。
- `linkTo`：昵称可点击到目标路由。
- 375px 下结构为 `inline-flex; flex-wrap: wrap`（断言 class/样式）。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test src/components/UserIdentityName.spec.ts`
Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现组件**

建议结构：
```vue
<span class="user-identity-name">
  <span v-if="title" class="user-identity-name__title" :style="{ color: titleColor }">
    【{{ title.name }}】
  </span>
  <RouterLink v-if="linkTo" :to="linkTo" class="user-identity-name__name">
    <RatingLevelName v-if="ratingLevel" :name="displayName" :level="ratingLevel" />
    <template v-else>{{ displayName }}</template>
  </RouterLink>
  <span v-else class="user-identity-name__name">
    <RatingLevelName v-if="ratingLevel" :name="displayName" :level="ratingLevel" />
    <template v-else>{{ displayName }}</template>
  </span>
  <span v-for="badge in badges" :key="badge.id" class="user-identity-name__badge">
    <!-- 复用 IdentityShowcase 的徽章渲染，或抽出 BadgeList -->
  </span>
</span>
```
如果复用 `IdentityShowcase` 会重复 RTooltip/Avatar 逻辑，建议抽出 `BadgeList.vue`；`IdentityShowcase` 改为 `TitleTag + BadgeList` 的兼容包装，或标记 deprecated 后删除。

- [ ] **Step 4: 替换核心页面**

- `SubmissionsView.vue`：列表提交者、详情用户列改为 `UserIdentityName`，传入 `linkTo` 用户主页。
- `LeaderboardView.vue`：两处（前三名卡片、完整表格）改为 `UserIdentityName`，传入 `ratingLevel`。
- `ContestRanklist.vue`：选手列改为 `UserIdentityName`。
- `UserProfileView.vue`：hero 的 `<h1>` 内使用 `UserIdentityName`（`badges=[]`），标题在昵称前；profile surface 的徽章列表保留。
- `AppearanceView.vue`：当前装扮预览用 `UserIdentityName` 或 `IdentityShowcase` 显示 `【称号】` + 徽章（无昵称时只显示称号/徽章）。

- [ ] **Step 5: 更新页面测试**

逐个更新快照/文本断言：
- `SubmissionsView.spec.ts` 断言 `【荣耀王者】Alice`。
- `LeaderboardView.spec.ts` 断言称号在昵称前，Rating 颜色仍生效。
- `ContestRanklist.spec.ts`、`UserProfileView.spec.ts`、`AppearanceView.spec.ts` 同步。

- [ ] **Step 6: 运行前端测试与 lint**

Run:
```bash
corepack pnpm --filter web test src/components/UserIdentityName.spec.ts
corepack pnpm --filter web test src/components/RatingLevelName.spec.ts
corepack pnpm --filter web test src/views/SubmissionsView.spec.ts
corepack pnpm --filter web test src/views/LeaderboardView.spec.ts
corepack pnpm --filter web test src/components/ContestRanklist.spec.ts
corepack pnpm --filter web test src/views/UserProfileView.spec.ts
corepack pnpm --filter web test src/views/AppearanceView.spec.ts
corepack pnpm --filter web lint
```
Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add apps/web/src/components/UserIdentityName.vue apps/web/src/components/UserIdentityName.spec.ts \
  apps/web/src/components/IdentityShowcase.vue apps/web/src/components/IdentityShowcase.spec.ts \
  apps/web/src/components/RatingLevelName.vue apps/web/src/components/RatingLevelName.spec.ts \
  apps/web/src/views/SubmissionsView.vue apps/web/src/views/SubmissionsView.spec.ts \
  apps/web/src/views/LeaderboardView.vue apps/web/src/views/LeaderboardView.spec.ts \
  apps/web/src/components/ContestRanklist.vue apps/web/src/components/ContestRanklist.spec.ts \
  apps/web/src/views/UserProfileView.vue apps/web/src/views/UserProfileView.spec.ts \
  apps/web/src/views/AppearanceView.vue apps/web/src/views/AppearanceView.spec.ts
git commit -m "feat: show titles before nicknames everywhere"
```

---

## Self-Review

- **Spec coverage:** R09、R08、R10、R05、R02 均有任务；R01/R03/R04/R06/R07/R11/R12/R13 属于后续 Phase B–D，不在本计划。
- **Type consistency:** `UserIdentityName` 的 `title`/`badges`/`ratingLevel` 与现有 `EquippedTitle`/`EquippedBadge` 一致；`badgeSize` 在 site store 与 `IdentityShowcase` 间保持 `number`。
- **Review Focus tests:** 称号颜色无 `#`、深色贡献图、通知失败回滚、Rating 名字颜色、徽章大小边界均安排了测试。
- **Proportion:** 计划覆盖 5 个需求，任务按可独立验收的交付物拆分；未展开具体组件内部实现细节。
