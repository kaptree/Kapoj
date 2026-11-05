# M4 第一阶段：Rating、榜单与签到设计

> 依据 `DEVELOPMENT.md` 3.6、5.5 与 `docs/任务清单.md` M4-1–M4-5。
> M4 已扩展为多个阶段；本文只覆盖第一阶段：Rating 结算、等级、Rating 历史、三个榜单、活跃度、签到金币、共享积分与对应前端。
> 学习路线（含管理前端/用户自定义）、社交互动（评论/点赞/关注/题解/通知）、Rating 衰减/赛季重置属于后续 M4 阶段，另行设计。

## 1. 目标

- **Rating**：Rated 比赛结束后按 Codeforces 风格 Elo 变体结算，写入 `rating_history` 并更新 `users.rating`；同一用户同一比赛只结算一次。
- **等级颜色**：8 个 Rating 等级（灰/绿/青/蓝/紫/橙/红/黑红），提供名称、颜色与黑红特殊渲染数据。
- **Rating 历史**：公开用户 Rating 变化记录与简单曲线，展示每场比赛的赛前/赛后 Rating 和名次。
- **三个榜单**：Rating 榜（默认）、活跃度榜、共享榜；分页、公开、装备徽章/称号、Redis 缓存。
- **活跃度**：提交 +2、参赛 +10、每日签到 +5（活跃度）；每日上限与对象去重。
- **签到金币**：每日首次签到额外 +5 金币，写金币流水；每天只一次。
- **共享积分**：用户创建/克隆比赛 +20；`shared_items` 为后续题解点赞预留。
- **前端**：排行榜页（三 Tab、等级颜色、签到）、用户 Rating 历史卡片/曲线、导航与文档。

## 2. M4 阶段划分

| 阶段 | 内容 | 状态 |
|------|------|------|
| M4-1（本文） | Rating、等级、Rating 历史、三榜、活跃度、签到金币、共享积分、前端 | 当前设计 |
| M4-2 | 学习路线：树、管理前端、用户自定义路线 | 后续设计 |
| M4-3 | 社交：评论、点赞、关注、用户题解、通知 | 后续设计 |
| M4-4 | Rating 衰减、赛季重置 | 后续设计 |

M5（证书、AI、题包导入、头像上传）仍属于 M5，不并入 M4。

## 3. 非目标（本阶段）

- 不实现评论、点赞、关注、题解、通知；`activity_logs`/`shared_items` 结构保留，后续阶段接入。
- 不实现 Rating 衰减/赛季重置；本阶段只做比赛结算。
- 不实现学习路线。
- 不实现赛季表；`rating_history` 预留 `kind`/`contest_id` 结构，后续扩展。
- 不实现管理员 Rating 调整接口；如需修正用数据库/后续阶段。
- 不实现 M5 功能。

## 4. Rating

### 4.1 参与结算的选手

- 比赛报名者中，在该比赛时间窗内至少创建过 1 次提交的用户。
- 0 提交报名者不参与 Rating，避免“报名不参赛也掉分”。
- 使用比赛现有榜单排名（ICPC/IOI/OI 规则），并按 `LastSubmissionAt != nil` 过滤。
- M4-A 需补齐 `contest` 榜单 ICPC 路径未设置 `LastSubmissionAt` 的问题；该字段仅内部使用，不新增 API 字段。

### 4.2 算法 v1

1. 设参与人数 `n`，赛前 Rating `R_i`。
2. 期望得分：

   `E_i = Σ_{j≠i} 1 / (1 + 10^((R_j - R_i) / 400))`

3. 实际得分：`A_i = n - rank_i`（rank 为 1-based；并列名次相同）。
4. `delta_i = round(32 * (A_i - E_i))`，限制在 `[-100, 100]`。
5. 新 Rating：`max(0, R_i + delta_i)`。
6. `n <= 1` 时 delta=0，仍记录历史。
7. 同一场比赛所有人使用结算开始时的 Rating 快照；不按更新后的 Rating 继续计算。
8. 公式版本写入代码注释与文档；后续可调 K/上限。

### 4.3 结算事务

