# 个人主页贡献图、词云评价与荣誉卡片实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将用户个人主页升级为成长档案：贡献图、提交/通过统计卡、词云评价、公开荣誉证书，并保持现有资料/关注/Rating 历史功能。

**Architecture:** 后端新增 `/users/:id/activity`（提交统计 + 365 天日历）、`/users/:id/certificates`（approved 证书摘要）和 `/users/:id/evaluations` 系列（一人一条 upsert/删除）。前端把 `UserProfileView` 改为双栏，新增 `ContributionGrid`、`ProfileEvaluationCard`、`ProfileCertificatesCard`，评价与证书模块独立加载。

**Tech Stack:** Go + Gin + GORM + PostgreSQL；Vue 3 + PrimeVue 4；OpenAPI 生成类型；Vitest。

**Spec:** `docs/superpowers/specs/2026-11-12-profile-redesign-design.md`

## Global Constraints

- 「提交总数」= 全部 submissions（含客观题答案）；「通过题数」= `status='accepted'` 的去重 `problem_id`。
- 贡献图只统计 `language <> 'answer'`，最近 365 天（含今天），日期按 `time.Local`，缺失日期补 0；生产建议 `TZ=Asia/Shanghai`。
- 颜色档位：0 灰；1–2 浅绿；3–5 中浅绿；6–9 绿；≥10 深绿。
- 评价内容 1–40 个 rune，trim，拒绝换行/控制字符；纯文本渲染。
- 同一 `(author_user_id, target_user_id)` 唯一；可更新；不能评价自己；目标必须 active。
- 评价列表/词云/证书/贡献图公开；写评价/删除需要登录。
- 作者可删除自己的评价；主页主人可删除自己页面上的任意评价；游客不可写/删。
- 证书只返回 `approved`，不返回图片/驳回原因/审核人/邮箱；前端用验证码链接 `/verify?code=...`。
- 统计卡只放用户个人主页，不放站点首页。
- 个人主页双栏：主栏贡献图 + Rating 历史；右栏词云评价 + 荣誉证书；`<900px` 单栏。
- 每个模块独立 loading/error/retry；单个接口失败不阻塞其他模块。
- 不新增权限；不改变现有关注、Rating 历史、证书审核、提交链路。
- 后端测试使用 `go test -p 1`；前端跑 test/lint/build。
- 不提交 `Export.zip` 或 `.superpowers` 临时文件。

## Review Focus

- 活动统计口径：全部提交 vs 代码提交 vs 去重 AC 不能混；365 天补零、时区、streak 边界必须正确（Task 1）。
- 评价 upsert 唯一性、自己评价自己、非 active 目标、内容校验、作者/主页主人删除权限（Task 2）。
- 证书只返回 approved 且不泄露图片/审核隐私（Task 3）。
- OpenAPI/生成类型/客户端路径与分页/请求体一致（Task 4）。
- 个人主页统计卡在 Rating 左侧、贡献图颜色/日期/legend、双栏布局与移动端（Task 5）。
- 词云字号/排序、评价列表分页/加载更多、删除确认、游客/本人/其他用户状态（Task 6）。
- 证书卡片核验链接与空态、本人 CTA（Task 7）。

---

### Task 1: 后端活动统计与贡献日历

