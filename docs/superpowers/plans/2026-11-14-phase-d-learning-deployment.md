# Phase D：学习路线付费与部署设置 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 R11 付费学习路线、参与/推荐、排序与搜索筛选，以及 R12 后台部署域名配置与 CDN 资源前缀。

**Architecture:** 学习路线新增价格、购买、推荐与冗余计数；购买和推荐在数据库事务内完成金币流水与计数重算。付费路线引用的题目强制私有，并在题库发布与详情访问处接入学习路线守卫。站点设置新增 deployment JSONB，后台维护域名；前端用站点 store 统一解析 CDN/资源域名。

**Tech Stack:** Go + Gin + GORM + PostgreSQL、Vue 3 + PrimeVue 4、Vitest、OpenAPI TypeScript。

**Spec:** [`docs/持续开发需求.md`](../../持续开发需求.md) R11/R12、D7/D8/D10、Q4/Q8。

## Global Constraints

- 付费路线引用的题目必须 `is_public=false`；付费路线期间禁止公开该题；从所有活跃付费路线移除后可重新公开。
- 已购买用户永久访问；作者/管理员免费访问；不退款、不支持试看、不能自荐。
- 参与人数 = 付费路线“购买用户 ∪ 有进度用户”，免费路线“有进度用户”，按用户去重。
- 普通用户不能编辑 `sortOrder`；官方路线始终排在用户路线前；官方管理员可调整任意路线排序。
- 搜索按字面量、忽略大小写匹配标题/简介，转义 `%`/`_`；日期按 UTC 解析。
- 部署设置只保存域名/CDN/强制 HTTPS/备注；禁止保存云厂商密钥、数据库密码/JWT 密钥。
- 所有新增/变更接口更新 OpenAPI 并运行 `pnpm gen:api`；每项先写失败测试。

## Review Focus

- 并发购买/重复购买不得重复扣币；余额不足必须整体回滚。
- 免费路线、作者、管理员访问不应生成购买记录或扣币。
- 付费路线未购买时树内容必须为空，不能只靠前端隐藏。
- 付费路线题目公开/私有冲突必须返回 40001；题目详情不能绕过购买校验。
- 域名归一化后不能残留查询参数、锚点、用户信息或路径；公开 `/api/site` 不返回部署备注。

---

### Task 1: 学习路线价格、购买、推荐与计数

**Files:**
- Modify: `apps/server/internal/learning/model.go`
- Modify: `apps/server/internal/learning/dto.go`
- Modify: `apps/server/internal/learning/repository.go`
- Modify: `apps/server/internal/learning/path_service.go`
- Modify: `apps/server/internal/learning/progress.go`
- Create: `apps/server/internal/learning/purchase.go`
- Tests: `apps/server/internal/learning/purchase_test.go`

**Interfaces:**
- `LearningPath` 增加 `PriceCoins int64`、`ParticipantCount int`、`RecommendCount int`。
- 新表 `LearningPathPurchase`、`LearningPathRecommendation`，唯一 `(user_id, path_id)`。
- `PathItem` 增加 `priceCoins`、`purchased`、`participantCount`、`recommendCount`、`recommendedByMe`、`canEdit`、`canEditSort`。
- `PathInput` 增加 `PriceCoins int64`。
- `Service.Purchase(ctx, v, pathID) (*PurchaseResult, error)`；`PurchaseResult{Path PathItem, Coins int64}`。
- `Service.Recommend(ctx, v, pathID) (*PathItem, error)` / `Unrecommend(ctx, v, pathID) (*PathItem, error)`。
- `Repository.HasPurchase(ctx, userID, pathID) (bool, error)`、`RecalculateParticipantCount(ctx, pathIDs []int64) error`。

- [x] **Step 1: 写失败测试**

覆盖：免费路线不扣币；付费路线扣币并写 `learning_path_purchase` 流水；重复购买幂等；余额不足回滚；作者/管理员免费；买后 `purchased=true`；推荐/取消推荐计数；自荐 40001；未购买付费路线推荐 40001；参与人数按购买∪进度去重；普通用户创建自动排序、更新不改变排序；管理员更新排序生效。

- [x] **Step 2: 实现模型与迁移**

增加字段/表；`db.go` 增加 `learning_path_counts_backfill_v1`，用 SQL 重算 `participant_count`/`recommend_count`。

- [x] **Step 3: 实现购买与推荐事务**

购买：锁定路线行 → 校验价格/可见性 → 已购买直接返回 → `SpendCoinsTx(userID, price, "learning_path_purchase", "learning_path", pathID)` → 插入购买记录 → 重算参与人数 → 返回路径与余额。推荐：校验非作者与访问权限 → 插入/删除 + ON CONFLICT → 重算推荐数。

- [x] **Step 4: 更新列表/详情查询**

