# M4-4 Rating 衰减与赛季重置设计

> 目标：为高分段引入可配置的温和 Rating 衰减，并按赛季对所有用户做软重置；衰减/重置写入 `rating_history` 并在用户主页可见；策略由 `rating.settings.manage` 权限在管理后台调整。

## 1. 背景与现状

- 现有 Rating 只由 Rated 比赛结算产生：`rating_history.kind=contest`，Elo v1，`delta` 限制 ±100，新 Rating `max(0, ...)`。
- `rating_history` 已预留可空 `contest_id` 与 `kind`，但只有比赛结算；没有衰减/赛季记录、赛季表或幂等运行标记。
- `users.rating` 是当前 Rating 缓存；Rating 榜按 `users.rating` 排序，结算成功后递增 Redis 版本键。
- `rating/runs` 尚未实现；现有扫描器每 60 秒结算已结束的 Rated 比赛。

## 2. 目标与非目标

### 目标

- 高分段衰减：仅当 Rating > 可配置阈值（默认 2400）且连续未参加 Rated 比赛达到可配置天数（默认 60 天）时，按可配置月比例（默认 3%）向阈值衰减。
- 赛季重置：按可配置周期（默认 6 个月）对所有 Rating ≠ 1500 的用户向 1500 软重置可配置比例（默认 30%）。
- 策略可在管理后台查看/修改；新增权限 `rating.settings.manage`。
- 衰减与重置写入 `rating_history`（`kind=decay` / `season_reset`），用户主页 Rating 历史展示类型；公开赛季信息可查询。
- 幂等：重复扫描、多实例、重启不会重复扣分/重置；失败可重试。

### 非目标

- 不做按题目/比赛/活跃度的衰减，不做排名奖励、赛季奖励、称号/徽章赛季限定。
- 不提供手动“立即重置全站”按钮；赛季重置由后台任务按配置自动执行。
- 不删除/重写历史比赛结算记录；不做 Rating 算法变更。
- 不做管理员逐用户手动调 Rating；不新增用户侧关闭衰减的开关。
- 不做赛季排行榜快照/归档；Ranking 榜仍按当前 `users.rating` 实时查询。

## 3. 已确认策略

### 3.1 衰减

- 触发条件：
  - `decayEnabled=true`；
  - `users.status=active`；
  - `users.rating > decayThreshold`（默认 2400）；
  - 最近一次 `rating_history.kind=contest` 的 `created_at` 距当前 ≥ `decayInactiveDays`（默认 60 天）；无比赛历史的用户不衰减。
- 每月最多执行一次，运行键 `decay:YYYY-MM`（UTC）。
- 单次衰减：
  ```text
  above = rating - threshold
  raw   = round(above * decayPercent / 100)
  decay = min(decayMax, max(1, raw))   // 只对 rating > threshold
  new   = max(threshold, rating - decay)
  ```
  默认 `decayPercent=3`、`decayMax=100`。例如 2600 分、阈值 2400：`above=200`，`raw=6`，降到 2594；2401 分降 1 到 2400；2450 分 `raw=round(1.5)=2`，降到 2448。
- 取整统一使用 `math.Round`（与现有 Elo 结算一致），禁止整数截断；`rating > threshold` 时 `decay` 至少为 1。
- 衰减记录：`kind=decay`、`periodKey=YYYY-MM`、`rank=0`、`ratingBefore/ratingAfter`、`delta=after-before`、`createdAt=now`。
- 若当月运行已处理过（`rating_runs.processed_at` 非空），跳过；若服务跨月停机，只处理当前月，不回补历史月份。

### 3.2 赛季重置

- 赛季由锚点日期 `seasonAnchor`（默认当前月 1 日 UTC）与 `seasonMonths`（默认 6）计算；赛季 `[anchor + n*months, anchor + (n+1)*months)`。
- 赛季结束后首次扫描处理“刚结束的上一赛季”：
  - 计算当前赛季 `S` 与上一赛季 `S-1`；运行键 `season:<S-1 开始日期>`（例如 `season:2026-01-01`）。
  - 若服务跨多个赛季停机，只处理最近结束的上一赛季一次，不连续回补多个赛季；更早赛季视为跳过。
