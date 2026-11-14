# Phase B：题库权限、附件、测试数据与导入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付出题人/管理员题库权限、测试数据与公开附件分离、题面图片粘贴上传、测试数据在线编辑，以及题包覆盖/`.ans`/批量测试数据上传。

**Architecture:** 先加 RBAC 权限与 ownership 边界，再以 `problem_attachments` 表承载公开附件和题面图片；测试数据继续使用 `problem_testcases` 与私有存储。随后增加测试数据内容读写接口和编辑弹窗，最后扩展 HydroOJ 导入与批量上传。所有写接口先做“管理员全部 / 出题人仅自己”的校验，再进入业务层。

**Tech Stack:** Go + Gin + GORM + PostgreSQL、Vue 3 + PrimeVue 4、Vitest、Playwright。

**Spec:** [`docs/持续开发需求.md`](../../持续开发需求.md)

**Status:** ✅ Tasks 1–8 complete (2026-11-14). Verification: backend problem/problemapi/rbac/site/siteapi tests pass; frontend 118 test files / 913 tests pass; lint and build pass. Real-browser E2E and cloud deployment acceptance remain for final review.

## Global Constraints

- 出题人可创建、编辑、发布、下线、删除自己的题目，但不能操作他人题目；`problem.manage_all`/super 可操作全部。
- 测试数据私有；普通附件/题面图片随题目可见性公开或隐藏。
- 题面图片必须本地存储，不依赖 CDN；MarkdownEditor 通过 `onUploadImg` 上传。
- 题包导入的“题目 ID”指 slug；覆盖题面+测试数据，保留提交记录，覆盖后强制下线。
- 同名 `.ans`/`.out` 同时存在时使用 `.out`。
- 测试数据在线编辑单文件上限 2 MiB；允许空输入/空输出。
- 所有新增接口更新 OpenAPI 并生成前端类型；每个任务先写失败测试。
- 不改动工作区中与本阶段无关的未提交文件。

## Review Focus

- 出题人不能通过 list/detail/testcase/attachment/import 接口看到或修改他人题目；隐藏题对无权限用户仍返回 404。
- `includeAnswer`/`includeSolution` 必须按“管理员全部 / 作者本人”收敛，不能因为拥有 `problem.update` 就读取他人题目的答案或题解。
- 附件与测试数据不能互相串接口；测试数据永远不能从公开附件接口下载。
- 题面图片上传失败不能插入坏 Markdown；隐藏题图片的访问边界要明确。
- 覆盖导入不能改变数字 ID、创建者、提交记录；覆盖后必须下线。

---

### Task 1: RBAC 权限与出题人角色

**Files:**
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/seed.go`
- Modify: `apps/server/internal/rbac/seed_test.go`
- Modify: `apps/server/internal/rbac/roles_test.go`（权限计数/角色列表）
- Modify: `apps/web/src/permissions.ts`
- Modify: `apps/web/src/permissions.spec.ts`

**Interfaces:**
- Produces: `PermProblemManageAll = "problem.manage_all"`、`RoleCodeProblemAuthor = "problem_author"`。
- Role `problem_author` 权限：`problem.create`、`problem.update`、`problem.publish`、`problem.delete`、`problem.testcase.manage`、`problem.checker.manage`。
- `content_admin` 增加 `problem.manage_all`。

- [ ] **Step 1: 写失败测试**

在 `seed_test.go` 断言权限目录包含 `problem.manage_all`，`content_admin` 拥有它，内置角色包含 `problem_author` 且权限集合正确。
在 `roles_test.go` 更新角色数量/权限计数断言。
在 `permissions.spec.ts` 断言前端常量包含新权限。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/rbac -run 'Seed|Roles|Permission' -count=1`
Expected: FAIL，权限/角色不存在。

- [ ] **Step 3: 实现后端目录与角色**

- `catalog.go`：新增权限常量/目录项，`problem_author` 角色；`content_admin` 权限加入 `problem.manage_all`。
- `seed.go`：`SeedVersion = "2"`；首次 seed 自动包含新角色权限；已存 version 1 的库执行 `migrateSeedV2`，幂等给 `content_admin` 补 `problem.manage_all`、给 `problem_author` 补默认权限。不要恢复管理员已删除的其他权限。

- [ ] **Step 4: 实现前端权限常量**

