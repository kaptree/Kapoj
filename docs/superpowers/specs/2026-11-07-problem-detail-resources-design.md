# 题目详情页资源区、官方题解与解题思路论坛设计

> 目标：把题目详情页右侧的 Tabs 改成四个可点击资源卡片；提示单独放在题目描述下方；官方题解支持创建者编辑与金币解锁；解题思路改成“帖子 + 详情 + 回复”的论坛；提交统计和关联比赛各自成页。

## 1. 背景与现状

当前题目详情页：

- 左侧题面、输入输出格式、样例。
- 右侧一个 `Tabs`，包含：提示、解题思路、解题代码、提交统计、关联比赛。
- `hintMd` 是题目字段，所有能看题的人都能读。
- `solutionMd` / `solutionCode` 是题目字段，仅题目管理权限或本人已通过该题时由后端返回。
- 没有题目讨论区、帖子、回复。
- 没有“按题目查询关联比赛”的 API 或页面，仅在 URL 带 `contestId` 时显示返回比赛链接。

## 2. 目标

- 题目描述下方固定展示“提示”卡片。
- 右侧从上到下列出可点击资源卡片：官方题解、解题思路、提交统计、关联比赛。
- 官方题解：
  - 由题目创建者（或超级管理员）上传/编辑说明与代码。
  - 创建者、超级管理员、已通过该题的用户免费查看。
  - 其他用户可支付金币解锁；价格由创建者设置；价格 0 表示所有人免费。
  - 解锁记录永久保留，创建者改价不影响已解锁用户。
- 解题思路：
  - 每个登录用户可发布帖子（标题 + Markdown 正文，可含代码块）。
  - 帖子列表、帖子详情、回复；作者可编辑/删除自己的内容。
  - 题目创建者和拥有 `problem.update` 的管理员可删除（审核）他人的帖子和回复。
- 提交统计：独立页面，展示提交数/通过数/通过率，并可进入该题提交记录。
- 关联比赛：独立页面，列出包含该题且当前用户可见的比赛。
- 迁移现有数据：价格默认 0；现有 `solutionMd`/`solutionCode` 继续作为官方题解内容。

## 3. 非目标

- 不做通用全站论坛、版块、关注、通知、点赞、收藏、富文本或附件上传。
- 不做题解付费分成、退款、优惠券、提现。
- 不删除现有 `solutionMd`/`solutionCode` 字段，也不迁移历史数据。
- 不做论坛全文搜索、热度算法、置顶/精华、举报审核流。
- 不做 AI 辅助提示（已在 M5-A 独立完成）。
- 不重做题目列表、题目编辑主表单的其他部分。

## 4. 方案选择

### 4.1 选定方案：扩展题目域 + 独立资源页面

- 在 `problem` 域增加官方题解访问/购买/讨论模型与 API。
- 新增 5 个前端页面：官方题解、解题思路列表、思路详情、统计、关联比赛。
- 详情页只负责展示提示卡片和四个资源入口，不把正文塞回 Tabs。
- 复用现有 `solutionMd`/`solutionCode` 作为官方题解内容，减少数据迁移。

### 4.2 被否决方案

- **通用论坛子系统**：需要版块、审核、通知和跨对象关联；当前只有题目讨论，成本过高。
- **仅前端改卡片/Tabs**：无法实现官方题解付费、帖子、回复和关联比赛查询，不满足目标。
- **把官方题解继续留在题目编辑表单**：无法表达“创建者专属编辑”和价格/购买状态，也会继续让所有题目管理员改题解。

## 5. 页面与交互

### 5.1 题目详情页

