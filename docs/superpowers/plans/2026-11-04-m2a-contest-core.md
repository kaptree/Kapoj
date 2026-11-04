# M2-A 比赛核心 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 比赛管理员可以创建、修改、删除、克隆比赛并配置题目；用户可按公开/密码/邀请码三种方式报名；比赛详情与公告具备访问控制。

**Architecture:** 新增 `contest` 领域包（模型、校验、仓储、服务、DTO）和 `contestapi` HTTP 层。比赛写操作仅限 `contest_admin` / `super_admin`；读接口按公开性、报名状态和比赛开始时间裁剪。比赛题目通过整体替换写入，报名凭证用 bcrypt 哈希存储。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL、bcrypt；前端类型由 OpenAPI 生成。榜单、比赛提交与前端页面放在 M2-B/M2-C。

**Spec:** `DEVELOPMENT.md` 3.4、4.4、4.1 角色；`docs/任务清单.md` M2-1、M2-2、M2-5。

## Global Constraints

- 比赛写接口需要 `contest_admin` 或 `super_admin`；`problem_admin` 不能管理比赛。
- 比赛状态由服务器时间计算：`upcoming`（未开始）、`running`（进行中）、`ended`（已结束）。
- 非公开比赛对非报名者/非管理员返回 40400；比赛密码/邀请码只存 bcrypt 哈希，绝不返回明文。
- 比赛题目展示顺序由请求数组顺序决定；`displayId` 为单个大写字母 A–Z，同一比赛内唯一；题目不能重复；最多 26 题。
- 比赛题目在开赛前只对管理员可见；报名者在开赛后可看到。
- 报名：`open` 直接报名；`password` 校验密码；`invite` 校验邀请码；比赛结束后拒绝报名；重复报名幂等。
- 删除比赛级联删除题目关联、报名与公告；M2-B 引入比赛提交后再改为有提交时禁止物理删除/仅归档。
- 计 Rating 只存 `is_rated` 标记，M4 才结算。
- 所有文档与错误文案使用中文；日志不记录比赛密码/邀请码。

## Review Focus

- 比赛密码/邀请码是否可能通过响应、日志或克隆泄露。
- 非公开比赛、未报名用户、开赛前题目列表的访问边界。
- 报名并发/重复报名、三种报名方式的错误分支。
- 比赛题目替换的原子性、displayId/problemId 唯一性与开赛时间校验。
- 删除/克隆的级联与数据完整性。

---

### Task 1: 比赛模型、仓储、校验与服务

**Files:**
- Create: `apps/server/internal/contest/model.go`
- Create: `apps/server/internal/contest/status.go`
- Create: `apps/server/internal/contest/validate.go`
- Create: `apps/server/internal/contest/repository.go`
- Create: `apps/server/internal/contest/service.go`
- Create: `apps/server/internal/contest/dto.go`
- Modify: `apps/server/internal/db/db.go`
- Test: `apps/server/internal/contest/service_test.go`
- Test: `apps/server/internal/contest/repository_test.go`

**Interfaces:**
- Produces:
  ```go
  type Mode string
  const ( ModeICPC Mode = "icpc"; ModeIOI Mode = "ioi"; ModeOI Mode = "oi" )
  func (m Mode) Valid() bool

  type RegisterType string
  const ( RegisterOpen RegisterType = "open"; RegisterPassword RegisterType = "password"; RegisterInvite RegisterType = "invite" )
  func (r RegisterType) Valid() bool

  type Status string
  const ( StatusUpcoming Status = "upcoming"; StatusRunning Status = "running"; StatusEnded Status = "ended" )
  func StatusAt(start, end, now time.Time) Status

  type Contest struct {
      ID int64; Title string; DescriptionMD string
      StartTime, EndTime time.Time
      Mode Mode; IsRated, IsPublic bool
      PasswordHash, InviteCodeHash string
      RegisterType RegisterType
      CreatedBy int64; CreatedAt, UpdatedAt time.Time
  }
  type ContestProblem struct {
      ID, ContestID, ProblemID int64
      DisplayID string; SortOrder, Score int
  }
  type ContestParticipant struct {
      ID, ContestID, UserID int64
      RegisterTime time.Time
      ExtraInfo dbtypes.List[string]
  }
  type Announcement struct {
      ID, ContestID int64; Title, ContentMD string
      CreatedBy int64; CreatedAt time.Time
  }

  type ProblemInput struct {
      ProblemID int64  `json:"problemId"`
      DisplayID string `json:"displayId"`
      Score     int    `json:"score"`
  }
  type Input struct {
      Title string `json:"title"`
      DescriptionMD string `json:"descriptionMd"`
      StartTime time.Time `json:"startTime"`
      EndTime time.Time `json:"endTime"`
      Mode Mode `json:"mode"`
      IsRated bool `json:"isRated"`
      IsPublic bool `json:"isPublic"`
      RegisterType RegisterType `json:"registerType"`
      Password string `json:"password"`
      InviteCode string `json:"inviteCode"`
      Problems []ProblemInput `json:"problems"`
  }
  func (in Input) Normalize() (Input, error)

  type Viewer struct{ UserID int64; CanManage bool }
  type ListQuery struct { Keyword string; Mode Mode; Status Status; IncludeHidden bool }
  type Repository interface {
      Create(ctx, c *Contest, problems []ContestProblem) error
      Update(ctx, c *Contest, problems []ContestProblem) error
      FindByID(ctx, id int64) (*Contest, error)
      FindProblems(ctx, contestID int64) ([]ProblemRow, error)
      List(ctx, f ListFilter, offset, limit int) ([]ListRow, int64, error)
      Delete(ctx, id int64) error
      IsParticipant(ctx, contestID, userID int64) (bool, error)
      AddParticipant(ctx, contestID, userID int64) error
      ListParticipants(ctx, contestID int64) ([]ParticipantRow, error)
      CountParticipants(ctx, contestID int64) (int64, error)
  }
  type ProblemRow struct { ContestProblem; ProblemTitle, ProblemSlug string }
  type ListRow struct { Contest; ProblemCount, ParticipantCount int64 }
  type ParticipantRow struct { UserID int64; Username, Nickname string; RegisterTime time.Time }

  type Service struct { repo Repository }
  func NewService(repo Repository) *Service
  func (s *Service) Create(ctx, v Viewer, in Input) (*Contest, error)
  func (s *Service) Update(ctx, v Viewer, id int64, in Input) (*Contest, error)
  func (s *Service) Get(ctx, v Viewer, id int64) (*Contest, error)
  func (s *Service) Problems(ctx, v Viewer, id int64) ([]ProblemRow, error)
  func (s *Service) List(ctx, v Viewer, q ListQuery, page, size int) (httpx.Page[ListItem], error)
  func (s *Service) Delete(ctx, v Viewer, id int64) error
  func (s *Service) Clone(ctx, v Viewer, id int64, in Input) (*Contest, error)
  func (s *Service) Register(ctx, userID, id int64, password, inviteCode string) error
  func (s *Service) Participants(ctx, v Viewer, id int64) ([]ParticipantRow, error)
  func (s *Service) IsAdmin(v Viewer) bool
  ```