`permissions.ts` 增加 `PERM.problem.manageAll`，加入 `PROBLEM_MANAGE_PERMISSIONS` 与 `ADMIN_ACCESS_PERMISSIONS`；更新测试。

- [ ] **Step 5: 运行测试与生成类型**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/rbac -count=1
corepack pnpm --filter web test src/permissions.spec.ts
```
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/rbac apps/web/src/permissions.ts apps/web/src/permissions.spec.ts
git commit -m "feat: add problem author role and manage-all permission"
```

---

### Task 2: 题目 ownership、列表范围与隐藏题默认展示

**Files:**
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problem/dto.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problem/service_test.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `apps/web/src/utils/problemQuery.ts`
- Modify: `apps/web/src/views/ProblemListView.vue`
- Modify: `apps/web/src/views/ProblemListView.spec.ts`
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.spec.ts`
- Modify: `apps/web/src/views/ProblemEditView.vue`

**Interfaces:**
- `problem.Viewer` 增加 `UserID int64`、`CanManageOwn bool`、`CanManageAll bool`。
- `problem.Filter`/`ListQuery` 增加 `OwnerID int64`；`DetailDTO` 增加 `CanEdit bool`。
- `GET /api/problems?scope=public|mine|all&includeHidden=`。

- [ ] **Step 1: 写失败测试**

服务层：
- 作者 `Viewer{UserID, CanManageOwn:true}` 可以 `Get` 自己的隐藏题，不能 `Get` 他人隐藏题；管理员 `CanManageAll` 可看全部。
- `List` 的 `scope=mine` 只返回自己的题；`scope=all` 需要管理员。
- `DetailDTO.CanEdit` 对作者本人/管理员 true，对他人 public 题 false。
Handler：
- 作者 token 可更新/删除/测试数据/checker 自己的题；不能操作他人题（403/404）。
- 作者不能读取他人题目的答案/题解字段。
- 列表 `scope=mine` 默认包含自己的隐藏题。

- [ ] **Step 2: 运行后端测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/problem ./internal/problemapi -run 'Ownership|Scope|CanEdit|Hidden' -count=1`
Expected: FAIL。

- [ ] **Step 3: 实现服务/仓储**

- `Get`：公开题所有人；隐藏题仅 `CanViewHidden`、`CanManageAll`、或 `CanManageOwn && CreatedBy==UserID`。
- `List`：`OwnerID` 过滤；`scope=mine` 强制 owner；`scope=all` 要求 `CanManageAll`；legacy `includeHidden` 对作者只放开自己的隐藏题。
- `DetailDTO` 增加 `CanEdit`，handler 按 ownership 设置。
- `includeSolution`/`includeAnswer` 只在 `CanManageAll` 或作者本人时为 true；`problem.update` 本身不再自动放开他人题解/答案。

- [ ] **Step 4: 实现写接口 ownership 校验**

在 `problemapi` 增加：
```go
func (h *handler) canManageProblem(c *gin.Context, id int64) (bool, error)
func (h *handler) requireProblemManage(c *gin.Context, id int64) bool
```
对 update/delete/testcase list/update/reorder/delete/upload/checker/testcase-generation 全部调用。管理员/ super 放行；作者仅 `created_by == userID`；无权限统一 404 或 403（不泄露隐藏题存在性）。

- [ ] **Step 5: 实现前端题库范围**

- `problemQuery.ts` 增加 `scope`。
- `ProblemListView.vue`：
  - `canManageAll`/`canManageOwn` 计算；
  - 范围 Select（公开题库/我的题目/全部题目，按权限显示）；
  - 默认范围管理员 `all`、出题人 `mine`、普通用户 `public`；
  - 管理范围默认 `showHidden=true`；
  - 请求带 `scope` 与 `includeHidden`。
- `ProblemDetailView.vue` 编辑按钮改用 `problem.canEdit`；无权限不显示。
- `ProblemEditView.vue` 加载后若 `!problem.canEdit` 显示禁止访问，不提交。

- [ ] **Step 6: 运行前后端测试与构建**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/problem ./internal/problemapi -count=1
corepack pnpm --filter web test src/views/ProblemListView.spec.ts src/views/ProblemDetailView.spec.ts src/views/ProblemEditView.spec.ts
corepack pnpm --filter web build
```
Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/web/src/utils/problemQuery.ts \
  apps/web/src/views/ProblemListView.vue apps/web/src/views/ProblemListView.spec.ts \
  apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts \
  apps/web/src/views/ProblemEditView.vue
git commit -m "feat: scope problem management to authors and admins"
```