```text
[题目标题] [提交代码] [编辑题目]
[难度] [类型] [时间] [内存] [来源] [标签]

+-------------------------------+   +------------------+
| 题目描述                       |   | 官方题解         |
| ...                           |   | 通过免费 / 金币 X |
|                               |   +------------------+
| 输入格式                       |   | 解题思路         |
| 输出格式                       |   | 帖子/回复         |
| 样例                           |   +------------------+
|                               |   | 提交统计         |
| +---------------------------+ |   | 通过率 / 提交数   |
| | 提示                       | |   +------------------+
| | hintMd                    | |   | 关联比赛         |
| +---------------------------+ |   | 查看包含本题比赛  |
+-------------------------------+   +------------------+
```

- 提示卡片：位于题面/样例之后、同一左栏内；无提示时显示“暂无提示”。
- 右侧卡片均为整块可点击的 `RouterLink`，不使用 Tabs。
- 官方题解卡片显示“免费”或“金币 N”；页面内再展示是否已通过、已解锁或需购买。
- 解题思路卡片不要求先显示精确计数；进入列表页后展示帖子总数。
- 提交统计卡片可使用已加载的 `ProblemStats` 显示通过率。
- 移动端右栏卡片顺序排在题面之后，卡片全宽。

### 5.2 官方题解页 `/problems/:id/official-solution`

- 路由名：`problem-official-solution`。
- 展示：官方说明（Markdown）、官方代码、价格、访问状态。
- 状态：
  - `creator`：创建者/超级管理员，显示内容 + “编辑官方题解”。
  - `accepted`：已通过，显示内容。
  - `purchased`：已购买，显示内容。
  - `free`：价格为 0，任何人可看。
  - `locked`：登录用户未通过/未购买，显示价格和“支付 N 金币查看”按钮。
  - `guest`：游客且价格 > 0，显示“登录后购买/通过后免费”。
- 创建者编辑表单：
  - 官方说明：MarkdownEditor，最多 50,000 字符。
  - 官方代码：Textarea，最多 65,536 字节。
  - 价格：InputNumber，0–1,000,000 金币；0 表示免费。
  - 保存后刷新页面；不修改题面/测试数据。
- 购买：
  - 二次确认后调用购买接口。
  - 成功刷新官方题解并 `auth.fetchMe()` 更新金币。
  - 金币不足、已通过、创建者、价格 0 等错误显示中文提示。
- 未通过用户仍可看到价格和访问条件，不显示题解正文/代码。

### 5.3 解题思路列表 `/problems/:id/solutions`

- 路由名：`problem-solutions`。
- 帖子卡片/表格：标题、作者、回复数、发布时间、摘要。
- 分页，按创建时间倒序。
- 登录用户显示“发布思路”按钮；游客可阅读但点击发布跳登录。
- 发布对话框：标题（1–100 字符）+ MarkdownEditor 正文（1–50,000 字符）。
- 空状态：“还没有人分享思路”。

### 5.4 思路详情 `/problems/:id/solutions/:postId`

- 路由名：`problem-solution-detail`。
- 展示帖子标题、作者、时间、Markdown 正文、回复列表。
- 登录用户可回复（Markdown，1–10,000 字符）。
- 作者可编辑/删除自己的帖子；题目创建者或 `problem.update` 管理员可删除帖子（审核）。
- 回复作者可编辑/删除；题目创建者/管理员可删除他人回复。
- 删除帖子后列表/详情 404；删除回复后从列表消失。
- 编辑帖子/回复在页面内对话框完成，不新增独立路由。

### 5.5 提交统计 `/problems/:id/statistics`

- 路由名：`problem-statistics`。
- 展示提交数、通过数、通过率、通过率进度条。
- 提供“查看提交记录”链接到 `/submissions?problemId=<id>`。
- 复用现有 `GET /problems/:id/statistics`。

### 5.6 关联比赛 `/problems/:id/contests`

- 路由名：`problem-contests`。
- 展示包含该题的比赛卡片：标题、状态、时间、赛制、报名状态。
- 只展示当前用户可见的比赛：公开比赛、本人已报名比赛、或有 `contest.view_hidden` 的比赛。
- 比赛列表 API 增加 `problemId` 筛选；带 `problemId` 时使用“公开 OR 本人已报名 OR 有 view_hidden”联合可见性，而不是复用 `mine=true` 的互斥筛选。
- 空状态：“暂无关联比赛”。