- 结算前获取 PostgreSQL 全局事务级 advisory lock（固定 key），保证不同比赛不会并发读取同一批用户的旧 Rating；扫描器按 `end_time ASC, id ASC` 顺序逐场处理，多实例也串行。
- 锁定 `contests` 行，确认 `is_rated && end_time < now && rating_settled_at IS NULL`。
- 若仍有 `pending`/`judging` 提交且 `now < end_time + KAPOJ_RATING_JUDGE_GRACE`（默认 10 分钟），本轮跳过；超过宽限期用当前已判结果结算并设置 marker，避免卡死。
- 逐人 `INSERT rating_history ... ON CONFLICT (user_id, contest_id) WHERE contest_id IS NOT NULL DO NOTHING`；只有插入成功才更新 `users.rating`。
- 全部完成后设置 `contests.rating_settled_at=now`；无参与者也要设置。
- 结算失败不设置 marker，下轮重试。

### 4.4 扫描器

- 默认每 60 秒扫描最多 20 场；`KAPOJ_RATING_SCAN_INTERVAL` 配置（10s–10m）。
- 评测宽限 `KAPOJ_RATING_JUDGE_GRACE` 默认 10m（1m–60m）。
- 扫描失败记录日志，不退出进程；context 取消后停止 ticker。

### 4.5 等级

| key | 名称 | 颜色 | Rating |
|-----|------|------|--------|
| gray | 灰名 | `#808080` | 0–1199 |
| green | 绿名 | `#008000` | 1200–1399 |
| cyan | 青名 | `#03A89E` | 1400–1599 |
| blue | 蓝名 | `#0000FF` | 1600–1899 |
| purple | 紫名 | `#AA00AA` | 1900–2099 |
| orange | 橙名 | `#FF8C00` | 2100–2399 |
| red | 红名 | `#FF0000` | 2400–2599 |
| black_red | 黑红名 | `#000000` + accent `#FF0000` | 2600+ |

- API 返回 `level: {key,name,color,accentColor}`；黑红名 `accentColor` 非空，前端首字符黑、后续红。
- Rating <0 按 0 处理（结算时已 clamp）。

### 4.6 Rating 历史

- `GET /api/users/:id/rating-history?page=&page_size=` 公开；返回按时间倒序的 `rating_history`（只含 `kind=contest` 的本阶段数据），字段：`contestId, contestTitle, rank, ratingBefore, ratingAfter, delta, createdAt`。
- 用户主页新增“Rating 历史”卡片：表格 + 简单 SVG 折线（无新增图表依赖）；无记录显示空状态。
- 不提供修改/删除历史接口。

## 5. 活跃度与签到

### 5.1 积分规则

| action | 活跃度 | 金币 | 对象/去重 | 每日上限 |
|--------|--------|------|-----------|----------|
| submission | 2 | 0 | `ref_type=submission`, `ref_id=提交id` | 40 活跃度 |
| contest_join | 10 | 0 | `ref_type=contest`, `ref_id=比赛id` | 50 活跃度 |
| signin | 5 | 5 | `ref_type=day`, `ref_id=YYYYMMDD` | 每天 1 次 |

- 每日上限按 `(user_id, action, 本地日期)` 求和；达到上限后不再写活跃度，返回 no-op，不影响主操作。
- 签到金币写入 `coin_transactions`（`reason=signin`, `ref_type=day`, `ref_id=YYYYMMDD`），与活跃度日志、余额更新在同一事务；重复签到不重复发币。
- 并发：签到事务锁用户行，先查 `activity_logs` 当天记录，再写日志+流水+余额。
- `activity_logs` 唯一约束 `(user_id, action, ref_type, ref_id)`。
- 提交创建钩子：`submissionapi` 创建成功后调用 `activity.RecordSubmission`；失败只日志。
- 参赛钩子：`contestapi` 报名成功后调用 `activity.RecordContestJoin`；失败只日志。
- 签到接口：`POST /api/activity/signin`；返回 `{ awarded, activityPoints, coins, date }`。
- 摘要接口：`GET /api/activity/summary` 返回 `{ signedInToday, todayPoints, totalPoints }`。
- 活跃度榜只包含总活跃度 >0 的用户，按总分倒序、user_id 升序；分页。

### 5.2 共享积分

```text
shared_items (
  id, user_id, type,      -- contest / solution（本阶段只用 contest）
  ref_id, like_count, created_at
)
-- 唯一约束：(type, ref_id)
```

- 创建与克隆比赛都写入 `type=contest`，每场 20 分；与 `created_by` 语义一致。
- 服务启动时幂等回填已有 `contests.created_by>0` 到 `shared_items`，`ON CONFLICT DO NOTHING`。
- 共享榜分数 = `20 * contest shared_items 数 + 5 * solution like_count`；本阶段第二项恒为 0。
- 共享榜只包含分数 >0 的用户，按分数倒序、user_id 升序；分页。