**Files:**
- Create: `apps/server/internal/submission/activity.go`
- Create: `apps/server/internal/submission/activity_test.go`
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/userapi/handler.go`
- Modify: `apps/server/internal/userapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`

**Interfaces:**
- Produces:
  - `submission.ActivityDay`, `submission.UserActivity`
  - `func (s *Service) UserActivity(ctx context.Context, userID int64) (*UserActivity, error)`
  - `userapi.Options.Activity ActivityLookup`
  - `GET /api/users/:id/activity`

- [ ] **Step 1: Write failing tests**

`apps/server/internal/submission/activity_test.go`：使用 `testutil.PG(t)` 建用户与 submissions，固定 `time.Local` 日期。

- 构造今天、昨天、前天代码提交，及一条 `answer` 提交、一条 accepted 题目、重复 AC 同一题。
- 断言：
  - `SubmissionCount` 含 answer；`AcceptedProblemCount` 按 problem 去重。
  - `ContributionTotal` 不含 answer。
  - `Days` 长度 365，首尾日期正确，0 提交日期存在。
  - `CurrentStreak` 和 `LongestStreak`。
  - 今天无提交时 `CurrentStreak=0`。
- 用户无提交时返回全 0/365 个 0 天。

`userapi/handler_test.go`：用 fake `ActivityLookup`，断言 `/api/users/:id/activity` 200、字段形状；用户不存在 404；未登录也可访问。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/submission/ ./internal/userapi/
```

Expected: FAIL，`UserActivity`/路由未定义。

- [ ] **Step 3: Implement**

`submission/activity.go`：

```go
type ActivityDay struct {
    Date  string `json:"date"`
    Count int64  `json:"count"`
}

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

- `now := time.Now().In(time.Local)`；`end := local midnight today`；`start := end.AddDate(0,0,-364)`。
- `CountUserActivity` 一条 SQL：
  ```sql
  SELECT COUNT(*) AS total,
         COUNT(DISTINCT problem_id) FILTER (WHERE status = 'accepted') AS accepted
  FROM submissions WHERE user_id = ?
  ```
- `ListCodeSubmissionTimes(ctx,userID,start)`：
  ```sql
  SELECT created_at FROM submissions
  WHERE user_id = ? AND language <> 'answer' AND created_at >= ?
  ORDER BY created_at
  ```
- 在 Go 中按 `created_at.In(time.Local).Format("2006-01-02")` 计数。
- 生成 365 个日期；`ContributionTotal` 求和；`CurrentStreak` 从今天倒推；`LongestStreak` 遍历。
- Repository 接口增加 `CountUserActivity`、`ListCodeSubmissionTimes`。

`userapi`：

```go
type ActivityLookup interface {
    UserActivity(ctx context.Context, userID int64) (*submission.UserActivity, error)
}
type Options struct { IdentityLookup IdentityLookup; RBAC *rbac.Service; Activity ActivityLookup }
```

- `GET /users/:id/activity`：parse id；`h.svc.Profile` 确认用户存在；调用 `h.activity.UserActivity`；`httpx.OK`。
- `router.go`：把 `userapi.RegisterRoutes` 移到 `submissionSvc` 创建之后，并设置 `userOpts.Activity = submissionSvc`；avatar 路由保持原位置。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/submission/ ./internal/userapi/ ./internal/router/
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/submission/activity.go apps/server/internal/submission/activity_test.go apps/server/internal/submission/repository.go apps/server/internal/userapi/handler.go apps/server/internal/userapi/handler_test.go apps/server/internal/router/router.go
git commit -m "feat(profile): add user activity calendar"
```

---

### Task 2: 后端词云评价模型、接口与权限

**Files:**
- Modify: `apps/server/internal/social/model.go`
- Create: `apps/server/internal/social/evaluation.go`
- Create: `apps/server/internal/social/evaluation_test.go`
- Modify: `apps/server/internal/socialapi/handler.go`
- Modify: `apps/server/internal/socialapi/handler_test.go`
- Modify: `apps/server/internal/db/db.go`

**Interfaces:**
- Produces:
  - `social.UserEvaluation`
  - `social.EvaluationItem`, `EvaluationCloudItem`, `EvaluationPage`, `EvaluationInput`
  - `ListEvaluations(ctx, viewerID, targetID, page, size)`
  - `SaveEvaluation(ctx, authorID, targetID, content)`
  - `DeleteMyEvaluation(ctx, authorID, targetID)`
  - `DeleteEvaluationAsTarget(ctx, viewerID, targetID, evaluationID)`
  - routes:
    - `GET /api/users/:id/evaluations`
    - `PUT /api/users/:id/evaluations/mine`
    - `DELETE /api/users/:id/evaluations/mine`
    - `DELETE /api/users/:id/evaluations/:evaluationId`

