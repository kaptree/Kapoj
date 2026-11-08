# M5-7 证书认证 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用户提交证书图片，管理员审核通过/驳回；通过后生成唯一验证码，公开核验证书类型、等级、持有者昵称与通过日期；证书图片仅本人与管理员可见。

**Architecture:** 新增 `apps/server/internal/certificate` 领域（类型/证书模型、仓储、服务、seed）+ `certificateapi`（公开类型/核验、用户提交/我的证书/图片、管理员审核/类型 CRUD）；图片用现有 `storage.Storage` 保存 `certificates/<id>`，通过鉴权接口读取；前端新增 `/settings/certificates`、`/verify`、`/admin/certificates`，并增加 `certificate.manage` 权限、UserMenu/AdminNav 入口、OpenAPI/生成类型。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、本地 storage、Vue 3、PrimeVue 4、Vitest。

**Spec:** `docs/superpowers/specs/2026-11-08-m5-7-certificate-design.md`

## Global Constraints

- 新增权限 `certificate.manage`（第 37 个，分组「证书」，默认仅 `super_admin`）；用户提交/我的证书/图片只需登录；公开类型列表/公开核验无需登录。
- 证书类型：内置 7 类；首次启动且表为空时 seed；管理员 CRUD；有证书引用时禁止删除；停用后公开列表不返回；允许改名，历史证书使用提交时 `certificate_type_name` 快照。
- 证书图片：≤5 MiB，仅 `image/png`、`image/jpeg`、`image/webp`（magic bytes），本地 storage key `certificates/<id>`；仅 owner 或 `certificate.manage` 可读，其他 404；不暴露 storage key。
- 公开核验只返回类型、等级、持有者昵称、通过日期；不返回图片、邮箱、用户 ID、审核人、驳回原因。
- 验证码 `KAPOJ-XXXX-XXXX`，crypto 随机、唯一、大小写不敏感；通过时生成；pending/rejected 无验证码。
- 状态 `pending` → `approved`/`rejected`；仅 pending 可审核；驳回原因 1–500 字符；终态不可再审核。
- 允许同一用户同类型多张证书；不做唯一约束；不提供删除/撤回/重新审核；不做审核通知、公开用户主页展示。
- 图片/证书/类型操作写审计；驳回原因/验证码不写审计。
- 前端只用 PrimeVue 4；不新增依赖；中文文案；图片通过鉴权 Blob 加载并管理 object URL。
- 不接入 MinIO；不改 M5-8 头像；不推送/合并，除非用户确认；`Export.zip` 保持未跟踪。

## Review Focus

1. **图片访问边界**：owner/管理员可读，其他用户 404；公开核验无图；DTO/日志不泄露 storage key。测试：owner 200、他人 404、管理员 200、无 token 401、public verify 无 imageUrl。
2. **审核状态机/验证码**：仅 pending 可审；approve 生成唯一 code、写审核人/时间；reject 原因必填；重复审核 40001；公开核验只认 approved。测试：状态流转、code 唯一、大小写容错、pending/rejected invalid。
3. **提交校验与清理**：类型不存在/停用、level 空/超长、空图/超 5 MiB/非图片均 40001；store/DB 失败清理行/文件。测试：表驱动 + 失败清理。
4. **类型历史完整性**：seed 只空表、删除被引用类型 40001、停用后公开不返回、改名后历史证书/核验仍用快照名。测试：service/handler。
5. **权限/审计**：`certificate.manage` 计数 37；普通用户 403；审核/类型操作审计 action/target；公开接口无需登录。
6. **前端**：我的证书状态/驳回原因/验证码复制/Blob 预览、公开核验输入/query/无效态、管理员审核/驳回原因/类型 CRUD、导航权限、object URL 清理、390px/暗亮主题。

---

### Task 1: 证书类型、模型、迁移、seed 与权限（CERT-A）

**Files:**
- Create: `apps/server/internal/certificate/model.go`
- Create: `apps/server/internal/certificate/dto.go`
- Create: `apps/server/internal/certificate/repository.go`
- Create: `apps/server/internal/certificate/service.go`
- Create: `apps/server/internal/certificate/service_test.go`
- Create: `apps/server/internal/certificateapi/handler.go`
- Create: `apps/server/internal/certificateapi/type_handler.go`
- Create: `apps/server/internal/certificateapi/handler_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/audit.go`
- Modify: `apps/server/internal/rbac/roles_test.go`
- Modify: `apps/server/internal/rbac/seed_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`