---

### Task 3: 题目附件后端

**Files:**
- Create: `apps/server/internal/problem/attachment.go`
- Create: `apps/server/internal/problem/attachment_repository.go`
- Create: `apps/server/internal/problem/attachment_test.go`
- Create: `apps/server/internal/problemapi/attachment_handler.go`
- Create: `apps/server/internal/problemapi/attachment_handler_test.go`
- Modify: `apps/server/internal/problem/model.go`
- Modify: `apps/server/internal/db/db.go`（AutoMigrate）
- Modify: `apps/server/internal/router/router.go`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `ProblemAttachment {ID, ProblemID, Kind, Filename, StorageKey, ContentType, Size, CreatedBy, CreatedAt}`
- `kind`: `attachment` / `statement_image`
- 接口：
  - `GET /api/problems/:id/attachments`
  - `POST /api/problems/:id/attachments`（multipart `file`，可多个）
  - `GET /api/problems/:id/attachments/:aid`
  - `DELETE /api/problems/:id/attachments/:aid`
  - `POST /api/problems/:id/assets`（multipart `file`，返回 `{url, markdown}`）
  - `GET /api/problems/:id/assets/:aid`（内联图片）

- [ ] **Step 1: 写失败测试**

- 附件 CRUD：上传/列表/下载/删除；测试数据不在附件列表。
- 普通用户不能上传/删除；作者只能操作自己的题；管理员全部。
- 图片上传校验 PNG/JPEG/WebP/GIF、≤5 MiB；附件 ≤20 MiB。
- 隐藏题附件/图片对无权限用户 404。

- [ ] **Step 2: 实现模型/仓储/服务**

- `problem_attachments` 迁移，索引 `(problem_id, kind)`。
- 存储键：`problems/<id>/attachments/<random>`、`problems/<id>/assets/<random>.<ext>`。
- 上传先写存储再写 DB；DB 失败删文件；删除先删 DB 再 best-effort 删文件。
- 列表/下载按题目可见性；测试数据接口不变。

- [ ] **Step 3: 注册路由与 OpenAPI**

- 附件写接口使用 `problem.update` + ownership。
- 公开读接口使用题目可见性（含隐藏题作者/管理员）。
- OpenAPI 增加 schema 与路径，生成前端类型。

- [ ] **Step 4: 运行后端测试**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/problem ./internal/problemapi -run 'Attachment|Asset' -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/server/internal/db apps/server/internal/router packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat: add problem attachments and statement image storage"
```

---

### Task 4: 附件前端与题面图片粘贴上传

**Files:**
- Create: `apps/web/src/components/ProblemAttachmentManager.vue`
- Create: `apps/web/src/components/ProblemAttachmentManager.spec.ts`
- Create: `apps/web/src/components/ProblemAttachmentsCard.vue`
- Create: `apps/web/src/components/ProblemAttachmentsCard.spec.ts`
- Modify: `apps/web/src/api/problems.ts`
- Modify: `apps/web/src/views/ProblemEditView.vue`
- Modify: `apps/web/src/views/ProblemEditView.spec.ts`
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.spec.ts`
- Modify: `apps/web/src/components/MarkdownEditor.vue`
- Modify: `apps/web/src/components/MarkdownEditor.spec.ts`

**Interfaces:**
- `MarkdownEditor` 增加 `problemId?: number`、`uploadImage?: (file: File) => Promise<string>`；有 `problemId` 时使用附件 assets 接口，否则禁用上传并提示先保存。
- 附件管理器暴露 `attachments` 列表、上传、删除、复制链接。

- [ ] **Step 1: 写失败测试**

- 编辑器图片按钮/paste 调用上传函数，成功后插入 `![](url)`；失败不插入。
- 附件管理器上传/删除/复制；详情页有附件卡片；无附件不显示。
- 新建题目未保存时禁用图片上传。

- [ ] **Step 2: 实现 API + 组件**

- `api/problems.ts` 增加附件/图片接口。
- `MarkdownEditor.vue` 移除 `image` 排除与 `no-upload-img`，接入 `onUploadImg`；粘贴图片复用上传函数。
- `ProblemAttachmentManager.vue`：上传/拖拽、列表、复制、删除。
- `ProblemAttachmentsCard.vue`：详情下载列表。
- 编辑页在测试数据区域前/后增加附件区域；详情页右侧资源卡片增加附件。