- [ ] **Step 1: Write failing tests**

`social/evaluation_test.go`：

- 建用户 A/B/C。
- B 评价 A：1 条；B 再次保存改为“更新后”：仍 1 条，`createdAt` 保留、`updatedAt` 更新。
- 自己评价自己 → 40001；非 active 目标 → 40001。
- 空/41 字/带换行/控制字符 → 40001；首尾空格 trim。
- 列表：游客可读；`total`、分页、cloud count/排序、`myEvaluation`、`canEvaluate`、`canDelete`（作者、主页主人）。
- B 删除 mine → 幂等；A 删除 B 的评价 → 成功；C 删除 A 页面的评价 → 403；不存在 404。
- 目标不存在 404。

`socialapi/handler_test.go`：路由 401/403/200、请求体、envelope 形状、canEvaluate。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/social/ ./internal/socialapi/ ./internal/db/
```

Expected: FAIL，模型/路由未定义。

- [ ] **Step 3: Implement**

`social/model.go`：

```go
type UserEvaluation struct {
    ID           int64     `gorm:"primaryKey"`
    TargetUserID int64     `gorm:"not null;uniqueIndex:uniq_user_evaluations_author_target,priority:2;index:idx_user_evaluations_target_updated,priority:1"`
    AuthorUserID int64     `gorm:"not null;uniqueIndex:uniq_user_evaluations_author_target,priority:1"`
    Content      string    `gorm:"size:40;not null"`
    CreatedAt    time.Time `gorm:"type:timestamptz;not null"`
    UpdatedAt    time.Time `gorm:"type:timestamptz;not null;index:idx_user_evaluations_target_updated,priority:2"`
}
func (UserEvaluation) TableName() string { return "user_evaluations" }
```

`db.Migrate` 加 `&social.UserEvaluation{}`。

`social/evaluation.go`：

```go
type EvaluationInput struct { Content string `json:"content"` }
type EvaluationItem struct {
    ID        int64       `json:"id"`
    Content   string      `json:"content"`
    Author    UserSummary `json:"author"`
    CreatedAt time.Time   `json:"createdAt"`
    UpdatedAt time.Time   `json:"updatedAt"`
    CanDelete bool        `json:"canDelete"`
}
type EvaluationCloudItem struct { Text string `json:"text"`; Count int64 `json:"count"` }
type EvaluationPage struct {
    Total       int64                  `json:"total"`
    Items       []EvaluationItem       `json:"items"`
    Cloud       []EvaluationCloudItem  `json:"cloud"`
    MyEvaluation *EvaluationItem       `json:"myEvaluation"`
    CanEvaluate bool                   `json:"canEvaluate"`
}
```

- `normalizeEvaluationContent`：trim；rune 1–40；拒绝 `\n\r\t` 和 `unicode.IsControl`。
- `SaveEvaluation`：
  - ID 校验；自己 → 40001；`requireActiveTarget`。
  - `INSERT ... ON CONFLICT (author_user_id,target_user_id) DO UPDATE SET content=excluded.content, updated_at=now()`.
  - 回查并返回 item。
- `ListEvaluations`：
  - `requireUserExists`；目标 active 查询用于 canEvaluate。
  - count + page join users；cloud SQL `GROUP BY content ORDER BY COUNT(*) DESC, MIN(id) ASC LIMIT 24`。
  - `myEvaluation` 当 viewer>0 且 viewer!=target。
  - `CanDelete = viewer==target || viewer==author`。
- `DeleteMyEvaluation`：`WHERE author_user_id=? AND target_user_id=?`，幂等。
- `DeleteEvaluationAsTarget`：viewer 必须等于 targetID，否则 403；`WHERE id=? AND target_user_id=?`；0 行 → 404。
- `requireActiveTarget`/`requireUserExists` 复用 follow.go 的同包私有函数。

`socialapi`：

```go
r.GET("/users/:id/evaluations", h.evaluations)
r.PUT("/users/:id/evaluations/mine", middleware.RequireAuth(), h.saveEvaluation)
r.DELETE("/users/:id/evaluations/mine", middleware.RequireAuth(), h.deleteMyEvaluation)
r.DELETE("/users/:id/evaluations/:evaluationId", middleware.RequireAuth(), h.deleteEvaluation)
```

- 静态 `/mine` 路由必须在 `/:evaluationId` 之前注册。
- `pathID` 处理非法 ID。
- 删除路由把 `currentUserID(c)` 作为 viewer 传给 `DeleteEvaluationAsTarget`。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/social/ ./internal/socialapi/ ./internal/db/ ./internal/router/
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/social/model.go apps/server/internal/social/evaluation.go apps/server/internal/social/evaluation_test.go apps/server/internal/socialapi/handler.go apps/server/internal/socialapi/handler_test.go apps/server/internal/db/db.go
git commit -m "feat(profile): add user evaluations and word cloud"
```