**Interfaces:**
- Produces:
  ```go
  type CertificateType struct {
      ID int64; Name string; Description string
      IsActive bool; SortOrder int
      CreatedAt time.Time; UpdatedAt time.Time
  }
  func (CertificateType) TableName() string { return "certificate_types" }

  type TypeInput struct {
      Name string `json:"name"`
      Description string `json:"description"`
      IsActive *bool `json:"isActive"`
      SortOrder int `json:"sortOrder"`
  }
  type TypeView struct { ID int64; Name, Description string; IsActive bool; SortOrder int }

  type Repository interface {
      ListTypes(ctx context.Context, includeInactive bool) ([]CertificateType, error)
      FindType(ctx context.Context, id int64) (*CertificateType, error)
      CreateType(ctx context.Context, t *CertificateType) error
      UpdateType(ctx context.Context, t *CertificateType) error
      DeleteType(ctx context.Context, id int64) error
  }
  type Service struct { repo Repository; store storage.Storage }
  func NewService(repo Repository, store storage.Storage) *Service
  func (s *Service) ListActiveTypes(ctx context.Context) ([]TypeView, error)
  func (s *Service) ListAllTypes(ctx context.Context) ([]TypeView, error)
  func (s *Service) CreateType(ctx context.Context, in TypeInput) (*TypeView, error)
  func (s *Service) UpdateType(ctx context.Context, id int64, in TypeInput) (*TypeView, error)
  func (s *Service) DeleteType(ctx context.Context, id int64) error
  func SeedTypes(ctx context.Context, db *gorm.DB) error

  // certificateapi
  type Options struct { RBAC *rbac.Service }
  func RegisterRoutes(public, api, uploads gin.IRouter, s *certificate.Service, opts ...Options)

  // rbac
  PermCertificateManage PermissionCode = "certificate.manage"
  PermissionGroupCertificate = "证书"
  AuditTargetCertificate = "certificate"
  AuditTargetCertificateType = "certificate_type"

  // router.Deps
  Certificate *certificate.Service
  ```
- Routes:
  - Public `GET /api/certificate-types` → active types.
  - Admin `GET /api/admin/certificate-types` → all types (`certificate.manage`).
  - Admin `POST/PUT/DELETE /api/admin/certificate-types[/:id]` → CRUD + audit target `certificate_type`.
- Consumes: `storage.Storage`, `rbac` middleware, `httpx` errors/pages, `db.Migrate`.

