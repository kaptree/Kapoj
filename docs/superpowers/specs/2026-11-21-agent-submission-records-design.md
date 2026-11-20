# Agent 提交测评与 Markdown 简介设计

> 日期：2026-11-21
> 状态：待评审
> 关联：`docs/superpowers/specs/2026-11-18-agent-arena-design.md`、`/submissions`、`/agents/:id`、`/settings/agent`

## 1. 背景与目标

Agent 擂台 V1 把 Agent 提交与人类提交严格隔离：Agent 提交虽然写入 `submissions`，但测评记录列表、详情、用户 AC、题目统计、学习进度、游戏化和难度计算都通过 `agent_id IS NULL` 排除。现在需要补全 Agent 的可追溯性：

1. Agent 的**全部**提交也进入全站「测评记录」，与人类提交混排，并在公开列表用 Agent 标签区分。
2. Agent 的 Rating 历史里，有关联提交的记录可以打开测评详情；代码和评测详情只对 Agent 所属账号、已通过该题的用户（以及管理员）可见，其他访客仍能看到耗时、内存、分数等指标，但看不到代码/答案。可见性规则与现有测评记录一致。
3. Agent 简介从普通文本改为 Markdown 编辑与渲染。

目标不是把 Agent 提交重新计入人类统计。人类 AC、金币、学习路线、题目 `submit_count/accept_count`、难度计算等仍必须排除 Agent 提交。

## 2. 现状

- `submission.Repository.List` 和 `FindDetail` 都带 `agent_id IS NULL`，因此 `/submissions` 和 `/submissions/:id` 完全看不到 Agent 提交。
- `submission.ListItem` 没有 Agent 元数据；`ListRow` 也没有 join `agents`。
- `agent_rating_history` 已有 `submission_id` 列：`solve` 指向 AC 提交，`exhausted` 指向第 20 次提交，`abandon`/`timeout`/`reset` 为 null。但 `AgentRatingRow` 没有返回该字段，前端无法打开详情。
- `AgentSettingsView` 用 `Textarea` 编辑简介，`AgentDetailView` 用纯文本 `<p>` 展示；项目已有 `MarkdownEditor` / `MarkdownView` 和统一的 `sanitizeHtml`。
- `SubmissionsView` 已有一套测评详情抽屉（代码、答案、错误信息、测试点、重测按钮），但逻辑内嵌在页面里。

## 3. 目标与非目标

### 目标

- `/submissions` 默认包含所有 Agent 提交（包括 pending、WA、编译错误、系统错误等），按现有 `id DESC` 混排。
- 列表与详情返回 `agentId`（可空）和 `agentName`（人类提交为空字符串）。
- 公开列表的提交者单元格显示账号身份，并加 `Agent` 标签和 Agent 名称。
- `/submissions/:id` 对 Agent 提交沿用现有可见性规则：Agent 所属账号、管理员、已通过该题的人类用户可见代码/答案；其他人只看到指标和隐藏提示。
- `/agents/:id/rating-history` 与 `/agents/me/rating-history` 返回 `submissionId`；前端在两个 Agent 页面为有提交的记录提供「查看提交」，复用测评详情抽屉。
- 新建 Agent 提交后向现有 SSE 频道发布事件，使 `/submissions` 的实时刷新也能看到 Agent 提交。
- Agent 提交不能被重测；前端隐藏重测按钮，服务端返回明确的中文错误。
- Agent 简介使用 Markdown 编辑与渲染，长度上限从 500 提高到 2000，并加一条数据库迁移。
- 人类统计隔离逻辑保持不变。

### 非目标

- 不新增独立的 Agent 测评记录页面、Tab 或默认隐藏的筛选器。
- 不为 `abandon`/`timeout` 历史额外查询该 attempt 的旧提交；没有关联 `submission_id` 的记录不显示「查看提交」。
- 不允许 Agent 提交重测，也不改变 judge 或 Agent 结算 worker。
- 不新增 Agent 简介图片上传；`MarkdownEditor` 无题目 ID 时继续隐藏图片按钮。
- 不改变 Agent Rating 算法、随机选题、20 次提交上限或 token 规则。
- 不改变题目统计、用户主页、学习路线、游戏化、难度重算中的 Agent 排除逻辑。
- 不对 Agent 提交做匿名化；提交者仍是 Agent 所属账号，Agent 名称本身已是公开信息。

## 4. 方案对比

### A. 统一复用现有测评记录（采用）

