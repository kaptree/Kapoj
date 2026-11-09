# 比赛题目可见性、比赛上下文与榜单展示设计

> 目标：比赛选中的题目在赛前与赛时从题库派生隐藏，赛后可访问并展示本场统计；比赛中的题目页只保留本场题目导航；比赛期间按比赛设置禁用参赛者 AI；排行榜按首杀/通过/未通过着色，ICPC 通过格显示错误次数。

## 1. 背景与现状

- 比赛题目关系只存在 `contest_problems`，题目本身只有 `problems.is_public`，没有“因比赛而暂时不可见”的状态。
- 公开题被加入比赛后，赛前/赛时仍可从题库直接访问，也可以不带 `contestId` 普通提交；存在提前看题和赛前刷提交的风险。
- 隐藏题被加入比赛后，`CanViewProblem` 允许报名用户在开赛后访问，且赛后仍可访问；这与“赛后恢复原隐藏状态”不一致。
- `ProblemDetailView` 已识别 `?contestId`，但右侧仍展示官方题解、解题思路、提交统计、关联比赛；AI 提示按钮也在题目页中。
- AI 接口是全局的：`complete/format/optimize` 只接收代码与语言，`hint` 只接收 `problemId`；没有比赛上下文，也没有比赛级 AI 开关。
- 比赛编辑页没有 AI 开关；比赛详情页没有每题时间/空间限制、赛后本场统计和个人状态。
- 排行榜 `ContestRankCell` 已有 `attempts`、`acceptedAt`，ICPC 通过格显示 `AC (n)`，未通过显示 `-n`；没有首杀/通过/未通过颜色区分。

## 2. 目标与非目标

### 目标

- 比赛题目采用派生可见性：不修改 `problems.is_public`，由比赛时间窗口决定是否锁定。
- 锁定窗口为“赛前 + 比赛中”：题目加入任意未结束的比赛后，从题库和普通访问中隐藏；比赛结束后恢复原公开/隐藏状态。
- 参赛者在比赛开始后可从比赛页进入锁定题；管理员按既有题目/比赛权限访问。
- 比赛结束后，比赛题目列表展示本场提交数、通过人数和当前用户状态，并提供跳转题库题目页的按钮。
- 比赛题目列表以标签展示每题时间/空间限制。
- 比赛期间默认禁用参赛者 AI；比赛编辑页可开关；后端按题目所属比赛强制校验。
- 比赛中的题目详情页右侧只列出本场所有题目并支持跳转，不展示官方题解、解题思路、提交统计、关联比赛；AI 禁用时不展示 AI 提示入口。
- 排行榜所有赛制着色：首杀绿色、后续通过浅绿、提交未通过红色；ICPC 通过格显示 `-错误次数`，总罚时规则不变。

### 非目标

- 不修改 `problems.is_public`，不新增“比赛专用题”或按比赛覆盖公开状态的字段。
- 不改变比赛赛制、计分公式、Rating 结算、报名方式或题目冻结规则。
- 不做比赛内 AI 使用统计、审计查询或按用户白名单。
- 不重构题目详情页整体布局；只在比赛上下文替换右侧资源区。
- 不改变普通练习提交、题目编辑、测试数据管理的既有流程。

### 反目标

- 任何未报名用户在锁定窗口内通过题库、题目详情、统计、题解、讨论或 AI 接口看到锁定题内容。
- 任何普通用户在锁定窗口内不带 `contestId` 提交锁定题，污染练习统计。
- 比赛结束后把原本隐藏的题错误公开，或把原本公开的题错误隐藏。
- 为多个比赛共用同一题时，因某一场结束就提前解锁，或某一场上线就永久隐藏。

## 3. 约束

- 后端 Go + Gin + GORM + PostgreSQL；迁移使用 `db.Migrate` 的 `AutoMigrate`。
- 前端 Vue 3 + PrimeVue；API 类型来自 `packages/api/openapi.yaml` 生成的 `schema.d.ts`。
- 比赛开始后已有冻结规则：开始时间、赛制、Rating、题目列表不可改；`disableAi` 允许赛中修改。
- 题目可能同时属于多场比赛；锁定状态必须按“是否存在未结束比赛”聚合。
- 游客、普通用户、参赛者、题目管理员、比赛管理员、超级管理员的权限边界必须一致地落在后端。

