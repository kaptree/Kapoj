# 昵称 Rating 颜色同步实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 公共页面所有用户昵称按服务端 rating 等级上色；Agent 名称按 Agent rating 上色，Agent 作者按作者 rating 上色。

**Architecture:** 扩展后端公共身份装饰器和各公共 DTO，让每个用户摘要带 `level`（`rating.LevelFor`），Agent 摘要额外带 Agent 自己的 `level`。前端统一复用 `RatingLevelName`/`UserIdentityName`，不复制色阶。

**Tech Stack:** Go 1.26 + Gin + GORM、OpenAPI 3 + 生成 TypeScript、Vue 3 + TypeScript + PrimeVue、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-11-21-rating-nickname-colors-design.md`

## Global Constraints

- 颜色唯一来源：`apps/server/internal/rating/level.go` 的 `LevelFor(rating)`；前端不得硬编码色值或边界。
- 公共页面全覆盖；管理后台页面（Admin*）不改。
- Agent 名称用 `AgentView.level`（Agent rating）；作者昵称用 `AgentAuthor.level`（作者用户 rating）。
- 黑红名保持首字 `level.color`、其余 `level.accentColor`（空时 `#FF0000`）。
- `RatingLevelName` 的 `level` 可选并支持 `inheritTypography`；`title`/`aria-label`（昵称 + 等级名）必须保留，不能只靠颜色传达等级。
- API 只增字段，不改旧字段语义；无 rating 的用户按 0 计算灰名。
- 活动/贡献榜基础行仍缓存；level 在缓存读取后装饰，rating 变化不要求失效榜单缓存。
- 不新增数据库迁移。
- 不改变 Admin 页面、邮件、CSV 导出、审计日志的展示。

## Review Focus

- **等级来源一致性**：所有前端颜色必须来自服务端 `level`；测试断言 `RatingLevelName` 的 `style.color` 等于 fixture level 的颜色，而不是前端自己算。
- **遗漏的公共昵称**：`grep -R "nickname" apps/web/src/{views,components}` 中除表单/筛选/Admin 外不应再有裸昵称；测试覆盖每个迁移点。
- **Agent 与作者颜色不同源**：Agent 名称用 Agent rating，作者用用户 rating；测试用不同 rating 的 Agent/作者断言两种颜色。
- **黑红名**：首字/剩余字颜色必须分别正确；测试覆盖。
- **空/缺失 level**：旧数据或用户不存在时不能崩溃；后端始终返回 level，前端组件在无 level 时保持默认色。
- **缓存与实时性**：活动榜缓存命中后仍按当前 rating 装饰 level；测试覆盖缓存路径。

---

### Task 1: 公共身份 level 与核心列表 DTO

**Files:**
- Modify: `apps/server/internal/gamification/public.go`
- Modify: `apps/server/internal/gamification/public_test.go`
- Modify: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/problem/stats.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `apps/server/internal/contest/ranklist.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `apps/server/internal/contestapi/handler_test.go`
- Modify: `apps/server/internal/activityapi/handler.go`
- Modify: `apps/server/internal/activityapi/handler_test.go`
- Modify: `apps/server/internal/user/dto.go`
- Modify: `apps/server/internal/userapi/handler_test.go`

**Interfaces:**
- Produces: `gamification.Identity.Level rating.Level`；`Identities` 批量查 `users.rating`。
- Produces: `submission.ListItem.Level`、`problem.SubmissionMetric.Level`、`contest.RankRow.Level`、`activityapi.LeaderboardRow.Level`、`user.MeDTO.Level`、`user.ProfileDTO.Level`。
- Consumes: `rating.LevelFor(int) rating.Level`。

- [ ] **Step 1: 写失败的 identity 测试**

在 `apps/server/internal/gamification/public_test.go` 增加：

```go
func TestIdentitiesIncludesRatingLevel(t *testing.T) {
    // 复用现有测试 env，创建 rating=1500（青名） 和 rating=2600 的用户
    // got := svc.Identities(ctx, []int64{cyanID, blackRedID})
    // assert.Equal(t, "cyan", got[cyanID].Level.Key)
    // assert.Equal(t, "black_red", got[blackRedID].Level.Key)
    // assert.Equal(t, "#FF0000", got[blackRedID].Level.AccentColor)
}
```

使用现有 helper 创建用户；直接更新 `users.rating` 到 2600。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/gamification -run TestIdentitiesIncludesRatingLevel -v`