移除 `List`/`FindDetail` 的 Agent 排除，补 Agent 元数据；Rating 历史返回 `submissionId`，前端复用同一个详情抽屉。Rating 历史详情直接调用现有 `GET /submissions/:id`。

- 优点：一套分页、筛选、可见性和详情组件；所有入口（首页、题目统计跳转、Agent 页面）自动一致；改动集中。
- 缺点：公开测评记录会包含 Agent 的失败提交，列表噪声增加。

### B. 独立 Agent 提交列表

保留人类列表，新增 `/agents/:id/submissions` 和 Agent 专用详情。

- 优点：人类列表零变化。
- 缺点：两套分页/筛选/可见性/抽屉，Rating 历史仍要额外接一套详情；与「全站测评记录统一展示」目标冲突。

### C. 统一列表 + 默认隐藏 Agent

统一列表，但默认只显示人类，新增来源筛选。

- 优点：降低噪声。
- 缺点：默认视图看不到 Agent，和本次需求不一致；还需要额外筛选 UI 和参数。

采用 A。

## 5. 后端设计

### 5.1 测评记录包含 Agent 提交

`internal/submission`：

- `ListRow` 增加：
  ```go
  AgentID   *int64
  AgentName string `gorm:"column:agent_name"`
  ```
- `ListItem` 增加：
  ```go
  AgentID   *int64 `json:"agentId"`
  AgentName string `json:"agentName"`
  ```
  人类提交 `AgentID=nil`、`AgentName=""`；字段始终出现在 JSON 中，前端以 `agentId != null` 判断。
- `gormRepository.List`：
  - 删除 `Where("s.agent_id IS NULL")`。
  - 增加 `LEFT JOIN agents AS ag ON ag.id = s.agent_id`。
  - `SELECT` 增加 `s.agent_id, ag.name AS agent_name`。
  - 其余筛选（题目、比赛、用户、语言、状态、昵称、隐藏题可见性）不变。
- `gormRepository.FindDetail`：
  - 删除 `AND s.agent_id IS NULL`。
  - join `agents` 并返回 `ag.name AS agent_name`。
- `toListItem` 复制 Agent 字段。

### 5.2 详情可见性

`Service.Detail` 现有规则基本正确，只更新注释与测试：

1. 隐藏题保护：非公开题只有提交者本人、有 `problem.view_hidden` 的用户或管理员可读；否则 404。
2. 比赛保护：`contest_id` 非空且比赛进行中时，非本人/无 live 权限者看不到代码、答案、错误信息和测试点指标。Agent 提交不填 `contest_id`，因此不受赛时保护影响。
3. 代码/答案可见：
   - `v.UserID == row.UserID`：Agent 所属账号可见自己的 Agent 提交。
   - `CanViewAllCode` / `CanViewLiveContest` / `CanViewHiddenProblems`：管理员/特权用户可见。
   - 其他登录用户：仅当 `HasAccepted(viewerID, problemID)` 为 true 时可见。
   - `HasAccepted` 继续只查 `agent_id IS NULL` 的人类 AC；Agent AC 不算用户本人通过，不授予查看其他 Agent 代码的权限。
   - 不满足时 `CodeHidden`/`AnswerHidden=true`，指标和测试点统计仍返回。
- `GET /submissions` 和 `GET /submissions/:id` 保持公开可选登录，响应仍设置 `Cache-Control: private, no-store`。

### 5.3 重测

- `submission.Service.Rejudge` 在调用 `ResetForRejudge` 前检查 `sub.AgentID != nil`，返回 `40001`/`Agent 提交不支持重测`。
- Repository 继续保留 `ErrNotRejudgeable` 防线，防止管理员直接按 ID 回退人类计数。
- 前端详情抽屉在 `detail.agentId != null` 时隐藏重测按钮。

### 5.4 Rating 历史返回提交 ID

`internal/agent`：

- `AgentRatingRow` 增加：
  ```go
  SubmissionID *int64 `json:"submissionId"`
  ```
- `ratingHistoryPage` 的 SQL `SELECT` 增加 `h.submission_id`；`ratingHistoryRow` 增加对应扫描字段。
- 返回规则沿用数据库：
  - `solve`：AC 提交 ID。
  - `exhausted`：第 20 次提交 ID。
  - `abandon` / `timeout` / `reset`：null。
- 不新增 Agent 专用提交详情接口；前端复用 `GET /submissions/:id`，可见性由 5.2 保证。

### 5.5 SSE 实时刷新

`internal/agent`：

- 新增可选依赖：
  ```go
  type Publisher interface {
      Publish(ctx context.Context, submissionID int64)
  }
  func WithPublisher(p Publisher) Option
  ```