---

### Task 3: 后端公开证书列表

**Files:**
- Modify: `apps/server/internal/certificate/dto.go`
- Modify: `apps/server/internal/certificate/repository.go`
- Modify: `apps/server/internal/certificate/service.go`
- Modify: `apps/server/internal/certificateapi/handler.go`
- Test: `apps/server/internal/certificate/service_test.go`
- Test: `apps/server/internal/certificateapi/handler_test.go`

**Interfaces:**
- Produces:
  - `certificate.ProfileCertificateView`
  - `func (s *Service) ListApprovedByUser(ctx context.Context, userID int64, limit int) ([]ProfileCertificateView, int64, error)`
  - `GET /api/users/:id/certificates`

- [ ] **Step 1: Write failing tests**

- 创建 approved/pending/rejected 证书各一条；`ListApprovedByUser` 只返回 approved，按 `reviewed_at DESC, id DESC`。
- 返回字段只有 id/type/level/verifyCode/approvedAt；无 image/rejectReason/reviewedBy。
- 用户无证书返回空切片 + total 0。
- handler：游客 200，形状正确；未知用户返回空列表。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/certificate/ ./internal/certificateapi/
```

Expected: FAIL。

- [ ] **Step 3: Implement**

`dto.go`：

```go
type ProfileCertificateView struct {
    ID         int64      `json:"id"`
    Type       string     `json:"type"`
    Level      string     `json:"level"`
    VerifyCode string     `json:"verifyCode"`
    ApprovedAt *time.Time `json:"approvedAt"`
}
```

`repository.go`：

```go
ListApprovedByUser(ctx context.Context, userID int64, limit int) ([]Certificate, int64, error)
```

- `WHERE user_id = ? AND status = 'approved' ORDER BY reviewed_at DESC, id DESC LIMIT ?`；Count 单独查询。

`service.go`：

- 校验 userID>0；limit 默认/上限 50。
- 组装 `ProfileCertificateView`；`ApprovedAt = ReviewedAt`；verifyCode 为空时跳过或空串（approved 应有值）。

`certificateapi/handler.go`：

- `public.GET("/users/:id/certificates", h.userCertificates)`。
- 解析 id；调用 service；`httpx.OK(c, gin.H{"total": total, "items": items})` 或定义 `ProfileCertificatePage`。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/certificate/ ./internal/certificateapi/ ./internal/router/
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/certificate/dto.go apps/server/internal/certificate/repository.go apps/server/internal/certificate/service.go apps/server/internal/certificate/service_test.go apps/server/internal/certificateapi/handler.go apps/server/internal/certificateapi/handler_test.go
git commit -m "feat(profile): expose approved certificates"
```