## 6. 数据模型

### 6.1 题目新增字段

```text
problems.solution_price_coins BIGINT NOT NULL DEFAULT 0
-- 0..1,000,000，0 表示免费
```

- 现有 `solution_md`、`solution_code` 继续作为官方题解说明/代码。
- `ProblemDetail` 始终返回 `solutionPriceCoins`（用于右侧卡片显示“免费/金币 N”）。
- `PUT /problems/:id` 不再写入 `solution_md`/`solution_code`，并从 `ProblemInput`/`ProblemUpdateInput` 中移除这两个字段；`ProblemDetail` 仍返回它们。官方题解 `PUT /problems/:id/official-solution` 成为唯一写入口，避免题目编辑保存时把题解清空。

### 6.2 官方题解解锁

```text
problem_solution_unlocks (
  id, user_id, problem_id, price_paid, created_at
)
-- unique(user_id, problem_id)
-- 解锁永久有效；价格变化不影响历史记录
```

### 6.3 解题思路帖子

```text
problem_solution_posts (
  id, problem_id, user_id, title, content_md,
  created_at, updated_at, deleted_at
)
-- deleted_at 软删除；列表/详情过滤已删除
```

### 6.4 思路回复

```text
problem_solution_replies (
  id, post_id, user_id, content_md,
  created_at, updated_at, deleted_at
)
-- deleted_at 软删除；帖子删除后回复整体不可见
```

- 不新增外键级联要求；应用层事务/查询保证一致性。
- 帖子/回复作者摘要来自 `users` 表，只返回 ID/username/nickname。

## 7. API 设计

### 7.1 官方题解

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/problems/:id/official-solution` | 可选 | 返回价格、访问状态、可编辑/可购买；有权限时返回正文和代码。隐藏题要求 `problem.view_hidden`/super，不沿用比赛参与者 fallback |
| PUT | `/api/problems/:id/official-solution` | 登录 | 创建者/超级管理员保存说明、代码、价格 |
| POST | `/api/problems/:id/official-solution/purchase` | 登录 | 支付金币解锁 |

`OfficialSolutionView`：

```json
{
  "priceCoins": 100,
  "access": "free|accepted|purchased|creator|locked|guest",
  "canEdit": false,
  "canPurchase": true,
  "solutionMd": "...",
  "solutionCode": "..."
}
```

- `solutionMd`/`solutionCode` 只在 allowed 访问状态返回；locked/guest 不返回。
- 隐藏题：无 `problem.view_hidden`/super 返回 404；不沿用比赛参与者 fallback，避免比赛期间通过题解资源绕过题面可见性。
- 题目详情页对无 `problem.view_hidden` 的非公开题隐藏/禁用四个资源卡片（比赛参赛者仍可看题面，但资源区不开放）。
- `PUT` 输入：

```json
{
  "solutionMd": "...",
  "solutionCode": "...",
  "priceCoins": 100
}
```

- 价格范围 0–1,000,000；代码 ≤65,536 字节；说明 ≤50,000 字符。
- `ProblemDetail.solutionPriceCoins` 对所有可读题目返回，不受题解访问状态影响。
- 只有 `problem.created_by == userID` 或 super 可编辑。
- 购买：
  - 价格 0、已通过、创建者、已解锁 -> 返回当前访问视图，不扣费。
  - 价格 >0 且未解锁 -> 事务内写 `problem_solution_unlocks` + 扣金币 + 金币流水。
  - 金币不足 -> 40001。
  - 重复请求幂等，不重复扣费。

### 7.2 解题思路

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/problems/:id/solutions` | 可选 | 帖子分页列表。隐藏题要求 `problem.view_hidden`/super，不沿用比赛参与者 fallback |
| POST | `/api/problems/:id/solutions` | 登录 | 发布帖子 |
| GET | `/api/problems/:id/solutions/:postId` | 可选 | 帖子详情 + 回复 |
| PUT | `/api/problems/:id/solutions/:postId` | 登录 | 作者编辑 |
| DELETE | `/api/problems/:id/solutions/:postId` | 登录 | 作者或创建者/`problem.update` 删除 |
| POST | `/api/problems/:id/solutions/:postId/replies` | 登录 | 回复 |
| PUT | `/api/problems/:id/solutions/:postId/replies/:replyId` | 登录 | 回复作者编辑 |
| DELETE | `/api/problems/:id/solutions/:postId/replies/:replyId` | 登录 | 回复作者/创建者/管理员删除 |