- `Submit` 在事务提交后 `Publish(ctx, submissionID)`；入队失败标记 `system_error` 后也发布一次，让列表能看到失败记录。
- `cmd/server/main.go` 用 `submissionevents.NewRedisPublisher(rdb)` 注入；不改变 `submissionevents` 协议。
- SSE 事件仍只包含 `submissionId`，不泄露代码或 Agent 信息。

### 5.6 Markdown 简介

- `agentInputMaxIntro` 从 500 改为 2000；`Agent.ModelIntro` 的 gorm size 改为 2000。
- 新增迁移 `apps/server/migrations/0005_agent_intro_length`：
  ```sql
  -- up
  ALTER TABLE public.agents
      ALTER COLUMN model_intro TYPE character varying(2000);
  -- down
  ALTER TABLE public.agents
      ALTER COLUMN model_intro TYPE character varying(500)
      USING left(model_intro, 500);
  ```
- 后端仍按 Unicode 字符数校验，保存原始 Markdown 源文本；渲染在前端完成。
- OpenAPI：`AgentInput.modelIntro.maxLength` 改为 2000，说明支持 Markdown；`AgentView.modelIntro` 说明为 Markdown 源文本。
- 不新增后端 Markdown 渲染或存储字段。

## 6. 前端设计

### 6.1 API 类型与 OpenAPI

- 更新 `packages/api/openapi.yaml`：
  - `SubmissionListItem` 增加 nullable `agentId` 和 `agentName`。
  - `SubmissionDetail` 说明沿用列表字段并补充 Agent 可见性。
  - `AgentRatingRow` 增加 nullable `submissionId`。
  - `AgentInput.modelIntro` 上限 2000、支持 Markdown。
  - `/submissions`、`/submissions/{id}`、`/agents/{id}/rating-history` 描述补充 Agent 行为。
- 运行 `pnpm gen:api` 更新 `apps/web/src/api/schema.d.ts`，再同步 `types.ts` 的别名（如需要）。

### 6.2 测评记录列表

`SubmissionsView.vue`：

- 提交者单元格在 `UserIdentityName` 下方（或右侧）显示：
  - `Tag value="Agent" severity="info"`，仅 `data.agentId != null` 时显示。
  - Agent 名称（`data.agentName`），使用 muted/mono 小字。
- 人类提交保持原样。
- 列表筛选和分页不变；总数自然包含 Agent 提交。

### 6.3 共享测评详情抽屉

从 `SubmissionsView.vue` 抽出 `apps/web/src/components/SubmissionDetailDrawer.vue`：

- Props：`visible`、`submissionId: number | null`、`canRejudge?: boolean`。
- Emits：`update:visible`、`rejudged`。
- 内部负责 `getSubmission`、重试、`rejudgeSubmission` 和 toast。
- 内容保持现有：提交 ID、状态、用户、题目、语言、分数、耗时、CPU、内存、时间、代码/答案、错误信息、测试点表、隐藏提示。
- Agent 提交额外显示 `Agent` 标签和 Agent 名称。
- `detail.agentId != null` 时隐藏重测按钮。
- `SubmissionsView` 改用它；`AgentDetailView` 和 `AgentSettingsView` 复用同一组件。

### 6.4 Agent Rating 历史

`AgentDetailView.vue` 与 `AgentSettingsView.vue`：

- 表格增加「提交」列：
  - `data.submissionId != null`：显示「查看」按钮/链接，打开抽屉。
  - 否则显示 `—`；`abandon`/`timeout`/`reset` 没有关联提交。
- 抽屉使用 `SubmissionDetailDrawer`，`canRejudge=false`。
- 公开 Agent 详情页仍不展示 Token、当前 attempt 等私有数据；仅新增按权限可见的提交详情入口。

### 6.5 Markdown 简介

`AgentSettingsView.vue`：

- 引入 `MarkdownEditor` 和 `useTheme`。
- 创建与编辑两个表单的「模型简介」字段都替换为 `MarkdownEditor`，`height="240px"`，`:theme="isDark ? 'dark' : 'light'"`。
- 不传 `problemId`，图片按钮保持隐藏。
- 校验上限改为 2000，提示改为「最多 2000 个字符，支持 Markdown」。
- 保存时仍提交原始 Markdown 文本。

`AgentDetailView.vue`：

- 引入 `MarkdownView` 和 `useTheme`。
- `agent.modelIntro` 非空时用 `MarkdownView` 渲染，外层保留 `.profile-intro` 的 max-width。
- 依赖现有 `sanitizeHtml`；不加 `v-html`。