Expected: FAIL，`Identity` 没有 `Level` 字段或 level 为空。

- [ ] **Step 3: 扩展 `gamification.Identity`**

`apps/server/internal/gamification/public.go`：

```go
import "github.com/kaptree/kapoj/apps/server/internal/rating"

type Identity struct {
    Badges []BadgeSummary `json:"equippedBadges"`
    Title  *TitleSummary  `json:"equippedTitle"`
    Level  rating.Level   `json:"level"`
}
```

在 `Identities` 中批量查询：

```go
var ratings []struct {
    ID     int64
    Rating int
}
if err := s.repo.db.WithContext(ctx).Table("users").
    Select("id, rating").Where("id IN ?", ids).Scan(&ratings).Error; err != nil {
    return nil, err
}
levels := make(map[int64]rating.Level, len(ratings))
for _, r := range ratings {
    levels[r.ID] = rating.LevelFor(r.Rating)
}
```

设置每个 identity 的 `Level`（找不到时用 `rating.LevelFor(0)`）。`emptyIdentity` 也设置灰名 level。

- [ ] **Step 4: 运行 identity 测试确认通过**

Run: `cd apps/server && go test ./internal/gamification -v`

Expected: PASS。

- [ ] **Step 5: 为 SubmissionListItem 写失败测试**

在 `apps/server/internal/submissionapi/handler_test.go` 现有列表测试中，给两个用户设置不同 rating，断言：

```go
item["level"].(map[string]any)["key"] == "cyan"
```

并断言 `equippedBadges`/`equippedTitle` 原有逻辑不变。

- [ ] **Step 6: 给 submission DTO 与 handler 加 level**

`submission/dto.go`：

```go
import "github.com/kaptree/kapoj/apps/server/internal/ratinglevel"

type ListItem struct {
    // ...existing
    Level      ratinglevel.Level `json:"level"`
    AgentLevel ratinglevel.Level `json:"agentLevel"`
}
```

`submissionapi.decorateItems`：

```go
items[i].Level = identity.Level
```

`submission` 查询已 join `agents`，同时 select `ag.rating`；人类提交的 `AgentLevel` 使用 `ratinglevel.LevelFor(0)`（灰名），Agent 提交用 `ratinglevel.LevelFor(agentRating)`。测试覆盖人类提交和 Agent 提交两种。

Run: `cd apps/server && go test ./internal/submissionapi -run TestList -v`

Expected: PASS。

- [ ] **Step 7: 为 SubmissionMetric 写失败测试并实现**

在 `problemapi/handler_test.go` 的 submission-stats 测试中，给用户设置 rating=1500（青名），断言 `level.key == "cyan"`。

`problem/stats.go` 增加 `Level rating.Level`；`problemapi.decorateMetricIdentities` 增加 `items[i].Level = identity.Level`。

Run: `cd apps/server && go test ./internal/problemapi -run TestSubmissionStats -v`

Expected: PASS。

- [ ] **Step 8: 为 ContestRankRow 写失败测试并实现**

在 `contestapi/handler_test.go` ranklist 测试中，给参与者设置 rating=2600，断言 `row["level"].(map[string]any)["key"] == "black_red"`。

`contest/ranklist.go` 增加 `Level rating.Level`；`contestapi` ranklist 装饰循环增加 `list.Rows[i].Level = identity.Level`。

Run: `cd apps/server && go test ./internal/contestapi -run TestRanklist -v`

Expected: PASS。

- [ ] **Step 9: 为活动/分享/富豪榜写失败测试并实现**

在 `activityapi/handler_test.go` 三个榜单测试中，给用户设置 rating=1500（青名）/2600，断言每行 `level.key`。

`activityapi/handler.go` 的 `LeaderboardRow` 增加：

```go
Level rating.Level `json:"level"`
```

装饰循环增加 `rows[i].Level = identity.Level`。

Run: `cd apps/server && go test ./internal/activityapi -v`

Expected: PASS。

- [ ] **Step 10: 为 Me/Profile 写失败测试并实现**

`user/dto.go` 增加 `Level rating.Level`，`ToMe`/`ToProfile` 设置 `rating.LevelFor(u.Rating)`。`userapi/handler_test.go` 断言 `/users/me` 和 `/users/:id` 返回 level。

Run: `cd apps/server && go test ./internal/userapi -v`

Expected: PASS。

- [ ] **Step 11: 运行相关包并提交**

Run:

