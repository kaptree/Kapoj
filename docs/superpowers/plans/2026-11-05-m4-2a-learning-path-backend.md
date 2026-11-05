# M4-2A 学习路线后端：模型、权限与 CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付学习路线后端基础：路线/节点模型、官方/自定义权限、路线与节点 CRUD、树结构校验、列表与结构树 API。

**Architecture:** 新增 `learning` 包与 `learningapi`；路线/节点 GORM 模型；服务层集中权限与树校验；路线列表/树 API 在 A 阶段返回结构（进度字段先为游客态），M4-2B 接入服务端 AC 进度、事件与题目守卫。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、testify。

**Spec:** `docs/superpowers/specs/2026-11-05-m4-learning-paths-design.md` 3–5、8、10。

## Global Constraints

- 任意登录用户可创建自定义路线，默认 `is_public=false`；官方路线仅 `problem_admin`/`super_admin`，且强制公开。
- 作者可编辑/删除自己的路线；管理员可编辑/删除任意路线；作者不能修改 `isOfficial`/`isActive`。
- `DELETE` 软删除；inactive 路线仅管理员 `includeInactive=true` 可见/可恢复；作者不能自行重新上架被管理员下架的路线。
- 恢复 `isActive` 或切换 `isPublic=true` 前必须重新校验所有节点题目存在且公开；失效返回 40001。
- 节点父级必须同路线、不可成环、最大深度 8；只允许删叶子。
- 节点 1–50 个不重复公开题目；`required_count` 1–题目数。
- 本阶段不实现进度计算/AC 事件/题目 LearningGuard（M4-2B）；树 DTO 的进度字段先返回游客态（0/根 active/其余 locked）。
- 所有用户可见文案中文；不推送、不合并分支。

## Review Focus

1. **权限矩阵**：官方/自定义、作者/他人/管理员、公开/私有/inactive；测试放 Task 1/2。
2. **树完整性**：父节点同路线、环、深度、叶子删除；测试放 Task 2。
3. **题目/required_count 校验**：数量、重复、非公开、required 范围；测试放 Task 2。
4. **恢复/公开校验**：inactive 题目失效时不能恢复/公开；测试放 Task 1/2。
5. **软删除可见性**：inactive 仅管理员可见/恢复，作者不能；测试放 Task 1。

---

### Task 1: 模型、迁移、路线 CRUD 与权限

