# 个人主页贡献图、词云评价与荣誉卡片设计

> 目标：把用户个人主页升级为「成长档案」：GitHub 风格贡献图、右侧词云评价、已认证证书荣誉卡片，并在 Rating 左侧展示提交总数与通过题数。

## 1. 背景与现状

- `UserProfileView.vue` 目前只有头像、身份、简介、粉丝/关注、Rating/金币/注册时间和 Rating 历史。
- 提交数据已有 `submissions` 表；接受题数可按 `status='accepted'` 去重 `problem_id` 统计。
- 证书已有 `certificates` 表与审核流程，但公开核验接口只按验证码查询；没有“某用户已认证证书列表”的公开接口。
- 社交模块已有 `user_follows`、通知、题解点赞；没有用户评价模型。
- 个人主页目前最大宽度 720px，信息密度低，缺少成长可视化和互动区。

## 2. 目标与非目标

### 目标

- 个人主页顶部增加「提交总数 / 通过题数」卡片，放在 Rating 卡片左侧。
- 增加 GitHub 风格贡献图：最近 365 天，按每日代码提交次数从灰色到浅绿再到深绿，带月份/星期标签、图例、当前连续/最长连续天数。
- 右侧增加「词云评价」：
  - 登录用户可以给其他用户写一句短评价；同一人对同一主页只能一条，可更新。
  - 评价文本按重复次数放大，形成词云；下方罗列原始评价，显示评价者昵称/头像、内容和时间。
  - 评价者本人和被评价主页主人可以删除评价；游客只读。
- 词云评价下方增加「荣誉证书」卡片：列出该用户已通过审核的证书类型、等级、通过日期和核验链接。
- 个人主页改为双栏布局：主栏成长数据，右栏互动与荣誉；移动端单栏。
- 保留现有头像、身份徽章/称号、简介、关注、粉丝、Rating、金币、注册时间和 Rating 历史。

### 非目标

- 不做评价预审核、举报、敏感词过滤或管理员审核队列。
- 不做匿名评价；评价者昵称/头像公开。
- 不做证书图片公开；证书卡片只展示类型/等级/日期/验证码。
- 不改变证书审核流程、提交流程或图片权限。
- 不把提交总数/通过题数加到站点首页（用户已确认只放个人主页）。
- 不做贡献图点击按日筛选提交记录；不新增按日期查询提交的接口。
- 不统计代码行数、比赛成绩或题目难度分布。
- 不做实时推送；评价提交后本人页面刷新可见。

### 反目标

- 贡献图把客观题答案提交算作代码提交。
- 词云评价按字符随机拆词，导致语义破碎。
- 任何人可以删除别人页面上的评价，或未登录用户可以写评价。
- 用户给自己写评价。
- 证书卡片公开证书图片或审核隐私字段。
- 个人主页因 5 个接口串行请求而明显变慢。
- 365 个格子造成移动端横向溢出或页面卡顿。

## 3. 约束

- 后端 Go + Gin + GORM + PostgreSQL；前端 Vue 3 + PrimeVue 4；OpenAPI 生成类型。
- 个人主页公开；评价列表、贡献图、证书列表公开；写评价/删除评价需要登录。
- 评价内容 1–40 个字符（按 rune 计），trim 后保存；拒绝换行/控制字符；渲染为纯文本，不解析 Markdown/HTML。
- 同一 `(author_user_id, target_user_id)` 唯一，upsert 更新内容与 `updated_at`。
- 贡献图只统计 `language <> 'answer'` 的提交；所有状态都计为一次提交。
- 通过题数为 `status='accepted'` 的去重 `problem_id` 数。
- 「提交总数」统计全部提交（含客观题答案）；「通过题数」统计全部 AC 的去重题目。
- 贡献图日期沿用现有签到/活跃度的 `time.Local`（服务器本地时区），最近 365 天（含今天），固定补齐 0 提交日期；生产部署应设置 `TZ=Asia/Shanghai`。
- 证书列表只返回 `status='approved'`；不返回图片、驳回原因、审核人、用户邮箱。
- 不新增权限；评价写入只要求登录，删除校验作者/主页主人。
- 个人主页请求并行加载，单个模块失败不阻塞其他模块。
- 现有 Rating 历史、关注、身份展示和 404/加载态保持。

## 4. 方案选择

### 4.1 个人主页布局

- **方案 A：双栏（选用）**。主栏贡献图 + Rating 历史，右栏词云评价 + 荣誉证书；信息层级清楚，符合“右侧词云、下方证书”的要求。
- 方案 B：单栏纵向堆叠。移动端自然，但桌面浪费宽度，词云和证书不够突出。
- 方案 C：Tab 切换（成长/评价/荣誉）。交互更重，且用户要求同时看到词云与证书。