---

### Task 4: OpenAPI、生成类型与前端客户端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/users.ts`
- Modify: `apps/web/src/api/social.ts`
- Modify: `apps/web/src/api/certificates.ts`
- Create: `apps/web/src/api/profile.spec.ts`

**Interfaces:**
- Produces:
  - `getUserActivity(id)`
  - `listUserEvaluations(userId, page, pageSize)`
  - `saveMyEvaluation(userId, content)`
  - `deleteMyEvaluation(userId)`
  - `deleteUserEvaluation(userId, evaluationId)`
  - `listUserCertificates(userId)`
  - Generated schemas: `UserActivity`, `ActivityDay`, `ProfileCertificate`, `ProfileCertificatePage`, `EvaluationAuthor`, `EvaluationItem`, `EvaluationCloudItem`, `EvaluationPage`, `EvaluationInput`

- [ ] **Step 1: Write failing client tests**

`apps/web/src/api/profile.spec.ts` mock `./client`：

- `getUserActivity(7)` → GET `/users/7/activity`。
- `listUserEvaluations(7,2,10)` → GET `/users/7/evaluations` params `{page:2,page_size:10}`。
- `saveMyEvaluation(7,'大佬')` → PUT `/users/7/evaluations/mine` body `{content:'大佬'}`。
- `deleteMyEvaluation(7)` → DELETE `/users/7/evaluations/mine`。
- `deleteUserEvaluation(7,5)` → DELETE `/users/7/evaluations/5`。
- `listUserCertificates(7)` → GET `/users/7/certificates`。

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/profile.spec.ts
```

Expected: FAIL，函数未导出。

- [ ] **Step 3: Implement contract/client**

`openapi.yaml` 增加：

- `GET /users/{id}/activity` → `UserActivityResponse`。
- `GET /users/{id}/certificates` → `ProfileCertificatePageResponse`。
- `GET /users/{id}/evaluations` → `EvaluationPageResponse`，query page/page_size。
- `PUT /users/{id}/evaluations/mine` body `EvaluationInput` → `EvaluationItemResponse`。
- `DELETE /users/{id}/evaluations/mine` → `EmptyResponse`。
- `DELETE /users/{id}/evaluations/{evaluationId}` → `EmptyResponse`。
- Schema 按设计文档字段定义；`ProfileCertificatePage` required `[total, items]`；`EvaluationPage` required `[total, items, cloud, myEvaluation, canEvaluate]`，`myEvaluation` nullable。
- `UserActivity.days` 365 条由服务端保证，OpenAPI 只描述数组。

`types.ts` 增加别名；`users.ts`/`social.ts`/`certificates.ts` 增加函数。

重新生成：

```bash
corepack pnpm gen:api
```

- [ ] **Step 4: Run tests/typecheck**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/profile.spec.ts
corepack pnpm --filter web exec vue-tsc -b --force
```

- [ ] **Step 5: Commit**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/users.ts apps/web/src/api/social.ts apps/web/src/api/certificates.ts apps/web/src/api/profile.spec.ts
git commit -m "feat(api): add profile activity evaluation and certificate contracts"
```

---

### Task 5: 个人主页布局、统计卡与贡献图

**Files:**
- Rewrite: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/views/UserProfileView.spec.ts`
- Create: `apps/web/src/components/profile/ContributionGrid.vue`
- Create: `apps/web/src/components/profile/ContributionGrid.spec.ts`

**Interfaces:**
- Consumes: `getUserActivity(id)`、`UserActivity`
- Produces:
  - 双栏 `.profile-body` / `.profile-aside` 布局
  - 统计卡在 Rating 左侧
  - `ContributionGrid` props `activity: UserActivity`

- [ ] **Step 1: Write failing tests**

`UserProfileView.spec.ts`：