- 列表/详情/回复对游客可见；写操作要求登录。
- 隐藏题延续题目可见性：无 `problem.view_hidden`/super 返回 404，不沿用比赛参与者 fallback；不泄露帖子和题解。
- 帖子列表按 `created_at DESC, id DESC`，分页 20 条。
- 回复按 `created_at ASC, id ASC`，全部返回（单帖回复上限 500 条，超出分页后续可扩展；M5 先限制 500）。
- 帖子/回复输入：
  - 标题 1–100 字符。
  - 帖子正文 1–50,000 字符。
  - 回复正文 1–10,000 字符。
- 编辑/删除权限在服务层校验，不依赖前端。

### 7.3 关联比赛

- `GET /api/contests?problemId=<id>` 增加可选筛选。
- 只返回当前用户可见的比赛；复用 `contest.Viewer` 和现有列表可见性规则。
- 返回现有 `ContestPage`，不新增 schema。

### 7.4 统计

- 复用 `GET /api/problems/:id/statistics`，不新增接口。

## 8. 权限与可见性

- 官方题解编辑：题目创建者或 super；不新增 RBAC 权限码。
- 官方题解查看：
  - 创建者/super：免费。
  - 已通过（存在 `submissions.status=accepted`）：免费。
  - 已有 unlock：免费。
  - `priceCoins == 0`：免费（游客也可看）。
  - 否则 locked。
- 帖子/回复编辑：仅作者。
- 帖子/回复删除：作者，或题目创建者，或拥有 `problem.update` 的管理员。
- 帖子/回复读取：与题目可见性一致；隐藏题无权限 404。
- 购买：登录用户；价格 0/已通过/创建者/已解锁不扣费。
- 关联比赛：沿用 contest 可见性。
- 所有写接口需要认证；管理删除使用现有 `problem.update`，不新增权限。

## 9. 金币扣费

- 复用 `coin_transactions`、`users.coins` 和现有唯一索引。
- 新增 `gamification.SpendCoinsTx(tx,...)` 供 `problem` 事务调用；或新增等价的事务内扣费 helper。
- 购买事务：
  1. 锁用户金币行/校验余额。
  2. 插入 `problem_solution_unlocks`（唯一 user_id+problem_id；冲突表示已解锁）。
  3. 插入金币流水（reason `problem_solution`，refType `problem_solution`，refID problemID，唯一幂等）。
  4. 扣减 `users.coins`。
  5. 提交事务。
- 价格变化不影响已解锁记录。
- 购买成功后前端刷新 `/users/me`。

## 10. 前端路由与组件

新增路由：

```text
/problems/:id/official-solution   problem-official-solution
/problems/:id/solutions           problem-solutions
/problems/:id/solutions/:postId   problem-solution-detail
/problems/:id/statistics          problem-statistics
/problems/:id/contests            problem-contests
```

组件：