```bash
cd apps/server
go test ./internal/gamification ./internal/submissionapi ./internal/problemapi ./internal/contestapi ./internal/activityapi ./internal/userapi
```

Expected: 全部 PASS。

```bash
git add apps/server/internal
git commit -m "feat(rating): expose rating levels on core public summaries"
```

---

### Task 2: 其余公共用户摘要与 Agent 等级

**Files:**
- Modify: `apps/server/internal/agent/service.go`
- Modify: `apps/server/internal/agentapi/handler_test.go`
- Modify: `apps/server/internal/learning/dto.go`
- Modify: `apps/server/internal/learning/repository.go`
- Modify: `apps/server/internal/learning/path_service_test.go`
- Modify: `apps/server/internal/problem/discussion.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `apps/server/internal/social/model.go`
- Modify: `apps/server/internal/social/follow.go`
- Modify: `apps/server/internal/social/notification.go`
- Modify: `apps/server/internal/social/evaluation.go`
- Modify: `apps/server/internal/social/*_test.go`（覆盖 level）
- Modify: `apps/server/internal/certificate/dto.go`
- Modify: `apps/server/internal/certificate/repository.go`
- Modify: `apps/server/internal/certificate/service.go`
- Modify: `apps/server/internal/certificate/*_test.go`
- Modify: `apps/server/internal/agentapi/agent_handler_test.go`

**Interfaces:**
- Consumes: Task 1 的 `rating.LevelFor`。
- Produces: `agent.AgentView.Level`、`agent.AgentAuthor.Level`、`learning.UserSummary.Level`、`problem.AuthorSummary.Level`、`social.UserSummary.Level`、`certificate.PublicCertificateView.Level`。

- [ ] **Step 1: Agent 测试先失败**

在 `agentapi` 测试中创建 Agent rating=2600、作者 rating=1500（青名），断言：

```go
agentData["level"].(map[string]any)["key"] == "black_red"
author["level"].(map[string]any)["key"] == "cyan"
```

Run: `cd apps/server && go test ./internal/agentapi -run TestPublic -v`

Expected: FAIL。

- [ ] **Step 2: 给 AgentView/AgentAuthor 加 level**

`agent/service.go`：

```go
import "github.com/kaptree/kapoj/apps/server/internal/rating"

type AgentAuthor struct {
    ID       int64        `json:"id"`
    Username string       `json:"username"`
    Nickname string       `json:"nickname"`
    Level    rating.Level `json:"level"`
}

type AgentView struct {
    // ...existing
    Level  rating.Level `json:"level"`
    Author AgentAuthor  `json:"author"`
}
```

列表查询增加 `u.rating AS author_rating`；`agentListRow` 增加 `AuthorRating int`；`view()` 设置 `Level: rating.LevelFor(r.Rating)` 和 `Author.Level: rating.LevelFor(r.AuthorRating)`。

`buildAgentView` 查询作者时 `Select("id, username, nickname, rating")`，设置作者 level；AgentView 设置 Agent level。

Run: `cd apps/server && go test ./internal/agentapi ./internal/agent -v`

Expected: PASS。

- [ ] **Step 3: 学习路线作者测试先失败**

在 `learning/path_service_test.go` 给作者 rating=2600，断言 `official.Author.Level.Key == "black_red"`。

Run: `cd apps/server && go test ./internal/learning -run TestPath -v`

Expected: FAIL。

- [ ] **Step 4: 给 learning.UserSummary 加 level**

`learning/dto.go` 增加 `Level rating.Level`；`pathItemColumns` 增加 `u.rating AS author_rating`；`pathRow` 增加 `AuthorRating int`；`item()` 设置 `Level: rating.LevelFor(r.AuthorRating)`。

Run: `cd apps/server && go test ./internal/learning -v`

Expected: PASS。

- [ ] **Step 5: 题目作者/讨论作者测试先失败**

在 `problem`/`problemapi` 测试中给作者 rating=1500（青名），断言 `author["level"].(map[string]any)["key"] == "cyan"`；讨论回复作者同理。

Run: `cd apps/server && go test ./internal/problem ./internal/problemapi -run 'Author|Discussion|Solution' -v`

Expected: FAIL。

- [ ] **Step 6: 给 problem.AuthorSummary 加 level**

`problem/discussion.go` 的 `AuthorSummary` 增加 `Level rating.Level`；`authors` 查询 `id, username, nickname, rating` 并设置 level。

`problem/repository.go` 的 `FindAuthor` 查询 `id, username, nickname, rating` 并设置 level。

Run: `cd apps/server && go test ./internal/problem ./internal/problemapi -v`

Expected: PASS。

- [ ] **Step 7: Social 摘要测试先失败**

在 `social/follow_test.go`、`notification_test.go`、`evaluation_test.go` 创建 rating=2600 的用户，断言 `UserSummary.Level.Key == "black_red"`。

Run: `cd apps/server && go test ./internal/social -v`

Expected: FAIL。

- [ ] **Step 8: 给 social.UserSummary 加 level**

`social/model.go` 增加 `Level rating.Level`；在 `followUsers`、`ListNotifications`、`evaluationRow.item()` 构造 `UserSummary` 时设置 `Level: rating.LevelFor(r.Rating)`。其他构造点用 `rg -n "UserSummary{"` 检查。

Run: `cd apps/server && go test ./internal/social -v`

Expected: PASS。

- [ ] **Step 9: 证书核验测试先失败**

在 certificate 测试中给持有者 rating=1500（青名），断言 `view.RatingLevel.Key == "cyan"`；无效验证码时 `RatingLevel == nil` 且不影响 `Valid=false`；`view.Level`（证书等级）保持原字符串。

Run: `cd apps/server && go test ./internal/certificate -run Verify -v`

Expected: FAIL。

- [ ] **Step 10: 证书核验 level**

`certificate/dto.go` 的 `PublicCertificateView` 增加 `RatingLevel *ratinglevel.Level `json:"ratingLevel"``；保留原有 `Level string`。

`certificate/repository.go` 的 `FindByVerifyCode` 返回 `(*Certificate, string, int, error)`，查询 select `u.nickname, u.rating`；更新接口、`GormRepository` 实现、测试 mock。

`certificate/service.go` 的 `Verify` 接收 rating 并设置 `RatingLevel: ptr(ratinglevel.LevelFor(userRating))`；无效码返回 `Valid:false` 时 `RatingLevel=nil`。

Run: `cd apps/server && go test ./internal/certificate -v`

Expected: PASS。

- [ ] **Step 11: 运行相关包并提交**

```bash
cd apps/server
go test ./internal/agent ./internal/agentapi ./internal/learning ./internal/problem ./internal/problemapi ./internal/social ./internal/certificate
```

Expected: 全部 PASS。

```bash
git add apps/server/internal
git commit -m "feat(rating): expose levels on agents, learning, social and certificates"
```

---

### Task 3: OpenAPI 契约与生成类型

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`（通过 `corepack pnpm gen:api`）

**Interfaces:**
- Consumes: Tasks 1–2 的 JSON 字段。
- Produces: 前端 `level` 类型可用。

- [ ] **Step 1: 更新 schema**

在以下 schema 增加 `level: { $ref: '#/components/schemas/RatingLevel' }`，并把 `level` 加入 `required`：

- `SubmissionListItem`
- `SubmissionMetric`
- `ContestRankRow`
- `ActivityLeaderboardRow`
- `SharingLeaderboardRow`
- `WealthLeaderboardRow`
- `Me`
- `Profile`
- `AgentAuthor`
- `AgentView`
- `SocialUserSummary`
- Learning path 的用户摘要（`author` 内联或 `UserSummary` schema）
- `AuthorSummary`（题目/题解/讨论）
- `PublicCertificateView`（`valid=true` 时返回；schema 中可设为必填，服务端无效时零值）
- `NotificationItem.actor`（引用 `SocialUserSummary` 时自动生效）

- [ ] **Step 2: 生成前端类型**

Run: `corepack pnpm gen:api`

Expected: `apps/web/src/api/schema.d.ts` 出现对应 `level: components["schemas"]["RatingLevel"]`。

- [ ] **Step 3: 验证生成可重复**

Run: `corepack pnpm gen:api && git diff --exit-code -- apps/web/src/api/schema.d.ts`

Expected: 第二次生成无差异（第一次 diff 已提交前检查）。

- [ ] **Step 4: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(api): expose rating levels on public user summaries"
```

---

### Task 4: 前端核心列表与四个排行榜

**Files:**
- Modify: `apps/web/src/views/SubmissionsView.vue`
- Modify: `apps/web/src/components/SubmissionDetailDrawer.vue`
- Modify: `apps/web/src/views/ProblemStatisticsView.vue`
- Modify: `apps/web/src/components/ContestRanklist.vue`
- Modify: `apps/web/src/views/LeaderboardView.vue`
- Modify: corresponding `.spec.ts` files

**Interfaces:**
- Consumes: Task 3 生成的 `level` 类型；现有 `RatingLevelName`/`UserIdentityName`。
- Produces: 测评/统计/ranklist/四榜昵称颜色。

- [ ] **Step 1: 写失败测试**

在 `SubmissionsView.spec.ts`、`ProblemStatisticsView.spec.ts`、`ContestRanklist.spec.ts`、`LeaderboardView.spec.ts` 的 fixture 中加入 `level`，断言：

```ts
expect(wrapper.find('.rating-level-name').attributes('style')).toContain('color: rgb(3, 168, 158)')
```

对黑红名 fixture 断言首字/剩余字两个 span 的颜色；对活动/富豪/贡献榜也断言传入 level（例如 cyan）。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- SubmissionsView.spec.ts ProblemStatisticsView.spec.ts ContestRanklist.spec.ts LeaderboardView.spec.ts`

Expected: FAIL，昵称没有 `rating-level-name` 或颜色不对。

- [ ] **Step 3: 传 level**

- `SubmissionsView` / `SubmissionDetailDrawer` / `ProblemStatisticsView` / `ContestRanklist`：给 `UserIdentityName` 增加 `:rating-level="data.level"`（详情用 `detail.level`）；Agent 名称用 `RatingLevelName :level="data.agentLevel"`（详情 `detail.agentLevel`），人类提交时 agentName 为空。
- `LeaderboardView` 卡片和表格：`:rating-level="row.level"`、`:rating-level="data.level"`，删除 `board.key === 'rating'` 条件。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web test -- SubmissionsView.spec.ts ProblemStatisticsView.spec.ts ContestRanklist.spec.ts LeaderboardView.spec.ts`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/SubmissionsView.vue apps/web/src/components/SubmissionDetailDrawer.vue \
  apps/web/src/views/ProblemStatisticsView.vue apps/web/src/components/ContestRanklist.vue \
  apps/web/src/views/LeaderboardView.vue apps/web/src/**/*.spec.ts