- `Input.Normalize`：title trim 1–100；description ≤100000 字符；`start < end`；mode/registerType 合法；problems ≤26；displayId 匹配 `^[A-Z]$` 且唯一；problemId 唯一且 >0；score 1–1000。`password`/`inviteCode` trim，长度 4–64；create 时 `password` 类型必须有密码，`invite` 类型必须有邀请码。
- 状态：`now < start` upcoming；`start <= now < end` running；否则 ended（`end` 时刻算结束）。
- `Create`：`v.CanManage` 否则 `httpx.ErrForbidden`；bcrypt 哈希 `password`/`inviteCode`（空则空串）；写库。
- `Update`：加载原比赛；`password`/`inviteCode` 为空时保留原哈希，非空时重新哈希；整体替换题目。
- `Clone`：加载原比赛与题目；标题默认原标题 + "（副本）"（若 `in.Title` 非空用其）；时间默认原时间，若 `in.StartTime/EndTime` 非零用新值；复制 mode/isRated/isPublic/registerType/哈希与题目；不复制报名/公告；`v.CanManage`。
- `Get`：不存在 404；`!IsPublic` 且非管理员且非报名者 404；返回比赛。`Problems` 单独接口再按开赛时间裁剪。
- `Register`：比赛不存在 404；已结束 40001；已报名直接成功；open 直接；password/invite 校验 bcrypt，错误 40001；并发重复报名由唯一约束兜底（`ON CONFLICT DO NOTHING`）。
- 仓储错误：`ErrNotFound`、`ErrDuplicateProblem`、`ErrDuplicateDisplay`；服务映射 404/40001。
- `db.Migrate` 增加四张表；外键：contest_problems(contest_id) CASCADE、problem_id RESTRICT；participants(contest_id) CASCADE；announcements(contest_id) CASCADE。
- ExtraInfo 使用 `dbtypes.List[string]`，默认 `[]`。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateCloneAndAccess(t *testing.T) {
    // manager 创建公开 ICPC 比赛 + 2 题；
    // 普通用户 GET 可见；非公开比赛非报名者 404；
    // clone 复制题目/mode/时间，不复制报名。
}

func TestRegistrationModes(t *testing.T) {
    // open 直接成功；password 错误 40001 / 正确成功；
    // invite 错误 40001 / 正确成功；重复报名幂等；结束后 40001。
}

