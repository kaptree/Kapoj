# M3 游戏化模块设计

> 依据 `DEVELOPMENT.md` 3.5、4.1、4.6、5.1–5.4 与 `docs/任务清单.md` M3-1–M3-8。
> 本文确认后，再拆分为 M3-A、M3-B、M3-C 三个实施计划。

## 1. 目标

让用户在刷题、参赛和登录过程中获得持续反馈：

- **金币**：统一账户余额与不可变流水；所有奖励/消费在同一数据库事务内完成。
- **成就**：管理员配置条件，用户满足条件后自动解锁并发放金币/徽章奖励，重复事件不重复发奖。
- **任务**：管理员发布题目任务；用户领取后，在任务截止前 AC 全部题目即自动完成并发放金币。
- **徽章**：管理员上传/上架徽章；用户用金币购买、最多装备 3 个，并在公开页面展示。
- **称号**：管理员配置解锁条件；用户达到条件后解锁，用金币购买并装备 1 个，公开显示颜色样式。
- **前端**：成就页、任务大厅、徽章商城、称号商店、装扮管理与游戏化管理后台。

## 2. 非目标

- 不在 M3 实现 Rating 结算、Rating 榜、活跃度积分、共享积分（M4）。
- 不实现评论、点赞、关注；`solution.liked` 等事件类型只预留契约，不产生实际事件。
- 不实现每日签到奖励（M4 活跃度）；M3 只记录登录日期，用于“连续登录”成就。
- 不实现 MinIO；徽章图片沿用本地文件存储，M5 再接对象存储。
- 不做徽章/称号的实时 SSE 推送；页面刷新时读取最新状态。
- 不实现用户间赠送、交易或金币提现。

## 3. 已确认选择

### 3.1 事件通道：Asynq 队列 + Web worker

用户已确认采用 Asynq，而不是 Redis Pub/Sub 或数据库 outbox。理由：

- 与现有评测队列共用 Redis 和 Asynq，运维成本低。
- 任务可重试、可观测，事件不会因 Web 进程短暂重启而直接丢失。
- 依靠数据库唯一约束实现幂等，重复投递不会重复发奖。

被否决的方案：

- **Redis Pub/Sub**：实现最轻，但 Web 重启或瞬时断线会丢事件，只能靠读取时补算；适合通知，不适合奖励结算。
- **数据库 outbox + 轮询**：可靠性最高，但需要额外事件表和轮询延迟；本项目规模下 Asynq 足够。

### 3.2 其他实现选择

- 徽章图片存本地 `Storage`，通过 `GET /api/badges/:id/image` 公开读取；M5 迁移 MinIO 时只替换 `Storage` 实现。
- 游戏化管理接口仅 `super_admin` 可用；`problem_admin`、`contest_admin` 不具备游戏化配置权限。
- 徽章装备上限默认 3，通过 `KAPOJ_MAX_EQUIPPED_BADGES` 配置（1–10）。
- 成就条件支持单条件与 `all_of` 组合；不支持任意嵌套，避免配置死循环。
- 任务 AC 只统计“领取之后”的提交；领取前已经 AC 的题目不计入该任务进度。
- 成就/徽章/称号/任务删除均采用软删除（`is_active=false`），保留用户已获得的记录与金币流水。

## 4. 总体架构

### 4.1 后端包

新增：

- `apps/server/internal/gamification`：模型、仓储、领域服务、条件求值、金币结算、公开身份查询。
- `apps/server/internal/gamificationapi`：HTTP 路由与 DTO 映射。
- `apps/server/internal/gamequeue`：Web 侧 Asynq 契约、发布器、worker 适配。
- `apps/judge/internal/gamequeue`：judge 侧事件发布器（与 Web 契约保持同步）。
- `apps/judge/internal/pipeline`：AC 后发布 `submission.accepted`。

修改：

- `auth`：登录成功后记录登录日并发布 `user.login`。
- `contest`/`contestapi`：报名成功后发布 `contest.join`。
- `submission`/`submissionapi`：列表/详情附带用户装备徽章与称号。
- `contest`/`contestapi`：榜单行附带装备徽章与称号。
- `user`/`userapi`：公开资料返回装备徽章与称号。
- `router`、`cmd/server/main.go`、`config`、`db`：装配新服务、worker、配置与迁移。

### 4.2 事件契约

队列：`game`；任务类型：`game:event`；最大重试 5 次；超时 30 秒。