- [ ] **Step 3: 运行前端测试与构建**

Run:
```bash
corepack pnpm --filter web test src/components/ProblemAttachmentManager.spec.ts src/components/ProblemAttachmentsCard.spec.ts src/components/MarkdownEditor.spec.ts src/views/ProblemEditView.spec.ts src/views/ProblemDetailView.spec.ts
corepack pnpm --filter web build
```
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add apps/web/src/components/ProblemAttachmentManager.vue apps/web/src/components/ProblemAttachmentManager.spec.ts \
  apps/web/src/components/ProblemAttachmentsCard.vue apps/web/src/components/ProblemAttachmentsCard.spec.ts \
  apps/web/src/api/problems.ts apps/web/src/views/ProblemEditView.vue apps/web/src/views/ProblemEditView.spec.ts \
  apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts \
  apps/web/src/components/MarkdownEditor.vue apps/web/src/components/MarkdownEditor.spec.ts
git commit -m "feat: manage problem attachments and paste statement images"
```

---

### Task 5: 测试数据在线查看/编辑 API

**Files:**
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problem/dto.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problem/service_test.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `GET /api/problems/:id/testcases/:tid/content` → `{input, output, inputSize, outputSize, score, isSample}`
- `PUT /api/problems/:id/testcases/:tid/content` body `{input, output}`
- `POST /api/problems/:id/testcases/online` body `{input, output, score, isSample}`

- [ ] **Step 1: 写失败测试**

- 作者/管理员可读/写/在线创建；普通用户不可。
- 超过 2 MiB 返回 40001；空输入/空输出允许。
- 写入失败不破坏旧文件；成功后旧文件被清理。
- 输入/输出必须同时提交。

- [ ] **Step 2: 实现服务/仓储/路由/OpenAPI**

- 读取存储文本，超过 2 MiB 返回 `tooLarge` 元信息或 40001。
- 更新写入新 key，DB 更新，成功后删旧 key。
- 在线创建复用 `AddTestcase`，写入两个存储 key，失败清理。

- [ ] **Step 3: 运行测试**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/problem ./internal/problemapi -run 'TestcaseContent|OnlineTestcase' -count=1`
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat: add testcase content read and online editing APIs"
```

---

### Task 6: 测试数据在线编辑弹窗

**Files:**
- Create: `apps/web/src/components/TestcaseEditDialog.vue`
- Create: `apps/web/src/components/TestcaseEditDialog.spec.ts`
- Modify: `apps/web/src/api/problems.ts`
- Modify: `apps/web/src/components/TestcaseManager.vue`
- Modify: `apps/web/src/components/TestcaseManager.spec.ts`

**Interfaces:**
- `TestcaseEditDialog` props：`problemId`、`testcase?: Testcase`、`visible`；emits `saved`。
- 新建模式左右分栏输入/输出，分数默认 10、样例默认 false。

- [ ] **Step 1: 写失败测试**

- 编辑加载内容，保存调用 put；新建保存调用 online create。
- 超过 2 MiB 显示错误，保存禁用。
- 未保存关闭二次确认；保存成功后刷新列表。

- [ ] **Step 2: 实现弹窗与集成**

- `TestcaseEditDialog.vue` 左右 `Textarea`（移动端上下），信息条、分数/样例、下载按钮。
- `TestcaseManager.vue` 行“查看/编辑”和工具栏“在线创建测试点”。

- [ ] **Step 3: 运行测试与构建**

Run:
```bash
corepack pnpm --filter web test src/components/TestcaseEditDialog.spec.ts src/components/TestcaseManager.spec.ts
corepack pnpm --filter web build
```
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add apps/web/src/components/TestcaseEditDialog.vue apps/web/src/components/TestcaseEditDialog.spec.ts \
  apps/web/src/api/problems.ts apps/web/src/components/TestcaseManager.vue apps/web/src/components/TestcaseManager.spec.ts
git commit -m "feat: edit and create testcases in a split dialog"
```

---

### Task 7: HydroOJ 导入 `.ans` 与覆盖

