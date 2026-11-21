# 昵称 Rating 颜色同步设计

日期：2026-11-21
状态：待批准
关联：用户要求「公共页面所有用户昵称颜色与 rating 定义同步；Agent 名称按 Agent rating 上色；管理后台除外」

## 1. 目标

公共页面中所有用户昵称统一显示服务端 rating 等级颜色；Agent 列表/详情中的 Agent 名称使用 Agent 自己的 rating 等级颜色，Agent 作者昵称使用作者用户的 rating 等级颜色。

颜色唯一来源是后端 `apps/server/internal/rating/level.go` 的 `LevelFor(rating)`，前端不再复制色阶。

## 2. 非目标

- 不改管理后台页面（用户管理、证书审核、Agent 管理、学习路线管理、审计等）。
- 不改邮件、CSV 导出、审计日志等非页面文本。
- 不改 rating 等级边界或色值。
- 不把颜色作为唯一信息：`RatingLevelName` 继续保留 `title`/`aria-label` 中的等级名称。
- 不改变现有黑红名「首字黑、其余红」的展示规则。

## 3. 现状

- `RatingLevelName.vue` 已实现等级颜色和黑红名；`UserIdentityName.vue` 已接受 `ratingLevel`，但只有 Rating 榜传入。
- 后端 `rating.Level` 只出现在 `RatingLeaderboardRow`；其他用户摘要没有 `level` 字段。
- 活动/富豪/贡献榜、测评记录、Agents、主页/个人菜单/个人主页等仍显示默认文字颜色。
- 多个公共页面仍是裸昵称文本：Agents 作者、主页昵称、用户菜单、个人主页、粉丝列表、学习路线作者、题目作者、题解/讨论作者、通知 actor、证书核验。

## 4. 方案选择

### A. 服务端统一返回 level（采用）

扩展公共身份装饰器，所有用户摘要带 `level`；Agent 摘要额外带 Agent 自己的 `level`。前端统一用 `RatingLevelName`。

- 优点：色值只有一份；批量查询；活动榜缓存不受影响；新增页面只要用同一字段。
- 缺点：DTO、OpenAPI schema、前端渲染点较多。

### B. 前端按 rating 计算

后端只补 `rating`，前端复制 `LevelFor`。

- 缺点：色阶和边界双份维护，易漂移；不采用。

### C. 前端批量查询 level

新增 `/users/levels?ids=`。

- 缺点：额外请求、缓存/失败态复杂，容易漏页面；不采用。

## 5. 覆盖清单

| 页面/区域 | 后端摘要 | 前端渲染点 |
|---|---|---|
| 测评记录列表 | `SubmissionListItem` + `level` | `SubmissionsView` → `UserIdentityName` |
| 测评详情抽屉 | `SubmissionListItem` + `level` | `SubmissionDetailDrawer` |
| 题目提交统计 | `SubmissionMetric` + `level` | `ProblemStatisticsView` |
| Rating 榜 | 已有 `RatingLeaderboardRow.level` | `LeaderboardView`（保持） |
| 活跃榜 | `ActivityLeaderboardRow` + `level` | `LeaderboardView` 全部榜单传 level |
| 富豪榜 | `WealthLeaderboardRow` + `level` | 同上 |
| 贡献榜 | `SharingLeaderboardRow` + `level` | 同上 |
| 比赛 Ranklist | `ContestRankRow` + `level` | `ContestRanklist` |
| Agents 列表 | `AgentView.level`（Agent rating）+ `AgentAuthor.level`（作者 rating） | Agent 名称 `RatingLevelName`；作者昵称 `RatingLevelName` |
| Agent 详情 | 同上 | `AgentDetailView` |
| Agent 设置页作者 | `AgentAuthor.level` | `AgentSettingsView` |
| 主页个人卡 | `Me.level` | `HomeProfileCard` |
| 主页问候语 | `Me.level` | `HomeView` |
| 顶部用户菜单 | `Me.level` | `UserMenu` |
| 个人主页 | `Profile.level` | `UserProfileView` |
| 粉丝/关注列表 | `social.UserSummary.level` | `FollowListDialog` |
| 学习路线作者 | `learning.UserSummary.level` | `LearningPathSummary`、`LearningPathListView` |
| 题目作者 | `problem.AuthorSummary.level` | `ProblemDetailView` |
| 题解/讨论作者 | `problem.AuthorSummary.level` | `ProblemSolutionsView`、`ProblemSolutionDetailView`、官方题解页（如有作者） |
| 通知 actor | `social.UserSummary.level` | `NotificationsView` |
| 证书核验 | `PublicCertificateView.level` | `CertificateVerifyView` |
| 用户评价作者 | `social.UserSummary.level` | 当前评价弹窗不展示作者；API 保持一致 |