```json
{
  "type": "submission.accepted",
  "userId": 12,
  "submissionId": 345,
  "contestId": 7,
  "occurredAt": "2026-11-04T12:00:00Z"
}
```

事件类型：

| 类型 | 产生位置 | 说明 |
|------|---------|------|
| `submission.accepted` | judge 写完成绩后 | 只带 `submissionId`，worker 重新读取提交确认状态与用户/题目/比赛 |
| `user.login` | Web 登录成功后 | 登录日已同步写入 `user_login_days`；事件只负责触发判定 |
| `contest.join` | Web 报名成功后 | 报名关系已同步写库；事件只负责触发判定 |

设计要点：

- **事件不是事实来源**。事实来源始终是数据库：AC 提交、报名记录、登录日。worker 只做“重新计算并结算”。
- judge 投递失败只记录警告，不让评测失败；用户访问成就/任务/称号页时会做一次幂等补算。
- worker 与读取接口都调用同一个 `EvaluateUser`，因此重复投递、丢失事件和并发请求最终收敛到同一结果。
- 为避免 AC 事件在极短时间内先于数据库可见，worker 读取提交后若状态不是 `accepted` 直接忽略；正常投递发生在事务提交之后。

### 4.3 幂等与事务

统一原则：

- 金币余额变动与 `coin_transactions` 写入同一事务。
- 自动奖励依赖唯一约束 + `ON CONFLICT DO NOTHING` 判断是否首次生效。
- 任务完成先锁定 `user_task_records` 行，只允许 `claimed → completed` 一次。
- 购买徽章/称号先锁定用户行，再校验余额并扣款。
- 限量徽章购买先锁定 `badges` 行，再校验并扣减库存。

唯一约束：

| 表 | 约束 |
|----|------|
| `user_achievements` | `(user_id, achievement_id)` |
| `user_task_records` | `(user_id, task_id)` |
| `user_badges` | `(user_id, badge_id)` |
| `user_titles` | `(user_id, title_id)` |
| `user_login_days` | `(user_id, login_date)` |
| `coin_transactions` | `(user_id, reason, ref_type, ref_id)`，仅 `ref_type <> ''` 时生效 |

## 5. 数据模型

新增 GORM 模型并加入 `db.Migrate`。时间字段统一 `timestamptz`。

### 5.1 金币

```text
coin_transactions (
  id, user_id, amount, type, reason,
  ref_type, ref_id, created_at
)
```

- `amount`：正数入账、负数消费；`admin_adjust` 可正可负。
- `type`：`earn` / `spend` / `admin_adjust`。
- `reason`：固定枚举值 `achievement`、`task`、`badge_purchase`、`title_purchase`、`admin_adjust`。
- `ref_type`/`ref_id`：成就 id、任务 id、徽章 id、称号 id；管理员调整时为空/0。
- 管理员调整允许填写 1–100 字说明，写入 `reason` 之外的补充字段？不新增字段，说明直接作为 `reason` 的一部分不现实；因此管理员调整使用 `reason='admin_adjust'`，另加 `note` 字段（最长 200 字）。自动奖励 `note` 为空。

> 实现补充：为保留管理员备注，`coin_transactions` 增加 `note` 列；唯一约束仍按 `(user_id, reason, ref_type, ref_id)`。

### 5.2 成就

```text
achievements (
  id, code, name, description, icon_url,
  condition_type, condition_config_json,
  reward_coins, reward_badge_id,
  sort_order, is_active, created_at, updated_at
)
user_achievements (
  id, user_id, achievement_id, unlocked_at
)
```

条件配置：

- `ac_count`：`{"threshold": 1}`，按去重后的 AC 题目数计。
- `contest_count`：`{"threshold": 5}`，按报名过的比赛数计。
- `login_streak`：`{"threshold": 30}`，按截至今天的连续登录天数计。
- `created_contest`：`{"threshold": 1}`，按用户创建的比赛数计。
- `all_of`：`{"conditions":[{"type":"ac_count","threshold":100},{"type":"contest_count","threshold":5}]}`。

规则：