## 4. 方案选择

### 4.1 题目隐藏方式

- **方案 A：派生可见性（选用）**。保留 `problems.is_public`，用 `contest_problems + contests.end_time` 派生锁定；赛后自动恢复。
- 方案 B：开赛时把 `problems.is_public` 改为 false，结束再改回。实现直观，但无法处理多比赛共用题、原本隐藏题、并发编辑和失败回滚。
- 方案 C：只对非参赛者隐藏。参赛者仍可从题库访问，无法阻止赛前看题和普通提交。

选择 A。它满足“不改题目原始状态”，也能正确聚合多场比赛。

### 4.2 参赛者访问判定

- **方案 A：按“是否报名了包含该题的进行中比赛”判定（选用）**。`contestId` 只作为前端上下文，后端不信任它。
- 方案 B：要求请求必须带 `contestId` 且校验归属。更容易“只从比赛页进入”，但所有题目详情/提交/统计接口都要新增参数，且仍可被伪造。
- 方案 C：比赛期间只要登录就允许。无法区分参赛者。

选择 A。前端仍从比赛页进入，但后端以报名关系和比赛时间作为真实边界。

### 4.3 赛后统计口径

- **方案 A：本场提交数 + 通过人数（去重用户）+ 我的状态（选用）**。`提交 12 / 通过 3`，个人显示“已通过 · 2 次”或“未通过 · 3 次”。
- 方案 B：本场提交数 + AC 提交数。实现简单，但“通过 5”可能只是同一人多次 AC，不如通过人数直观。
- 方案 C：仅我的状态。缺少本场汇总。

选择 A。`submitCount` 统计已评测提交（排除 `pending`/`judging`），`acceptCount` 统计该题至少有一次 AC 的去重用户数。

### 4.4 AI 禁用校验

- **方案 A：请求携带 `problemId`，后端按题目所属进行中比赛校验（选用）**。
- 方案 B：只按前端传入的 `contestId` 校验。移除 `contestId` 即可绕过。
- 方案 C：仅前端禁用。不满足“后端强制校验”。

选择 A。`complete/format/optimize` 新增必填 `problemId`；`hint` 已有 `problemId`。后端先用比赛关系判断题目是否锁定、用户是否可访问，再判断该比赛是否禁用 AI。

### 4.5 首杀标记

- **方案 A：后端计算 `firstBlood`（选用）**。按每题最早 `acceptedAt` 标记唯一格；相同时间按用户 ID 稳定选择。
- 方案 B：前端扫描所有行求最早时间。多端展示可能不一致，OI 自见榜单也容易算错。

选择 A。先对完整榜单计算首杀，再做 OI 赛时自见过滤，保证自见时颜色仍正确。

## 5. 设计

### 5.1 数据模型与迁移

`contests` 新增：

```text
disable_ai bool not null default true
```

- `contest.Contest.DisableAI bool`，GORM 默认 true。
- `AutoMigrate` 为已有比赛补列并默认 true，即存量比赛默认禁用 AI。
- `contest.Input.DisableAI *bool`：创建时缺省为 true；更新时缺省保留原值，显式 `false` 才关闭。
- 不新增表、不修改 `problems` 表。

### 5.2 派生锁定

新增/调整 `contest.Repository` 方法：

```go
// 是否存在未结束比赛包含该题（upcoming + running）
IsProblemLocked(ctx, problemID int64, now time.Time) (bool, error)

// 用户是否报名了包含该题的进行中比赛（running only）
CanViewProblem(ctx, userID, problemID int64, now time.Time) (bool, error)

// 用户是否在进行中、禁用 AI 且包含该题的比赛中报名
AIBlockedForUser(ctx, userID, problemID int64, now time.Time) (bool, error)
```

SQL 语义：