- mock `getUserActivity` 返回固定 `UserActivity`。
- 断言页面文本/顺序：提交总数、通过题数卡在 Rating 之前；贡献图标题；右栏占位/组件存在；Rating 历史仍渲染。
- 活动接口失败不阻塞资料/关注/Rating 历史，显示该卡片错误/重试。
- 现有关注/404/路由切换测试保持通过。

`ContributionGrid.spec.ts`：

- 365 天数据（可构造少量 days + 0 填充）：
  - 0 次/1–2/3–5/6–9/≥10 分别有 `is-level-0..4` 类。
  - 月份/星期标签存在。
  - 摘要显示 `过去一年 N 次代码提交 · 当前连续 X 天 · 最长连续 Y 天`。
  - 每个格子有 date/count 的 `title` 和 `aria-label`。
  - 图例存在。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/UserProfileView.spec.ts src/components/profile/ContributionGrid.spec.ts
```

Expected: FAIL。

- [ ] **Step 3: Implement layout and grid**

`UserProfileView.vue`：

- imports `getUserActivity`, `ContributionGrid`.
- `activity = ref<UserActivity | null>(null)`, `activityLoading`, `activityError`.
- `loadProfile` 并行 `getProfile`、`getUserSocial`、`getUserActivity`；活动失败只设置 `activityError`，不阻塞 profile/social。
- 资料卡统计网格：
  1. `.career-card`：提交总数 / 通过题数（两个大数字）
  2. Rating
  3. 金币
  4. 注册时间
- 页面：
  ```vue
  <div class="profile-body">
    <div class="profile-main">
      <AppSurface ...><ContributionGrid v-if="activity" :activity="activity" /> ...</AppSurface>
      <RatingHistoryCard ... />
    </div>
    <aside class="profile-aside" data-test="profile-aside">
      <AppSurface ... data-test="evaluation-slot">词云评价（Task 6）</AppSurface>
      <AppSurface ... data-test="certificate-slot">荣誉证书（Task 7）</AppSurface>
    </aside>
  </div>
  ```
  Task 6/7 会替换占位内容。
- CSS：`max-width: var(--kapoj-content-max)`；`.profile-body { display:grid; grid-template-columns:minmax(0,1fr) 340px; gap: var(--kapoj-space-4); align-items:start; }`；`@media (max-width:900px){ grid-template-columns:minmax(0,1fr); }`。
- 统计卡 `grid-template-columns: 1.25fr repeat(3,1fr)`；移动端 2/1 列。

`ContributionGrid.vue`：

- 计算 `firstWeekday`、`weeks`、`monthLabels`。
- 365 天；前后补空白。
- `level(count)` 函数：0→0，1–2→1，3–5→2，6–9→3，≥10→4。
- 结构：月份行、星期标签列、格子网格、图例、摘要。
- 日期解析用 `new Date(date + 'T00:00:00')` 本地时间。
- 使用 CSS 变量 `--contrib-0..4`，在 `:root` 或组件内定义深浅主题值。
- `title`/`aria-label`。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/UserProfileView.spec.ts src/components/profile/ContributionGrid.spec.ts
corepack pnpm --filter web test
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/UserProfileView.vue apps/web/src/views/UserProfileView.spec.ts apps/web/src/components/profile/ContributionGrid.vue apps/web/src/components/profile/ContributionGrid.spec.ts
git commit -m "feat(profile): add activity stats and contribution grid"
```

---

### Task 6: 词云评价卡片

