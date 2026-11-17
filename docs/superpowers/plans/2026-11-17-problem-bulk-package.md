# 题目批量管理与题包导入导出 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 kapOJ 题包 v1 导入/导出、导入即公开、题库管理员批量操作与单题下载卡片。

**Architecture:** 在 problem 领域新增题包读写与批量操作服务；HTTP 层新增单题/批量下载、批量动作和通用题包导入；前端题库页增加多选批量模式，导入页增加公开选项，详情页右侧增加下载卡片。保留现有 HydroOJ 导入。

**Tech Stack:** Go + Gin + GORM + storage.Storage、Vue 3 + PrimeVue、Vitest、OpenAPI。

**Spec:** [`docs/superpowers/specs/2026-11-17-problem-bulk-package-design.md`](../specs/2026-11-17-problem-bulk-package-design.md)

## Global Constraints

- 题包格式 `kapOJ 题包 v1`，单题与批量包均可再次导入。
- 题包包含测试数据、checker、附件/题面图片、客观题答案、官方题解；仅创建者/`problem.manage_all` 可下载。
- 批量管理/批量导出仅 `problem.manage_all`。
- 导入即公开只对拥有 `problem.publish` 的用户生效；新建和覆盖题逐题发布，失败不回滚。
- 批量删除沿用现有规则：无提交物理删除；有提交只下线；学习路线引用逐题失败。
- 不导出数字 ID、提交统计、创建者 ID、用户提交。
- 所有新增接口更新 OpenAPI 并生成前端类型；先写失败测试。

## Review Focus

- 题包 ZIP 不得包含越界路径、重复附件名或未授权题目。
- 覆盖导入必须保留原题目 ID/创建者/提交统计，并清理旧测试点/附件/checker。
- 导入后公开失败（付费路线/学习路线/缺测试点）不能留下错误状态或回滚其他题。
- 批量删除部分失败不能误删其他题目；文件清理只在 DB 删除成功后。
- 批量导出任一题目越权时必须整批拒绝，不能返回部分 ZIP。

---

### Task 1: 题包格式与单题导出

**Files:**
- Create: `apps/server/internal/problem/package.go`
- Create: `apps/server/internal/problem/package_export.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Create: `apps/server/internal/problemapi/package_handler.go`
- Tests: `package_export_test.go`、`problemapi/package_handler_test.go`

**Interfaces:**
- `PackageManifest`, `PackageFileManifest`, `TestcaseManifest`.
- `Service.ExportProblem(ctx, id int64, w io.Writer) (PackageManifest, error)`.
- `GET /api/problems/:id/package`（创建者/`problem.manage_all`）。

- [ ] **Step 1: 测试单题 ZIP 结构、图片 URL 占位、附件/checker/题解/答案、非法权限。**
- [ ] **Step 2: 实现 manifest 与流式 ZIP 导出。**
- [ ] **Step 3: 注册下载接口并设置 `Content-Disposition`/`no-store`。**
- [ ] **Step 4: 测试并提交。**

### Task 2: 批量导出

**Files:**
- Modify: `apps/server/internal/problem/package_export.go`
- Create: `apps/server/internal/problemapi/bulk_handler.go`
- Tests: `package_export_test.go`、`bulk_handler_test.go`

**Interfaces:**
- `Service.ExportProblems(ctx, ids []int64, w io.Writer) (ProblemsManifest, error)`.
- `POST /api/problems/bulk/export` body `{problemIds}`，返回总 ZIP。

- [ ] **Step 1: 测试批量 ZIP 目录、根 manifest、100 题上限、越权整批拒绝。**
- [ ] **Step 2: 实现批量导出和流式响应。**
- [ ] **Step 3: 测试并提交。**

### Task 3: kapOJ 题包导入与导入即公开

**Files:**
- Create: `apps/server/internal/problem/package_import.go`
- Create: `apps/server/internal/problemapi/package_import_handler.go`
- Modify: `apps/server/internal/problem/import_hydro.go`
- Modify: `apps/server/internal/problemapi/import_hydro_handler.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Tests: `package_import_test.go`、`import_hydro_test.go`、handler tests

**Interfaces:**
- `Service.ImportPackage(ctx, createdBy int64, r io.ReaderAt, size int64, opts PackageImportOptions) (*PackageImportResult, error)`.
- `HydroImportOptions` 增加 `Publish bool`、`CanPublish bool`；`HydroImportedProblem` 增加 `Published bool`、`PublishError string`。
- `POST /api/problems/import/package`；Hydro import 接受 `publish=true`。
- 自动识别 `kapoj-problem` / `kapoj-problems` / Hydro。

- [ ] **Step 1: 测试 kapOJ 单题/批量往返、覆盖导入保留 ID、图片 URL 重写、验证发布失败逐题返回。**
- [ ] **Step 2: 实现 manifest 校验、创建/覆盖、testdata/checker/附件/题解导入。**
- [ ] **Step 3: 实现发布选项与权限校验。**
- [ ] **Step 4: 测试并提交。**

### Task 4: 批量公开/隐藏/删除

**Files:**
- Create: `apps/server/internal/problem/bulk.go`
- Modify: `apps/server/internal/problemapi/bulk_handler.go`
- Tests: `bulk_test.go`、`bulk_handler_test.go`

**Interfaces:**
- `Service.SetVisibility(ctx, id int64, public bool) error`.
- `BulkResult{ID, Status, Offlined, Error}`。
- `POST /api/problems/bulk/actions` body `{action, problemIds}`，仅 `problem.manage_all`。

- [ ] **Step 1: 测试 publish/hide/delete 三种动作、有提交只下线、学习路线逐题失败、部分成功。**
- [ ] **Step 2: 实现服务与 handler，记录审计汇总。**
- [ ] **Step 3: 测试并提交。**

### Task 5: 前端题库批量管理与详情下载卡片

**Files:**
- Modify: `apps/web/src/api/problems.ts`、`apps/web/src/api/types.ts`
- Modify: `apps/web/src/views/ProblemListView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/views/ProblemImportView.vue`
- Tests: 对应 specs
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `importProblemPackage(file, options)`、`exportProblemPackage(id)`、`bulkProblemAction`、`exportProblems(ids)`。
- 导入选项 `publish`。
- 题库批量模式：选择列、公开/隐藏/删除/下载/取消、结果面板。

- [ ] **Step 1: 写失败前端测试。**
- [ ] **Step 2: 实现 API/页面/权限展示。**
- [ ] **Step 3: OpenAPI + gen:api，测试/lint/build。**

### Task 6: 全量验证与部署

- [ ] server/judge `go test -p 1 ./...`
- [ ] web 全量测试/lint/build
- [ ] 测试库真实导出→导入往返
- [ ] 重建 server 容器，线上验证单题下载、批量接口、页面入口
- [ ] 更新文档与提交

## Self-Review

- 下载安全、批量权限、覆盖导入回滚、发布失败逐题返回均有测试任务。
- 单题与批量包共用 manifest；Hydro 导入保持兼容。
- 前端批量选择使用 DataTable 原生 selection，避免自维护跨页选择状态。