```sql
-- IsProblemLocked
SELECT EXISTS (
  SELECT 1
  FROM contest_problems cp
  JOIN contests c ON c.id = cp.contest_id
  WHERE cp.problem_id = ? AND c.end_time > ?
);

-- CanViewProblem（比赛开始后、结束前）
SELECT EXISTS (
  SELECT 1
  FROM contest_participants p
  JOIN contest_problems cp ON cp.contest_id = p.contest_id
  JOIN contests c ON c.id = p.contest_id
  WHERE p.user_id = ? AND cp.problem_id = ?
    AND c.start_time <= ? AND c.end_time > ?
);

-- AIBlockedForUser
SELECT EXISTS (
  SELECT 1
  FROM contest_problems cp
  JOIN contests c ON c.id = cp.contest_id
  JOIN contest_participants p ON p.contest_id = c.id AND p.user_id = ?
  WHERE cp.problem_id = ? AND c.disable_ai = true
    AND c.start_time <= ? AND c.end_time > ?
);
```

锁定矩阵：

| 访问者 | 赛前（upcoming） | 比赛中（running） | 赛后（ended） |
|---|---|---|---|
| 游客/未报名 | 404/题库不可见 | 404/题库不可见 | 按 `is_public` 恢复 |
| 参赛者 | 404/题库不可见 | 可访问（比赛页进入） | 按 `is_public` 恢复 |
| 题目管理员（`problem.view_hidden`） | 可访问 | 可访问 | 可访问 |
| 比赛管理员（`contest.update`） | 可访问公开锁定题 | 可访问公开锁定题 | 可访问公开题 |
| 超级管理员 | 可访问 | 可访问 | 可访问 |

说明：

- 比赛管理员绕过“锁定”，但不绕过题目原始隐藏；锁定题若原本隐藏，仍需 `problem.view_hidden` 才能看题面。
- 参赛者在比赛结束后不再因“曾报名”获得隐藏题访问权；公开题按原 `is_public` 恢复。
- 多场比赛共用题目时，只要还有任意一场未结束，题目保持锁定；最后一场结束后恢复。
- “题目管理员”按用途拆分，避免只读管理员越权：
  - 题面/锁定访问：`problem.view_hidden` 或 super。
  - 统计访问：`problem.view_hidden`、`contest.update` 或 super。
  - 题解字段、官方题解、讨论资源：`problem.update`/`problem.delete`/`problem.publish`/`testcase.manage`/`checker.manage`（即现有 `CanViewSolution`）或 super。
  - 锁定时强制 `includeSolution=false` 只对非“题解资源管理员”生效。

### 5.3 题库列表与题目访问

`problem.Viewer` 新增 `CanViewLocked bool`；`problem.Filter` 新增 `IncludeLocked bool`、`Now time.Time`。

- `problemapi` 从 RBAC 派生：
  - `CanViewHidden`：`problem.view_hidden`
  - `CanViewLocked`：`problem.view_hidden` 或 `contest.update`
  - `CanViewSolution`：沿用现有题解管理权限
- `problem.Repository.List` 在 `!IncludeLocked` 时追加：

```sql
AND NOT EXISTS (
  SELECT 1
  FROM contest_problems cp
  JOIN contests c ON c.id = cp.contest_id
  WHERE cp.problem_id = problems.id AND c.end_time > ?
)
```

- 题库列表对普通用户和参赛者都不展示锁定题；拥有 `CanViewLocked` 的用户（`problem.view_hidden` 或 `contest.update`）自动包含公开锁定题，便于继续管理比赛。该行为由服务端根据权限派生，不新增 `includeLocked` 查询参数。
- `GET /problems/:id`：
  1. 查询 `IsProblemLocked`。
  2. 若锁定且不是题目管理员/比赛管理员，调用 `CanViewProblem`；不通过返回 404。
  3. 参赛者通过时临时设置 `CanViewHidden=true`，允许原本隐藏的锁定题在赛中查看。
  4. 锁定期间对非“题解资源管理员”强制 `includeSolution=false`，即使本人曾通过该题也不返回题解字段。