明确排除：管理后台列表、比赛报名管理弹窗（`CanReadParticipants` 管理员）、ContestSimilarity 管理面板、邮件/导出。

## 6. 后端设计

### 6.1 公共身份装饰器

`gamification.Identity` 增加：

```go
Level rating.Level `json:"level"`
```

`Service.Identities` 在现有徽章/称号批量查询之外，再批量查询 `users.id, rating`，为每个用户设置 `Level = rating.LevelFor(rating)`。该查询与徽章/称号一样按用户 ID 去重；用户不存在时保持零值（灰名），但正常调用不会传入不存在的用户。

这会自动让所有已经在用 `IdentityLookup` 的接口具备 level 能力：

- `ratingapi`（已有 level，可继续使用 base row 的 level）
- `activityapi`（3 个榜单）
- `submissionapi`
- `contestapi`
- `problemapi`（提交统计）
- `userapi`（Profile；Me 直接计算）

### 6.2 DTO 字段

为以下结构增加 `Level rating.Level json:"level"`，并在构造时赋值：

- `submission.ListItem`：`decorateItems` 从 `identity.Level` 赋值。
- `problem.SubmissionMetric`：`decorateMetricIdentities` 从 `identity.Level` 赋值。
- `contest.RankRow`：`contestapi` 装饰时赋值。
- `activityapi.LeaderboardRow`：装饰时从 `identity.Level` 赋值；`activity.LeaderboardRow` 基础缓存行不变。
- `user.MeDTO` / `user.ProfileDTO`：`ToMe`/`ToProfile` 直接 `rating.LevelFor(u.Rating)`。
- `agent.AgentView`：`Level = rating.LevelFor(a.Rating)`。
- `agent.AgentAuthor`：列表查询增加 `u.rating AS author_rating`，`buildAgentView` 查询作者 rating，赋值作者 level。
- `learning.UserSummary`：`pathItemColumns` 增加 `u.rating AS author_rating`，`pathRow.item()` 赋值。
- `problem.AuthorSummary`：`FindAuthor` 查询 `rating`；`DiscussionService.authors` 查询 `rating`；赋值 level。
- `social.UserSummary`：粉丝/关注、通知、评价查询已经 select `rating`，构造时赋值 `Level`。
- `certificate.PublicCertificateView`：证书核验查询用户 rating 并赋值 level；不改管理员证书视图。

### 6.3 OpenAPI

在 `packages/api/openapi.yaml` 对应 schema 增加可空/必填 `level` 引用 `#/components/schemas/RatingLevel`：

`SubmissionListItem`、`SubmissionMetric`、`ContestRankRow`、`ActivityLeaderboardRow`、`SharingLeaderboardRow`、`WealthLeaderboardRow`、`Me`、`Profile`、`AgentView`、`AgentAuthor`、`LearningPath UserSummary`、`AuthorSummary`、`SocialUserSummary`、`NotificationItem`（actor 内嵌）、`PublicCertificateView`。

保持向后兼容：新增字段只增不改；`level` 在用户摘要中必填，服务端始终返回（无 rating 的用户按 0 计算灰名）。

运行 `corepack pnpm gen:api` 更新 `apps/web/src/api/schema.d.ts`。

## 7. 前端设计

### 7.1 复用现有组件

- `RatingLevelName`：昵称 + level，负责普通颜色和黑红名首字/剩余字。
- `UserIdentityName`：已有 `ratingLevel` prop；测评、榜单、统计、ranklist 直接传 `:rating-level="...level"`。
- Agent 名称、裸昵称、个人主页标题、主页问候、用户菜单等没有徽章/称号上下文的，直接用 `RatingLevelName`。

### 7.2 各页面改法