- `code` 创建后不可修改，格式 `[a-z0-9][a-z0-9_-]{1,63}`。
- 条件阈值必须为正整数；`all_of` 至少 2 个条件，最多 8 个，且不允许嵌套 `all_of`。
- 解锁时在事务内写入 `user_achievements`；若插入成功，再发放金币与徽章。
- 金币奖励写 `coin_transactions`（`reason=achievement`, `ref_type=achievement`, `ref_id=成就id`）。
- 徽章奖励写 `user_badges`；若已拥有则跳过。
- 成就列表返回每个条件的当前值/目标值，供前端显示进度。
- 管理员新增成就后，已有用户在下一次读取成就/登录/AC 时补算解锁。
- 奖励徽章必须在配置成就时存在且上架；发放时直接写入 `user_badges`，**不占用商城库存**，即使徽章后来下线或库存为 0 也照常发放，保证已解锁成就的奖励不丢失。
- 已解锁的成就即使后来被管理员下线，仍在“我的成就”和成就页中显示；未解锁且已下线的成就仅管理员可见。

### 5.3 任务

```text
tasks (
  id, title, description_md, problem_ids_json,
  reward_coins, start_time, end_time,
  is_active, created_by, created_at, updated_at
)
user_task_records (
  id, user_id, task_id, status,
  claimed_at, completed_at
)
```

规则：

- `problem_ids_json` 为 1–20 个不重复的公开题目 id。
- 任务有效期 `start_time < end_time`；领取要求 `start <= now < end`。
- 领取写入 `claimed`；已存在记录时：`completed` 返回“已完成，不能再次领取”，`claimed` 幂等返回当前记录。
- 完成条件：对任务内全部题目，在 `claimed_at` 之后存在 `accepted` 提交。
- 完成后在同一事务内：更新记录为 `completed`、写金币流水、增加余额。
- 一个任务对同一用户只结算一次；重复 AC 事件不会重复发金币。
- 任务已有领取记录后，冻结 `problem_ids_json` 与 `reward_coins`；可修改标题、描述、结束时间与上下架状态。
- 删除为软删除；已领取用户仍可在截止前继续完成，完成后照常发奖。
- 完成判定同时要求 `claimed_at <= judged_at < end_time`；任务截止后未完成记录保持 `claimed`，不能领取、不能完成，也不能再次领取。
- 管理员延长结束时间后，已领取用户可在新截止前继续完成；缩短结束时间会导致超出新截止的 AC 不再计入。
- 任务大厅对已领取用户继续显示已下线或已结束的任务，并显示“已过期/已下线”状态；未领取用户只看到上架且未结束的任务。

### 5.4 徽章

```text
badges (
  id, name, description, image_url,
  price_coins, category, is_limited, stock,
  sort_order, is_active, created_at, updated_at
)
user_badges (
  id, user_id, badge_id, is_equipped, purchased_at
)
```

规则：

- `price_coins >= 0`；`stock` 仅在 `is_limited=true` 时有意义，必须 `>= 0`。
- 购买：校验上架、未拥有、库存、余额；锁定徽章行，扣库存、扣金币、写流水、写 `user_badges`，全在同一事务。
- 装备上限读取配置（默认 3）；装备前锁定用户行并重新统计，避免并发超限。
- 装备/卸下只影响 `is_equipped`；同一徽章不能重复装备。
- 删除为软删除；已购买用户仍保留并展示。
- 图片：
  - 管理员可填 http(s) 外链，或上传 png/jpeg/webp/gif（≤ 2 MiB）。
  - 上传文件存 `badges/<id>.<ext>`，`image_url` 写为 `/api/badges/<id>/image`。
  - 公开图片接口从 `Storage` 读取，设置 `Cache-Control: public, max-age=3600`。
- `GET /badges` 返回 `owned`、`equipped`；游客只看到商品牌面。
- 已拥有但已下线的徽章仍出现在自己的商城列表/装扮管理中，可继续装备或卸下；未拥有的下线徽章仅管理员可见。
- `category` 为管理员填写的自由文本（1–20 字，默认“其他”），前端按数据中出现的分类聚合筛选；不做固定枚举。
- 限量库存只约束商城购买；成就奖励发放不扣库存。

### 5.5 称号

```text
titles (
  id, name, color, unlock_condition_type, unlock_condition_json,
  price_coins, sort_order, is_active, created_at, updated_at
)
user_titles (
  id, user_id, title_id, is_equipped,
  unlocked_at, purchased_at
)
```

解锁条件：

- `rating`：`{"rating": 1600}`，当前用户 Rating ≥ 阈值。
- `ac_count`：`{"threshold": 100}`，去重 AC 题目数 ≥ 阈值。
- `achievement`：`{"achievementCode": "first-ac"}`，用户已解锁该成就。

规则：