- [ ] **Step 1: 写失败测试**

  `certificate/service_test.go`（真实 `testutil.PG`）：
  - `TestSeedTypesOnlyWhenEmpty`：空表 seed 7 条；再次 seed 不重复；已有自定义类型时不覆盖。
  - `TestListActiveAndAllTypes`：active 排序；includeInactive 返回停用。
  - `TestCreateUpdateDeleteType`：合法创建/更新；名称重复 40001；空名称/超长/描述超长/排序越界 40001。
  - `TestDeleteTypeInUse`：插入引用证书后删除 40001「已有证书使用该类型，请先停用」；无引用可删。
  - `TestTypeRenameKeepsCertificateSnapshot`：插入 `certificate_type_name` 快照的证书，改名后读证书仍用快照名（与 Task 2 模型一起验证；可先插入行）。

  `certificateapi/handler_test.go`：
  - `TestPublicCertificateTypes`：无 token 200，只返回 active。
  - `TestAdminCertificateTypesPermission`：无 token 401、普通用户 403、`certificate.manage` 用户 200/201/200/200。
  - `TestAdminCertificateTypeValidationAndAudit`：重复名称 40001；成功后审计 target `certificate_type`、action `certificate.manage`。
  - `TestDeleteTypeInUseHTTP`：40001。

  RBAC 测试：权限目录 36→37，包含 `certificate.manage`；默认仅 super_admin；`seed_test.go` 与 `roles_test.go` 的计数更新。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/certificate/ ./internal/certificateapi/ ./internal/rbac/ \
    -run 'TestSeedTypes|TestListActive|TestCreateUpdateDeleteType|TestDeleteType|TestPublicCertificate|TestAdminCertificate|TestPermission' -v
  ```
  Expected: FAIL（包/类型不存在）。

- [ ] **Step 3: 实现**

  - `certificate/model.go`：`CertificateType` 表 `certificate_types`（unique name、`is_active`、`sort_order`）与完整 `Certificate` 模型/`Status`（Task 2 使用；Task 1 的「类型被引用不可删」测试需要 `certificates` 表存在）。
  - `certificate/repository.go`：GORM 类型 CRUD；`DeleteType` 用 `DELETE ... WHERE id=? AND NOT EXISTS (SELECT 1 FROM certificates WHERE certificate_type_id=?)` + 先 `FindType` 区分 404/引用。
  - `certificate/service.go`：`Validation` name 1–50、description ≤200、sort 0–10000；`IsActive` nil 创建默认 true/更新保留；`ErrTypeInUse` 映射 40001；seed 表为空时插入 7 条默认类型（按 sort 1..7）。
  - `certificateapi`：`handler{svc, rbac}`；`RegisterRoutes` 注册 public/admin 类型路由；`middleware.RequirePermission(resolver, "certificate.manage")`、`middleware.AuditAdminAction(..., "certificate_type")`；错误 `httpx.Fail`。
  - `rbac/catalog.go`：权限/分组/定义（SortOrder 在 AI 之后）；`PermissionGroupCertificate`。
  - `rbac/audit.go`：target 常量。
  - `db.Migrate`：`&certificate.CertificateType{}` 与 `&certificate.Certificate{}`（Task 1 建表，Task 2 加 API）。
  - `router.Deps.Certificate`；`main` 构造 `certificate.NewService(certificate.NewGormRepository(g), store)`，`certificate.SeedTypes`，传入 router。
  - 更新 RBAC 计数断言 36→37；`content_admin` 不自动授予新权限。
  - Handler 测试可用 `router.New` 真实 DB/RBAC 或最小 Gin + middleware；沿用现有 problemapi/gamificationapi fixture 模式。

- [ ] **Step 4: 运行测试、gofmt、lint**

  ```bash
  cd apps/server
  gofmt -w internal/certificate internal/certificateapi internal/rbac internal/db internal/router cmd/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/certificate/ ./internal/certificateapi/ ./internal/rbac/ ./internal/db/ ./internal/router/
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
  ```
  Expected: PASS；lint 0 issues。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/server/internal/certificate apps/server/internal/certificateapi \
    apps/server/internal/rbac apps/server/internal/db apps/server/internal/router apps/server/cmd/server
  git commit -m "feat(server): add certificate types and permission"
  ```

---

### Task 2: 用户提交、我的证书、鉴权图片与公开核验（CERT-B）

**Files:**
- Modify: `apps/server/internal/certificate/model.go`
- Modify: `apps/server/internal/certificate/dto.go`
- Modify: `apps/server/internal/certificate/repository.go`
- Modify: `apps/server/internal/certificate/service.go`
- Modify: `apps/server/internal/certificate/service_test.go`
- Create: `apps/server/internal/certificateapi/certificate_handler.go`
- Modify: `apps/server/internal/certificateapi/handler.go`
- Modify: `apps/server/internal/certificateapi/handler_test.go`
- Modify: `apps/server/internal/db/db.go`

**Interfaces:**
- Produces:
  ```go
  type Status string
  const (
      StatusPending Status = "pending"; StatusApproved Status = "approved"; StatusRejected Status = "rejected"
  )
  type Certificate struct {
      ID int64; UserID int64; CertificateTypeID int64; CertificateTypeName string
      Level string; ImageKey string; ImageContentType string
      Status Status; RejectReason string; VerifyCode *string
      SubmittedAt time.Time; ReviewedAt *time.Time; ReviewedBy *int64
      CreatedAt time.Time; UpdatedAt time.Time
  }
  func (Certificate) TableName() string { return "certificates" }

  type CertificateView struct {
      ID int64; Type string; Level string; Status Status; ImageURL string
      RejectReason string; VerifyCode string; SubmittedAt time.Time; ReviewedAt *time.Time
  }
  type PublicCertificateView struct {
      Valid bool; Type string; Level string; Nickname string; ApprovedAt *time.Time
  }
  type Image struct { Data []byte; ContentType string }

  // Repository additions:
  CreateCertificate(ctx, *Certificate) error
  FindCertificate(ctx, id int64) (*Certificate, error)
  UpdateCertificateImage(ctx, id int64, key, contentType string) error
  DeleteCertificate(ctx, id int64) error
  ListMine(ctx, userID int64, offset, limit int) ([]Certificate, int64, error)
  FindByVerifyCode(ctx, code string) (*Certificate, string /*nickname*/, error)

  func (s *Service) Submit(ctx context.Context, userID, typeID int64, level string, image io.Reader) (*CertificateView, error)
  func (s *Service) ListMine(ctx context.Context, userID int64, page, size int) (*httpx.Page[CertificateView], error)
  func (s *Service) Image(ctx context.Context, id, viewerID int64, canManage bool) (*Image, error)
  func (s *Service) Verify(ctx context.Context, code string) (*PublicCertificateView, error)
  ```