**Files:**
- Create: `apps/web/src/components/profile/ProfileEvaluationCard.vue`
- Create: `apps/web/src/components/profile/ProfileEvaluationCard.spec.ts`
- Modify: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/views/UserProfileView.spec.ts`

**Interfaces:**
- Consumes: `listUserEvaluations`, `saveMyEvaluation`, `deleteMyEvaluation`, `deleteUserEvaluation`, `useAuthStore`, `useConfirm`, `useToast`
- Produces: 右栏词云评价卡片，替换 Task 5 的 `evaluation-slot`

- [ ] **Step 1: Write failing tests**

`ProfileEvaluationCard.spec.ts`：

- 游客：词云/列表渲染，显示登录 CTA，无输入框。
- 本人：无输入框，显示“不能给自己评价”。
- 其他登录用户：输入框、40 字计数器；发布调用 `saveMyEvaluation`；已有 myEvaluation 时预填并显示“更新评价”。
- 词云：cloud 按 count 生成字号 class/style；重复词放大。
- 列表：作者昵称/内容/时间；`canDelete` 显示删除；加载更多分页。
- 主页主人删除他人评价：`confirm.require`，accept 后调用 `deleteUserEvaluation`；取消不调用。
- 空态/错误/重试。

`UserProfileView.spec.ts`：stub `ProfileEvaluationCard`，断言右栏顺序（评价在上、证书在下）。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/components/profile/ProfileEvaluationCard.spec.ts src/views/UserProfileView.spec.ts
```

Expected: FAIL。

- [ ] **Step 3: Implement**

`ProfileEvaluationCard.vue`：

- props `targetUserId: number`，可选 `targetActive?: boolean`。
- state：`pageData`, `loading`, `error`, `page`, `pageSize=10`, `myContent`, `saving`, `loadingMore`。
- `load(page=1, append=false)` 调 `listUserEvaluations`；append 时合并 items。
- `canEdit` = `pageData?.canEvaluate`；`isSelf` = auth.user?.id === targetUserId。
- 词云：
  - `cloud` 已按 count 排序；字号 `0.95 + Math.min(item.count - 1, 4) * 0.18` rem。
  - 颜色通过稳定 hash 从 `--kapoj-primary/success/warning/special/re` 选 class。
- 表单：
  - `Textarea` maxlength 40、autoResize、chars counter；按钮 disabled 空/保存中。
  - 保存成功 toast，重载第一页。
- 列表：
  - Avatar：`author.avatarUrl`，失败 fallback 首字母；昵称 RouterLink。
  - 内容纯文本（`{{ }}`），时间 `formatDateTime(updatedAt)`。
  - 删除：作者直接删自己的；主页主人删除他人时 `confirm.require`。
  - `加载更多` 调下一页。
- 空态/加载/错误。

`UserProfileView.vue`：用 `<ProfileEvaluationCard :target-user-id="profile.id" />` 替换占位。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/components/profile/ProfileEvaluationCard.spec.ts src/views/UserProfileView.spec.ts
corepack pnpm --filter web test
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/profile/ProfileEvaluationCard.vue apps/web/src/components/profile/ProfileEvaluationCard.spec.ts apps/web/src/views/UserProfileView.vue apps/web/src/views/UserProfileView.spec.ts
git commit -m "feat(profile): add evaluation word cloud"
```

---

### Task 7: 荣誉证书卡片与布局整合

**Files:**
- Create: `apps/web/src/components/profile/ProfileCertificatesCard.vue`
- Create: `apps/web/src/components/profile/ProfileCertificatesCard.spec.ts`
- Modify: `apps/web/src/views/UserProfileView.vue`
- Modify: `apps/web/src/views/UserProfileView.spec.ts`

**Interfaces:**
- Consumes: `listUserCertificates(userId)`, `ProfileCertificate`
- Produces: 右栏证书卡片，替换 Task 5 的 `certificate-slot`

- [ ] **Step 1: Write failing tests**

`ProfileCertificatesCard.spec.ts`：

- 加载成功：每张卡片显示类型/等级/通过日期/验证码/核验链接 `/verify?code=...`。
- 空态：`暂无已认证证书`；本人显示“去提交证书”链接，游客不显示。
- 错误/重试。
- 不渲染图片/驳回原因。

`UserProfileView.spec.ts`：stub 评价/证书组件，断言 aside 顺序。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/components/profile/ProfileCertificatesCard.spec.ts src/views/UserProfileView.spec.ts
```