- `SubmissionsView`、`SubmissionDetailDrawer`、`ProblemStatisticsView`、`ContestRanklist`：`UserIdentityName` 增加 `:rating-level`。
- `LeaderboardView`：卡片和完整表格都改为 `:rating-level="row.level"` / `data.level`，删除 `board.key === 'rating'` 限制。
- `AgentsView`：Agent 名称用 `<RatingLevelName :name="data.name" :level="data.level" />`；作者用 `<RatingLevelName :name="data.author.nickname || data.author.username" :level="data.author.level" />`。
- `AgentDetailView`、`AgentSettingsView`：同上。
- `HomeProfileCard`：`<h2>` 内用 `RatingLevelName`（`user.level`）。
- `HomeView`：问候语拆成「问候，」+ `RatingLevelName`，不再用纯字符串 `greetingText`。
- `UserMenu`：触发器名称和下拉摘要昵称用 `RatingLevelName`（`auth.user.level`）。
- `UserProfileView`：`profile-hero__title` 内用 `RatingLevelName`（`profile.level`）。
- `FollowListDialog`：昵称用 `RatingLevelName`（`user.level`）。
- `LearningPathSummary`、`LearningPathListView`：作者用 `RatingLevelName`（`author.level`）。
- `ProblemDetailView`：作者链接内用 `RatingLevelName`（`author.level`）。
- `ProblemSolutionsView`、`ProblemSolutionDetailView`：作者/回复作者用 `RatingLevelName`（`author.level`）。
- `NotificationsView`：actor 名称用 `RatingLevelName`（`actor.level`）。
- `CertificateVerifyView`：昵称用 `RatingLevelName`（`result.level`）。

### 7.3 样式与可访问性

- 不新增全局颜色类；颜色只由 `RatingLevelName` 内联 style 提供。
- 标题/链接现有 font-weight、hover、布局由外层 class 保持；`RatingLevelName` 的 span 设 `white-space: nowrap`，长昵称需要 `overflow-wrap` 的地方在外层处理。
- `RatingLevelName` 已有 `title`/`aria-label`（含等级名称），不改变。
- 黑红名保持现有首字黑色、其余红色逻辑；暗色主题下沿用现有组件行为。

## 8. 性能与缓存

- `Identities` 新增一次 `users.id, rating` 批量查询；已有徽章/称号各一次，总计仍为常数次，不随行数增长。
- 活动/贡献榜基础行仍缓存 60 秒；level 在缓存后按当前 rating 装饰，rating 变化不需要失效榜单缓存。
- Agent、学习路线、题目作者、Social 查询把 `u.rating` 加到现有 JOIN/SELECT，不产生 N+1。
- 证书核验只查单个用户 rating。

## 9. 测试

### 后端

- `gamification/public_test.go`：`Identities` 返回正确 level；黑红名边界和灰名。
- `submissionapi/handler_test.go`：列表和详情带 level；装饰不改变隐藏逻辑。
- `contestapi/handler_test.go`：ranklist 每行带作者 level。
- `activityapi/handler_test.go`：活动/分享/富豪榜每行带 level；缓存命中后仍装饰。
- `problemapi/handler_test.go`：提交统计带 level。
- `userapi/handler_test.go`：Me/Profile 带 level。
- `agent`/`agentapi` 测试：AgentView.level 来自 Agent rating，author.level 来自作者 users.rating。
- `learning`、`social`、`problem`、`certificate` 测试：作者/actor/核验 level。
- `go test ./...`（至少相关包）。

### 前端

- `UserIdentityName.spec.ts`：已有 ratingLevel 测试保持；新增“非 Rating 榜页面传入 level 时渲染等级颜色”。
- 各页面已有测试增加断言：传入 fixture level 后，`.rating-level-name` 的 style color 等于 level.color；黑红名首字/剩余字颜色。
- Agents 测试：Agent 名称使用 Agent level，作者使用 author level。
- Home/UserMenu/UserProfile/Follow/Learning/Problem/Solutions/Notifications/Certificate 测试至少各覆盖一个昵称颜色。
- `corepack pnpm test:web`、`corepack pnpm lint:web`、`corepack pnpm build:web`。

### 端到端

- 现有 e2e 用户 rating 1500（服务端等级颜色已知）时，在测评记录和个人主页断言 `.rating-level-name` 的颜色；不硬编码 3 金币等无关值。
- 不新增管理后台断言。

## 10. 风险与回滚

- 新增字段是向后兼容的；旧前端忽略 `level`。
- 若某页面遗漏 level，昵称回落为默认颜色；通过覆盖清单和测试逐页检查。
- 不改数据库，无需迁移；回滚前端/后端提交即可。
- `gamification` 新增对 `rating` 包的依赖，但 `rating` 不反向依赖 `gamification`，无循环导入。
- 黑红名在暗色背景的可见性沿用现有组件；如后续需要，可单独调整组件对比度，不在本设计内改色值。