- `ProblemDetailView.vue`：移除 Tabs；提示卡片 + 右侧四个资源卡片。
- `ProblemOfficialSolutionView.vue`：内容/锁定/购买/创建者编辑。
- `ProblemSolutionsView.vue`：列表 + 发布对话框。
- `ProblemSolutionDetailView.vue`：帖子 + 回复 + 编辑/删除。
- `ProblemStatisticsView.vue`。
- `ProblemContestsView.vue`。
- 复用 `MarkdownView`、`MarkdownEditor`、`AppSurface`、`AppPageHeader`、`AppEmptyState`、`AppErrorState`、`AppLoadingState`、`Paginator`、`Dialog`、`useToast`、`useConfirm`。

API 客户端：

- `apps/web/src/api/problems.ts`：官方题解、购买、帖子/回复方法。
- `apps/web/src/api/contests.ts`：`problemId` 筛选参数。
- `apps/web/src/api/types.ts` 从 OpenAPI 生成类型。

## 11. 迁移与兼容

- `problems.solution_price_coins` 默认 0：现有题解默认对所有人免费。
- 现有 `solutionMd`/`solutionCode` 保持原值。
- 现有题目编辑页移除官方题解字段编辑；编辑入口迁移到官方题解页（API 仍兼容旧字段）。
- 现有详情页不再显示 Tabs；老 URL 无变化，新增子页面。
- 现有隐藏题题解行为：以前通过后看；现在通过后免费看，未通过可付费；创建者/管理员始终可看。
- 数据库迁移只增表/列；不删除旧字段。

## 12. 验收

### 12.1 后端

- 官方题解访问状态：free/accepted/purchased/creator/locked/guest；locked 不返回正文。
- 创建者/super 可编辑；其他用户 403/404。
- 购买原子性：余额不足不写 unlock；重复购买不扣费；并发购买只扣一次。
- 隐藏题访问控制。
- 帖子/回复 CRUD、作者权限、管理员删除、软删除、分页、回复上限。
- 关联比赛 `problemId` 过滤与可见性。
- `SpendCoinsTx` 测试与现有金币测试回归。

### 12.2 前端

- 详情页提示卡片位置正确；右侧四张卡片可导航。
- 官方题解 locked/free/accepted/purchased/creator 五种状态。
- 创建者编辑保存、价格修改。
- 购买后内容出现且金币余额刷新。
- 帖子列表/详情/发布/回复/编辑/删除。
- 统计页和关联比赛页。
- 移动端卡片纵向布局、表格/编辑器可用。

### 12.3 E2E

- 游客/普通用户/已通过用户/创建者四种身份访问官方题解。
- 普通用户用金币购买后查看；余额不足提示。
- 发帖、回复、编辑、删除；管理员删除他人帖子。
- 隐藏题帖子/题解对无权限用户 404。
- 详情页四个卡片跳转与返回。

## 13. Builder check 决议

1. **关联比赛可见性**：带 `problemId` 的列表使用“公开 OR 本人已报名 OR `contest.view_hidden`/super”的联合查询，不复用 `mine=true` 的互斥语义。
2. **题目主更新不再写官方题解**：`solutionMd`/`solutionCode` 从 `ProblemInput`/`ProblemUpdateInput` 移除，`PUT /problems/:id` 不写这两列；官方题解专用 PUT 是唯一写入口，避免全量替换清空官方题解。
3. **隐藏题资源可见性**：官方题解和讨论区对非公开题要求 `problem.view_hidden`/super，不沿用比赛参与者题面 fallback；详情页对无权限用户隐藏/禁用资源卡片，防止比赛期间通过题解/讨论绕过题面可见性。
4. **详情页价格展示**：`ProblemDetail` 始终返回 `solutionPriceCoins`，用于资源卡片显示“免费/金币 N”。

## 14. 待实施计划决定

- 具体文件拆分与包边界（建议 `problem` 域新增 `official_solution.go`、`discussion.go`，`problemapi` 新增对应 handler）。
- 论坛列表卡片/表格的具体组件形态。
- 金币扣费 helper 的最终签名（建议 `gamification.SpendCoinsTx`）。
- 回复 500 条上限是否改为分页（第一阶段固定上限，后续可扩展）。