Expected: FAIL。

- [ ] **Step 3: Implement**

- `ProfileCertificatesCard.vue`：props `targetUserId`；`auth` 判本人；加载列表；卡片样式（深/浅主题、`pi pi-verified`、Tag 已认证、mono 验证码、核验 RouterLink）。
- 空态本人 CTA `/settings/certificates`。
- `UserProfileView.vue` 替换占位，保持评价在上、证书在下。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/components/profile/ProfileCertificatesCard.spec.ts src/views/UserProfileView.spec.ts
corepack pnpm --filter web test
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/profile/ProfileCertificatesCard.vue apps/web/src/components/profile/ProfileCertificatesCard.spec.ts apps/web/src/views/UserProfileView.vue apps/web/src/views/UserProfileView.spec.ts
git commit -m "feat(profile): add certificate honor cards"
```

---

### Task 8: 全量验证、真实冒烟与文档

**Files:**
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`（当前进度一行，可选）

- [ ] **Step 1: Backend checks**

```bash
cd /Users/admin/codespace/kapoj/apps/server
test -z "$(gofmt -l .)"
go vet ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./...
```

- [ ] **Step 2: Frontend checks**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm gen:api
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

- [ ] **Step 3: Real API smoke**

隔离临时库/端口，不动主服务：

- 建用户 A/B/C；A 有跨多天的代码提交、answer 提交、重复 AC；B/C 评价 A；A 有 approved/pending 证书。
- `GET /users/A/activity`：365 天、统计口径、streak 正确。
- `GET /users/A/evaluations`：cloud/total/my/canEvaluate/canDelete。
- `PUT /users/A/evaluations/mine` 作为 B：创建；再次更新仍 1 条；C 不能删除；A 能删除。
- `GET /users/A/certificates`：仅 approved，无隐私字段。
- 清理临时数据/服务。

- [ ] **Step 4: Browser smoke**

- 用户 A 主页：统计卡在 Rating 左；贡献图颜色/图例/月份/连续天数；Rating 历史；词云、评价列表、证书卡。
- 游客：只读；登录后 B 评价 A，词云/列表更新；再次编辑仍一条；A 删除评价；证书核验链接通过。
- 375px/1440px 无横向溢出；深/浅主题可读。
- 若浏览器不可用，记录延后，不声称已验证。

- [ ] **Step 5: Update docs**

- `docs/任务清单.md`：M9 标题 ✅，PROFILE-A..H 全部 ✅，写交付摘要、验证结果、已知取舍。
- `docs/开发日志.md`：追加 `2026-11-12：M9 个人主页成长档案`。
- `README.md`：当前进度补一行（可选）。

- [ ] **Step 6: Commit**

```bash
git add docs/任务清单.md docs/开发日志.md README.md
git commit -m "docs: record profile redesign delivery"
```

---

## Self-Review

- **Spec coverage:** 活动统计/贡献图（Task 1、5）、评价模型/接口（Task 2、6）、公开证书（Task 3、7）、契约/客户端（Task 4）、布局/统计卡（Task 5）、全量验证/文档（Task 8）。
- **Type consistency:** `UserActivity`、`ActivityDay`、`EvaluationPage`、`EvaluationItem`、`ProfileCertificate`、`getUserActivity`、`listUserEvaluations`、`saveMyEvaluation`、`deleteMyEvaluation`、`deleteUserEvaluation`、`listUserCertificates` 在任务间一致。
- **Review Focus mapping:** 统计口径/时区/streak（Task 1）、评价唯一性/权限/校验（Task 2）、证书隐私（Task 3）、契约（Task 4）、布局/贡献图（Task 5）、词云/分页/删除（Task 6）、证书卡（Task 7）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出签名、字段、权限和测试断言，不逐行抄写实现。