选择 A；桌面 `minmax(0,1fr) 340px`，`< 900px` 单栏。

### 4.2 成长数据接口

- **方案 A：一个 `GET /users/:id/activity`（选用）**。一次返回提交总数、通过题数、365 天日历、连续天数；个人主页只需一个额外请求。
- 方案 B：`/stats` 与 `/contributions` 两个接口。职责更细，但个人主页多一次 RTT，且数据需要保证同一天口径。
- 方案 C：把统计塞进 `Profile` DTO。Profile 被多处复用，导致每次读资料都做聚合查询，不合适。

选择 A。

### 4.3 评价存储

- **方案 A：新表 `user_evaluations`，放在 `social` 包（选用）**。评价是社交内容；可复用 `UserSummary`、登录上下文和 `/users/:id/...` 路由风格。
- 方案 B：放进 `user` 包。会让用户模块承担内容/社交职责。
- 方案 C：JSON 字段存在 users 表。无法分页/聚合/去重，排除。

选择 A。

### 4.4 词云形态

- **方案 A：自由文本按重复度放大，flex-wrap 排列（选用）**。相同文本合并计数，字号按出现次数放大；下方保留原始评价列表，语义和可读性最好。
- 方案 B：随机位置词云。视觉更像词云，但容易重叠、移动端不可控。
- 方案 C：预设标签。整齐但个性化弱，用户已选自由文本。

选择 A。

### 4.5 证书展示

- **方案 A：公开摘要接口，只展示类型/等级/通过日期/验证码（选用）**。不暴露证书图片和审核隐私；通过验证码可跳转公开核验页。
- 方案 B：复用 `/certificates/my`。需要登录且只返回本人，不适用。
- 方案 C：公开证书图片。可能包含真实姓名/证件信息，隐私风险高。

选择 A。

## 5. 设计

### 5.1 数据模型

`user_evaluations`：

```go
type UserEvaluation struct {
    ID           int64     `gorm:"primaryKey"`
    TargetUserID int64     `gorm:"not null;uniqueIndex:uniq_user_evaluations_author_target,priority:2;index:idx_user_evaluations_target_updated,priority:1"`
    AuthorUserID int64     `gorm:"not null;uniqueIndex:uniq_user_evaluations_author_target,priority:1"`
    Content      string    `gorm:"size:40;not null"`
    CreatedAt    time.Time `gorm:"type:timestamptz;not null"`
    UpdatedAt    time.Time `gorm:"type:timestamptz;not null;index:idx_user_evaluations_target_updated,priority:2"`
}
```

- 加入 `db.Migrate`。
- 不建 FK（与现有社交表一致）；服务层校验用户存在/active。
- 唯一索引保证一人一条；upsert 使用 `ON CONFLICT (author_user_id, target_user_id) DO UPDATE`。

### 5.2 用户成长接口

`GET /api/users/:id/activity`（公开）：

```json
{
  "submissionCount": 128,
  "acceptedProblemCount": 64,
  "contributionTotal": 96,
  "startDate": "2025-11-13",
  "endDate": "2026-11-12",
  "currentStreak": 4,
  "longestStreak": 18,
  "days": [
    { "date": "2025-11-13", "count": 0 },
    { "date": "2025-11-14", "count": 3 }
  ]
}
```

- `submissionCount`：该用户全部提交数（含客观题答案）。
- `acceptedProblemCount`：`COUNT(DISTINCT problem_id) WHERE status='accepted'`。
- `contributionTotal`：365 天窗口内代码提交总数（等于 `days` 求和）。
- `days` 固定 365 条，从本地日期 `today-364` 到 `today`，缺失日期补 0。
- `currentStreak`：从今天向前连续有代码提交的天数；今天为 0 则为 0。
- `longestStreak`：365 天窗口内最长连续天数。
- 日期字符串 `YYYY-MM-DD`，服务器本地时区。
- 路由放在 `userapi`；`userapi.Options` 增加 `ActivityLookup`，由 `submission.Service` 实现。
- handler 先确认用户存在，再返回统计；用户不存在 404。

`submission.Service` 新增：

```go
type ActivityDay struct { Date string `json:"date"`; Count int64 `json:"count"` }
type UserActivity struct {
    SubmissionCount      int64         `json:"submissionCount"`
    AcceptedProblemCount int64         `json:"acceptedProblemCount"`
    ContributionTotal    int64         `json:"contributionTotal"`
    StartDate            string        `json:"startDate"`
    EndDate              string        `json:"endDate"`
    CurrentStreak        int           `json:"currentStreak"`
    LongestStreak        int           `json:"longestStreak"`
    Days                 []ActivityDay `json:"days"`
}
func (s *Service) UserActivity(ctx context.Context, userID int64) (*UserActivity, error)
```