## 6. 榜单与缓存

### 6.1 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/leaderboard/rating?page=&page_size=` | Rating 榜，默认 |
| GET | `/api/leaderboard/activity?page=&page_size=` | 活跃度榜 |
| GET | `/api/leaderboard/sharing?page=&page_size=` | 共享榜 |

行结构：

```json
{
  "rank": 1,
  "userId": 2,
  "username": "m2bsolver1",
  "nickname": "m2bsolver1",
  "score": 1500,
  "level": { "key": "cyan", "name": "青名", "color": "#03A89E", "accentColor": null },
  "equippedBadges": [],
  "equippedTitle": null
}
```

- Rating 榜 `score` 为 Rating；活跃度/共享榜 `score` 为积分。
- 使用现有 `gamification.Service.Identities` 批量装饰装备；分页后一次查询。
- 纯游客可看榜单；不要求登录。

### 6.2 Redis 缓存

- 缓存基础行（不含装备），key `kapoj:leaderboard:<kind>:v<version>:<page>:<size>`，TTL 60 秒。
- 每种榜单维护 Redis 版本号 `kapoj:leaderboard:<kind>:version`；Rating 结算、活跃度写入、共享项写入后 `INCR`。
- 读取缓存命中直接使用；未命中查库并写缓存；Redis 错误降级查库，不返回 5xx。
- 只缓存基础行，装备变更无需失效缓存。
- 分页上限沿用 `httpx.ParsePage`（page_size ≤100）。

## 7. 数据模型

新增/修改：

```text
rating_history (
  id, user_id, contest_id NULL, kind,   -- 本阶段 kind=contest
  rank, rating_before, rating_after, created_at
)
-- 唯一约束：(user_id, contest_id) WHERE contest_id IS NOT NULL
-- 为后续 decay/season 预留 kind 与可空 contest_id

activity_logs (
  id, user_id, action, points, ref_type, ref_id, created_at
)
-- 唯一约束：(user_id, action, ref_type, ref_id)
-- 索引：(user_id, created_at), (created_at)

shared_items (
  id, user_id, type, ref_id, like_count, created_at
)
-- 唯一约束：(type, ref_id)

contests.rating_settled_at timestamptz NULL
```

- 迁移加入 `db.Migrate`；`shared_items` 回填在服务启动后执行。
- `rating_history.contest_id` 可空，本阶段只写比赛结算；后续衰减/赛季可写 `kind=decay/season_reset` 且 `contest_id=NULL`。

## 8. 权限与安全

- 三个榜单、Rating 历史公开；签到/摘要需要登录。
- 提交/参赛活跃度由服务端钩子写入；不提供客户端直接加分接口。
- 签到金币与流水同一事务，余额不能为负（签到只加）。
- 榜单缓存不含邮箱、角色等敏感字段；只含公开用户名/昵称/分数/等级/装备。
- Rating 结算只读提交时间/状态与榜单，不返回代码。
- 管理接口：本阶段无新增管理接口；比赛创建/克隆权限沿用现有 contest_admin/super_admin。

## 9. 前端

### 9.1 排行榜页

- 路由 `/leaderboard`，导航移除“M4 上线”。
- 三个 Tab：Rating（默认）、活跃度、共享。
- 表格列：排名、用户（昵称 + 装备展示）、分数、等级（Rating Tab）。
- 用户列链接个人主页；等级按颜色渲染，黑红名首字符黑、其余红。
- 活跃度 Tab 顶部显示今日积分/总积分与“签到 +5 金币”按钮；已签到禁用并显示中文状态。
- 分页、加载/错误/空状态；移动端表格横向滚动。

### 9.2 Rating 历史

- 用户主页（`/users/:id`）增加“Rating 历史”卡片：最近若干场比赛表格 + 简单 SVG 折线；无记录显示“暂无 Rating 记录”。
- 不在本阶段做独立 Rating 历史页；卡片足够覆盖非目标中的“Rating 历史曲线页面”。

### 9.3 导航

- 侧边栏“排行榜”启用。
- 不新增学习路线/社交入口（后续阶段）。

## 10. 测试与验收

### 10.1 后端

