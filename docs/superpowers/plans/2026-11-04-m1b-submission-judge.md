# M1-B 提交与评测服务 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用户可以把 C/C++、Python3、Java、Go 代码提交到公开题目，Asynq 异步排队，独立评测进程在 Docker 沙盒中编译运行，写回六类判题结果并正确更新题目统计。

**Architecture:** Web 服务只负责校验、写 `submissions(status=pending)`（同一事务递增 `submit_count`）并投递 Asynq 任务。`apps/judge` 是独立 Go 模块与进程，消费 `judge` 队列，直接读 PostgreSQL 与共享存储目录，用 Docker CLI 以“复制文件进容器”的方式编译和运行，不依赖宿主机路径挂载，因此 judge 本身也可以跑在容器里。每个测试点使用独立容器，先编译一次、再逐点运行；系统错误交给 Asynq 重试，重试耗尽写 `system_error`。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis + `github.com/hibiken/asynq`、Docker CLI、本地文件存储（`storage.Storage` 约定与 M1-A 相同）。

**Spec:** `DEVELOPMENT.md` 的 2.3、2.4、3.3、4.2、9；`docs/任务清单.md` 的 M1-5 至 M1-7。

## Global Constraints

- 不得更换后端技术栈：Go + Gin + GORM + PostgreSQL 17 + Redis（Asynq）+ Docker 沙盒。
- 所有文档、日志面向用户的文案使用中文；代码标识符使用英文。
- 判题状态值严格为：`pending`、`judging`、`accepted`、`wrong_answer`、`time_limit_exceeded`、`memory_limit_exceeded`、`runtime_error`、`compile_error`、`system_error`。
- 支持语言：`c`、`cpp`、`python3`、`java`、`go`。M1 不评测 `interactive` 题目，提交时直接拒绝。
- 新提交必须写 `pending`；创建提交与 `problems.submit_count + 1` 在同一事务；只有提交从 `judging` 转为 `accepted` 时 `accept_count + 1`，且只加一次。
- 系统错误（Docker 不可用、存储文件缺失、checker 编译失败、DB 失败）走 Asynq 重试，最多 3 次；耗尽后写 `system_error`。用户代码导致的 CE/WA/TLE/MLE/RE 不重试。
- 日志不得包含源代码、测试数据、程序输出或 checker 内容。
- 题目测试数据与 checker 由 Web 服务与 judge 共享同一个 `KAPOJ_STORAGE_DIR`；容器模式下使用同一个 `storage` 卷。
- 单次提交代码上限 64 KiB；运行输出上限 1 MiB；编译超时 30 秒；提交冷却 3 秒/用户。
- 特判 checker 约定：argv 依次为 `<checker> <输入文件> <选手输出> <标准答案>`，退出码 0 为通过，非 0 为答案错误；checker 超时或无法运行属于系统错误。

## Review Focus

- 未公开题目、交互题、缺少测试点或缺少 checker 的题目不能被评测，也不能通过错误信息泄露题目存在性。
- TLE/MLE/手机关闭等异常必须真正停掉并删除容器，不能留下孤儿容器或僵尸进程。
- Asynq 重试和重复任务不能重复累加 `accept_count`，也不能覆盖已经终态的提交。
- 特判题的 checker 结论必须优先于文本比对；checker 自身故障不能被误判成选手 WA 或 CE。
- 输出超限、行尾空白、CRLF、空输出等边界必须按约定得到 WA/AC，而不是崩溃或误判。

---

### Task 1: 提交领域模型、仓储与服务