`HomeView.vue`（一致性）：

- 由于 `/submissions` 现在包含 Agent 提交，首页「最近练习」按 `userId` 查询时会看到本人 Agent 的提交。
- 在最近练习行内增加小号 `Agent` 标签（`agentId != null`），避免与人类练习混淆；不改变查询。

## 7. 权限与安全

- 公开列表只暴露提交元数据，不暴露代码/答案。
- 代码/答案可见性只由现有 `Service.Detail` 判断，Agent 提交与人类提交同一套规则。
- `HasAccepted` 保持人类提交过滤；Agent AC 不能解锁其他 Agent 的代码。
- Markdown 渲染继续经过 `sanitizeHtml`，禁止 script、iframe、事件处理器和非 http(s) 图片地址。
- 简介图片上传不开放，避免引入新的存储/权限面。
- 重测在前后端双重阻止。
- 不新增权限码；Agent 所属账号通过 `submissions.user_id` 自然获得本人可见性。

## 8. 迁移、兼容与文档

- 新增 `0005_agent_intro_length` 迁移；已有 `agents.model_intro` 数据原样保留，只扩宽列。
- 下载回滚会截断到 500 字符，回滚前需确认没有超长简介；迁移文件里用 `USING left(...)` 明确该行为。
- 不需要新增列或表；`agent_rating_history.submission_id` 已有。
- OpenAPI 与生成类型同步更新；旧 Rating 历史行 `submission_id` 为 null 时不显示入口。
- 更新 Agent Arena 设计文档中「Agent 提交只通过 Agent API/Agent 详情查看」的旧约束，避免后续实现冲突。

## 9. 测试

### 后端

- `submission` repository/service：
  - 公开列表包含 Agent 行，并返回 `agentId`/`agentName`；人类行字段为空。
  - `FindDetail` 可读取 Agent 提交。
  - Agent 所属账号看到自己的代码与答案。
  - 已有人类 AC 的查看者看到 Agent 代码；没有 AC 的登录用户只看到隐藏提示和指标。
  - 只有 Agent AC 的用户不能解锁其他 Agent 代码。
  - `HasAccepted`、`CountUserActivity`、`ListCodeSubmissionTimes`、题目计数器仍排除 Agent 提交。
  - Agent 提交重测返回明确错误。
- `agent`：
  - Rating 历史 solve/exhausted 返回 `submissionId`，abandon/timeout/reset 为 null。
  - `WithPublisher` 在提交成功后发布；入队失败也发布。
  - 简介 2000 字符校验通过，2001 拒绝。
- `db`：
  - 迁移后 `agents.model_intro` 为 varchar(2000)。
- 路由/OpenAPI 相关响应字段回归。

### 前端

- `SubmissionsView` 渲染 Agent 标签和 Agent 名称；点击 Agent 行打开详情；详情抽屉隐藏重测。
- `AgentDetailView` / `AgentSettingsView` 的 Rating 历史：
  - `submissionId` 非空显示「查看」，点击调用 `getSubmission` 并展示代码/测试点。
  - `submissionId` 为空不显示入口。
  - 代码隐藏提示正常展示。
- Markdown：
  - 设置页使用 `MarkdownEditor`，保存原始 Markdown。
  - 公开详情页渲染 Markdown，而不是纯文本。
  - 2000/2001 字符校验。
- 更新现有 AgentDetailView 测试中「不出现提交代码」的断言，改为按权限出现入口。
- 共享抽屉组件保持 SubmissionsView 现有详情、重测、隐藏提示测试通过。

## 10. 验收标准

- 游客打开 `/submissions` 能看到 Agent 提交，带 Agent 标签和 Agent 名称；人类提交外观不变。
- Agent 所属账号打开自己 Agent 的任意提交详情能看到代码/答案/测试点；已通过该题的人类用户能看到代码；其他用户看到指标和隐藏提示。
- 管理员在人类提交上仍可重测；Agent 提交没有重测按钮，直接调接口返回明确错误。
- Agent 详情页和「我的 Agent」Rating 历史里，solve/exhausted 记录可打开对应提交；abandon/timeout/reset 不显示入口。
- Agent 简介在设置页用 Markdown 编辑器编写，在公开详情页按 Markdown 渲染，最多 2000 字符。
- 用户 AC 数、金币、学习进度、题目提交/通过计数、难度计算不因本次改动包含 Agent 提交。
- 新建 Agent 提交后，打开的 `/submissions` 页面能通过 SSE 触发刷新。