- `GET /problems/:id/statistics` 在锁定期间对**所有非管理员**返回 404，包括进行中比赛的参赛者；参赛者通过比赛页只能看到题面与本场导航，不能通过直接调用统计接口看到“通过提交”数据。这里的“管理员”指 `problem.view_hidden`、`contest.update` 或 super。
- `GET /problems/:id/official-solution`、`PUT`、`purchase`，以及 `/solutions` 列表/详情/发帖/回复/点赞在锁定期间对非“题解资源管理员”（现有 `CanViewSolution` 或 super）返回 404，避免绕过 UI 直接读取题解或讨论。
- 题目不存在与无权访问统一 404，不泄露锁定题存在性。

### 5.4 普通提交与比赛提交

`submission.ContestLookup` 增加 `IsProblemLocked`。

- `contestId` 为空时：
  - 若题目被锁定，且访问者没有 `problem.view_hidden` 或 `contest.update`，返回 404。
  - 提示语义与“题目不存在”一致，避免引导用户绕过比赛入口。
- `contestId` 非空时：
  - 保持现有校验：比赛存在、进行中、已报名（管理员例外）、题目属于该比赛。
  - 比赛提交不受“锁定”二次拦截；否则参赛者无法正常提交。
- 比赛结束后锁定消失；公开题恢复普通提交，隐藏题仍要求 `problem.view_hidden`。

### 5.5 赛后统计与题目跳转

`ContestProblemItem` 新增：

```text
timeLimitMs      int
memoryLimitMb    int
isPublic         bool
locked           bool     // 当前时刻是否仍被其他未结束比赛锁定
submitCount      int
acceptCount      int
myStatus         string   // accepted / wrong_answer / ... / ""
myAttempts       int
```

- `ProblemRow` 联表补充 `p.time_limit_ms`、`p.memory_limit_mb`、`p.is_public`。
- `locked` 由 `IsProblemLocked` 在同一请求时刻批量计算；它表示“全局是否仍锁定”，不是当前用户能否访问。
- 仅当比赛状态为 `ended` 时填充统计；赛前/赛中以零值返回，前端不展示。
- 新增仓储查询：
  - `ProblemStats(ctx, contestID)`：按 `problem_id` 聚合已评测提交数、AC 去重用户数。
  - `UserProblemStats(ctx, contestID, userID)`：按题返回提交次数、是否 AC、最近一次状态。
  - `LockedProblemIDs(ctx, problemIDs, now)`：一次查询返回仍被未结束比赛锁定的题目 ID 集合。
- 统计口径与排行榜一致：只统计“报名用户 + 报名后 + 比赛时间窗内”的提交，并排除 `pending`/`judging`；管理员测试提交若不在参赛者表中也不计入。
- `myStatus`：有 AC 时为 `accepted`；否则为最近一次已评测提交状态；无提交为空字符串。只有 `pending`/`judging` 时按“未提交”处理。
- 前端比赛题目列表：
  - 始终增加“限制”列，用两个 `Tag` 显示 `X ms`、`Y MB`。
  - 赛后增加“本场”列：`提交 {submitCount} / 通过 {acceptCount}`。
  - 赛后增加“我的”列：`已通过 · N 次`（success）、`未通过 · N 次`（danger）、`未提交`（secondary）。
  - 赛后操作列提供“题库”按钮，链接到 `/problems/:id`；标题链接也改为不带 `contestId`。
  - 赛中标题链接保持 `?contestId`，操作列保留“提交”。
  - 赛后跳转是否可用看 `isPublic && !locked`：原本隐藏（`isPublic=false`）显示“未公开”，仍被其他未结束比赛锁定（`locked=true`）显示“比赛中”，两种情况都禁用跳转，避免跳到 404。

### 5.6 比赛中的题目详情页

`ProblemDetailView`：

- 当 `contestId > 0` 时加载比赛详情；`contestMode = contest.status === 'running'`。
- `contestMode` 下：
  - 右侧资源区只渲染“本场题目”列表：编号、标题、当前题高亮；每项链接到 `/problems/:id?contestId=...`。
  - 不渲染官方题解、解题思路、提交统计、关联比赛卡片。
  - 跳过统计接口请求。
  - 当 `contest.disableAi=true` 且当前用户不是比赛管理员（`contest.update`/super）时隐藏 AI 提示按钮；管理员仍可见。无论是否隐藏，题面自带的 `hintMd` 提示区都保留。
  - 顶部保留“返回比赛”按钮。