**Files:**
- Create: `apps/server/internal/submission/status.go`
- Create: `apps/server/internal/submission/model.go`
- Create: `apps/server/internal/submission/validate.go`
- Create: `apps/server/internal/submission/repository.go`
- Create: `apps/server/internal/submission/service.go`
- Create: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/db/db.go`
- Test: `apps/server/internal/submission/status_test.go`
- Test: `apps/server/internal/submission/validate_test.go`
- Test: `apps/server/internal/submission/repository_test.go`
- Test: `apps/server/internal/submission/service_test.go`

**Interfaces:**
- Consumes: `problem.Repository` 的 `FindByID(ctx, id) (*problem.Problem, error)`、`CountTestcases(ctx, problemID) (int, error)`；`httpx.AppError` 与 `httpx.ErrNotFound`。
- Produces:
  ```go
  type Status string
  const (
      StatusPending Status = "pending"
      StatusJudging Status = "judging"
      StatusAccepted Status = "accepted"
      StatusWrongAnswer Status = "wrong_answer"
      StatusTimeLimitExceeded Status = "time_limit_exceeded"
      StatusMemoryLimitExceeded Status = "memory_limit_exceeded"
      StatusRuntimeError Status = "runtime_error"
      StatusCompileError Status = "compile_error"
      StatusSystemError Status = "system_error"
  )
  func (s Status) Terminal() bool

  type Language string
  const (
      LanguageC Language = "c"
      LanguageCPP Language = "cpp"
      LanguagePython3 Language = "python3"
      LanguageJava Language = "java"
      LanguageGo Language = "go"
  )

  type Input struct {
      ProblemID int64    `json:"problemId"`
      Language  Language `json:"language"`
      Code      string   `json:"code"`
  }
  func (in Input) Normalize() (Input, error)

  type JudgeCaseResult struct {
      TestcaseID int64  `json:"testcaseId"`
      Status     Status `json:"status"`
      TimeMS     int    `json:"timeMs"`
      MemoryKB   int    `json:"memoryKb"`
      Message    string `json:"message,omitempty"`
  }

  type Submission struct {
      ID          int64                       `gorm:"primaryKey"`
      UserID      int64                       `gorm:"not null;index:idx_submissions_user_created,priority:1"`
      ProblemID   int64                       `gorm:"not null;index:idx_submissions_problem_status,priority:1"`
      ContestID   *int64
      Language    Language                    `gorm:"size:20;not null"`
      Code        string                      `gorm:"type:text;not null"`
      Status      Status                      `gorm:"size:30;not null;default:pending;index:idx_submissions_problem_status,priority:2"`
      TimeUsedMS  int                         `gorm:"column:time_used_ms;not null;default:0"`
      MemoryUsedKB int                        `gorm:"column:memory_used_kb;not null;default:0"`
      Score       int                         `gorm:"not null;default:0"`
      ErrorMessage string                     `gorm:"type:text;not null;default:''"`
      JudgeResult dbtypes.List[JudgeCaseResult] `gorm:"column:judge_result_json;type:jsonb;not null;default:'[]'"`
      CreatedAt   time.Time                   `gorm:"type:timestamptz;not null;index:idx_submissions_user_created,priority:2"`
      JudgedAt    *time.Time                  `gorm:"type:timestamptz"`
  }

  type Viewer struct{ CanManage bool }

  type ProblemLookup interface {
      FindByID(ctx context.Context, id int64) (*problem.Problem, error)
      CountTestcases(ctx context.Context, problemID int64) (int, error)
  }
  type Enqueuer interface {
      Enqueue(ctx context.Context, submissionID int64) error
  }
  type Limiter interface {
      Allow(ctx context.Context, userID int64) (bool, error)
  }

  type Repository interface {
      Create(ctx context.Context, sub *Submission) error
      FindByID(ctx context.Context, id int64) (*Submission, error)
      MarkSystemError(ctx context.Context, id int64, message string) error
  }

  type Service struct { /* repo, problems, queue, limiter */ }
  func NewService(repo Repository, problems ProblemLookup, queue Enqueuer, limiter Limiter) *Service
  func (s *Service) Create(ctx context.Context, v Viewer, userID int64, in Input) (*Submission, error)

  type Created struct {
      ID     int64  `json:"id"`
      Status Status `json:"status"`
  }
  func ToCreated(sub *Submission) Created
  ```

**Behavior (exact):**
- `Input.Normalize`：`ProblemID > 0`；`Language` 必须是五个值之一；`Code` 去除首尾空白后不能为空，按字节数不超过 `65536`；保留代码原文（不 trim）。
- `Service.Create` 顺序：
  1. `in.Normalize()`，失败返回 `httpx.ErrValidation`。
  2. `problems.FindByID`；`problem.ErrNotFound` 映射 `httpx.ErrNotFound`。
  3. `!p.IsPublic && !v.CanManage` → `httpx.ErrNotFound`。
  4. `p.Type == problem.TypeInteractive` → `httpx.ErrValidation("交互题暂不支持评测")`。
  5. `p.Type == problem.TypeSpecial && p.CheckerPath == ""` → `httpx.ErrValidation("题目缺少 checker，暂不能提交")`。
  6. `CountTestcases == 0` → `httpx.ErrValidation("题目还没有测试点")`。
  7. `limiter.Allow`；false → `httpx.NewError(42902, http.StatusTooManyRequests, "提交过于频繁，请稍后再试")`。
  8. 创建 `Submission{UserID, ProblemID, Language, Code, Status: StatusPending, JudgeResult: dbtypes.List[JudgeCaseResult]{}}`。
  9. `repo.Create`（事务插入 + `UPDATE problems SET submit_count = submit_count + 1`）。
  10. `queue.Enqueue`；失败时 `repo.MarkSystemError(sub.ID, "任务入队失败")` 并返回底层错误（HTTP 500）。
- `Repository.Create` 使用 GORM 事务；题目行不存在时返回 `problem.ErrNotFound`（用 FK 错误或先查）。
- `Repository.MarkSystemError` 只更新 `status IN ('pending','judging')`，写 `error_message`、`judged_at=now()`，不碰 `accept_count`。
- `db.Migrate` 增加 `&submission.Submission{}`。
- 索引名与 3.3 节一致：`idx_submissions_problem_status`、`idx_submissions_user_created`。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateRejectsHiddenInteractiveAndEmptyProblem(t *testing.T) {
    // 用真实 PostgreSQL + 真实 problem.Repository 建题；
    // hidden 题目对普通用户 40400，对 manager 允许创建提交；
    // interactive 题目返回 40001；没有测试点的题目返回 40001。
}

func TestCreateIncrementsSubmitCountAndEnqueues(t *testing.T) {
    // 创建公开题 + 1 个测试点；Create 后：
    // submissions.status == pending；problem.submit_count == 1；
    // fake enqueuer 收到同一个 submission id。
}

func TestCreateIsRateLimitedBeforeInsert(t *testing.T) {
    // fake limiter 返回 false；Create 返回 42902；
    // submissions 表仍为空，enqueuer 未被调用。
}

func TestCreateEnqueueFailureMarksSystemError(t *testing.T) {
    // fake enqueuer 返回错误；Create 返回错误；
    // 该提交 status == system_error，submit_count 仍为 1。
}

func TestCodeSizeLimit(t *testing.T) {
    // 65536 字节通过；65537 字节返回 40001。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/submission/`