- Repository 新增聚合查询：总提交、接受题数、窗口内代码提交时间列表。
- 时间列表在 Go 中按 `time.Local` 分组，避免 SQL 时区差异；365 天数据量可控。

### 5.3 证书公开列表

`GET /api/users/:id/certificates`（公开）：

```json
{
  "total": 2,
  "items": [
    {
      "id": 12,
      "type": "CSP-J",
      "level": "一等奖",
      "verifyCode": "KAPOJ-ABCD-EFGH",
      "approvedAt": "2026-10-01T12:00:00Z"
    }
  ]
}
```

- `certificate.Service.ListApprovedByUser(ctx, userID, limit)`，按 `reviewed_at DESC, id DESC`，最多 50 条。
- 只返回 approved；`approvedAt = reviewed_at`；不返回图片、驳回原因、审核人。
- 路由挂在 `certificateapi` 的 public 组。
- 前端证书卡片用 `RouterLink` 到 `/verify?code=<verifyCode>`。

### 5.4 评价接口

`GET /api/users/:id/evaluations?page=1&page_size=10`（公开，可选登录）：

```json
{
  "total": 12,
  "items": [
    {
      "id": 5,
      "content": "代码清晰，讲解耐心",
      "author": { "id": 2, "username": "bob", "nickname": "Bob", "avatarUrl": "/api/users/2/avatar", "rating": 1680 },
      "createdAt": "2026-11-10T10:00:00Z",
      "updatedAt": "2026-11-11T09:30:00Z",
      "canDelete": true
    }
  ],
  "cloud": [
    { "text": "大佬", "count": 4 },
    { "text": "代码清晰", "count": 2 }
  ],
  "myEvaluation": null,
  "canEvaluate": true
}
```

- `items` 按 `updated_at DESC, id DESC` 分页；`page_size` 默认 10、最大 50。
- `cloud` 对该用户全部评价按 `content` 分组计数，按 count desc、最早 id asc 排序，取前 24。
- `canDelete`：viewer 是作者或目标主页主人。
- `myEvaluation`：当前登录用户对该主页的评价；游客为 null。
- `canEvaluate`：已登录、非本人、目标用户 active。
- 游客可以看列表和词云；未登录 `canEvaluate=false`。

写入：

```http
PUT /api/users/:id/evaluations/mine
{ "content": "大佬" }
```

- 登录、非本人、目标 active；内容 trim 后 1–40 rune，拒绝控制字符/换行；upsert。
- 返回保存后的 `EvaluationItem`。
- `DELETE /api/users/:id/evaluations/mine`：作者删除自己的评价，幂等。
- `DELETE /api/users/:id/evaluations/:evaluationId`：只有目标主页主人可以删除该页任意评价；非主人 403/404。
- 评价不写通知、不写审计。
- 目标不存在 404；目标非 active 时列表可读、写入 40001（“该用户当前不可评价”）。

### 5.5 前端个人主页布局

```text
┌──────────────────────────────────────────────────────────────┐
│ 头像  昵称/身份/简介                        关注 / 粉丝      │
│ ┌────────────┐ ┌────────┐ ┌────────┐ ┌────────────┐          │
│ │ 提交/通过  │ │ Rating │ │ 金币   │ │ 注册时间   │          │
│ └────────────┘ └────────┘ └────────┘ └────────────┘          │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────┬───────────────────────────┐
│ 贡献图（365 天）                  │ 词云评价                  │
│ 当前连续 / 最长连续 / 年度提交     │ 词云 + 我的评价 + 列表     │
│                                  │                           │
│ Rating 历史                       │ 荣誉证书                  │
└──────────────────────────────────┴───────────────────────────┘
```

- 页面最大宽度从 720px 提升到 1200px。
- 顶部资料卡：左侧头像/身份/简介，右侧关注按钮；下面四张统计卡：
  1. 「提交 / 通过」卡（左）
  2. Rating
  3. 金币
  4. 注册时间
- `< 900px`：双栏变单栏；`< 600px`：统计卡 2 列/1 列，贡献图横向滚动。
- 主栏：`ContributionGrid` + `RatingHistoryCard`。
- 右栏：`ProfileEvaluationCard` + `ProfileCertificatesCard`。
- 三个模块各自有 loading/error/empty 状态；个人资料先渲染，右栏数据独立加载，不阻塞。