- 非比赛上下文或赛后：保持现有资源卡片和统计。
- 比赛题目列表加载失败时右侧显示轻量错误态，不阻塞题面。
- 后端不以 `contestId` 作为授权依据：用户手动去掉 `contestId` 只会改变前端布局，题目/统计/题解接口仍按锁定规则返回 404，AI 仍按比赛设置返回 40302。这是有意的 UI 上下文取舍，不是安全边界。

### 5.7 比赛 AI 开关

- 比赛编辑页新增 `ToggleSwitch`“比赛期间禁用 AI”，默认开启。
- 创建/更新请求新增 `disableAi`；创建缺省 true，更新缺省保留原值。
- `aiapi` 新增可选 `ContestAccess`：
  - `IsProblemLocked`
  - `CanViewProblem`
  - `AIBlockedForUser`
- 所有 AI 请求先校验 `problemId`：
  - `problemId <= 0` → 40001。
  - `problemId` 必须对应真实题目，否则 404；`complete/format/optimize` 不读取题面，因此不再额外做“隐藏题”可见性判断，`hint` 沿用现有隐藏题检查。
  - 题目锁定且用户不是题目/比赛管理员、也不是进行中比赛的参赛者 → 404，避免 AI 成为锁定题的旁路。
  - 题目处于进行中比赛、用户是该比赛参赛者、且 `disable_ai=true`、用户没有 `contest.update`/super → `40302 比赛期间已禁用 AI`。
- `complete/format/optimize` 请求新增必填 `problemId`；`hint` 沿用 `problemId`。
- 前端提交页：
  - 加载比赛详情，若 `status === 'running' && disableAi` 且当前用户不是比赛管理员（`contest.update`/super）则 `aiButtonsDisabled=true`，显示“比赛期间已禁用 AI”提示。
  - `complete/format/optimize` 调用都携带 `problemId`。
  - 后端 40302 时显示对应中文提示。
- 题目详情页在比赛禁用 AI 且当前用户不是比赛管理员时隐藏 AI 提示按钮。
- 管理员（`contest.update` 或 super）可绕过比赛 AI 禁用；普通参赛者不能。`disableAi=false` 时所有用户正常使用。

### 5.8 排行榜

`RankCell` 新增 `firstBlood bool`。

- 计算完所有行后，对每道题找最早 `acceptedAt` 的已通过格；相同时间按 `userId` 升序选唯一格，置 `firstBlood=true`。
- 首杀在完整榜单上计算，再做 OI 赛时自见过滤，保证自见行颜色正确。
- 前端 `ContestRanklist`：
  - 首杀：绿色前景 + 绿色浅底 + 加粗。
  - 后续通过：浅绿色前景/浅绿底。
  - 有提交但未通过：红色前景/浅红底。
  - 无提交：保持默认。
  - 所有赛制应用同一套颜色；分数格仍显示分数。
- ICPC 格文本：
  - 已通过：`-{attempts}`，`attempts` 为 AC 前的错误提交次数。3 次提交通过显示 `-2`，0 次错误显示 `-0`。
  - 未通过：`-{attempts}`（保持现有语义，用红色区分）。
- 总罚时不变：`通过时间分钟 + 20 × AC 前错误次数`。
- 榜单头部增加小图例：首杀 / 通过 / 未通过。

## 6. API 与契约

`packages/api/openapi.yaml` 更新：

- `ContestInput.disableAi`：boolean，默认 true，更新缺省保留。
- `ContestListItem`/`ContestDetail`：`disableAi` boolean。
- `ContestProblemItem`：新增 `timeLimitMs`、`memoryLimitMb`、`isPublic`、`locked`、`submitCount`、`acceptCount`、`myStatus`、`myAttempts`。
- `ContestRankCell`：新增 `firstBlood` boolean。
- `AICompleteInput`/`AIFormatInput`/`AIOptimizeInput`：新增必填 `problemId`（integer, minimum 1）。
- `/ai/*` 文档补充：锁定题不可访问；比赛禁用 AI 返回 40302；管理员例外。
- 重新生成 `apps/web/src/api/schema.d.ts`，更新 `types.ts` 别名。