- 重置公式（对所有 `rating != 1500` 的用户，含非 active 用户）：
  ```text
  new = max(0, round(1500 + (rating - 1500) * (100 - seasonResetPercent) / 100))
  ```
  默认 `seasonResetPercent=30`：2600 → 2270；1200 → 1290；1500 不变（不写历史）。
- 记录：`kind=season_reset`、`periodKey=赛季开始日期`、`rank=0`、`ratingBefore/ratingAfter`、`delta`、`createdAt=now`。
- 若某用户计算出的 `new == rating`（含 `seasonResetPercent=0`、1500、或四舍五入后不变），不写该用户历史、不更新 Rating；本次赛季运行仍会标记为已处理（`processed_at=now`），即“该赛季边界已过去”，后续提高比例不会追溯重置该赛季。
- 赛季名称/展示：使用日期范围（`YYYY-MM-DD ~ YYYY-MM-DD`）；公开接口返回当前赛季的 `key/name/startsAt/endsAt/nextResetAt`。
- 锚点默认取首次读取设置时的当前月 1 日；创建默认设置时，把该锚点下“上一赛季”的运行键预标记为已跳过（`processed_at=now`），因此首次部署不会立即触发一次历史赛季重置，第一次重置发生在新赛季结束后。管理员修改锚点/月数时同样预标记新的“上一赛季”，下一次扫描按新周期计算。

### 3.3 设置

单例 `rating_settings`，字段与默认值：

```text
id=1
decay_enabled             bool    true
decay_threshold           int     2400   -- 1500..4000
decay_inactive_days       int     60     -- 1..365
decay_percent             int     3      -- 1..100
decay_max                 int     100    -- 1..1000
season_enabled            bool    true
season_months             int     6      -- 1..24
season_reset_percent      int     30     -- 0..100
season_anchor             date    当前月 1 日
updated_by                int64
updated_at                timestamptz
```

- 修改设置不追溯已发生的衰减/重置；下一次运行使用新值。
- `seasonAnchor` 只能由管理员设置；设置为未来日期时当前赛季从未来开始，`nextResetAt` 相应变化。
- 修改 `seasonAnchor` 或 `seasonMonths` 时，在新周期下计算出的“上一赛季”运行键会以 `processed_at=now` 预标记为已跳过，避免改设置后立即触发一次全站重置；真正的重置发生在下一个完整赛季结束后。

## 4. 数据模型

### 4.1 rating_settings

见 3.3。单例主键 `id=1`；首次读取设置时若不存在则按默认值创建（`ON CONFLICT DO NOTHING`），`season_anchor` 默认取当次读取时间的当月 1 日（UTC），并预标记上一赛季运行键为已跳过。这样空库/测试库被清空后也能自动恢复默认设置且不会立即重置。

### 4.2 rating_runs

```text
rating_runs (
  key         text primary key,   -- decay:YYYY-MM / season:YYYY-MM-DD
  kind        text not null,      -- decay / season_reset
  processed_at timestamptz null,
  created_at  timestamptz not null
)
```

- 运行键唯一；`processed_at` 为空表示已认领但未完成，可被下次扫描重试。
- 认领与处理在同一事务内；失败回滚（不留下 processed 标记）。

### 4.3 rating_history 扩展

```text
rating_history (
  ... 现有列 ...
  period_key text not null default ''
)
-- 新增部分唯一索引：
-- (user_id, kind, period_key) WHERE period_key <> ''
```

- 比赛结算 `period_key=''`，继续使用现有 `(user_id, contest_id) WHERE contest_id IS NOT NULL` 唯一索引。
- 衰减/重置 `period_key` 非空；同一用户同一 `kind+period_key` 最多一行。
- `kind` 新增枚举值 `decay`、`season_reset`；`rank=0`、`contest_id=NULL`。

## 5. 后台任务与幂等