- `color` 为 `#RRGGBB`；`price_coins >= 0`。
- 满足条件时写入 `user_titles`（`unlocked_at=now`，`purchased_at=NULL`）。
- 购买要求已解锁且未购买；扣除金币并设置 `purchased_at`。
- 装备要求已购买；同一事务内先将该用户所有称号 `is_equipped=false`，再装备目标称号。
- 删除为软删除；已解锁/已购买记录保留，仍可在“我的称号”和装扮管理中查看、购买（若已解锁未购买）和装备。
- 未解锁且已下线的称号仅管理员可见；已解锁未购买的称号仍可购买，避免用户已达成条件却因下架无法兑换。
- Rating 结算在 M4 上线后，读取称号列表时补算即可解锁对应称号。

### 5.6 登录日

```text
user_login_days (
  id, user_id, login_date, created_at
)
```

- `login_date` 使用服务端本地日期（`time.Local`）。
- 登录成功时同步写入（唯一约束去重），再投递 `user.login` 事件。
- 连续登录按“今天向前连续存在的日期数”计算；当天未登录时当前连续天数为 0。

## 6. 公开身份展示

新增统一公开身份结构：

```json
{
  "equippedBadges": [
    { "id": 1, "name": "早鸟", "imageUrl": "/api/badges/1/image" }
  ],
  "equippedTitle": { "id": 3, "name": "卷王", "color": "#7C3AED" }
}
```

展示位置：

- `GET /users/:id/profile`：个人主页头部。
- `GET /submissions`、`GET /submissions/:id`：提交者昵称旁。
- `GET /contests/:id/ranklist`：榜单选手列。
- 前端统一用 `IdentityShowcase` 组件渲染；称号优先显示带颜色的文字，徽章显示小图标 + tooltip。

实现方式：

- 每页/每榜按用户 id 批量查询装备中的徽章与称号，避免 N+1。
- 徽章按 `sort_order, id` 排序；称号最多 1 个。
- 即使徽章/称号后来被管理员下线，已装备的仍展示，避免用户“装备消失”。

## 7. API

所有接口继续使用统一 `{ code, message, data }` 包装；分页使用 `{ total, items }`。

### 7.1 成就

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/achievements` | 公开（可选登录） | 成就列表；登录时含解锁状态与进度；`includeInactive=true` 仅 super_admin |
| GET | `/api/users/me/achievements` | 登录 | 我的成就（已解锁 + 未解锁） |
| POST | `/api/achievements` | super_admin | 创建 |
| PUT | `/api/achievements/:id` | super_admin | 更新（code 不可改） |
| DELETE | `/api/achievements/:id` | super_admin | 软删除 |

### 7.2 任务

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/tasks` | 公开（可选登录） | 任务大厅；登录时含领取/完成/进度；`includeInactive=true` 仅 super_admin |
| POST | `/api/tasks` | super_admin | 创建 |
| PUT | `/api/tasks/:id` | super_admin | 更新（已领取后冻结题目与奖励） |
| DELETE | `/api/tasks/:id` | super_admin | 软删除 |
| POST | `/api/tasks/:id/claim` | 登录 | 领取 |

### 7.3 徽章

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/badges` | 公开（可选登录） | 商城；登录时含 `owned`/`equipped`；`includeInactive=true` 仅 super_admin |
| POST | `/api/badges` | super_admin | 创建 |
| PUT | `/api/badges/:id` | super_admin | 更新 |
| DELETE | `/api/badges/:id` | super_admin | 软删除 |
| POST | `/api/badges/:id/image` | super_admin | multipart 上传图片 |
| GET | `/api/badges/:id/image` | 公开 | 读取上传图片 |
| POST | `/api/badges/:id/purchase` | 登录 | 购买 |
| POST | `/api/badges/:id/equip` | 登录 | 装备 |
| POST | `/api/badges/:id/unequip` | 登录 | 卸下 |
| GET | `/api/users/:id/badges` | 公开 | 用户拥有的徽章及装备状态 |

### 7.4 称号

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/titles` | 公开（可选登录） | 商店；登录时含解锁/购买/装备状态与条件进度；`includeInactive=true` 仅 super_admin |
| POST | `/api/titles` | super_admin | 创建 |
| PUT | `/api/titles/:id` | super_admin | 更新 |
| DELETE | `/api/titles/:id` | super_admin | 软删除 |
| POST | `/api/titles/:id/purchase` | 登录 | 购买 |
| POST | `/api/titles/:id/equip` | 登录 | 装备 |
| POST | `/api/titles/:id/unequip` | 登录 | 卸下 |