- Routes:
  - `uploads.POST("/certificates")` auth → multipart `typeId`,`level`,`file`.
  - `api.GET("/certificates/my")` auth.
  - `api.GET("/certificates/:id/image")` auth (owner/admin).
  - public `GET /api/certificates/verify?code=...`.
- Consumes: Task 1 service/repo, `middleware.RequireAuth`, `CurrentClaims`, `CurrentAccess`, storage, `http.DetectContentType`.

- [ ] **Step 1: 写失败测试**

  `certificate/service_test.go`：
  - `TestSubmitCertificate`：active type → pending、快照类型名、level、image key/contentType、我的列表可见；`Image` owner 返回 bytes/contentType。
  - `TestSubmitValidation`：类型不存在/停用、level 空/超 50、空图、>5 MiB、text/plain → 40001；失败不留行/文件。
  - `TestListMinePagination`：多张按 submitted_at desc；total。
  - `TestCertificateImageAccess`：owner 200；其他用户 404；管理员 200；不存在 404。
  - `TestVerifyApprovedOnly`：approved 返回 valid + type/level/nickname/approvedAt；pending/rejected/unknown `{valid:false}`；大小写/空格/无连字符容错；不返回图片/userId/email/rejectReason。
  - `TestSubmitStorageFailureCleansRow`：storage Put 失败/update 失败删除行/文件（用 fake storage 或不可写路径）。

  `certificateapi/handler_test.go`：
  - `TestSubmitCertificateHTTP`：登录 multipart 201；游客 401；缺字段/非法类型 40001；body 超限映射。
  - `TestMyCertificatesHTTP`：只返回本人；分页。
  - `TestCertificateImageHTTP`：owner 200 Content-Type；他人 404；管理员 200；无 token 401。
  - `TestPublicVerifyHTTP`：无 token 200；approved/invalid；响应不含 `imageUrl`/`userId`/`email`/`rejectReason`。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/certificate/ ./internal/certificateapi/ \
    -run 'TestSubmit|TestListMine|TestCertificateImage|TestVerify|TestMyCertificates|TestPublicVerify' -v
  ```
  Expected: FAIL。

- [ ] **Step 3: 实现**

  - 模型：`Certificate` 全字段/`Status` 已由 Task 1 建表；确认 `VerifyCode *string` unique、索引 `(user_id, submitted_at)`、`(status, submitted_at)` 已就绪，缺失字段补齐。
  - 仓储：Create/Find/UpdateImage/Delete/ListMine/FindByVerifyCode（join users 取 nickname；`status='approved' AND verify_code=?`）。
  - Service.Submit：
    1. trim level；1–50 字符；`FindType` active；快照 name。
    2. 读图 ≤5 MiB+1；`http.DetectContentType` 允许 PNG/JPEG/WebP；空/超限/非图片 40001。
    3. 插入 pending（ImageKey 空）；`storage.Put("certificates/<id>")`；`UpdateCertificateImage`；失败删除行/文件。
  - Service.ListMine：page/size（`httpx.ParsePage` 在 handler）；视图 `imageUrl=/api/certificates/<id>/image`。
  - Service.Image：Find；owner 或 canManage 才读；storage.Open/Read；404 统一。
  - Service.Verify：normalize code（大写/去空格；8 位时补 `KAPOJ-` 和连字符）；查 approved；join nickname；返回 valid；否则 `{valid:false}`。
  - Handler：multipart `FormFile("file")`，`typeId`/`level`；上传错误中文；`middleware.RequireAuth`；public verify 走 public group；image 使用 `CurrentClaims` + `CurrentAccess` 判断 `certificate.manage`。
  - `certificateapi.RegisterRoutes` 增加 public/API/uploads 路由；public group 由 router 以无 `Authenticate` 的 `r.Group("/api", MaxBody)` 创建并传入。
  - 测试 fake storage/真实本地 storage；清理 object。

- [ ] **Step 4: 运行测试、gofmt、lint**

  ```bash
  cd apps/server
  gofmt -w internal/certificate internal/certificateapi internal/db
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/certificate/ ./internal/certificateapi/ ./internal/db/ ./internal/router/
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
  ```
  Expected: PASS。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/server/internal/certificate apps/server/internal/certificateapi apps/server/internal/db
  git commit -m "feat(server): add certificate submission and verification"
  ```