Expected: FAIL（包或类型不存在）。

- [ ] **Step 3: 实现模型、校验、仓储与服务**

按上面的 Interfaces 和 Behavior 实现。`Repository.Create` 的事务里先 `tx.Create(sub)`，再 `tx.Model(&problem.Problem{}).Where("id = ?", sub.ProblemID).UpdateColumn("submit_count", gorm.Expr("submit_count + 1"))`，并检查 `RowsAffected`。错误映射：`problem.ErrNotFound` → `httpx.ErrNotFound`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/submission/ ./internal/problem/`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/submission apps/server/internal/db/db.go
git commit -m "feat(server): submission domain and creation service"
```

---

### Task 2: 提交 HTTP 接口、Asynq 入队与 Redis 限流

**Files:**
- Create: `apps/server/internal/judgequeue/client.go`
- Create: `apps/server/internal/submission/limiter.go`
- Create: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `packages/api/openapi.yaml`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/api/submissions.ts`
- Test: `apps/server/internal/judgequeue/client_test.go`
- Test: `apps/server/internal/submission/limiter_test.go`
- Test: `apps/server/internal/submissionapi/handler_test.go`

**Interfaces:**
- Consumes: Task 1 的 `submission.Service`、`submission.Enqueuer`、`submission.Limiter`。
- Produces:
  ```go
  const TypeSubmission = "judge:submission"
  const QueueName = "judge"
  type Payload struct { SubmissionID int64 `json:"submissionId"` }

  type Client struct { /* *asynq.Client */ }
  func NewClient(redisAddr string) *Client
  func (c *Client) Close() error
  func (c *Client) Enqueue(ctx context.Context, submissionID int64) error

  type RedisLimiter struct { /* rdb *redis.Client; cooldown time.Duration */ }
  func NewRedisLimiter(rdb *redis.Client, cooldown time.Duration) *RedisLimiter
  func (l *RedisLimiter) Allow(ctx context.Context, userID int64) (bool, error)

  func RegisterRoutes(r gin.IRouter, s *submission.Service)
  ```
- `Client.Enqueue`：`asynq.NewTask(TypeSubmission, json.Marshal(Payload{...}), asynq.Queue(QueueName), asynq.MaxRetry(3))`，再 `EnqueueContext`。任务 payload 只用 `submissionId`。
- `RedisLimiter.Allow`：`SET kapoj:submit:cool:<userID> 1 NX EX <cooldown 秒>`；成功 true，`redis.Nil` false，其他错误返回 error。cooldown 小于 1 秒时按 1 秒处理。
- `Config` 增加 `SubmitCooldown time.Duration`，环境变量 `KAPOJ_SUBMIT_COOLDOWN`，默认 `3s`；必须大于 0。
- `router.Deps` 增加 `Enqueuer submission.Enqueuer`；`RegisterRoutes` 在 `api` 组注册 `POST /submissions`，需要登录（`middleware.RequireAuth()`），不需要管理员。
- `main`：创建 `judgequeue.NewClient(cfg.RedisAddr)`，`defer Close`，把它和 `submission.NewRedisLimiter(rdb, cfg.SubmitCooldown)` 传给 router。
- OpenAPI：`POST /submissions`，请求 `{problemId, language, code}`，201 返回 `{id, status}`；错误码包含 400、401、404、413、429；`42902` 写入 2.4 错误码表。生成的前端类型新增 `SubmissionCreated`、`SubmissionLanguage`，`apps/web/src/api/submissions.ts` 提供 `createSubmission(input)`，M1-C 直接复用。
- 路由体上限沿用 1 MiB（代码上限 64 KiB）。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateSubmissionEndpoint(t *testing.T) {
    // 登录用户 POST /api/submissions -> 201, status=pending,
    // 数据库 submit_count=1, fake enqueuer 收到 id。
}

func TestCreateSubmissionEndpointErrors(t *testing.T) {
    // 游客 -> 401；hidden 题目 -> 404；interactive -> 400；不存在 -> 404。
}

func TestCreateSubmissionRateLimit(t *testing.T) {
    // 同一用户连续两次；第一次 201，第二次 429 + code 42902；
    // 不同用户不受影响。
}

func TestQueuePayloadContract(t *testing.T) {
    // 断言 TypeSubmission == "judge:submission"，
    // 并且 payload JSON 只含 {"submissionId": <id>}。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/submissionapi/ ./internal/judgequeue/ ./internal/submission/`
Expected: FAIL（接口不存在）。

- [ ] **Step 3: 实现接口、队列、限流与装配**