git commit -m "feat(web): color nicknames on submissions and leaderboards"
```

---

### Task 5: 前端主页、个人页、Agents 与其餘公共昵称

**Files:**
- Modify: `apps/web/src/components/home/HomeProfileCard.vue`
- Modify: `apps/web/src/views/HomeView.vue`
- Modify: `apps/web/src/components/UserMenu.vue`
- Modify: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/components/social/FollowListDialog.vue`
- Modify: `apps/web/src/views/AgentsView.vue`
- Modify: `apps/web/src/views/AgentDetailView.vue`
- Modify: `apps/web/src/views/AgentSettingsView.vue`
- Modify: `apps/web/src/components/learning/LearningPathSummary.vue`
- Modify: `apps/web/src/views/LearningPathListView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/views/ProblemSolutionsView.vue`
- Modify: `apps/web/src/views/ProblemSolutionDetailView.vue`
- Modify: `apps/web/src/views/NotificationsView.vue`
- Modify: `apps/web/src/views/CertificateVerifyView.vue`
- Modify: corresponding specs

**Interfaces:**
- Consumes: Task 3 的 `level` 字段；`RatingLevelName`。
- Produces: 剩余公共页面昵称颜色；Agent 名称与作者分别按各自 rating 上色。

- [ ] **Step 1: 写失败测试**