---

### Task 3: 管理员审核、列表与审计（CERT-C）

**Files:**
- Modify: `apps/server/internal/certificate/dto.go`
- Modify: `apps/server/internal/certificate/repository.go`
- Modify: `apps/server/internal/certificate/service.go`
- Modify: `apps/server/internal/certificate/service_test.go`
- Create: `apps/server/internal/certificateapi/admin_handler.go`
- Modify: `apps/server/internal/certificateapi/handler.go`
- Modify: `apps/server/internal/certificateapi/handler_test.go`

**Interfaces:**
- Produces:
  ```go
  type AdminFilter struct { Status Status; Page, Size int }
  type CertificateUserView struct { ID int64; Username, Nickname string }
  type AdminCertificateView struct { CertificateView; User CertificateUserView }
  type ReviewInput struct { Action string `json:"action"`; Reason string `json:"reason"` }
  func (s *Service) ListAdmin(ctx context.Context, status Status, page, size int) (*httpx.Page[AdminCertificateView], error)
  func (s *Service) Review(ctx context.Context, actorID, id int64, in ReviewInput) (*CertificateView, error)
  ```
- Routes:
  - `GET /api/admin/certificates?status=&page=&page_size=` (`certificate.manage`).
  - `POST /api/admin/certificates/:id/review` body `{action:"approve"|"reject",reason}` (`certificate.manage`).
- Audit: handler after successful review uses `rbac.RecordAudit` (best-effort) action `certificate.manage`, target `certificate`, targetID, summary「通过证书/驳回证书」；类型 CRUD 已由 middleware audit。

- [ ] **Step 1: 写失败测试**

  `certificate/service_test.go`：
  - `TestAdminListFiltersAndPagination`：pending 默认；status 筛选；用户摘要；total。
  - `TestReviewApprove`：pending → approved、reviewed_by/at、verify_code 非空；公开 Verify 变为 valid；重复 approve/reject 40001。
  - `TestReviewReject`：原因必填/1–500；→ rejected、reject_reason、verify_code NULL；公开 Verify invalid；原因仅 owner/admin 可见。
  - `TestReviewNotFound`/`TestReviewTerminal` 40001/404。
  - `TestVerifyCodeUniqueRetry`：冲突后生成新 code（可注入 fixed generator 或在仓储层重试）。
  - `TestReviewNoSensitivePublicFields`。

  `certificateapi/handler_test.go`：
  - `TestAdminCertificateList`：普通用户 403；管理员 200；status 筛选。
  - `TestAdminReviewHTTP`：approve/reject；非管理员 403；未登录 401；非法 action/缺原因 40001。
  - `TestReviewAudit`：审计 action `certificate.manage`、target `certificate`、targetID；不写驳回原因/验证码。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/certificate/ ./internal/certificateapi/ \
    -run 'TestAdminList|TestReview|TestVerifyCode|TestAdminCertificate|TestReviewAudit' -v
  ```
  Expected: FAIL。

- [ ] **Step 3: 实现**

  - 仓储 `ListAdmin`：join `users`（id/username/nickname），status 可选，按 submitted_at desc, id desc，total。
  - 仓储 `Review`：`UPDATE certificates SET status=?, reject_reason=?, verify_code=?, reviewed_at=?, reviewed_by=? WHERE id=? AND status='pending'`；返回 affected；服务先 Find 区分 404/terminal。
  - Service `Review`：action approve/reject；approve 生成唯一 code（crypto/rand，Crockford 字母表去 I/L/O/U/0/1，`KAPOJ-XXXX-XXXX`，冲突重试 5 次）；reject 原因 trim 1–500，code NULL；返回 view。
  - Handler：`parseStatus`（pending/approved/rejected/all）；review 解析 `ReviewInput`；成功后 `rbac.RecordAudit`（`AuditMeta{ActorID,IP,UserAgent}`），失败只 warn；`middleware.RequirePermission` + 路由。
  - 公开 Verify 使用 Task 2 的方法；无需改。
  - 错误文案中文：`只有待审核的证书可以审核`、`驳回原因需为 1–500 字`、`未知证书` 等。

- [ ] **Step 4: 运行测试、gofmt、lint**

  ```bash
  cd apps/server
  gofmt -w internal/certificate internal/certificateapi
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/certificate/ ./internal/certificateapi/ ./internal/rbac/ ./internal/router/
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
  ```
  Expected: PASS。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/server/internal/certificate apps/server/internal/certificateapi
  git commit -m "feat(server): add certificate review"
  ```