**Files:**
- Create: `apps/server/internal/learning/model.go`
- Create: `apps/server/internal/learning/repository.go`
- Create: `apps/server/internal/learning/path_service.go`
- Create: `apps/server/internal/learning/path_service_test.go`
- Create: `apps/server/internal/learning/dto.go`
- Create: `apps/server/internal/learning/validate.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Create: `apps/server/internal/learningapi/handler.go`
- Create: `apps/server/internal/learningapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`

**Interfaces:**
- Consumes: `user.RoleProblemAdmin`/`RoleSuperAdmin`、`problem` 表、`dbtypes.List`、`testutil.PG`。
- Produces:
  - `learning.LearningPath{ID,Title,DescriptionMD,CoverURL string; IsOfficial,IsPublic,IsActive bool; CreatedBy int64; SortOrder int; CreatedAt,UpdatedAt time.Time}`。
  - `learning.LearningPathNode{ID,PathID int64; ParentID *int64; Title,DescriptionMD string; ProblemIDs dbtypes.List[int64]; SortOrder int; RequiredCount int; CreatedAt,UpdatedAt}`。
  - `learning.UserLearningProgress{ID,UserID,PathID,NodeID int64; Status string; SolvedCount int; CompletedAt *time.Time; UpdatedAt time.Time}`（本阶段只建表，不计算）。
  - `learning.Viewer{UserID int64; CanManage bool}`。
  - `learning.PathInput{Title,DescriptionMD,CoverURL string; IsOfficial,IsPublic,IsActive *bool; SortOrder int}`。
  - `learning.PathItem{...; Author UserSummary; NodeCount int; MyCompletedCount int}`。
  - `(*Service).ListPaths(ctx,v,includeInactive,mine) ([]PathItem,error)`。
  - `(*Service).FindPath(ctx,v,id) (*PathItem,error)`。
  - `CreatePath/UpdatePath/DeletePath`。
  - API：`GET /api/learning-paths`、`GET /api/learning-paths/:id`（元信息，含节点数）；`POST/PUT/DELETE /api/learning-paths`。
  - Router `Deps.Learning *learning.Service` optional；existing tests without Learning skip routes。

- [ ] **Step 1: 写失败模型/权限测试**

```go
func TestPathPermissionMatrix(t *testing.T) {
    // admin creates official path; user creates custom private path
    // owner can update own; other user 403/404; admin can update any
    // non-admin cannot set isOfficial or isActive
}
func TestInactivePathVisibility(t *testing.T) {
    // author deletes own path -> inactive
    // author list without includeInactive doesn't see it; admin includeInactive sees
    // author cannot restore; admin can
}
func TestPathValidation(t *testing.T) {
    // title/description/cover/sort; official forced public
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ -run 'TestPath' -v`
Expected: FAIL（包/类型不存在）。

- [ ] **Step 3: 实现模型、迁移、仓储与路线服务**

- `db.Migrate` 追加三个模型；`db_test.go` 断言表/唯一索引。
- `LearningPath.CreatedBy` 必填；`IsOfficial` 时 `IsPublic=true`。
- `CreatePath`：
  - `v.UserID<=0` → 401；
  - `IsOfficial=true` 仅 `v.CanManage`；
  - 自定义默认 `IsPublic=false, IsActive=true, CreatedBy=v.UserID`；
  - admin 可按输入设置 official/public/active。
- `UpdatePath`：
  - 仅 owner/admin；inactive 仅 admin 可访问/恢复；
  - 非 admin 不能改 `IsOfficial`/`IsActive`；official path 仅 admin；
  - 提交 `IsPublic=true` 或 admin 恢复 `IsActive=true` 时，调用 `validatePathProblemsPublic(ctx,id)`；任何节点题目失效返回 40001 “题目已下线，请先修复节点”；
  - 未提交字段保持原值；`IsOfficial` 强制 public。
- `DeletePath`：owner/admin；软删除 `is_active=false`。
- `ListPaths`：
  - admin `includeInactive` 看全部；否则只看 active 的 official/public + 自己的 private；
  - `mine=true` 只看 `created_by=v.UserID`（含自己的 private；inactive 仍仅 admin）；
  - 排序 official desc, sort_order, id；作者摘要 join users；节点数 count。
- `FindPath` 权限：active official/public 所有人；active private 仅 owner/admin；inactive 仅 admin；否则 40400。
- Author `UserSummary{ID,Username,Nickname}`。

- [ ] **Step 4: 实现 API 与路由**

- `learningapi` Options 无需身份装饰（M4-2C 可加）；`GET/POST/PUT/DELETE`。
- 路由 meta：`GET` 公开（可选 auth）；写接口 `RequireAuth`；official 由 service 校验。
- Router `Deps.Learning` optional。

- [ ] **Step 5: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/learning internal/learningapi internal/db internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ ./internal/learningapi/ ./internal/db/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/learning apps/server/internal/learningapi apps/server/internal/db apps/server/internal/router
git commit -m "feat(server): learning path models and CRUD"
```

---

### Task 2: 节点 CRUD、树结构与校验

**Files:**
- Create: `apps/server/internal/learning/node_service.go`
- Create: `apps/server/internal/learning/node_service_test.go`
- Create: `apps/server/internal/learning/tree.go`
- Create: `apps/server/internal/learning/tree_test.go`
- Modify: `apps/server/internal/learning/repository.go`
- Modify: `apps/server/internal/learning/dto.go`
- Modify: `apps/server/internal/learningapi/handler.go`
- Modify: `apps/server/internal/learningapi/handler_test.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 的 Path/Viewer/权限。
- Produces:
  - `NodeInput{Title,DescriptionMD string; ParentID *int64; ProblemIDs []int64; SortOrder int; RequiredCount *int}`。
  - `NodeItem{ID,ParentID,Title,DescriptionMD,ProblemIDs,Problems []ProblemSummary,SortOrder,RequiredCount,SolvedCount,Status,Children []NodeItem}`。
  - `ProblemSummary{ProblemID,Title,Slug string; Difficulty int}`。
  - `Tree{Path PathItem; Nodes []NodeItem}`。
  - `(*Service).Tree(ctx,v,pathID) (*Tree,error)`：本阶段结构树，游客态状态（根 active、其余 locked），`SolvedCount=0`；M4-2B 替换为真实进度。
  - `CreateNode/UpdateNode/DeleteNode`。
  - API：`GET /api/learning-paths/:id/tree`、`POST /api/learning-paths/:id/nodes`、`PUT/DELETE /api/learning-paths/:id/nodes/:nodeId`。

- [ ] **Step 1: 写失败节点/树测试**

```go
func TestNodeValidation(t *testing.T) {
    // parent same path; cycle A->B->A; depth >8; required_count 0/len+1; 0/51 problems; duplicate; private problem
}
func TestLeafOnlyDelete(t *testing.T)
func TestTreeStructureAndGuestStatus(t *testing.T) {
    // root active, child locked, solvedCount 0; children sorted
}
func TestNodePermissions(t *testing.T) {
    // owner/admin; others 404/403; official path only admin
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ -run 'TestNode|TestLeaf|TestTree' -v`
Expected: FAIL。

- [ ] **Step 3: 实现节点校验与 CRUD**

- 节点创建/更新：owner/admin；parent 必须同 path；禁止自环/成环；深度 ≤8；problem IDs 1–50 不重复、全部 `is_public=true`；required_count 默认 len，范围 1..len；sort_order 允许任意 int。
- 更新节点时若路径 inactive，仅 admin；若路径 `is_public=true`，题目校验失败返回 40001。
- 删除仅叶子：有子节点返回 40001；删除后级联进度（FK 或事务）。
- 树构建：查询节点按 `sort_order,id`；join problems 获取 title/slug/difficulty；嵌套 `Children`；根节点 parent_id NULL；孤儿节点防御性忽略/报错（数据完整性）。
- `Tree` 权限复用 `FindPath`；节点题目只返回公开题。
- 本阶段状态：游客/登录均 `root active`、`child locked`，`solvedCount=0`；M4-2B 接管。

- [ ] **Step 4: 实现 API、运行测试、提交**

Run: `cd apps/server && gofmt -w internal/learning internal/learningapi && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ ./internal/learningapi/ -v`
Expected: PASS。

```bash
git add apps/server/internal/learning apps/server/internal/learningapi
git commit -m "feat(server): learning path nodes and tree"
```

---

### Task 3: OpenAPI、生成类型、文档与全量检查

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端

**Interfaces:**
- Consumes: Task 1–2。
- Produces: M4-2A 状态文档与前端可用类型。

- [ ] **Step 1: 补 OpenAPI**

- Schemas：`LearningPathItem`、`LearningPathInput`、`LearningPathNodeItem`、`LearningNodeInput`、`LearningProblemSummary`、`LearningTree`、`LearningPathListResponse`。
- Paths：`GET /learning-paths`、`GET /learning-paths/{id}`、`POST/PUT/DELETE /learning-paths`、`GET /learning-paths/{id}/tree`、`POST/PUT/DELETE /learning-paths/{id}/nodes...`。
- 描述官方/自定义权限、私有/inactive 可见性、节点校验。
- `corepack pnpm gen:api`。

- [ ] **Step 2: 文档**

- `DEVELOPMENT.md` 3.8/4.5：M4-2A 已完成（模型/权限/CRUD/树结构）；M4-2B 进度/事件/守卫、M4-2C 前端待办。
- `docs/任务清单.md`：M4-6 🚧（模型/CRUD 完成，进度/守卫待办）；加 M4-2A 计划链接；M4-7 ⬜。
- `docs/开发日志.md`：追加 M4-2A 条目（权限矩阵、树校验、软删除/恢复、验证、复审修复）。
- `README.md`、`运行手册.md`：当前 M4-2A；说明学习路线 API 已可用但前端/进度未上线。
- `docs/primevue-migration.md`：不修改 UI-31（前端未做）。

- [ ] **Step 3: 全量检查**

```bash
cd /Users/admin/codespace/kapoj/apps/server
gofmt -l .
go vet ./...
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run

cd /Users/admin/codespace/kapoj/apps/judge
gofmt -l .
go vet ./...
go test -count=1 ./...

cd /Users/admin/codespace/kapoj
corepack pnpm test:web
corepack pnpm lint:web
corepack pnpm build:web
git diff --check
```

- [ ] **Step 4: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record m4-2a learning path backend"
```

---

## Self-Review

**Spec coverage:**

- 模型/迁移/权限/CRUD → Task 1。
- 节点校验/树结构/叶子删除 → Task 2。
- OpenAPI/文档 → Task 3。
- 进度/事件/LearningGuard → M4-2B；前端 → M4-2C。

**Type consistency:**

- `LearningPathNode.ParentID *int64` 与树根 NULL 一致。
- `PathInput` 的 `IsOfficial/IsPublic/IsActive *bool` 区分缺省保持与显式设置。
- 树 DTO 的 `Children []NodeItem` 与前端 TreeTable 兼容。

**Review Focus mapping:**

1. 权限矩阵 → Task 1/2 tests。
2. 树完整性 → Task 2 tests。
3. 题目/required → Task 2 tests。
4. 恢复/公开校验 → Task 1/2 tests。
5. 软删除可见性 → Task 1 tests。

**Proportion:** 计划固定权限、字段、校验和测试；具体 SQL/树构建按现有 M3/M4 风格完成。