按 Interfaces 实现。`handler.create` 从 `middleware.CurrentClaims` 取用户 id，调用 `submission.Service.Create`，成功后 `httpx.Created(c, submission.ToCreated(sub))`。测试用的 router 装配传 fake enqueuer；真实 `main` 使用 Asynq client。

- [ ] **Step 4: 更新 OpenAPI 并重新生成前端类型**

```bash
corepack pnpm gen:api
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
cd ../.. && corepack pnpm test:web && corepack pnpm build:web
```
Expected: 全部 PASS / build 成功。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/judgequeue apps/server/internal/submission/limiter.go apps/server/internal/submissionapi apps/server/internal/router apps/server/internal/config apps/server/cmd/server apps/web/src/api packages/api/openapi.yaml
git commit -m "feat(server): submission api, asynq queue and rate limit"
```

---

### Task 3: 评测模块骨架、数据库访问、存储读取与结果比对

**Files:**
- Create: `apps/judge/go.mod`
- Create: `apps/judge/internal/config/config.go`
- Create: `apps/judge/internal/model/model.go`
- Create: `apps/judge/internal/db/repository.go`
- Create: `apps/judge/internal/storage/reader.go`
- Create: `apps/judge/internal/verdict/verdict.go`
- Create: `apps/judge/internal/queue/contract.go`
- Test: `apps/judge/internal/config/config_test.go`
- Test: `apps/judge/internal/storage/reader_test.go`
- Test: `apps/judge/internal/verdict/verdict_test.go`
- Test: `apps/judge/internal/db/repository_test.go`

**Interfaces:**
- Produces:
  ```go
  // config
  type Config struct {
      DatabaseURL string
      RedisAddr   string
      StorageDir  string
      Concurrency int
      DockerBin   string
      Images      map[model.Language]string
  }
  func Load() (*Config, error)

  // model
  type Language string
  const ( LanguageC Language = "c"; ... )
  type Status string
  const ( StatusPending Status = "pending"; ... )
  type Submission struct {
      ID int64; UserID int64; ProblemID int64
      Language Language; Code string; Status Status
  }
  type Problem struct {
      ID int64; Type string; TimeLimitMS int; MemoryLimitMB int
      CheckerPath string
  }
  type Testcase struct {
      ID int64; InputFile string; OutputFile string; Score int; SortOrder int
  }
  type JudgeCaseResult struct {
      TestcaseID int64 `json:"testcaseId"`
      Status Status `json:"status"`
      TimeMS int `json:"timeMs"`
      MemoryKB int `json:"memoryKb"`
      Message string `json:"message,omitempty"`
  }

  // db
  var ErrSkip = errors.New("submission is not claimable")
  type FinishInput struct {
      Status Status
      TimeUsedMS int
      MemoryUsedKB int
      Score int
      ErrorMessage string
      Cases []model.JudgeCaseResult
  }
  type Repository struct { /* *pgxpool.Pool */ }
  func Open(ctx context.Context, dsn string) (*Repository, error)
  func (r *Repository) Close()
  func (r *Repository) Claim(ctx context.Context, id int64) (*model.Submission, *model.Problem, []model.Testcase, error)
  func (r *Repository) Finish(ctx context.Context, id int64, in FinishInput) error
  func (r *Repository) MarkSystemError(ctx context.Context, id int64, message string) error
  func (r *Repository) FindByID(ctx context.Context, id int64) (*model.Submission, error)

  // storage
  func Open(root, key string) (io.ReadCloser, error) // 与 Local.resolve 相同规则

  // verdict
  func Compare(output, answer []byte) bool // 忽略每行行尾空白与末尾空行，统一 CRLF
  func Aggregate(cases []model.JudgeCaseResult) (status model.Status, timeMS, memoryKB, score int)
  ```
- `Aggregate` 规则：没有 case → `system_error`；任一 case 非 `accepted` → 第一个非 AC 的 status；全部 AC → `accepted`；`timeMS`/`memoryKB` 取最大值；`score` 为 AC case 的分数之和（pipeline 只传已判 case）。
- `db.Claim`：
  ```sql
  UPDATE submissions SET status='judging'
  WHERE id=$1 AND status IN ('pending','judging')
  RETURNING id,user_id,problem_id,language,code,status
  ```
  无行时查一次状态：终态或不存在返回 `ErrSkip`，其他错误原样返回。随后加载 problem 与按 `sort_order` 排序的 testcases。
- `db.Finish` 在一个事务中：
  - `UPDATE submissions SET status=$, time_used_ms=$, memory_used_kb=$, score=$, error_message=$, judge_result_json=$, judged_at=now() WHERE id=$ AND status='judging'`
  - 只有影响行数为 1 且 `in.Status == StatusAccepted` 时执行 `UPDATE problems SET accept_count = accept_count + 1 WHERE id=$problemID`。
  - 影响行数为 0 时静默返回 nil（重复/终态任务）。
- `db.MarkSystemError`：只更新 `status IN ('pending','judging')`，写 `error_message`、`judged_at=now()`。
- 队列契约与 Web 服务一致：`TypeSubmission = "judge:submission"`、`QueueName = "judge"`、payload 字段 `submissionId`。
- 配置默认值：Redis `localhost:6379`、storage `data`、concurrency `2`、docker `docker`；镜像默认 `gcc:14`、`python:3.12-slim`、`eclipse-temurin:21-jdk`、`golang:1.26`。`KAPOJ_DATABASE_URL` 必填，concurrency 1–16。

- [ ] **Step 1: 写失败测试**

```go
func TestCompareIgnoresLineEndWhitespace(t *testing.T) {
    // "1 2\r\n3\r\n" 与 "1 2\n3\n\n" 相等；
    // "1 2\n4" 不等；空输出与 " \n" 相等。
}