---

### Task 4: OpenAPI、Blob 客户端、我的证书与公开核验页（CERT-D）

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/http.ts`
- Modify: `apps/web/src/api/http.spec.ts`
- Create: `apps/web/src/api/certificates.ts`
- Create: `apps/web/src/views/CertificateSettingsView.vue`
- Create: `apps/web/src/views/CertificateSettingsView.spec.ts`
- Create: `apps/web/src/views/CertificateVerifyView.vue`
- Create: `apps/web/src/views/CertificateVerifyView.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/UserMenu.vue`
- Modify: `apps/web/src/components/UserMenu.spec.ts`

**Interfaces:**
- Produces:
  ```ts
  // http.ts
  interface Http { getBlob(url: string): Promise<Blob> }
  // api/certificates.ts
  listCertificateTypes(): Promise<CertificateTypeView[]>
  submitCertificate(input: { typeId: number; level: string; file: File }): Promise<CertificateView>
  listMyCertificates(page?: number, pageSize?: number): Promise<Page<CertificateView>>
  getCertificateImage(id: number): Promise<Blob>
  verifyCertificate(code: string): Promise<PublicCertificateView>
  ```
- Routes: `/settings/certificates` name `certificate-settings` requiresAuth; `/verify` name `certificate-verify` public.
- UserMenu「我的证书」→ certificate-settings.
- Consumes: Task 1–3 API; PrimeVue; `AppPageHeader`/`AppSurface`/`AppFormField`/`DataTable`/`Dialog`/`Select`/`InputText`/`Message`/`Tag`/`Toast`.

- [ ] **Step 1: OpenAPI + 生成类型**

  - 添加所有 Task 1–3 的 paths/schemas：
    - `GET /certificate-types` public → `CertificateTypeListResponse`。
    - `POST /certificates` multipart auth → 201 `CertificateResponse`。
    - `GET /certificates/my` auth → `CertificatePageResponse`。
    - `GET /certificates/{id}/image` auth → `image/png`/jpeg/webp binary（401/403/404）。
    - `GET /certificates/verify?code=` public → `PublicCertificateResponse`（200 valid false/true；不暴露敏感字段）。
    - `GET/POST/PUT/DELETE /admin/certificate-types` + `GET /admin/certificates` + `POST /admin/certificates/{id}/review`。
  - Schemas：`CertificateTypeView`, `CertificateTypeInput`, `CertificateView`, `AdminCertificateView`, `CertificatePage`, `PublicCertificateView`, `CertificateReviewInput`；错误响应。
  - `corepack pnpm gen:api`；types 别名。

- [ ] **Step 2: 写失败测试**

  `http.spec.ts`：`getBlob` 发送 Bearer、返回原始 Blob、401 时走 refresh 重试（可用现有 adapter 模式）。
  `CertificateSettingsView.spec.ts`：
  - 加载类型 + 我的证书；空态；状态 Tag/驳回原因/验证码复制。
  - 提交校验：类型必填、等级 1–50、文件必填/≤5 MiB/PNG/JPEG/WebP；成功 multipart 调用 + Toast + 刷新。
  - 图片预览：`getCertificateImage` 返回 Blob，`URL.createObjectURL` 被调用；卸载 revoke。
  - 错误显示。
  `CertificateVerifyView.spec.ts`：
  - 输入 code 点击核验 → valid 显示类型/等级/昵称/日期；invalid 显示提示；`?code=` 自动核验；空 code 不调用；加载/错误。
  - 不渲染图片。
  `UserMenu.spec.ts`：登录后出现「我的证书」并跳转；游客不显示。
- [ ] **Step 3: 运行测试确认失败**

  ```bash
  cd /Users/admin/codespace/kapoj
  corepack pnpm --filter web test -- src/api/http.spec.ts src/views/CertificateSettingsView.spec.ts src/views/CertificateVerifyView.spec.ts src/components/UserMenu.spec.ts
  ```
  Expected: FAIL。

- [ ] **Step 4: 实现**

  - `http.ts`：重构 `send` 支持 `responseType: 'blob'` 或新增 `sendBlob`；`getBlob` 复用 refresh/401 逻辑；不解析 Envelope。
  - `api/certificates.ts` 方法；`types.ts` 别名。
  - `CertificateSettingsView.vue`：PageHeader + 申请认证按钮 + DataTable/列表 + Dialog 表单；图片 blob 预览/revoke；验证码复制 `${origin}/verify?code=...`；状态 Tag（待审核/已通过/已驳回）；驳回原因；空/加载/错误。
  - `CertificateVerifyView.vue`：PageHeader + 验证码表单 + 结果卡片；`route.query.code` 自动核验；valid 显示类型/等级/昵称/日期；invalid 中性提示；不显示图片。
  - 路由 + UserMenu 入口；权限无需额外（requiresAuth）。
  - 样式只 `--kapoj-*`；390px 表格滚动/单列。
- [ ] **Step 5: 运行测试、lint、build**

  ```bash
  corepack pnpm gen:api
  corepack pnpm test:web
  corepack pnpm lint:web
  corepack pnpm build:web
  ```
  Expected: PASS。
- [ ] **Step 6: 提交**

  ```bash
  git add packages/api/openapi.yaml apps/web/src/api apps/web/src/views/CertificateSettingsView.vue \
    apps/web/src/views/CertificateSettingsView.spec.ts apps/web/src/views/CertificateVerifyView.vue \
    apps/web/src/views/CertificateVerifyView.spec.ts apps/web/src/router/index.ts \
    apps/web/src/components/UserMenu.vue apps/web/src/components/UserMenu.spec.ts
  git commit -m "feat(web): add certificate pages"
  ```

---

### Task 5: 管理后台证书审核/类型管理与导航（CERT-E）

**Files:**
- Create: `apps/web/src/views/AdminCertificatesView.vue`
- Create: `apps/web/src/views/AdminCertificatesView.spec.ts`
- Modify: `apps/web/src/api/admin.ts`（若 Task 4 未放管理方法）
- Modify: `apps/web/src/permissions.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/components/layout/AdminNav.spec.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Routes: `/admin/certificates` name `admin-certificates`, `permission: certificate.manage`, title 证书审核.
- AdminNav content group item「证书审核」 if `auth.can(PERM.certificate.manage)`.
- API (admin.ts): `listAdminCertificates({status,page,pageSize})`, `reviewAdminCertificate(id,{action,reason})`, `listAdminCertificateTypes()`, `createAdminCertificateType`, `updateAdminCertificateType`, `deleteAdminCertificateType`.
- UI: Tabs「证书审核」「证书类型」；审核列表默认 pending、状态筛选、分页；Dialog 图片 blob、用户/类型/等级/时间、通过/驳回（原因必填）；类型 DataTable + Dialog CRUD + 删除确认；错误 Toast/Message。
- Consumes: Task 4 types/client; PrimeVue.