- 复用 Rating 扫描器 tick（默认 60 秒），在比赛结算后执行 `RunAdjustments(now)`；调整任务使用与结算相同的 PostgreSQL 全局 advisory lock，避免与比赛结算并发读写 Rating。
- 每次 tick：
  1. 读取 `rating_settings`；若衰减/赛季都关闭则返回。
  2. **衰减**：计算当前月键；在事务中检查/认领 `rating_runs`，锁定符合条件的用户行，逐人插入 history（`ON CONFLICT DO NOTHING`），仅插入成功才更新 `users.rating`；完成后 `processed_at=now`；无符合用户也标记完成。
  3. **赛季**：若 `seasonEnabled`，计算上一赛季；在事务中检查/认领 `rating_runs`，锁定 `rating != 1500` 的用户行，逐人插入 history + 更新 Rating；完成后标记；无符合用户也标记。
  4. 任一步成功修改 Rating 后递增 Rating 榜 Redis 版本键（best-effort）。
- 扫描失败只记录日志，不退出进程；事务回滚后下次重试。多实例由 advisory lock + `rating_runs` 唯一键串行。
- 不新增独立 worker 进程；复用现有 Rating worker 生命周期。

## 6. API

### 6.1 管理设置

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/admin/rating/settings` | `rating.settings.manage` | 返回设置 + 当前赛季 + 下次重置时间 |
| PUT | `/api/admin/rating/settings` | `rating.settings.manage` | 整体更新设置；校验范围；审计 |

`RatingSettingsView`：

```json
{
  "decayEnabled": true,
  "decayThreshold": 2400,
  "decayInactiveDays": 60,
  "decayPercent": 3,
  "decayMax": 100,
  "seasonEnabled": true,
  "seasonMonths": 6,
  "seasonResetPercent": 30,
  "seasonAnchor": "2026-11-01",
  "currentSeason": {
    "key": "2026-11-01",
    "name": "2026-11-01 ~ 2027-04-30",
    "startsAt": "2026-11-01T00:00:00Z",
    "endsAt": "2027-05-01T00:00:00Z",
    "nextResetAt": "2027-05-01T00:00:00Z"
  },
  "updatedAt": "..."
}
```

- PUT 只接受设置字段；`currentSeason` 只读。
- 校验：threshold 1500–4000；inactive days 1–365；percent 1–100；max 1–1000；months 1–24；reset percent 0–100；anchor 为合法日期。
- 设置修改写内容管理审计，target type `rating_settings`，action `rating.settings.manage`。

### 6.2 公开赛季

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/rating/season/current` | 当前赛季 `{key,name,startsAt,endsAt,nextResetAt}`；游客可读 |

### 6.3 Rating 历史

- `GET /api/users/:id/rating-history` 改为返回所有 kind：
  - `kind`: `contest|decay|season_reset`
  - `contestId`: 比赛为原 id；衰减/重置为 `0`
  - `contestTitle`: 比赛为标题（私密比赛仍为“私密比赛”）；衰减为“Rating 衰减”；重置为“赛季重置”
  - `rank`: 比赛为名次；衰减/重置为 `0`
  - `periodKey`: 衰减 `YYYY-MM`、重置赛季开始日期、比赛为空字符串
  - `ratingBefore/ratingAfter/delta/createdAt` 不变
- 查询使用 `LEFT JOIN contests`，对非比赛行不要求 contest；分页总数包含所有 kind。
- 现有“只返回 kind=contest”的行为变更，前端与测试同步更新。

## 7. 权限与审计

- 新增权限码 `rating.settings.manage`，分组“Rating”，默认只授予 `super_admin`（super 绕过）；不自动授予 content_admin。
- 管理 API 用 `RequirePermission`，审计 `rating.settings.manage`，target type `rating_settings`。
- 管理页 `/admin/rating`：仅 `rating.settings.manage` 可访问；表单读写设置，展示当前赛季/下次重置；不需要新前端依赖。
- 普通用户/游客不能修改设置；公开赛季与历史不泄露私密比赛信息。

## 8. 前端

- 管理后台新增“Rating 设置”导航项与 `/admin/rating` 页面：
  - 开关：衰减启用、赛季启用；
  - 数值：阈值、未活跃天数、月衰减比例、单次上限、赛季月数、重置比例；
  - 日期：赛季锚点；
  - 只读展示：当前赛季、下次重置；
  - 校验范围与服务端一致，保存后 Toast，错误中文。