func TestAggregate(t *testing.T) {
    // all accepted -> accepted, max time/memory, score sum；
    // [accepted, wrong_answer, ...] -> wrong_answer；空 -> system_error。
}

func TestClaimAndFinishAreIdempotent(t *testing.T) {
    // 插入 pending；Claim -> judging；Finish(accepted) -> accept_count=1；
    // 再次 Finish(accepted) 不改变 accept_count；Claim 已终态 -> ErrSkip。
}

func TestMarkSystemError(t *testing.T) {
    // pending/judging -> system_error；已 accepted 不变。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/...`
Expected: FAIL（模块/类型不存在）。

- [ ] **Step 3: 创建模块并实现**

```bash
cd apps/judge
go mod init github.com/kaptree/kapoj/apps/judge
go get github.com/hibiken/asynq github.com/jackc/pgx/v5 github.com/stretchr/testify
```
测试数据库辅助代码可以用 `CREATE TABLE IF NOT EXISTS` 建 `problems`、`problem_testcases`、`submissions` 三张表（只建测试所需列），并 `TRUNCATE ... RESTART IDENTITY CASCADE`；真实迁移由 Web 服务的 `db.Migrate` 负责。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/...`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/judge
git commit -m "feat(judge): data layer, storage reader and result comparison"
```

---

### Task 4: 判题流水线、Asynq worker 与进程入口（假沙盒）

**Files:**
- Create: `apps/judge/internal/sandbox/sandbox.go`（接口与结果类型）
- Create: `apps/judge/internal/pipeline/pipeline.go`
- Create: `apps/judge/internal/worker/worker.go`
- Create: `apps/judge/cmd/judge/main.go`
- Test: `apps/judge/internal/pipeline/pipeline_test.go`
- Test: `apps/judge/internal/worker/worker_test.go`

**Interfaces:**
- Consumes: Task 3 的 `db.Repository`、`storage.Open`、`verdict`。
- Produces:
  ```go
  // sandbox
  type CompileRequest struct {
      Language model.Language
      Source   []byte
      Checker  bool
  }
  type CompileResult struct {
      ArtifactDir string
      ErrorOutput string
      Failed      bool
  }
  type CheckerFiles struct {
      AnswerPath string
      OutputPath string
  }
  type RunRequest struct {
      Language         model.Language
      ArtifactDir      string
      InputPath        string
      Checker          *CheckerFiles
      TimeLimitMS      int
      MemoryLimitMB    int
      OutputLimitBytes int
  }
  type RunResult struct {
      Output          []byte
      Stderr          []byte
      ExitCode        int
      TimedOut        bool
      OOMKilled       bool
      TimeMS          int
      MemoryKB        int
      OutputTruncated bool
  }
  type Sandbox interface {
      Compile(ctx context.Context, req CompileRequest) (CompileResult, error)
      Run(ctx context.Context, req RunRequest) (RunResult, error)
  }

  // pipeline
  type Pipeline struct { /* repo, store root, sandbox */ }
  func New(repo *db.Repository, storageRoot string, sb sandbox.Sandbox) *Pipeline
  func (p *Pipeline) Run(ctx context.Context, submissionID int64) error
  func (p *Pipeline) MarkSystemError(ctx context.Context, submissionID int64, cause error)

  // worker
  type Worker struct { /* pipeline */ }
  func New(p *Pipeline) *Worker
  func (w *Worker) Handle(ctx context.Context, t *asynq.Task) error
  ```
- `Pipeline.Run`：
  1. `repo.Claim`；`ErrSkip` 直接返回 nil。
  2. 从 storage 读取提交代码不需要；代码已在 `Submission.Code`。
  3. `sandbox.Compile`（`Checker:false`）；`Failed` → `repo.Finish` 写 `compile_error`、`error_message=ErrorOutput`（截断 64 KiB）、cases 为空，返回 nil。
  4. 若题目 `type == "special"`：读取 `CheckerPath`，`sandbox.Compile(Checker:true)`；失败或读不到 → 返回系统错误（不写终态，交给重试）。
  5. 逐测试点：
     - 用 `storage.Open` 打开输入/标准输出，写到临时文件（输入给 sandbox，输出读入内存，最多 16 MiB）。
     - `sandbox.Run`；`TimedOut` 或 `TimeMS > TimeLimitMS` → `time_limit_exceeded`；`OOMKilled` → `memory_limit_exceeded`；`ExitCode != 0` → `runtime_error`；`OutputTruncated` → `wrong_answer`（消息「输出超过限制」）。
     - 普通题：`verdict.Compare(Output, Answer)`，不等 → `wrong_answer`。
     - 特判题：把选手输出与标准答案写临时文件，`sandbox.Run` 带 `Checker` 字段、5 秒/256 MB；checker 超时 → 返回系统错误；退出非 0 → `wrong_answer`（消息为 checker 输出的前 4 KiB）；退出 0 → `accepted`。
     - 记录 `JudgeCaseResult`（testcase id、status、time、memory、message）；遇到第一个非 AC 停止后续测试点。
  6. `verdict.Aggregate`；`repo.Finish`；返回 nil。DB 写入失败返回错误（重试）。
- `Worker.Handle`：解析 payload；JSON 非法记录 warning 并返回 nil。`Run` 返回错误时：`asynq.GetRetryCount(ctx) >= asynq.GetMaxRetry(ctx)` → `pipeline.MarkSystemError` 并返回 nil；否则返回 error 触发重试。
- `cmd/judge/main.go`：加载配置、打开 DB、构造 Asynq server（`asynq.Config{Concurrency: cfg.Concurrency, Queues: map[string]int{"judge": 1}}`）、启动、监听 SIGINT/SIGTERM 并 `Shutdown`。日志只包含 submission id、language、status，不包含代码或输出。

- [ ] **Step 1: 写失败测试（fake sandbox + 真实 PostgreSQL）**

```go
func TestPipelineAccepted(t *testing.T) {
    // 2 个测试点，fake sandbox 都返回 "3\n" 且 Compare 通过；
    // 提交 accepted，score=20，accept_count +1，judge_result_json 有 2 项。
}

func TestPipelineMapsUserVerdicts(t *testing.T) {
    // 分别构造 TimedOut / OOMKilled / ExitCode=1 / 输出超限 / 输出不等；
    // 状态依次为 TLE / MLE / RE / WA / WA。
}

func TestPipelineCompileError(t *testing.T) {
    // Compile Failed -> compile_error，error_message 含编译器输出，accept_count 不变。
}

func TestPipelineSpecialChecker(t *testing.T) {
    // checker exit 0 -> accepted；exit 1 -> wrong_answer；
    // checker 编译失败 -> Run 返回错误（系统错误），状态仍为 judging。
}

func TestWorkerFinalAttemptMarksSystemError(t *testing.T) {
    // fake sandbox 返回基础设施错误；
    // retryCount < maxRetry -> Handle 返回 error；
    // retryCount == maxRetry -> 返回 nil 且 status=system_error。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/pipeline/ ./internal/worker/`
Expected: FAIL。

- [ ] **Step 3: 实现流水线、worker 与 main**

严格按上面的判定顺序：超时/内存/退出码优先于输出比对；特判题用 checker 结论；任何基础设施错误都返回 error 且不写终态。时间、内存取所有已判 case 的最大值。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./... && go vet ./... && gofmt -l .`
Expected: PASS，`gofmt -l` 无输出。

- [ ] **Step 5: 提交**

```bash
git add apps/judge
git commit -m "feat(judge): verdict pipeline and asynq worker"
```

---

### Task 5: Docker 沙盒与五种语言

**Files:**
- Create: `apps/judge/internal/sandbox/docker.go`
- Create: `apps/judge/internal/sandbox/language.go`
- Create: `apps/judge/internal/sandbox/docker_test.go`
- Test: `apps/judge/internal/sandbox/language_test.go`

**Interfaces:**
- Consumes: Task 4 的 `sandbox.Sandbox` 接口。
- Produces:
  ```go
  type Docker struct { /* bin string; images map[model.Language]string; tmpRoot string */ }
  func NewDocker(bin string, images map[model.Language]string) *Docker
  func (d *Docker) Compile(ctx context.Context, req CompileRequest) (CompileResult, error)
  func (d *Docker) Run(ctx context.Context, req RunRequest) (RunResult, error)
  ```
- 语言命令表：
  | language | 镜像 | 源文件 | 编译命令 | 运行命令 |
  |---|---|---|---|---|
  | `c` | gcc:14 | `/work/main.c` | `gcc -O2 -std=c17 -o /tmp/main /work/main.c` | `/work/main` |
  | `cpp` | gcc:14 | `/work/main.cpp` | `g++ -O2 -std=c++17 -o /tmp/main /work/main.cpp` | `/work/main` |
  | `python3` | python:3.12-slim | `/work/main.py` | `python3 -m py_compile /work/main.py`（`PYTHONPYCACHEPREFIX=/tmp/pycache`） | `python3 /work/main.py` |
  | `java` | eclipse-temurin:21-jdk | `/work/Main.java` | `javac -encoding UTF-8 -d /tmp/classes /work/Main.java` | `java -Xmx<max(16,mem-64)>m -cp /work/classes Main` |
  | `go` | golang:1.26 | `/work/main.go` | `go build -o /tmp/main /work/main.go`（`GO111MODULE=off GOCACHE=/tmp/gocache GOPATH=/tmp/gopath CGO_ENABLED=0`） | `/work/main` |
  | checker | gcc:14 | `/work/checker.cpp` | `g++ -O2 -std=c++17 -o /tmp/checker /work/checker.cpp` | `/work/checker /input.txt /work/output.txt /work/answer.txt` |
- 编译容器参数：`--network none --cpus 1 --memory 1g --pids-limit 128 --cap-drop ALL --security-opt no-new-privileges --read-only --tmpfs /tmp:rw,exec,nosuid,size=512m --workdir /work --label kapoj.judge=1`。
- 运行容器参数：`--network none --cpus 1 --memory <M>m --memory-swap <M>m --pids-limit 64 --cap-drop ALL --security-opt no-new-privileges --read-only --tmpfs /out:rw,noexec,nosuid,size=8m,mode=1777 --tmpfs /tmp:rw,noexec,nosuid,size=64m,mode=1777 --workdir /work --label kapoj.judge=1`。
- 不使用宿主机 bind mount：用 `docker create` + `docker cp` 把源码/产物/输入复制进容器，`docker start -a` 启动，`docker inspect` 读取退出码与 `OOMKilled`，`docker cp` 取回 `/out`，最后 `docker rm -f`。这样 judge 在容器内运行时也能工作。
- 容器内 wrapper `/work/run.sh` 负责：
  - `ulimit -f 4096`（限制输出文件约 2 MiB）；
  - 记录 `date +%s%N` 前后的差值到 `/out/time_ms`；
  - 运行程序并重定向 `/input.txt`、`/out/stdout`、`/out/stderr`；
  - 读取 `/sys/fs/cgroup/memory.peak` 并转 KiB 写入 `/out/memory_kb`，不可用时写 0；
  - 原样返回程序退出码。
- 超时：context 截止时间 = `TimeLimitMS + 3000ms`；超时后 `docker kill` + `docker rm -f`，返回 `TimedOut:true`。若 `time_ms > TimeLimitMS` 也视为 TLE（由 pipeline 判定）。
- 输出：读取 `/out/stdout` 时最多读 `OutputLimitBytes+1` 字节；超过则 `OutputTruncated=true` 并只返回前 `OutputLimitBytes` 字节。
- 编译失败（容器退出非 0）返回 `CompileResult{Failed:true, ErrorOutput: <docker logs 前 64 KiB>}`；Docker 命令本身失败返回 error（系统错误）。
- 所有容器名格式 `kapoj-judge-<submission>-<random>`，`defer` 清理；测试断言没有残留 `kapoj.judge=1` 容器。

- [ ] **Step 1: 写失败测试**

```go
func TestLanguageCommands(t *testing.T) {
    // 断言五种语言的源文件名、编译命令、运行命令；
    // Java 的 -Xmx 随内存限制变化，最小 16m。
}

func TestDockerSandboxVerdicts(t *testing.T) {
    // 需要 KAPOJ_DOCKER_TESTS=1 且 docker 可用，否则 t.Skip。
    // C++: AC / WA / TLE / MLE / RE / CE 六种各跑一次；
    // 每次结束断言没有 kapoj.judge=1 的残留容器。
}

func TestDockerSandboxLanguages(t *testing.T) {
    // 需要 KAPOJ_DOCKER_TESTS=1；Python3、Java、Go 各跑一个 A+B 的 AC。
}

func TestDockerSandboxOutputLimit(t *testing.T) {
    // C++ 输出 2 MiB，OutputTruncated=true 且返回长度不超过限制。
}
```

- [ ] **Step 2: 运行单元测试确认失败**

Run: `cd apps/judge && go test -count=1 ./internal/sandbox/`
Expected: FAIL（Docker 类型不存在）。

- [ ] **Step 3: 实现 Docker 沙盒与语言表**

按 Interfaces 实现。编译产物复制到宿主机临时目录：普通二进制 `docker cp <compile>:/tmp/main <tmp>/main`；Java 目录 `docker cp <compile>:/tmp/classes/. <tmp>/classes`。运行容器创建后，把产物复制到 `/work`（Java 到 `/work/classes`），把输入复制到 `/input.txt`，再启动。checker 运行还要复制 `/work/output.txt` 与 `/work/answer.txt`。

- [ ] **Step 4: 运行沙盒测试**

```bash
cd apps/judge
KAPOJ_DOCKER_TESTS=1 go test -count=1 -v ./internal/sandbox/
```
Expected: 六种 C++ 结论与 Python3/Java/Go AC 全部 PASS；Docker 不可用时测试 SKIP。

- [ ] **Step 5: 提交**

```bash
git add apps/judge/internal/sandbox
git commit -m "feat(judge): docker sandbox with five languages"
```

---

### Task 6: 特判验收、容器化、文档与端到端验收

**Files:**
- Create: `apps/judge/Dockerfile`
- Create: `apps/judge/.dockerignore`
- Modify: `docker/docker-compose.yml`
- Modify: `.env.example`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Test: `apps/judge/internal/pipeline/checker_test.go`（若 Task 4 未覆盖完整 checker 语义）
- Test: `apps/server/internal/submissionapi/handler_test.go`（如 Task 2 有遗漏）

**Interfaces:**
- Consumes: 前面全部任务。
- Produces:
  - `docker compose --profile app up -d --build` 同时启动 `server` 与 `judge`；judge 挂载 `/var/run/docker.sock` 与共享 `storage:/data`。
  - 运行手册新增：拉取评测镜像、启动 judge、提交冒烟、查看队列积压、清理孤儿容器。
  - DEVELOPMENT.md 3.3 补实现差异；4.2 补实现状态；2.4 补错误码 42902；2.3 当前状态更新；9 节 M1 标记 🚧；任务清单 M1-5/6/7 标记 ✅；开发日志追加 M1-B 条目。
- `apps/judge/Dockerfile`：
  ```dockerfile
  FROM golang:1.26 AS build
  WORKDIR /src
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/judge ./cmd/judge

  FROM docker:cli
  COPY --from=build /out/judge /judge
  ENTRYPOINT ["/judge"]
  ```
- compose `judge`：
  ```yaml
  judge:
    profiles: ["app"]
    build:
      context: ../apps/judge
    environment:
      KAPOJ_DATABASE_URL: postgres://kapoj:kapoj@postgres:5432/kapoj?sslmode=disable
      KAPOJ_REDIS_ADDR: redis:6379
      KAPOJ_STORAGE_DIR: /data
      KAPOJ_JUDGE_DOCKER: docker
    volumes:
      - storage:/data
      - /var/run/docker.sock:/var/run/docker.sock
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
  ```

**端到端验收（必须逐项通过）：**
1. 启动 PostgreSQL、Redis、server、judge；`docker pull` 五个镜像。
2. 管理员创建公开题 + 2 个测试点；普通用户登录。
3. 提交 C++ AC → 轮询数据库 `accepted`，`submit_count=1`、`accept_count=1`，`judge_result_json` 有 2 项。
4. 提交 WA、CE、TLE、MLE、RE → 状态分别正确，且每个都在 10 秒内结束。
5. 提交 Python3、Java、Go 的 AC → 均 `accepted`。
6. 特判题：checker 返回 0 → AC；checker 返回 1 → WA；checker 编译失败 → `system_error`。
7. 同一提交被重复投递/重试 → `accept_count` 不再增加。
8. 隐藏题、交互题、无测试点题 → 提交接口分别返回 404/400/400。
9. 运行结束后 `docker ps -a --filter label=kapoj.judge=1` 为空。
10. server 与 judge 日志不含源代码、测试点内容、程序输出。
11. `corepack pnpm test:web`、`lint:web`、`build:web`、后端 `go test ./...`、`go vet`、`gofmt`、golangci-lint 全绿。

- [ ] **Step 1: 写/补齐失败测试**

```go
func TestSpecialCheckerEndToEnd(t *testing.T) {
    // 真实 Docker：checker exit 0 -> accepted；exit 1 -> wrong_answer；
    // checker 语法错误 -> system_error（或重试耗尽后）。
}
```

- [ ] **Step 2: 运行测试确认失败/缺口**

Run: `cd apps/judge && KAPOJ_DOCKER_TESTS=1 KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/pipeline/ ./internal/sandbox/`
Expected: 缺失场景为 FAIL。

- [ ] **Step 3: 实现容器化与文档**

按上面的 Dockerfile、compose、文档要求更新。运行手册中的 judge 启动示例：

```bash
docker pull gcc:14
docker pull python:3.12-slim
docker pull eclipse-temurin:21-jdk
docker pull golang:1.26

export KAPOJ_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj?sslmode=disable'
export KAPOJ_REDIS_ADDR=localhost:6380
export KAPOJ_STORAGE_DIR=data
cd apps/judge && go run ./cmd/judge
```

- [ ] **Step 4: 运行完整验收**

```bash
# 后端
cd apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
go vet ./...
gofmt -l .
golangci-lint run

# 评测
cd ../judge
KAPOJ_DOCKER_TESTS=1 KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
go vet ./...
gofmt -l .

# 前端
cd ../..
corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web

# 容器编排
docker compose -f docker/docker-compose.yml --profile app config
docker compose -f docker/docker-compose.yml --profile app build
```
Expected: 全部通过，无残留容器，无秘密/代码日志。

- [ ] **Step 5: 提交**

```bash
git add apps/judge/Dockerfile apps/judge/.dockerignore docker/docker-compose.yml .env.example README.md 运行手册.md DEVELOPMENT.md docs/任务清单.md docs/开发日志.md
git commit -m "feat: m1-b judge service, deployment and docs"
```

---

## Self-Review

**Spec coverage:** 3.3 的 submissions 字段/状态/索引由 Task 1 覆盖；4.2 的异步入队、Docker 隔离、六类判定、特判、重试、统计回写由 Task 1–6 覆盖；M1-5/6/7 分别对应 Task 2、Task 5、Task 4/6。M1-C 的测评记录列表、SSE、Monaco 提交页明确不在本计划内。

**Type consistency:** 服务端与 judge 各自定义队列契约，Task 2 与 Task 3 的 `TypeSubmission`、`QueueName`、`submissionId` 字面量一致；判题状态在两边使用同一组字符串；`submissionId` 在 HTTP JSON 是 `problemId/language/code`，在 Asynq payload 是 `submissionId`，不混用。

**Review Focus mapping:** 隐藏/交互题 → Task 1 测试；孤儿容器与 TLE/MLE → Task 5 真 Docker 测试；重试重复计数 → Task 3/4 幂等测试；checker 故障 → Task 4/6；输出边界 → Task 3 Compare 与 Task 5 输出限制测试。