- [ ] **Step 1: 写失败测试**

  `AdminCertificatesView.spec.ts`：
  - 权限/路由? 无权限不显示/`auth.can` false 隐藏动作。
  - 默认 pending 列表；状态筛选/分页调用参数；空/加载/错误。
  - 审核 Dialog 加载图片 Blob；approve 调 `reviewAdminCertificate(id,{action:'approve'})` + Toast + 刷新；reject 无原因阻止；有原因调用。
  - 类型 Tab：列表；创建/编辑校验（名称 1–50、描述 ≤200、排序）；删除确认；被引用 40001 显示 Toast。
  - object URL revoke。
  `AdminNav.spec.ts`：有 `certificate.manage` 显示「证书审核」；无权限隐藏。
- [ ] **Step 2: 运行测试确认失败**

  ```bash
  corepack pnpm --filter web test -- src/views/AdminCertificatesView.spec.ts src/components/layout/AdminNav.spec.ts
  ```
  Expected: FAIL。
- [ ] **Step 3: 实现**

  - `permissions.ts`：`certificate.manage` + `ADMIN_ACCESS_PERMISSIONS`。
  - `AdminNav` item in content group.
  - `AdminCertificatesView` with Tabs/DataTable/Dialog/InputText/Select/Textarea/Button/Tag/Toast/Confirm; blob image; review actions; type CRUD; responsive.
  - Route.
- [ ] **Step 4: 运行测试、lint、build**

  ```bash
  corepack pnpm test:web
  corepack pnpm lint:web
  corepack pnpm build:web
  ```
  Expected: PASS。
- [ ] **Step 5: 提交**

  ```bash
  git add apps/web/src/views/AdminCertificatesView.vue apps/web/src/views/AdminCertificatesView.spec.ts \
    apps/web/src/api/admin.ts apps/web/src/permissions.ts apps/web/src/components/layout/AdminNav.vue \
    apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/router/index.ts
  git commit -m "feat(web): add certificate admin"
  ```