至少覆盖：

- `AgentsView.spec.ts`：Agent rating=2600 → Agent 名称黑红；作者 rating=1500（青名） → 作者青名。
- `HomeView.spec.ts` / `UserMenu.spec.ts` / `UserProfileView.spec.ts`：`Me`/`Profile` level 渲染颜色。
- `FollowListDialog.spec.ts`：粉丝 level 渲染颜色。
- `LearningPathSummary.spec.ts` / `LearningPathListView.spec.ts`：作者 level。
- `ProblemDetailView.spec.ts` / `ProblemSolutionsView.spec.ts` / `ProblemSolutionDetailView.spec.ts`：作者 level。
- `NotificationsView.spec.ts`：actor level。
- `CertificateVerifyView.spec.ts`：核验昵称 level。

断言使用 `RatingLevelName` 的 `style.color` 或 `title`，不要断言完整 DOM 字符串。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- AgentsView.spec.ts HomeView.spec.ts UserMenu.spec.ts UserProfileView.spec.ts FollowListDialog.spec.ts LearningPathSummary.spec.ts LearningPathListView.spec.ts ProblemDetailView.spec.ts ProblemSolutionsView.spec.ts ProblemSolutionDetailView.spec.ts NotificationsView.spec.ts CertificateVerifyView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 替换裸昵称**