func TestValidation(t *testing.T) {
    // end<=start、重复 displayId、重复 problemId、非法 displayId、27 题 -> 40001。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/contest/`
Expected: FAIL（包不存在）。

- [ ] **Step 3: 实现模型、仓储、服务**

按 Interfaces 实现。密码使用 `golang.org/x/crypto/bcrypt`。仓储创建/更新使用事务；更新题目时先删后插，冲突映射为对应错误。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/contest apps/server/internal/db/db.go
git commit -m "feat(server): contest domain, crud and registration"
```

---

### Task 2: 比赛 HTTP API 与 OpenAPI

**Files:**
- Create: `apps/server/internal/contestapi/handler.go`
- Test: `apps/server/internal/contestapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `packages/api/openapi.yaml`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/api/contests.ts`

**Interfaces:**
- 路由（`r` 已带 Bearer 解析）：
  ```go
  GET    /contests                      // 公开列表；includeHidden 仅管理员
  GET    /contests/:id                  // 可选登录
  POST   /contests                      // contest_admin/super_admin
  PUT    /contests/:id                  // contest_admin/super_admin
  DELETE /contests/:id                  // contest_admin/super_admin
  POST   /contests/:id/clone            // contest_admin/super_admin
  POST   /contests/:id/register         // 登录用户
  GET    /contests/:id/participants     // contest_admin/super_admin
  ```
- Viewer：`UserID` 来自 claims，`CanManage` 当角色为 `contest_admin`/`super_admin`。
- DTO：
  ```go
  type ListItem struct {
      ID int64; Title, DescriptionMD string; StartTime, EndTime time.Time
      Mode Mode; IsRated, IsPublic bool; RegisterType RegisterType
      Status Status; ProblemCount, ParticipantCount int64
      CreatedAt time.Time
  }
  type ProblemItem struct { ProblemID int64; Title, Slug, DisplayID string; SortOrder, Score int }
  type Detail struct { ListItem; Problems []ProblemItem; Registered bool; CanManage bool; ServerTime time.Time }
  type ParticipantItem struct { UserID int64; Username, Nickname string; RegisterTime time.Time }
  type AnnouncementItem struct { ID, ContestID int64; Title, ContentMD string; CreatedAt time.Time }
  ```
- `POST /contests` 返回 201；`PUT` 整体替换（含 problems）；`register` 成功返回 `{registered:true}`；错误码 40001/40300/40400。
- OpenAPI 生成 `ContestListItem`、`ContestDetail`、`ContestProblemItem`、`ContestParticipant`、`ContestPageResponse` 等；前端 `contests.ts` 提供 `listContests/getContest/createContest/updateContest/deleteContest/cloneContest/registerContest/listParticipants`。

- [ ] **Step 1: 写失败测试**

```go
func TestContestEndpointsPermissions(t *testing.T) {
    // 游客可看公开列表；普通用户 POST -> 403；contest_admin POST -> 201。
}

func TestContestDetailAndRegistration(t *testing.T) {
    // 非公开比赛非报名者 404；密码报名错误 400；成功后详情 registered=true；
    // 开赛前普通用户 problems 为空，管理员可见。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='...' go test -count=1 ./internal/contestapi/`
Expected: FAIL。

- [ ] **Step 3: 实现接口并生成类型**

```bash
corepack pnpm gen:api
```

- [ ] **Step 4: 运行测试与构建**

Run: `cd apps/server && go test -count=1 -p 1 ./internal/contestapi/ ./internal/router/`
Run: `cd ../.. && corepack pnpm build:web`
Expected: PASS / build 成功。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/contestapi apps/server/internal/router packages/api/openapi.yaml apps/web/src/api
git commit -m "feat(server): contest api"
```

---

### Task 3: 比赛公告

**Files:**
- Modify: `apps/server/internal/contest/`（model 已有 Announcement，补 repository/service/DTO）
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `packages/api/openapi.yaml`
- Test: `apps/server/internal/contest/service_test.go`
- Test: `apps/server/internal/contestapi/handler_test.go`

**Interfaces:**
- 路由：
  ```go
  GET    /contests/:id/announcements
  POST   /contests/:id/announcements       // manager
  PUT    /contests/:id/announcements/:aid  // manager
  DELETE /contests/:id/announcements/:aid  // manager
  ```
- 公告标题 1–100 字符，内容 ≤50000 字符；按 `id DESC` 返回。
- 访问控制与比赛详情一致：公开比赛/报名者/管理员可读，非公开比赛非报名者 404；写仅管理员。

- [ ] **Step 1: 写失败测试**

```go
func TestAnnouncementCRUDPermissions(t *testing.T) {
    // 管理员创建/修改/删除；普通用户写 403；
    // 非公开比赛非报名者读 404；报名后可读；列表倒序。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='...' go test -count=1 ./internal/contest/ ./internal/contestapi/`
Expected: FAIL。

- [ ] **Step 3: 实现公告仓储/服务/接口**

按 Interfaces 实现，确保公告的 contest_id 与路径 id 一致（否则 404）。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/contest apps/server/internal/contestapi packages/api/openapi.yaml
git commit -m "feat(server): contest announcements"
```

---

## Self-Review

**Spec coverage:** 3.4 四张表、4.4 的 CRUD/克隆/三种报名/公告由 Task 1–3 覆盖；榜单、赛时保护、比赛提交、前端页面明确留给 M2-B/M2-C。

**Type consistency:** 模型、DTO、OpenAPI 使用同一组 `icpc/ioi/oi`、`open/password/invite`、`upcoming/running/ended` 字符串；前端类型由 OpenAPI 生成。

**Review Focus mapping:** 凭证泄露 → Task 1/2（不返回哈希、不日志）；访问边界 → Task 1/2/3；并发报名 → Task 1 唯一约束；题目替换原子性 → Task 1 事务；删除/克隆 → Task 1 测试。