`pathItemColumns` 增加价格、计数、`purchased`、`recommendedByMe`；`PathItem` 由服务填充 `canEdit/canEditSort`；排序改为 `is_official DESC, sort_order ASC, created_at DESC`；普通用户创建时 `sort_order=max+1`，更新时保留原值。

- [x] **Step 5: 测试并提交**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 ./internal/learning ./internal/db -count=1`
Expected: PASS。

---

### Task 2: 付费路线题目守卫与树访问控制

**Files:**
- Modify: `apps/server/internal/learning/repository.go`
- Modify: `apps/server/internal/learning/node_service.go`
- Modify: `apps/server/internal/learning/path_service.go`
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/repository.go`
- Tests: `apps/server/internal/learning/paid_problem_test.go`、`apps/server/internal/problem/service_test.go`

**Interfaces:**
- `problem.LearningGuard` 增加：
  - `IsProblemInPaidPath(ctx, problemID int64) (bool, error)`
  - `CanAccessPaidProblem(ctx, userID, problemID int64) (bool, error)`
- `learning.Service` 实现上述方法；问题服务在“私有题不可见”后尝试 `CanAccessPaidProblem`。
- `Tree` 增加 `locked bool`；未购买且非作者/管理员时 `nodes=[]`。
- `ProblemSummary` 增加内部 `IsPublic bool json:"-"`；`ProblemSummaries(ctx, ids, includePrivate bool)`。

- [x] **Step 1: 写失败测试**

- 免费路线节点仍要求公开题；付费路线节点公开题返回 40001 并列出题目。
- 免费路线改付费时若有公开题返回 40001；题目在付费路线期间发布返回 40001；移除后发布成功。
- 未购买用户树 `locked=true`、节点空；购买后可见私有题摘要；作者/管理员直接可见。
- 已购买用户 `problem.Service.Get` 可读付费题；未购买 404；作者/管理员可读。

- [x] **Step 2: 实现守卫**

学习仓库新增 `IsProblemInPaidPath`、`CanAccessPaidProblem`、`ProblemSummaries(includePrivate)`；节点校验按路线价格选择“必须公开/必须私有”；改价前校验冲突。题库发布时调用 paid-path 守卫；`Get` 私有分支调用购买守卫。

- [x] **Step 3: 测试并提交**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 ./internal/learning ./internal/problem ./internal/problemapi -count=1`
Expected: PASS。

---

### Task 3: R11 接口、OpenAPI 与前端

**Files:**
- Modify: `apps/server/internal/learning/dto.go`（如未完成）
- Modify: `apps/server/internal/learningapi/handler.go`
- Modify: `apps/web/src/api/learning.ts`
- Modify: `apps/web/src/views/LearningPathListView.vue`
- Modify: `apps/web/src/views/LearningPathDetailView.vue`
- Modify: `apps/web/src/views/LearningPathEditView.vue`
- Modify: `apps/web/src/views/AdminLearningPathsView.vue`
- Modify: `apps/web/src/components/learning/LearningNodeDialog.vue`
- Tests: 对应 `.spec.ts`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `GET /api/learning-paths?keyword=&createdFrom=&createdTo=&scope=&page=&page_size=` 返回分页；`scope=all|official|custom|mine`。
- `GET /api/learning-paths/:id/tree` 未购买返回 `locked=true, nodes=[]`。
- `POST /api/learning-paths/:id/purchase` 返回 `{path, coins}`。
- `POST|DELETE /api/learning-paths/:id/recommend` 返回更新后的 `PathItem`。

- [x] **Step 1: 写失败测试**

服务端 handler：分页/日期/关键字/scope；购买、推荐权限与 40001；OpenAPI 类型生成。前端：列表筛选写 URL、价格/参与/推荐/已购买标签；详情锁定 CTA、购买扣币刷新、推荐 toggle；编辑页价格输入、普通用户无排序；付费路线节点选择隐藏题。

- [x] **Step 2: 实现后端接口与 OpenAPI**

handler 解析查询参数、`httpx.ParsePage`；注册购买/推荐路由；更新 schema 并 `pnpm gen:api`。

- [x] **Step 3: 实现前端**

列表：搜索框、日期范围、scope、分页、卡片标签（免费/价格/参与/推荐/已购买）。详情：价格与计数、推荐按钮、锁定购买 CTA；购买后 `auth.fetchMe()`。编辑：价格输入；`sortOrder` 仅 `canManageOfficial` 可见。节点弹窗：付费路线按 `scope=mine/all + includeHidden` 搜索。

- [x] **Step 4: 测试并提交**

Run: `cd apps/server && go test -p 1 ./internal/learning ./internal/learningapi -count=1`；`corepack pnpm --filter web test`；`pnpm --filter web lint && pnpm --filter web build`；`pnpm gen:api`。

---

### Task 4: R12 部署设置后端

**Files:**
- Modify: `apps/server/internal/site/model.go`
- Modify: `apps/server/internal/site/dto.go`
- Modify: `apps/server/internal/site/service.go`
- Modify: `apps/server/internal/site/repository.go`
- Modify: `apps/server/internal/site/validate.go`
- Modify: `apps/server/internal/siteapi/handler.go`
- Tests: `apps/server/internal/site/service_test.go`、`apps/server/internal/siteapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `Deployment` DTO 字段：`siteDomain/apiDomain/cdnDomain/assetsDomain/forceHttps/notes`。
- `Settings.Deployment dbtypes.JSON[Deployment]`（`deployment_json`，默认 `{}`）。
- `PublicView.Deployment PublicDeployment{cdnDomain,assetsDomain}`；`AdminView` 增加完整 `deployment`。
- `Service.UpdateDeployment(ctx, actorID, in) (*AdminView, error)`。
- `GET/PUT /api/admin/site/deployment`，权限 `site.settings.manage`，写审计 `site_settings`。