---

### Task 6: 文档、全量检查与真实冒烟（CERT-F）

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `docs/primevue-migration.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端/评测/前端/真实冒烟

**Interfaces:**
- Consumes: Task 1–5。
- Produces: 文档、检查记录、真实冒烟证据。

- [ ] **Step 1: 文档**

  - `DEVELOPMENT.md`：§3.7 实现状态（字段/索引/权限 37/本地 storage/图片访问/验证码/快照）；§5.6 实现状态与 API；M5 进度 M5-7 ✅、M5-8 待办；权限表加 `certificate.manage`；前端页面/路由说明。
  - `docs/任务清单.md`：M5-7 ✅；UI-33 ✅；当前验证测试数/权限数；M5-8 待办。
  - `docs/开发日志.md`：2026-11-08 M5-7 条目（变更、文件、实现差异、验证）。
  - `docs/primevue-migration.md`：UI-33 done（三页面的 PrimeVue 实现）。
  - `README.md`：M5-7 完成、证书功能/验证码。
  - `运行手册.md`：证书图片存储目录/备份、5 MiB/格式、公开核验、权限、清理/排障。

- [ ] **Step 2: 全量检查**

  ```bash
  cd /Users/admin/codespace/kapoj/apps/server
  gofmt -l .
  go vet ./...
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./...
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run

  cd /Users/admin/codespace/kapoj/apps/judge
  gofmt -l .
  go vet ./...
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./...

  cd /Users/admin/codespace/kapoj
  corepack pnpm test:web
  corepack pnpm lint:web
  corepack pnpm build:web
  git diff --check
  ```
  Expected: 全部 PASS；lint 0 issues。

- [ ] **Step 3: 真实冒烟**

  - 用测试库/临时 storage 或主库临时数据；临时用户 A/B、类型（或默认类型）、临时 PNG（≤5 MiB）。
  - A 提交 → pending；A 的 `/certificates/my` 可见；A 图片 200；B 图片 404；游客 401。
  - 管理员（`certificate.manage`）审核通过 → approved + verifyCode；`/certificates/verify?code=` 返回类型/等级/A 昵称/日期且无图片/email/userId/rejectReason；大小写/去连字符容错。
  - 第二张证书驳回 → 原因必填；A 可见原因；公开核验 invalid；B 不可见原因。
  - 类型 CRUD：新增/改名/停用/删除；被引用删除 40001；改名后历史证书/核验仍用快照名。
  - 审计 action/target/summary；普通用户 403；游客 401；图片超 5 MiB/非图片/类型停用 40001。
  - 浏览器可用时检查 `/settings/certificates`、`/verify`、`/admin/certificates` 暗/亮/390px；不可用记录并用组件测试覆盖。
  - 清理临时证书/图片/类型/审计/用户；不删除既有数据。

- [ ] **Step 4: 提交**

  ```bash
  git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md docs/primevue-migration.md README.md 运行手册.md
  git commit -m "docs: record certificate authentication"
  ```

---

## Self-Review

**Spec coverage:**
- 类型模型/seed/权限/类型 CRUD → Task 1。
- 提交/我的证书/图片/公开核验 → Task 2。
- 管理员审核/验证码/审计 → Task 3。
- OpenAPI/Blob/我的证书/公开核验页 → Task 4。
- 管理后台审核/类型/导航 → Task 5。
- 文档/全量/冒烟 → Task 6。

**Type consistency:**
- `CertificateType`/`TypeInput`/`TypeView` Task 1；Task 2/3/4 使用。
- `Certificate`/`Status`/`CertificateView`/`PublicCertificateView` Task 2；Task 3/4 使用。
- `AdminFilter`/`AdminCertificateView`/`ReviewInput` Task 3；Task 5 使用。
- `certificate.manage` Task 1 后端，Task 5 前端；权限数 37。
- 验证码 `KAPOJ-XXXX-XXXX`；`CertificateTypeName` 快照贯穿公开核验/历史证书。

**Review Focus mapping:**
1. 图片边界 → Task 2 `TestCertificateImageAccess`/handler；Task 6 smoke。
2. 审核/验证码 → Task 3 tests + public verify。
3. 提交校验/清理 → Task 2 tests。
4. 类型历史 → Task 1/3 tests + smoke。
5. 权限/审计 → Task 1/3 tests + RBAC count。
6. 前端 → Task 4/5 specs + Task 6 browser/manual。