**Files:**
- Modify: `apps/server/internal/problem/import_hydro.go`
- Modify: `apps/server/internal/problem/import_hydro_test.go`
- Modify: `apps/server/internal/problemapi/import_hydro_handler.go`
- Modify: `apps/server/internal/problemapi/import_hydro_handler_test.go`
- Modify: `apps/web/src/views/ProblemImportView.vue`
- Modify: `apps/web/src/views/ProblemImportView.spec.ts`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- 导入字段 `conflictPolicy=ask|skip|overwrite`、`overwriteSlugs[]`、`skipSlugs[]`。
- `.in` 配对 `.out` 或 `.ans`；同名优先 `.out`。

- [ ] **Step 1: 写失败测试**

- `.in/.ans` 导入成功；`.ans`+`.out` 同时存在使用 `.out` 并提示。
- 冲突 ask 返回冲突列表且不写库；overwrite 覆盖题面+测试数据、保留 ID/创建者/提交、强制下线。
- 无权限覆盖他人题目失败；skip 只跳过冲突。
- 前端拖拽文件、冲突弹窗与重试。

- [ ] **Step 2: 实现后端解析/覆盖**

- 扩展配对逻辑；增加冲突预检与覆盖事务。
- 覆盖更新题面字段与测试数据，保留统计/提交；写审计；整包级错误回滚。

- [ ] **Step 3: 实现前端拖拽与冲突弹窗**

- `ProblemImportView.vue` 拖拽卡片、文件信息、冲突选择、结果分组。
- 保留原 File 对象，用户选择后重新提交。

- [ ] **Step 4: 运行测试**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/problem ./internal/problemapi -run 'ImportHydro|Ans|Conflict' -count=1
corepack pnpm --filter web test src/views/ProblemImportView.spec.ts
```
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem/import_hydro.go apps/server/internal/problem/import_hydro_test.go \
  apps/server/internal/problemapi/import_hydro_handler.go apps/server/internal/problemapi/import_hydro_handler_test.go \
  apps/web/src/views/ProblemImportView.vue apps/web/src/views/ProblemImportView.spec.ts packages/api/openapi.yaml
git commit -m "feat: support Hydro import overwrite and .ans testdata"
```

---

### Task 8: 批量测试数据上传

**Files:**
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problem/service_test.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `apps/web/src/api/problems.ts`
- Modify: `apps/web/src/components/TestcaseManager.vue`
- Modify: `apps/web/src/components/TestcaseManager.spec.ts`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `POST /api/problems/:id/testcases/batch` multipart `files` 多个，`score` 默认 10。
- 服务端按 basename 配对 `.in` + `.out`/`.ans`；全部通过才写入。

- [ ] **Step 1: 写失败测试**

- 多对文件批量上传成功，排序追加，分数默认 10。
- 缺输出/重复/超限/未配对整体失败，清理已写文件。
- 作者只能操作自己的题。

- [ ] **Step 2: 实现后端批量服务与接口**

- 解析 multipart 文件，配对；校验数量/大小；先写存储再批量 DB；失败清理。

- [ ] **Step 3: 实现前端批量拖拽区**

- `TestcaseManager.vue` 增加拖拽/多选、配对预览、未配对禁用上传。
- 上传成功后刷新列表。

- [ ] **Step 4: 运行测试**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/problem ./internal/problemapi -run 'BatchTestcase' -count=1
corepack pnpm --filter web test src/components/TestcaseManager.spec.ts
```
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/web/src/api/problems.ts \
  apps/web/src/components/TestcaseManager.vue apps/web/src/components/TestcaseManager.spec.ts packages/api/openapi.yaml
git commit -m "feat: batch upload paired testcase files"
```

---

## Self-Review

- **Spec coverage:** R04 由 Task 1–4 覆盖；R13 由 Task 5–6 覆盖；R03 由 Task 7–8 覆盖。
- **Ownership:** 每个写接口和隐藏题读取都有负向测试；附件与测试数据接口分离。
- **Type consistency:** `problem.manage_all`、`problem_author`、`ProblemAttachment`、`CanEdit` 在前后端任务中命名一致。
- **Review Focus tests:** 他人题目越权、答案/题解泄露、附件/测试数据串读、`.ans`/`.out` 优先级、覆盖后下线均有测试。
- **Proportion:** 计划覆盖 3 个需求，按可独立验收的交付物拆分；具体实现细节留给执行时结合现有模式补齐。