### 5.6 贡献图

`ContributionGrid.vue`：

- Props：`activity: UserActivity`。
- 53 列 × 7 行，列按周排列；第一周前面补空白，今天之后补空白。
- 单元格 11px，`gap: 3px`；外层 `overflow-x: auto`，移动端可横向滚动。
- 月份标签根据每周第一天所在月显示；左侧显示“一/三/五”。
- 颜色等级：
  - `0`: 灰色 `var(--kapoj-bg-elevated)` + 边框
  - `1`: 1–2 次，浅绿
  - `2`: 3–5 次，中浅绿
  - `3`: 6–9 次，绿色
  - `4`: ≥10 次，深绿
- 每个格子 `title` / `aria-label`：`2026-11-12：3 次代码提交`。
- 图例：`少 [0][1][2][3][4] 多`。
- 摘要：`过去一年 96 次代码提交 · 当前连续 4 天 · 最长连续 18 天`。
- `prefers-reduced-motion` 下不做任何动画。
- 前端按 `startDate` 与日期字符串生成网格；不依赖浏览器时区解析偏移（使用 `T00:00:00` 本地构造）。

### 5.7 词云评价卡片

`ProfileEvaluationCard.vue`：

- 自动加载 `GET /users/:id/evaluations`；提供 loading/error/empty。
- 词云区：
  - `display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 6px 14px; min-height: 120px`。
  - 标签字号：`0.95rem + min(count-1, 4) * 0.18rem`，最大约 1.67rem。
  - 颜色从主题强调色池按稳定 hash 选择：primary/success/warning/special/re。
  - 每个标签是纯文本，带 `title="出现 N 次"`；不做随机旋转，保证可读。
- 我的评价：
  - 未登录：登录 CTA。
  - 本人：提示“这是你的主页，不能给自己评价”。
  - 其他登录用户：`Textarea` 40 字上限、字数计数器；已有评价时按钮“更新评价”，无评价时“发布评价”。
  - 保存后刷新第一页、词云和 myEvaluation。
- 评价列表：
  - 每条：作者头像（失败显示首字母）、昵称链接、内容、`formatDateTime(updatedAt)`。
  - `canDelete` 时显示删除按钮；主页主人删除他人评价前弹确认。
  - “加载更多”按钮，追加下一页；没有更多时隐藏。
- 空状态：词云区显示“还没有评价”，列表区不重复空态。
- 目标用户非 active 时：显示只读提示，不显示输入框。

### 5.8 荣誉证书卡片

`ProfileCertificatesCard.vue`：

- 自动加载 `GET /users/:id/certificates`。
- 卡片列表：`pi pi-verified` 图标、类型名、等级、`已认证` Tag、通过日期、验证码（mono）、`核验`链接。
- 空态：`暂无已认证证书`；本人访问时显示“去提交证书”链接到 `/settings/certificates`。
- 只显示 approved；不加载证书图片。
- 桌面单列卡片，移动端纵向排列。

### 5.9 API 客户端与类型

- `users.ts`：`getUserActivity(id)`。
- `social.ts`：`listUserEvaluations(userId, page, pageSize)`, `saveMyEvaluation(userId, content)`, `deleteMyEvaluation(userId)`, `deleteUserEvaluation(userId, evaluationId)`。
- `certificates.ts`：`listUserCertificates(userId)`。
- `types.ts` 增加对应 Schema 别名。
- OpenAPI 增加路径与 schema：
  - `GET /users/{id}/activity`
  - `GET /users/{id}/certificates`
  - `GET /users/{id}/evaluations`
  - `PUT /users/{id}/evaluations/mine`
  - `DELETE /users/{id}/evaluations/mine`
  - `DELETE /users/{id}/evaluations/{evaluationId}`
  - `UserActivity`, `ActivityDay`, `ProfileCertificate`, `ProfileCertificateList`, `EvaluationAuthor`, `EvaluationItem`, `EvaluationCloudItem`, `EvaluationPage`, `EvaluationInput`.

### 5.10 前端文件

- `apps/web/src/views/UserProfileView.vue`（重构）
- `apps/web/src/views/UserProfileView.spec.ts`（扩展）
- `apps/web/src/components/profile/ContributionGrid.vue`
- `apps/web/src/components/profile/ContributionGrid.spec.ts`
- `apps/web/src/components/profile/ProfileEvaluationCard.vue`
- `apps/web/src/components/profile/ProfileEvaluationCard.spec.ts`
- `apps/web/src/components/profile/ProfileCertificatesCard.vue`
- `apps/web/src/components/profile/ProfileCertificatesCard.spec.ts`
- `apps/web/src/api/users.ts`
- `apps/web/src/api/social.ts`
- `apps/web/src/api/certificates.ts`
- `apps/web/src/api/types.ts`
- `apps/web/src/api/schema.d.ts`（生成）