- [x] **Step 1: 写失败测试**

空默认；合法保存/刷新持久化；尾部 `/` 归一化；缺 scheme、查询参数、锚点、用户信息、路径、非法端口、notes>500 返回 40001；`forceHttps` 接受 http 但服务端不回退；公开 `/api/site` 只含 CDN/资源域名，不含备注；无权限 403。

- [x] **Step 2: 实现模型、校验、服务与路由**

`normalizeDomain(raw, required)`：trim、去尾部 `/`、`url.Parse`、仅 http/https、Host 非空、无 User/Query/Fragment、Path 为空或 `/`；notes 按 rune 限制 500。公开视图只投影 `cdnDomain/assetsDomain`。

- [x] **Step 3: 测试并提交**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 ./internal/site ./internal/siteapi -count=1`
Expected: PASS。

---

### Task 5: R12 部署 Tab 与资源 URL 解析

**Files:**
- Modify: `apps/web/src/api/site.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/stores/site.ts`
- Modify: `apps/web/src/views/AdminSiteView.vue`
- Modify: `apps/web/src/components/IdentityShowcase.vue`
- Modify: `apps/web/src/views/BadgeShopView.vue`、`AppearanceView.vue`
- Modify: avatar surfaces：`UserMenu.vue`、`HomeView.vue`、`FollowListDialog.vue`、`UserProfileView.vue`、`ProblemStatisticsView.vue`
- Modify: `apps/web/src/components/ProblemAttachmentsCard.vue`、`ProblemAttachmentManager.vue`
- Modify: `apps/web/src/components/MarkdownViewContent.vue`、`MarkdownEditor.vue`
- Tests: `AdminSiteView.spec.ts`、`site.spec.ts`（如有）、组件 spec

**Interfaces:**
- `siteApi.getAdminSiteDeployment()` / `updateAdminSiteDeployment(input)`。
- store 增加 `deployment`、`assetsBaseUrl`、`assetUrl(path)`：`assetsDomain || cdnDomain`；绝对 URL/data/blob 原样返回。
- `AdminSiteView` 新增 `部署` Tab，含表单、实时预览、复制配置、HTTPS 警告、检查清单、敏感信息提示。

- [x] **Step 1: 写失败测试**

部署表单保存/归一化/错误；复制文本；`assetUrl` 在配置 CDN 后拼接、未配置回退；徽章/头像/附件/题面图片使用解析后的 URL。

- [x] **Step 2: 实现 store 与页面**

表单字段、校验、保存；`applyView` 填充 deployment；`applyMutation` 同步 store。资源组件统一调用 `site.assetUrl()`；Markdown 预览通过 `transformImgUrl` 接入。

- [x] **Step 3: 测试并提交**

Run: `corepack pnpm --filter web test && corepack pnpm --filter web lint && corepack pnpm --filter web build && corepack pnpm gen:api`。

---

### Task 6: 文档、全量验证与部署

- [ ] 更新 `docs/持续开发需求.md` Phase D 状态与实施记录。
- [ ] 全量后端：server/judge 顺序执行 `go test -p 1 ./...`。
- [ ] 前端全量测试、lint、build。
- [ ] `git diff --check`；只提交本阶段文件，不触碰用户未提交改动。
- [ ] 重建并重启 `kapoj-server`/`kapoj-judge`，线上验证购买、锁定树、CDN 配置与权限。

## Self-Review

- **Spec coverage:** R11 购买/隐藏/推荐/排序/搜索由 Task 1–3 覆盖；R12 配置/公开子集/CDN 由 Task 4–5 覆盖。
- **Review Focus:** 并发购买、作者免费、树锁定、发布冲突、域名归一化均有对应测试任务。
- **Proportion:** 计划按可独立验收的后端/前端交付物拆分；实现细节沿用现有 service/repository/view 模式。