## 7. 测试与验收

### 7.1 后端

- 派生锁定：
  - upcoming/running 锁定，ended 解锁；多比赛共用题在最后一场结束后解锁；删除比赛/移除题目后解锁。
  - `CanViewProblem` 仅 running 有效；赛后不再因报名获得隐藏题访问。
- 题库与题目接口：
  - 普通用户/参赛者列表不出现锁定题；拥有 `CanViewLocked` 的用户可见公开锁定题。
  - 锁定期间详情对参赛者开放，但统计、官方题解、讨论对普通用户/参赛者 404；对应管理员可访问管理所需内容。
  - 锁定期间详情不向非题解资源管理员返回题解字段。
  - 赛后公开题恢复，原隐藏题仍 404；被其他未结束比赛锁定的题仍不可跳转。
- 提交：
  - 锁定公开题无 `contestId` 提交 404；参赛者带 `contestId` 正常提交；赛后普通提交恢复。
- 赛后统计：
  - `submitCount` 排除 pending/judging，并只统计报名用户、报名后、比赛时间窗内的提交；`acceptCount` 去重用户；`myStatus`/`myAttempts` 正确；赛前/赛中不返回统计。
  - 管理员非参赛者的测试提交不计入；只有 judging 提交的用户显示“未提交”。
  - `locked` 正确反映其他未结束比赛。
- AI：
  - 生成/提示请求必须带合法 `problemId`。
  - 锁定题对未授权用户 404。
  - running + disableAi + 参赛者 → 40302；disableAi=false 或赛后 → 正常；管理员例外。
  - 创建比赛缺省 disableAi=true；更新显式 false 生效；赛中可改。
- 排行榜：
  - 首杀唯一、按最早时间；相同时间稳定；OI 自见仍保留首杀标记。
  - ICPC 3 次提交通过 `attempts=2`；总罚时 `分钟 + 40`。
- 迁移：`disable_ai` 列默认 true。

### 7.2 前端

- `ContestDetailView`：时间/空间标签；赛中无赛后统计；赛后统计、我的状态、题库跳转；隐藏题“未公开”/仍锁定题“比赛中”禁用跳转。
- `ProblemDetailView`：比赛模式隐藏四类资源卡、展示本场题目列表并高亮当前题；禁用 AI 时非管理员隐藏 AI 提示，管理员可见。
- `SubmitView`：比赛禁用 AI 时非管理员按钮禁用并显示提示；管理员可继续使用；生成请求携带 `problemId`；40302 提示。
- `ContestEditView`：AI 开关默认开启、加载/保存正确、赛中可改。
- `ContestRanklist`：首杀/通过/未通过 class；ICPC `-2`/`-0`；所有赛制着色。
- 运行 `pnpm gen:api`、前端单测、lint、`vue-tsc`/build。

### 7.3 真实冒烟

- 创建一场即将开始/进行中的比赛，加入公开题：
  - 游客/普通用户题库不显示；题面/统计/题解/讨论 404。
  - 参赛者赛前不能看，开赛后从比赛页可看并提交。
  - 赛中 AI 按钮禁用，直接调用 AI 返回 40302。
  - 赛后题目恢复，比赛页显示统计和题库跳转。
- 排行榜检查首杀/通过/未通过颜色与 ICPC `-2`。

## 8. 已知取舍

- 赛前锁定会让“已排期比赛”的公开题从题库提前消失；这是防止提前看题的有意行为。
- 比赛管理员可以看公开锁定题，但不能看原本隐藏题，除非拥有 `problem.view_hidden`。
- 赛后原本隐藏的题仍不可跳转，显示“未公开”；这遵循“恢复原公开/隐藏状态”的决定。
- AI 生成接口新增必填 `problemId` 是有意的契约变更；当前只有前端提交页调用，仓库内调用点会同步更新。
- `-0` 表示 0 次错误提交通过；保持“用 - 和数字填充”的统一格式。