后端文件：

- `apps/server/internal/social/{model,service,evaluation}.go` 及测试
- `apps/server/internal/socialapi/handler.go` 及测试
- `apps/server/internal/submission/{repository,service,activity}.go` 及测试
- `apps/server/internal/userapi/{handler.go,handler_test.go}`
- `apps/server/internal/certificate/{repository,service,dto}.go` 及测试
- `apps/server/internal/certificateapi/handler.go` 及测试
- `apps/server/internal/db/db.go`
- `apps/server/internal/router/router.go`
- `packages/api/openapi.yaml`

## 6. 测试与验收

### 6.1 后端

- `UserActivity`：
  - 总提交含客观题；贡献图排除 `answer`；接受题数按 `problem_id` 去重。
  - 365 天补齐、日期边界、0 提交日期。
  - current/longest streak 正确；今天无提交 current=0。
  - 用户不存在 404。
- 评价：
  - 一人一条 upsert；更新保留 `created_at`、更新 `updated_at`。
  - 自己评价自己 40001；非 active 目标 40001；游客/未登录写入 401。
  - 内容 trim、1–40 字、控制字符/换行拒绝。
  - 列表分页、cloud 计数/排序/前 24、`canDelete`、`myEvaluation`、`canEvaluate`。
  - 作者删除 mine 幂等；主页主人删除任意评价；非主人删除 403/404。
  - 目标不存在 404；游客列表可读。
- 证书：
  - 只返回 approved；不返回 pending/rejected。
  - 不返回 image/rejectReason/reviewedBy；按 reviewed_at desc。
  - 用户不存在/无证书返回空列表还是 404？用户不存在返回 404（由前端只对已加载 profile 调用）。
- 迁移：`user_evaluations` 表与唯一索引存在。
- OpenAPI 生成通过。

### 6.2 前端

- `UserProfileView`：
  - 资料、社交、活动统计并行加载；统计卡在 Rating 左侧。
  - 贡献图、Rating 历史、评价卡、证书卡按顺序渲染。
  - 单个模块失败不阻塞其他模块；原 404/加载态/关注逻辑保持。
- `ContributionGrid`：
  - 0/1–2/3–5/6–9/≥10 五档颜色类。
  - 365 个日期格子、补零、月份/星期标签、图例、aria/title。
  - 连续天数摘要。
- `ProfileEvaluationCard`：
  - 游客只读 + 登录 CTA；本人无表单；他人可发布/更新；40 字限制。
  - 词云字号按 count 增加；cloud 顺序稳定。
  - 列表分页/加载更多；作者删除；主页主人删除确认；空态/错误/加载。
  - 评价内容按纯文本渲染，不注入 HTML。
- `ProfileCertificatesCard`：
  - approved 卡片、验证码核验链接、空态、本人 CTA。
- 全量 web test / lint / build。

### 6.3 真实冒烟

- 种子数据：
  - 用户 A 有 365 天内多天代码提交、客观题提交、AC 题目、1 张 approved 证书、2 条来自他人的评价。
  - 用户 B/C 登录后可评价 A；A 可删除 B 的评价。
- 浏览器：
  - A 的主页：统计卡、贡献图颜色/图例/连续天数、Rating 历史、词云、评价列表、证书卡。
  - 游客：可看全部，不可评价。
  - B 评价 A：发布后词云/列表更新；再次编辑不新增第二条。
  - A 删除评价：确认后列表/词云更新。
  - 证书核验链接可打开 `/verify?code=...` 并通过。
  - 375px/1440px 无横向溢出；深/浅主题可读。
- 清理种子用户与评价/证书数据。

## 7. 已知取舍

- 评价无预审核/敏感词过滤；依赖主页主人删除，后续可加举报/审核。
- 评价者身份公开，不支持匿名；降低滥用但也限制表达。
- 证书卡片不展示图片，避免隐私泄露；用户需通过验证码核验。
- 贡献图沿用服务器本地时区（`time.Local`，生产建议 `TZ=Asia/Shanghai`），跨时区用户看到的“今天”可能不同。
- 365 天窗口固定；不做年份切换/全历史。
- 评价列表一次加载 10 条，词云基于全部评价聚合；评价量很大时需要缓存/物化。
- 个人主页增加 2 个独立请求（评价、证书）+ 1 个活动请求；并行加载，单模块失败可重试。
- 不做评价实时推送/通知；不新增通知类型。