### 7.5 金币

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/coins/transactions` | 登录 | 我的金币流水，分页，含当前余额 |
| POST | `/api/admin/users/:id/coins` | super_admin | 管理员调整金币（正负均可，附备注） |

返回示例：

```json
{
  "balance": 320,
  "total": 12,
  "items": [
    { "id": 9, "amount": -100, "type": "spend", "reason": "badge_purchase",
      "refType": "badge", "refId": 2, "note": "", "createdAt": "..." }
  ]
}
```

## 8. 前端

### 8.1 路由与导航

| 路由 | 页面 | 权限 |
|------|------|------|
| `/achievements` | 成就墙 | 公开 |
| `/tasks` | 任务大厅 | 公开 |
| `/shop/badges` | 徽章商城 | 公开 |
| `/shop/titles` | 称号商店 | 公开 |
| `/appearance` | 装扮管理（徽章/称号/金币明细） | 登录 |
| `/admin/gamification` | 游戏化管理（成就/任务/徽章/称号四个 Tab） | super_admin |

顶部导航新增“成长”入口，下列成就、任务、徽章商城、称号商店、装扮管理；用户菜单显示金币余额并链接装扮管理。管理入口仅 super_admin 可见。

### 8.2 页面行为

- 成就页：卡片网格；已解锁显示时间与奖励；未解锁显示条件进度条；支持“全部/已解锁/未解锁”状态筛选（前端过滤）。
- 任务大厅：任务卡片显示奖励、时间、题目数；未登录/未领取显示“登录领取”“领取”；已领取显示进度与“进行中”；已完成显示完成时间。
- 徽章商城：图片墙、价格、库存、分类；已拥有显示“已拥有/装备中”；购买前确认，成功后刷新余额与列表；金币不足禁用购买。
- 称号商店：名称按配置颜色展示；显示解锁条件进度；未解锁、可购买、已购买、装备中四种状态。
- 装扮管理：金币余额与流水；徽章网格可装备/卸下（按钮显示 1/3–3/3）；称号单选装备/卸下；操作后刷新 `auth` 中的用户信息。
- 游戏化管理：
  - 成就：CRUD、条件构造器（单条件/all_of）、奖励金币与徽章选择、排序、上下架。
  - 任务：CRUD、远程搜索题目、Markdown 描述、时间、奖励；已有领取后冻结题目与奖励。
  - 徽章：CRUD、图片上传预览、分类文本、限量库存、价格、排序、上下架。
  - 称号：CRUD、颜色选择、解锁条件、价格、排序、上下架。
- 公开展示：
  - 个人主页头部显示装备徽章与称号。
  - 测评记录用户列显示装备徽章/称号。
  - 比赛榜单选手列显示装备徽章/称号。
- 移动端：卡片/表格可横向滚动；管理页表单在窄屏单列。

### 8.3 前端状态

- 新增 `api/gamification.ts` 与生成类型。
- 新增 `IdentityShowcase.vue`、`CoinBalance.vue`、条件标签工具。
- `auth` store 增加 `refreshMe()`；购买、领取奖励、装备后刷新余额。
- 金币显示以服务端返回为准，不做本地乐观加减。

## 9. 权限与安全

- 公开读：成就、任务、徽章商城、称号商店、他人徽章、公开资料、徽章图片。
- 登录写：领取任务、购买/装备/卸下徽章与称号、查看自己的金币流水。
- super_admin：全部管理接口、管理员调币。
- 图片上传校验：大小、MIME、扩展名、存储 key；文件名不接受用户输入。
- 管理接口统一 `middleware.RequireAuth` + `RequireRole(super_admin)`。
- 所有用户输入继续做长度、枚举、整数范围与 Markdown 长度校验；错误信息中文。
- 不在日志中输出金币备注之外的敏感信息；图片读取不暴露存储根目录。

## 10. 错误与边界

- 40001：条件配置非法、任务题目为空/重复/不存在、时间非法、余额不足、库存不足、已拥有、未解锁、未购买、装备超限、任务已截止等。
- 40300：非 super_admin 调用管理接口。
- 40400：资源不存在或已下线且非管理员。
- 40400 也用于 `/api/badges/:id/image` 图片不存在。
- 重复领取已完成任务、重复购买、重复装备、重复领取已领取任务：返回 40001 或幂等成功；具体在实现计划中固定，并写入 OpenAPI 描述。
- 已领取任务到期后状态为 `expired`（由读取时按时间计算，不额外落库）；已领取任务被下线时仍显示为“已下线”，不可继续完成。
- 并发购买限量徽章必须只有一个成功；并发装备不能超过上限。
- 任务结束边界 `now >= end_time` 不允许领取；完成判定使用 AC 提交的 `judged_at`。

## 11. 测试与验收

### 11.1 后端

- 金币：入账/消费/管理员调整、余额非负、流水唯一、事务回滚。
- 成就：单条件、all_of、重复事件、并发事件、金币/徽章奖励只发一次、进度值。
- 任务：领取窗口、重复领取、领取前 AC 不计入、领取后 AC 自动完成、只结算一次、已有领取后冻结题目与奖励。
- 徽章：上传/读取、购买扣款、限量并发、装备上限、卸下、软删除后仍展示。
- 称号：三种解锁条件、购买、装备互斥、Rating 条件预留。
- 公开身份：批量查询、个人资料、测评记录、比赛榜单。
- 事件：judge AC 后投递；worker 处理重复/过期/非 AC 事件；读取时补算。
- API：角色 403、游客行为、参数校验、分页。

### 11.2 前端

- 工具函数单测：条件标签、金币格式、称号颜色校验。
- 组件测试：IdentityShowcase、状态标签。
- lint、类型检查、构建。

### 11.3 真实浏览器 E2E

1. 管理员创建成就（AC 1 题）与任务（1 题，奖励金币）。
2. 用户登录，领取任务，提交并 AC。
3. 成就自动解锁、任务自动完成、金币到账且流水各一条。
4. 管理员上架限量徽章与称号；用户购买、装备，余额正确扣减。
5. 个人主页、测评记录、比赛榜单显示装备；徽章图片可加载。
6. 重复 AC/刷新页面不重复发奖。
7. 普通用户访问管理接口/页面被拒绝；游客不能领取/购买。
8. 移动端页面可滚动阅读。

## 12. 交付拆分

### M3-A：金币、事件、成就、任务（后端）

- 金币流水/余额/管理员调币。
- Asynq game 队列、judge AC 事件、登录与参赛事件。
- 成就 CRUD、条件求值、金币奖励、读取补算。
- 任务 CRUD、领取、AC 自动完成、结算。
- 登录日记录。
- `achievements.reward_badge_id` 列先建好；M3-A 要求该字段为空（暂不支持徽章奖励），M3-B 徽章模型落地后开放配置与发放。
- OpenAPI 与后端测试。

### M3-B：徽章、称号、公开身份（后端）

- 徽章 CRUD、图片上传/读取、购买、限量库存、装备上限。
- 成就奖励徽章发放（不占库存）。
- 称号 CRUD、解锁、购买、装备互斥。
- 公开身份批量查询。
- 个人资料、测评记录、比赛榜单接入。
- OpenAPI 与后端测试。

### M3-C：前端、文档与验收

- 成就页、任务大厅、徽章商城、称号商店、装扮管理、游戏化管理。
- 导航、金币显示、IdentityShowcase、auth 刷新。
- 更新 `DEVELOPMENT.md`、`docs/任务清单.md`、`docs/开发日志.md`、`README.md`、`运行手册.md`。
- 全量测试与真实浏览器 E2E。

## 13. 风险与取舍

- **读取时补算会写库**：`GET /achievements`、`GET /tasks`、`GET /titles` 可能触发解锁与发奖。这是为了事件丢失时自愈；接口仍是幂等的，但会带来少量写放大，可接受。
- **Asynq 事件可能重复**：依赖唯一约束与行锁，不依赖“只投递一次”。
- **事件可能丢失**：读取补算兜底；金币奖励可能延迟到用户下次访问相应页面。
- **公开身份查询增加列表开销**：按页批量查询，限制在 3 个徽章 + 1 个称号。
- **Rating 条件暂不可达**：M4 上线 Rating 结算后自然生效；M3 测试管理员可手动调整 Rating？不提供该接口，测试用直接 SQL 或 M4 后再验收。
- **管理员调整金币是测试/运营后门**：仅 super_admin，必须写备注，流水可审计。

## 14. 留给实施计划的决定

- 具体 GORM 模型字段顺序、索引命名、仓储接口拆分。
- Asynq worker 的并发配置与优雅关闭细节。
- 前端管理页的具体组件拆分与表格/弹窗布局。
- 测试数据库夹具的具体组织方式。
- OpenAPI schema 的字段描述与示例。

以上均不改变本文确认的产品行为与权限边界。