- 用户主页 `RatingHistoryCard`：
  - 历史行显示类型标签（比赛/衰减/赛季重置）；衰减与重置不生成比赛链接；比赛行保持现有链接与私密比赛脱敏。
  - 图表使用 `ratingAfter`，包含所有 kind。
- 排行榜页：
  - 顶部显示当前赛季名称与下次重置时间（来自公开赛季接口）；失败时静默降级，不影响榜单。
- OpenAPI/生成类型同步：`RatingSettingsView`、`RatingSettingsInput`、`RatingSeasonView`、`RatingHistoryItem.kind/periodKey`。

## 9. 配置与迁移

- 新增迁移：`rating_settings`、`rating_runs`、`rating_history.period_key` + 部分唯一索引；默认设置行幂等插入。
- 不新增环境变量；策略全部存 DB 设置，避免部署配置漂移。
- 现有 `rating_history` 行 `period_key=''`，不影响现有唯一索引/历史读取。
- 权限目录新增第 35 个权限码；seed 只补齐目录，不自动授予已有角色；super_admin 天然可用。
- 前端类型生成；文档更新 DEVELOPMENT/README/任务清单/开发日志/运行手册/PrimeVue migration。

## 10. 测试与验收

### 10.1 后端

- 设置：默认值、GET/PUT 校验、权限 403/401、审计、锚点/当前赛季计算。
- 衰减：阈值边界（2400 不衰减、2401 降 1、2450 降 2、2600 降 6）、未活跃天数边界、单次上限、floor 到阈值、每月幂等、重复扫描不重复扣分、失败重试、跨月不回补。
- 赛季：结束边界、全用户软重置公式、1500 跳过、计算值不变的用户跳过且不写零 delta、`seasonResetPercent=0` 仍标记赛季已处理、首次创建默认设置不立即重置、修改锚点/月数不立即触发重置、跨多赛季只处理上一赛季、重复扫描幂等、失败重试。
- 历史：kind/periodKey/label/rank=0/contestId=0；私密比赛仍脱敏；分页 total 含所有 kind；空历史。
- 榜单缓存版本递增；比赛结算与调整共享 advisory lock 不互相破坏。
- 权限目录 35 个、seed 幂等、管理 API 审计。

### 10.2 前端

- 管理页：加载/保存/校验/权限；`rating.settings.manage` 才能看到入口。
- 历史卡片：三类标签、衰减/重置无比赛链接、比赛行不变、图表含新记录。
- 排行榜赛季信息：成功显示、失败降级、不阻塞榜单。
- 生成类型/lint/build。

### 10.3 真实冒烟

- 管理员修改阈值/比例，普通用户无权限；设置即时生效。
- 临时用户 Rating 2500、最后 Rated 比赛 61 天前：触发衰减并写历史、榜单更新；重复扫描不重复扣分。
- 将赛季锚点/周期调整为已结束赛季：触发全用户软重置；1500 用户不写历史；历史显示“赛季重置”。
- 私密比赛历史仍脱敏；缓存版本/榜单刷新。
- 清理临时设置/用户/历史/运行标记，恢复默认设置。

## 11. 交付拆分

- M4-4A：设置模型/权限/管理 API、赛季计算、运行标记、迁移与测试。
- M4-4B：衰减任务、历史写入、缓存失效与测试。
- M4-4C：赛季重置任务、历史写入、公开赛季 API、历史 API 多 kind 与测试。
- M4-4D：前端管理页、Rating 历史卡片、排行榜赛季信息、OpenAPI/生成类型。
- M4-4E：文档、全量检查、真实冒烟。

## 12. 留给实施计划的决定

- 具体包拆分（ratingadminapi 或在 adminapi 挂载）。
- 赛季键/名称格式的最终呈现。
- 衰减/赛季批量处理的分块大小与事务范围。
- 历史 API 是否保留旧 `contestId=0` 兼容（本设计保留）。
- 前端排行榜赛季信息的布局细节。