- `HomeProfileCard`：`<h2>` 内用 `RatingLevelName :name="user.nickname" :level="user.level" inherit-typography`。
- `HomeView`：问候语拆成 `{{ greeting }}，` + `RatingLevelName`（`auth.user.level`）；主页 Agent 名称同样用 `RatingLevelName`（数据有 Agent rating/level 时）；无登录用户保持“同学”。
- `UserMenu`：触发器 `.user-menu__name` 和下拉 `strong` 用 `RatingLevelName`。
- `UserProfileView`：`h1` 内用 `RatingLevelName :name="displayName" :level="profile.level" inherit-typography`。
- `FollowListDialog`：昵称用 `RatingLevelName :name="displayName(user)" :level="user.level"`。
- `AgentsView`：Agent 名称用 `RatingLevelName :name="data.name" :level="data.level"`；作者用 `RatingLevelName :name="data.author.nickname || data.author.username" :level="data.author.level"`。
- `AgentDetailView`/`AgentSettingsView`：同上（作者）。
- `LearningPathSummary`/`LearningPathListView`：作者用 `RatingLevelName`。
- `ProblemDetailView`：作者链接内用 `RatingLevelName`。
- `ProblemSolutionsView`/`ProblemSolutionDetailView`：作者/回复作者用 `RatingLevelName`。
- `NotificationsView`：actor 名称用 `RatingLevelName`；无 actor 保持“未知用户”。
- `CertificateVerifyView`：昵称用 `RatingLevelName`。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2。

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src
git commit -m "feat(web): color nicknames across public pages"
```

---

### Task 6: E2E 断言、文档与全量验证

**Files:**
- Modify: `apps/web/e2e/core-flow.spec.ts`
- Modify: `运行手册.md`（或 `README.md`）加一段“昵称颜色”说明
- No other code unless verification finds a defect

**Interfaces:**
- Consumes: Tasks 1–5。

- [ ] **Step 1: E2E 断言昵称等级**

在 `core-flow.spec.ts` AC 行断言后，增加：

```ts
await expect(row.locator('.rating-level-name')).toHaveAttribute('title', /青名/)
```

（e2e 用户初始 rating=1500（青名）；若等级边界改动，按服务端定义调整。）

- [ ] **Step 2: 文档说明**

在运行手册的用户/榜单章节加一句：昵称颜色由 rating 等级决定，Agent 名称由 Agent rating 决定，等级名称保留在 `title`/`aria-label` 中，颜色不是唯一信息。

- [ ] **Step 3: 后端全量测试**

Run:

```bash
cd apps/server
go test ./...
```

Expected: PASS。若本机缺少 Docker/Postgres 导致集成测试跳过或失败，记录具体包和原因；至少确保本次修改涉及的包全部通过。

- [ ] **Step 4: 前端测试与构建**

Run:

```bash
corepack pnpm --filter web test
corepack pnpm lint:web
corepack pnpm build:web
```

Expected: 全部 PASS。

- [ ] **Step 5: e2e**

Run:

```bash
corepack pnpm test:e2e:core
```

Expected: 1 passed；若 e2e 栈不可用，记录具体失败并至少完成前端单测。

- [ ] **Step 6: 遗漏扫描**

Run:

```bash
grep -R "nickname" apps/web/src/views apps/web/src/components --include='*.vue' \
  | grep -v -E 'Admin|filters|query|form|errors|validate|fields|Preview|Register|ProfileEdit'
```

Expected: 只剩 `UserIdentityName`/`RatingLevelName`、表单/筛选/Admin 相关；所有公共展示点已迁移。

- [ ] **Step 7: 提交**

```bash
git add apps/web/e2e/core-flow.spec.ts 运行手册.md
git commit -m "test(e2e): assert nickname rating color and document it"
```

---

## 自检记录

- 规格覆盖：设计文档 5 节覆盖清单的每一行都落在 Task 1/2/4/5。
- 接口一致性：所有 `level` 字段统一为 `rating.Level` / OpenAPI `RatingLevel`；Agent 名称与作者分别来自 `AgentView.level` 和 `AgentAuthor.level`。
- Review Focus 每条都有对应测试：核心列表、遗漏扫描、Agent/作者异源、黑红名、空 level、缓存。
- 无数据库迁移；API 只增字段。