- Rating 算法：已知 3/5 人案例、并列、n=1、负数 clamp、K=32/±100。
- 只结算有提交的报名者；0 提交不参与。
- 重复扫描/并发结算只写一次 rating_history，users.rating 只更新一次；不同比赛结算串行。
- 无参与者比赛也设置 settled_at。
- 开赛后 `is_rated` 冻结。
- 等级 8 个边界（1199/1200、…、2599/2600）与黑红 accent。
- 活跃度：提交/参赛/签到写入；每日上限；重复 ref 去重；签到幂等；签到金币只发一次；并发不超上限；钩子失败不影响主操作。
- 共享：创建/克隆比赛写 shared_item；启动回填幂等；共享分计算。
- 榜单：三榜排序/分页/空数据；装备装饰；缓存命中、版本失效、Redis 降级。
- Rating 历史接口分页/排序/不存在用户。
- API：公开/登录/管理员 403（本阶段无管理接口）；签到/摘要参数校验。

### 10.2 前端

- 工具函数单测：等级颜色/黑红渲染、榜单分页、Rating 历史 delta。
- 组件测试：`RatingLevelName`（黑红首字符拆分）、Rating 历史空状态/数据。
- lint、类型检查、build。

### 10.3 真实浏览器 E2E

1. 结束一场 Rated 比赛或触发扫描，Rating 榜更新、等级颜色正确、`rating_history` 一条。
2. 用户提交/参赛/签到，活跃度榜分数与每日上限正确；签到金币到账且重复签到不重复发。
3. 共享榜显示比赛创建/克隆分。
4. 三榜分页、装备展示、黑红名/普通等级颜色正确；移动端表格可滚动。
5. 用户主页 Rating 历史表格/曲线；无记录空状态。
6. 游客可看榜单/历史但不能签到；未登录访问签到 401。
7. 无 CDN/控制台错误；缓存失效后榜单及时更新。

## 11. 交付拆分

### M4-A：Rating、等级、历史与 Rating 榜（后端）

- RatingHistory 模型、等级映射、Elo v1 算法。
- `contests.rating_settled_at`、开赛后 `is_rated` 冻结。
- 修复 ICPC 榜单 `LastSubmissionAt`。
- 全局 advisory lock、按截止时间顺序、评测宽限。
- 周期扫描器、事务结算、并发/幂等测试。
- Rating 榜 API 与缓存基础、Rating 历史 API。
- OpenAPI/后端测试。

### M4-B：活跃度、签到金币、共享分与三榜（后端）

- ActivityLog、SharedItem 模型与回填。
- 提交/参赛/签到钩子；每日上限/去重；签到金币；签到/摘要接口。
- 三个榜单 API、装备装饰、Redis 版本缓存。
- OpenAPI/后端测试。

### M4-C：前端、文档与验收

- 排行榜页（三 Tab、等级颜色、签到）。
- 用户主页 Rating 历史卡片/曲线。
- 导航、文档、全量检查、真实浏览器 E2E。

## 12. 风险与取舍

- **Rating 公式是近似 Codeforces**：K=32、±100、只统计有提交者，与 Codeforces 实际公式不会完全一致；版本 v1 写入注释和文档。
- **后台扫描有延迟**：默认最多 60 秒；可配置更短。
- **全局 advisory lock 降低结算吞吐**：所有比赛串行结算，但 Rated 比赛数量远低于提交量，可接受。
- **活跃度/共享榜本阶段不完整**：评论/点赞/关注/题解点赞未实现；后续 M4 阶段接入后扩展。
- **签到金币固定 5**：本阶段不做连续签到/倍率；后续可配置。
- **Rating 历史页嵌在用户主页**：不单独开页面，减少前端范围；后续可独立。
- **缓存最终一致**：版本号失效 + 60 秒 TTL；Redis 故障降级数据库。
- **Rating 结算影响称号解锁**：结算后不主动遍历所有用户；用户访问称号页时读取补算，可能略有延迟。

## 13. Builder check 决议

1. 不同比赛结算串行：PostgreSQL 全局 advisory lock + `end_time` 顺序。
2. ICPC 参与判定：修 `LastSubmissionAt`，不另写查询。
3. 评测中提交：默认等 10 分钟，超过宽限按已判结果结算。
4. 活跃度/共享范围：本阶段只统计已有行为；社交留 M4-3。
5. 克隆比赛：计入共享分，与 `created_by` 一致。
6. Rating 历史表：`kind` + 可空 `contest_id`，为衰减/赛季预留；本阶段只写 contest。

## 14. 留给实施计划的决定

- 具体包拆分（`rating`/`activity` 内部文件）。
- Redis 版本 key 的 JSON 编码与缓存封装。
- SVG 曲线的具体实现。
- OpenAPI schema 描述与示例。
- E2E 夹具与 Rated 比赛测试数据组织。

以上不改变本文确认的产品行为、算法边界与权限。
