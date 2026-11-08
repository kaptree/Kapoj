# M5-4 AI 出题造数据 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 管理员提交标程后，AI 生成纯 Python 3 造数据脚本，judge 在 Docker 沙盒运行脚本生成输入、运行标程生成输出，管理后台预览确认后写入题目测试数据。

**Architecture:** 服务器新增 `testcasegen` 领域包（generation 表/创建/预览/确认/丢弃）与 AI `GenerateTestcaseScript`；judge 新增 `judge:testcase-generation` 任务，用现有 Docker 沙盒执行生成器与标程，把文件写入共享 storage 并回写 generation/case 表；前端新增 `/problems/:id/testcase-generator` 页面，从测试点管理入口进入。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Asynq、Docker、Python 3.12、Vue 3、PrimeVue 4、Monaco、Vitest。

**Spec:** `docs/superpowers/specs/2026-11-08-m5-4-ai-testdata-design.md`

## Global Constraints

- 只支持传统题（`type=traditional`）；生成 1–10 组，score=10，`is_sample=false`；确认只追加，不删除或覆盖既有测试点。
- 标程源码 ≤64 KiB；AI 生成脚本必须为纯 Python 3 标准库；脚本 stdin 读取一个 1-based 整数 seed，stdout 输出一组输入；不写文件、不联网、不读取环境变量。
- 所有 AI 脚本与标程只在 judge 的 Docker 沙盒执行；服务器进程绝不 `os/exec`、绝不执行用户/AI 代码。
- 沙盒沿用 network none、read-only、cap-drop、CPU/内存/pids/输出限制；生成器限制 5s/256MB/16MiB 输出，标程使用题目 time/memory 限制与 16MiB 输出上限。
- 生成文件 key：`problems/<problemID>/generated/<generationID>/<n>.in`、`.out`（n 从 1 开始）。
- 状态：`pending` → `running` → `ready` / `failed`；`ready` 可确认或丢弃；`confirmed` 不可丢弃；`failed` 可丢弃并清理；`pending/running` 不能丢弃。
- 预览最多 64 KiB，超长标记 `truncated`，非法 UTF-8 用替换字符；不暴露文件系统路径。
- 权限只复用 `problem.testcase.manage`，不新增权限码；确认/丢弃/创建写审计。
- AI 错误沿用：平台密钥缺失 40002、用户未配置 AI 模型 40001、限流 42903、上游失败 50200、Redis 不可用 50300。
- judge 任务 `MaxRetry(0)`、超时 30 分钟；失败由管理员重新发起，不自动重试。
- 不自动发布题目；不改变手工测试点接口；不新增 UI 依赖；不推送/合并分支。
- 中文文案；界面新页面使用 PrimeVue 4；`TestcaseManager` 仍是 Element Plus legacy 页面（UI-19），入口按钮沿用现有风格。

## Review Focus

1. **沙盒边界**：AI 脚本/标程只能经 judge `sandbox.Compile/Run` 执行，服务器无执行路径；生成器运行仍是 network none/read-only/资源限制。测试：judge pipeline 只调用 `sandbox.Sandbox`；生成器超时/非 0/输出截断均失败。
2. **失败清理**：任一组失败后不能留下半套 cases/文件，也不能把 `failed` 误标为 `ready`。测试：编译失败、第 N 组超时、空输出、输出截断后状态 `failed`、cases 为空、generated 目录为空。
3. **确认并发与幂等**：重复确认不重复插入；并发确认不产生重复 sort_order；score=10、is_sample=false、sort_order 接在现有测试点后。测试：已有 1 个测试点时确认 2 组，得到 sort 2/3；再次确认行数不变；非 ready 拒绝。
4. **输入边界**：非传统题、非法语言、count 0/11、空/超长源码、AI 未配置/用户未配置必须在创建前拒绝，不产生 generation 行或队列任务。测试：service/handler 表驱动。
5. **预览安全**：大于 64 KiB 截断、非法 UTF-8 替换、错误 case/kind 404/400，不读取任意路径。测试：storage 写入大文件/坏字节后预览。
6. **丢弃语义**：`confirmed` 永不删文件；`ready/failed` 丢弃删除 cases 与确定性 `n.in/n.out`；`pending/running` 拒绝。测试：各状态丢弃结果与文件存在性。

---

### Task 1: 服务器 generation 模型、AI 脚本生成、创建 API 与队列契约（GEN-A）

**Files:**
- Create: `apps/server/internal/ai/testcase_script.go`
- Create: `apps/server/internal/ai/testcase_script_test.go`
- Create: `apps/server/internal/testcasegen/model.go`
- Create: `apps/server/internal/testcasegen/repository.go`
- Create: `apps/server/internal/testcasegen/service.go`
- Create: `apps/server/internal/testcasegen/service_test.go`
- Create: `apps/server/internal/problemapi/testcase_generation.go`
- Create: `apps/server/internal/problemapi/testcase_generation_test.go`
- Modify: `apps/server/internal/judgequeue/client.go`
- Create: `apps/server/internal/judgequeue/generation_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`（`newFx` 注入 testcasegen service）
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`

**Interfaces:**
- Produces:
  ```go
  // ai
  type TestcaseScriptSample struct { Input string; Output string }
  type TestcaseScriptInput struct {
      Title, StatementMD, InputFormatMD, OutputFormatMD string
      Samples []TestcaseScriptSample
      StandardCode, Language string
      TestcaseCount int
  }
  func (s *Service) GenerateTestcaseScript(ctx context.Context, userID int64, in TestcaseScriptInput) (string, error)

  // testcasegen
  type Status string
  const (
      StatusPending Status = "pending"; StatusRunning Status = "running"
      StatusReady Status = "ready"; StatusFailed Status = "failed"
      StatusConfirmed Status = "confirmed"; StatusDiscarded Status = "discarded"
  )
  type Generation struct {
      ID int64; ProblemID int64; UserID int64; Status Status
      Language string; StandardCode string; GeneratorCode string
      TestcaseCount int; TimeLimitMS int; MemoryLimitMB int
      ErrorMessage string; CreatedAt time.Time; UpdatedAt time.Time
  }
  func (Generation) TableName() string { return "testcase_generations" }
  type GenerationCase struct {
      ID int64; GenerationID int64; SortOrder int
      InputFile string; OutputFile string; InputSize int64; OutputSize int64
  }
  func (GenerationCase) TableName() string { return "testcase_generation_cases" }

  type Repository interface {
      Create(ctx context.Context, g *Generation) error
      Find(ctx context.Context, problemID, id int64) (*Generation, error)
      ListCases(ctx context.Context, generationID int64) ([]GenerationCase, error)
      MarkFailed(ctx context.Context, id int64, message string) error
  }
  type ProblemReader interface { FindByID(ctx context.Context, id int64) (*problem.Problem, error) }
  type ScriptGenerator interface {
      GenerateTestcaseScript(ctx context.Context, userID int64, in ai.TestcaseScriptInput) (string, error)
  }
  type Enqueuer interface { EnqueueTestcaseGeneration(ctx context.Context, id int64) error }
  type CreateInput struct { Language, SourceCode string; TestcaseCount int }
  type CaseView struct { ID int64; SortOrder int; InputSize, OutputSize int64 }
  type View struct {
      ID int64; Status Status; Language string; TestcaseCount int
      GeneratorCode string; ErrorMessage string; CreatedAt time.Time; Cases []CaseView
  }
  func NewGormRepository(db *gorm.DB) Repository
  func NewService(repo Repository, problems ProblemReader, store storage.Storage,
      scripts ScriptGenerator, queue Enqueuer) *Service
  func (s *Service) Create(ctx context.Context, problemID, userID int64, in CreateInput) (*View, error)
  func (s *Service) Get(ctx context.Context, problemID, id int64) (*View, error)

  // judgequeue
  const TypeTestcaseGeneration = "judge:testcase-generation"
  type TestcaseGenerationPayload struct { GenerationID int64 `json:"generationId"` }
  func (c *Client) EnqueueTestcaseGeneration(ctx context.Context, generationID int64) error

  // problemapi / router / main
  Options.TestcaseGen *testcasegen.Service
  Deps.TestcaseGen *testcasegen.Service
  ```
- Consumes: `ai.Service.chatRequest/checkLimit/client`, `problem.Repository.FindByID`, `storage.Storage`, existing RBAC/audit middleware.

- [ ] **Step 1: 写失败测试**

  `ai/testcase_script_test.go`:
  - `TestGenerateTestcaseScriptBuildsPromptAndStripsFence`：fake `ChatClient` 捕获请求并返回 ` ```python\nprint(1)\n``` `；断言返回 `print(1)`，prompt 含题目标题、标准代码、`Python 3`、`标准库`、`stdin`、组数。
  - `TestGenerateTestcaseScriptValidatesInput`：count 0/11、空源码、>64 KiB 源码返回 40001 且不调用客户端。
  - `TestGenerateTestcaseScriptNoPlatformKey`：无平台密钥返回 40002；`TestGenerateTestcaseScriptEmptyResponse` 返回 50200。

  `testcasegen/service_test.go`（真实 `testutil.PG` + 本地 storage）：
  - `TestCreateRequiresTraditionalProblem`、`TestCreateValidatesInput`（语言/count/源码长度）、`TestCreateCallsAIAndEnqueues`（行 pending、脚本文本、queue id）、`TestCreateAIErrorWritesNothing`、`TestCreateEnqueueFailureMarksFailed`、`TestGetReturnsCasesAndNotFound`。
  - 用 fake `ProblemReader`/`ScriptGenerator`/`Enqueuer`；DB 用真实 `NewGormRepository`。

  `problemapi/testcase_generation_test.go`：
  - `TestCreateTestcaseGeneration`：problem_admin 200/201，响应 `status=pending`、`testcaseCount`；无权限 403、未登录 401、非传统题 400、非法 count 400。
  - `TestGetTestcaseGeneration`：轮询返回 cases；不存在 404。

  `judgequeue/generation_test.go`：断言任务类型 `judge:testcase-generation`、payload 仅 `generationId`、`MaxRetry(0)`、超时 30 分钟。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/ai/ ./internal/testcasegen/ ./internal/problemapi/ ./internal/judgequeue/ \
    -run 'TestGenerateTestcaseScript|TestCreateTestcaseGeneration|TestGetTestcaseGeneration|TestTaskPayload|TestGeneration' -v
  ```
  Expected: FAIL（包/类型不存在）。

- [ ] **Step 3: 实现**

  - `ai/testcase_script.go`：校验 count 1–10、源码 trim 非空且 ≤64 KiB；构造 system prompt：
    “只输出 Python 3 脚本；只用标准库；stdin 读整数 seed；`random.Random(seed)`；stdout 输出一组输入；不写文件/不联网/不读环境变量；遵守输入格式与范围、覆盖边界；输出 ≤16 MiB；不要 Markdown/解释。”
    user prompt 包含题面、输入/输出格式、样例（每段截断到 4 KiB）、标程、语言、组数；调用 `chatRequest` → `checkLimit` → `client.Chat`；`stripCodeFence`；空响应/脚本 >64 KiB 返回 `httpx.ErrBadGateway`。
  - `testcasegen/model.go`：GORM 模型；`testcase_generations` 索引 `(problem_id, created_at)`；`testcase_generation_cases` 唯一索引 `(generation_id, sort_order)`。
  - `testcasegen/repository.go`：`Create` 设置 `StatusPending`/时间；`Find` 按 problem_id+id，`gorm.ErrRecordNotFound` 原样返回；`ListCases` 按 sort_order；`MarkFailed` 更新 status/error/updated_at。
  - `testcasegen/service.go`：合法语言集合 `{c,cpp,python3,java,go}`；验证 count/源码/题目存在/`problem.TypeTraditional`；把 `problem.Samples` 转为 `ai.TestcaseScriptSample`；AI 成功后插入 pending 行；入队失败 `MarkFailed(...,"任务投递失败")` 并返回 `httpx.ErrUnavailable`；`Get` 组装 `View`（`Cases` 为空时返回 `[]`）。
  - `problemapi/testcase_generation.go`：`createTestcaseGeneration` 取 `claims.UserID`、`pathID("id")`、`BindJSON[testcasegen.CreateInput]`；`getTestcaseGeneration` 路径 `genId`；错误统一 `httpx.Fail`。
  - `problemapi/handler.go`：`Options.TestcaseGen`；`handler.testcaseGen`；仅当非 nil 注册两条 `problem.testcase.manage` 路由；创建走 audit。
  - `judgequeue/client.go`：新增 payload/task/enqueue；`asynq.Queue(QueueName)`、`asynq.MaxRetry(0)`、`asynq.Timeout(30*time.Minute)`。
  - `db/db.go`：`AutoMigrate` 增加 `&testcasegen.Generation{}, &testcasegen.GenerationCase{}`。
  - `router.go`：`Deps.TestcaseGen` 传入 `problemapi.Options{TestcaseGen: d.TestcaseGen}`。
  - `main.go`：构造 `testcaseGenSvc := testcasegen.NewService(testcasegen.NewGormRepository(g), problem.NewGormRepository(g), store, aiSvc, queue)`；传入 router。
  - `problemapi/handler_test.go`：`newFx` 增加 fake script generator（固定返回 `print(1)`）与 fake enqueuer，构造 service 注入；保持既有测试不变。

- [ ] **Step 4: 运行测试、gofmt、lint**

  ```bash
  cd apps/server
  gofmt -w internal/ai internal/testcasegen internal/problemapi internal/judgequeue internal/db internal/router cmd/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/ai/ ./internal/testcasegen/ ./internal/problemapi/ ./internal/judgequeue/ ./internal/db/ ./internal/router/
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
  ```
  Expected: PASS；lint 0 issues。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/server/internal/ai apps/server/internal/testcasegen apps/server/internal/problemapi \
    apps/server/internal/judgequeue apps/server/internal/db apps/server/internal/router apps/server/cmd/server
  git commit -m "feat(server): add AI testcase generation API"
  ```

---

### Task 2: judge `judge:testcase-generation` 任务与沙盒执行（GEN-B）

**Files:**
- Create: `apps/judge/internal/model/generation.go`
- Create: `apps/judge/internal/db/generation.go`
- Create: `apps/judge/internal/db/generation_test.go`
- Create: `apps/judge/internal/storage/writer.go`
- Create: `apps/judge/internal/storage/writer_test.go`
- Modify: `apps/judge/internal/storage/reader.go`（抽出共享 key 校验）
- Create: `apps/judge/internal/generation/pipeline.go`
- Create: `apps/judge/internal/generation/handler.go`
- Create: `apps/judge/internal/generation/pipeline_test.go`
- Create: `apps/judge/internal/generation/handler_test.go`
- Modify: `apps/judge/internal/queue/contract.go`
- Modify: `apps/judge/internal/queue/contract_test.go`
- Modify: `apps/judge/internal/testutil/pg.go`
- Modify: `apps/judge/cmd/judge/main.go`

**Interfaces:**
- Produces:
  ```go
  // model
  type Generation struct {
      ID, ProblemID int64
      Language Language
      StandardCode, GeneratorCode string
      TestcaseCount, TimeLimitMS, MemoryLimitMB int
  }
  const (
      GenerationPending = "pending"; GenerationRunning = "running"
      GenerationReady = "ready"; GenerationFailed = "failed"
  )
  type GenerationCase struct {
      ID, GenerationID int64; SortOrder int
      InputFile, OutputFile string; InputSize, OutputSize int64
  }

  // db
  func (r *Repository) ClaimGeneration(ctx context.Context, id int64) (*model.Generation, error)
  func (r *Repository) ResetGenerationCases(ctx context.Context, generationID int64) error
  func (r *Repository) AddGenerationCase(ctx context.Context, c *model.GenerationCase) error
  func (r *Repository) FinishGeneration(ctx context.Context, id int64, status, message string) error

  // storage
  func Put(root, key string, r io.Reader, maxBytes int64) (int64, error)
  func RemoveAll(root, key string) error

  // generation
  func New(repo *db.Repository, root string, sb sandbox.Sandbox) *Pipeline
  func (p *Pipeline) Run(ctx context.Context, generationID int64) error
  func (p *Pipeline) Handle(ctx context.Context, task *asynq.Task) error

  // queue
  const TypeTestcaseGeneration = "judge:testcase-generation"
  type TestcaseGenerationPayload struct { GenerationID int64 `json:"generationId"` }
  ```
- Consumes: `sandbox.Docker` Compile/Run/Cleanup, `storage.Open` key validation, `queue` contract, `db.Repository.Pool`.

- [ ] **Step 1: 写失败测试**

  `storage/writer_test.go`：`TestPutOpenRoundTrip`、`TestPutRejectsUnsafeKeys`（与 reader 同一组 key）、`TestPutTooLargeLeavesExisting`、`TestRemoveAll`（只删 root 内目录）。

  `db/generation_test.go`：
  - `TestClaimGenerationOnlyPending`：pending 可认领并置 running；第二次/终态 `db.ErrSkip`。
  - `TestGenerationCasesAndFinish`：insert/list 顺序、finish 写 status/message、只更新 running/pending。

  `generation/pipeline_test.go`（fake `sandbox.Sandbox` + 真实 DB/本地 storage）：
  - `TestGenerationStoresFilesAndCases`：2 组；generator Run 读取 seed 文件返回 `seed\n`，标程返回 `seed*2\n`；断言 4 个文件内容、2 行 cases（key/size/order）、状态 ready。
  - `TestGenerationFailuresCleanUp`：表驱动 generator 编译失败、标程编译失败、generator 超时/OOM/非 0/空输出/输出截断 → 状态 failed、消息含“第 N 组”或“编译失败”、cases 0、generated 目录不存在。
  - `TestGenerationClaimSkip`：非 pending 不执行沙盒。
  - `handler_test.go`：malformed payload 忽略；合法 payload 调用 pipeline。

  `queue/contract_test.go`：新增 `TypeTestcaseGeneration`/payload 断言。

  `testutil/pg.go`：新增两张表（字段与 GORM 模型一致）并加入 `TRUNCATE`。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/judge
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 ./internal/storage/ ./internal/db/ ./internal/generation/ ./internal/queue/ -v
  ```
  Expected: FAIL。

- [ ] **Step 3: 实现**

  - `storage/writer.go`：`resolve(root,key)` 校验（空、NUL/反斜杠、绝对路径、`..`、clean 后不一致）；`Put` 创建父目录、临时文件、`io.LimitReader(max+1)`、超限 `ErrTooLarge`、`os.Rename`；`RemoveAll` 只对合法 key `os.RemoveAll(filepath.Join(root, key))`。`reader.go` 复用 `resolve`。
  - `db/generation.go`：`ClaimGeneration` 用 `UPDATE testcase_generations SET status='running', updated_at=now() WHERE id=$1 AND status='pending' RETURNING ...`；无行返回 `ErrSkip`；`ResetGenerationCases` 删除 cases；`AddGenerationCase` 插入；`FinishGeneration` 更新 `status/message/updated_at WHERE id=$1 AND status IN ('pending','running')`。
  - `generation/pipeline.go`：
    1. Claim；`ErrSkip` 返回 nil。
    2. `ResetGenerationCases`；`storage.RemoveAll(root, problems/<pid>/generated/<gid>)`。
    3. Compile generator（`Language:"python3"`）；失败 → fail(“造数据脚本编译失败：” + 截断 8 KiB)。
    4. Compile standard（`Language:string(gen.Language)`）；失败 → fail(“标程编译失败：” + 截断 8 KiB)。
    5. i=1..count：写 seed `strconv.Itoa(i)+"\n"`；`Run` generator（5s/256MB/16MiB）；校验 `!TimedOut && !OOMKilled && ExitCode==0 && !OutputTruncated && len(TrimSpace(Output))>0`；`Put` input key；写临时输入文件；`Run` standard（`gen.TimeLimitMS`/`gen.MemoryLimitMB`/16MiB）；同样校验；`Put` output key；`AddGenerationCase`。
    6. 全部成功 `FinishGeneration(...,GenerationReady,"")`。
    7. 任一失败：`ResetGenerationCases`、`RemoveAll` generated 目录、`FinishGeneration(...,GenerationFailed,truncate(msg,8KiB))`，返回 nil；基础设施错误返回 error 供 Asynq 记录。
  - `generation/handler.go`：解析 `queue.TestcaseGenerationPayload`，`generationID <= 0` 忽略；调用 `Run` 并记录错误。
  - `cmd/judge/main.go`：`generation.New(repo, cfg.StorageDir, sb)`；`mux.HandleFunc(queue.TypeTestcaseGeneration, gen.Handle)`。
  - 测试 fake sandbox 的 `Compile` 根据源码内容返回成功/失败；`Run` 根据 `Language=="python3"` 读取 `InputPath` 返回 seed 数据，否则读取输入返回输出；记录所有请求以便断言资源限制。

- [ ] **Step 4: 运行测试、gofmt、lint**

  ```bash
  cd apps/judge
  gofmt -w internal/model internal/db internal/storage internal/generation internal/queue internal/testutil cmd/judge
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 ./internal/storage/ ./internal/db/ ./internal/generation/ ./internal/queue/ ./internal/pipeline/
  go vet ./...
  ```
  Expected: PASS。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/judge/internal/model apps/judge/internal/db apps/judge/internal/storage \
    apps/judge/internal/generation apps/judge/internal/queue apps/judge/internal/testutil apps/judge/cmd/judge
  git commit -m "feat(judge): run AI testcase generation"
  ```

---

### Task 3: 预览、确认入库、丢弃 API（GEN-C）

**Files:**
- Modify: `apps/server/internal/testcasegen/repository.go`
- Modify: `apps/server/internal/testcasegen/service.go`
- Modify: `apps/server/internal/testcasegen/service_test.go`
- Modify: `apps/server/internal/problemapi/testcase_generation.go`
- Modify: `apps/server/internal/problemapi/testcase_generation_test.go`

**Interfaces:**
- Produces:
  ```go
  var (
      ErrNotReady    = errors.New("testcase generation is not ready")
      ErrConfirmed   = errors.New("testcase generation is confirmed")
      ErrNotFinished = errors.New("testcase generation is still running")
  )
  // Repository is extended with:
  //   FindCase(ctx, generationID, caseID int64) (*GenerationCase, error)
  //   Confirm(ctx, problemID, id int64, cases []GenerationCase) (int, error)
  //   Discard(ctx, problemID, id int64) ([]string, error)
  func (r *gormRepository) FindCase(ctx context.Context, generationID, caseID int64) (*GenerationCase, error)
  func (r *gormRepository) Confirm(ctx context.Context, problemID, id int64, cases []GenerationCase) (int, error)
  func (r *gormRepository) Discard(ctx context.Context, problemID, id int64) ([]string, error)

  type Preview struct { Kind string; Content string; Truncated bool }
  type ConfirmResult struct { Count int `json:"count"` }
  func (s *Service) Preview(ctx context.Context, problemID, id, caseID int64, kind string) (*Preview, error)
  func (s *Service) Confirm(ctx context.Context, problemID, id int64) (*ConfirmResult, error)
  func (s *Service) Discard(ctx context.Context, problemID, id int64) error
  ```
- Consumes: Task 1 `Repository`/`Service`, `storage.Storage`, `problem.Testcase`, `httpx`.

- [ ] **Step 1: 写失败测试**

  `testcasegen/service_test.go` 新增：
  - `TestPreviewTruncatesAndSanitizes`：写入 70 KiB 含非法 UTF-8；断言 ≤64 KiB、`truncated=true`、替换字符存在。
  - `TestPreviewRejectsKindAndWrongCase`：kind 非 input/output 40001；case 不属于 generation / generation 不属于 problem 404。
  - `TestConfirmInsertsTestcasesAtEnd`：先插 1 个既有测试点；ready generation 2 个 cases；确认后 `problem_testcases` 共 3 行，新行 score=10、is_sample=false、sort 2/3；generation confirmed；重复确认返回 2 且不新增。
  - `TestConfirmRejectsNotReady`：pending/running/failed/discarded 40001。
  - `TestDiscardReadyDeletesCasesAndFiles`：ready + 文件存在；丢弃后 status discarded、cases 0、确定性 n.in/n.out 不存在。
  - `TestDiscardRejectsConfirmedAndRunning`：confirmed 40001 且文件保留；running/pending 40001。
  - `TestDiscardFailedCleansPartialFiles`：failed 且只有部分确定性文件；丢弃后全部不存在。

  `problemapi/testcase_generation_test.go` 新增：
  - `TestTestcaseGenerationPreview`：200、`truncated`、非法 kind 400。
  - `TestTestcaseGenerationConfirm`：POST confirm 返回 `{count:2}`；重复确认 count 不变；非 ready 400。
  - `TestTestcaseGenerationDiscard`：DELETE 200；confirmed 400；无权限 403。
  - 审计：创建/确认/丢弃各写一条 `problem.testcase.manage`。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/testcasegen/ ./internal/problemapi/ \
    -run 'TestPreview|TestConfirm|TestDiscard|TestTestcaseGeneration' -v
  ```
  Expected: FAIL。

- [ ] **Step 3: 实现**

  - 仓储 `FindCase`：`generation_id = ? AND id = ?`，未找到返回 `gorm.ErrRecordNotFound`。
  - 仓储 `Confirm`：事务内 `SELECT ... FOR UPDATE` generation（problem_id+id）；`confirmed` 直接返回 `len(cases)`（幂等）；非 `ready` 返回 `ErrNotReady`；锁 problem 行；`COALESCE(MAX(sort_order),0)`；插入 `problem.Testcase{ProblemID, InputFile, OutputFile, InputSize, OutputSize, Score:10, IsSample:false, SortOrder:max+i+1}`；更新 generation 为 confirmed；返回行数。
  - 仓储 `Discard`：事务锁 generation；`discarded` 返回 nil；`confirmed` → `ErrConfirmed`；`pending/running` → `ErrNotFinished`；`ready/failed` 收集 cases 的 input/output keys、删 cases、置 discarded；返回 keys。
  - Service `Preview`：校验 kind；`Find` generation；`FindCase`；`store.Open`；`io.LimitReader(64KiB+1)`；`strings.ToValidUTF8(...,"\uFFFD")`；`Truncated = len(raw)>64KiB`；storage 未找到映射 404。
  - Service `Confirm`：`Find` 载入 cases；若 status confirmed 直接返回 count（不锁行也可，但保持 repo 幂等）；否则 `repo.Confirm`；把 `ErrNotReady` 映射 40001。
  - Service `Discard`：`Find`；若 confirmed 返回 40001；若 pending/running 返回 40001；调用 repo；合并返回 keys 与确定性 keys（`n=1..TestcaseCount` 的 .in/.out），`store.Delete`，删除失败仅 `slog.Warn`。
  - Handler：新增 `previewTestcaseGeneration`（`kind` query）、`confirmTestcaseGeneration`、`discardTestcaseGeneration`；注册 `GET .../cases/:caseId/preview`、`POST .../confirm`、`DELETE ...`；confirm/discard 走 audit；路径参数用 `pathID`。
  - 错误映射：`ErrNotReady/ErrNotFinished/ErrConfirmed` → `httpx.ErrValidation` 对应中文；`gorm.ErrRecordNotFound` → 404。

- [ ] **Step 4: 运行测试、gofmt、lint**

  ```bash
  cd apps/server
  gofmt -w internal/testcasegen internal/problemapi
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/testcasegen/ ./internal/problemapi/ ./internal/db/ ./internal/router/
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
  ```
  Expected: PASS。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/server/internal/testcasegen apps/server/internal/problemapi
  git commit -m "feat(server): preview and confirm generated testcases"
  ```

---

### Task 4: OpenAPI、生成类型与前端造数据页面（GEN-D）

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/problems.ts`
- Create: `apps/web/src/views/TestcaseGeneratorView.vue`
- Create: `apps/web/src/views/TestcaseGeneratorView.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/TestcaseManager.vue`
- Modify: `apps/web/src/components/TestcaseManager.spec.ts`
- Modify: `docs/primevue-migration.md`（仅新增 UI-39 条目，正文说明放 Task 5 也可）

**Interfaces:**
- Produces:
  ```ts
  // types.ts
  export type TestcaseGenerationStatus = 'pending'|'running'|'ready'|'failed'|'confirmed'|'discarded'
  export type TestcaseGenerationInput = { language: SubmissionLanguage; sourceCode: string; testcaseCount: number }
  export type TestcaseGenerationCase = { id: number; sortOrder: number; inputSize: number; outputSize: number }
  export type TestcaseGenerationView = {
    id: number; status: TestcaseGenerationStatus; language: string; testcaseCount: number
    generatorCode: string; errorMessage: string; createdAt: string; cases: TestcaseGenerationCase[]
  }
  export type TestcaseGenerationPreview = { kind: 'input'|'output'; content: string; truncated: boolean }
  export type TestcaseGenerationConfirm = { count: number }

  // problems.ts
  createTestcaseGeneration(id, input: TestcaseGenerationInput): Promise<TestcaseGenerationView>
  getTestcaseGeneration(id, genId): Promise<TestcaseGenerationView>
  previewTestcaseGeneration(id, genId, caseId, kind): Promise<TestcaseGenerationPreview>
  confirmTestcaseGeneration(id, genId): Promise<TestcaseGenerationConfirm>
  discardTestcaseGeneration(id, genId): Promise<null>
  ```
- Routes: `/problems/:id(\\d+)/testcase-generator`，name `problem-testcase-generator`，meta `{ requiresAuth: true, permission: PERM.problem.testcase.manage, title: 'AI 造数据' }`。
- Consumes: Task 1/3 API, existing `CodeEditor`, PrimeVue UI components, `ApiError`.

- [ ] **Step 1: 更新 OpenAPI 并生成类型**

  - 新增 paths：
    - `POST /problems/{id}/testcase-generations` → `TestcaseGenerationResponse`（201）；400/401/403/404/429/502/503。
    - `GET /problems/{id}/testcase-generations/{genId}` → 同响应；404。
    - `GET /problems/{id}/testcase-generations/{genId}/cases/{caseId}/preview?kind=input|output` → `TestcaseGenerationPreviewResponse`；400/404。
    - `POST /problems/{id}/testcase-generations/{genId}/confirm` → `TestcaseGenerationConfirmResponse`；400/404。
    - `DELETE /problems/{id}/testcase-generations/{genId}` → `EmptyResponse`；400/404。
  - Schemas：`TestcaseGenerationCreateInput`（required language/sourceCode/testcaseCount；count 1–10；language enum）、`TestcaseGenerationView`（required 全部，status enum）、`TestcaseGenerationCase`、`TestcaseGenerationPreview`、`TestcaseGenerationConfirm`、各 Envelope。
  - `corepack pnpm gen:api`；types.ts 加别名；problems.ts 加方法。

- [ ] **Step 2: 写失败前端测试**

  `TestcaseGeneratorView.spec.ts`（mock `@/api/problems`、`CodeEditor`、PrimeVue toast/confirm）：
  - 表单：默认 cpp、count 5；源码为空/超过 64 KiB 时提示；创建调用 API 并显示 pending。
  - `TestcaseGeneratorCreatesAndPolls`：create 返回 pending；推进定时器后 GET 返回 ready；渲染 cases 表格、状态 Tag“待确认”。
  - `TestcaseGeneratorShowsAIErrors`：40002 显示“管理员尚未配置 AI 加密密钥”；40001 显示“尚未配置 AI 模型”并提供 `/settings/ai`；42903/50200/50300 显示服务端 message。
  - `TestcaseGeneratorPreview`：点击输入/输出预览调用 API，Dialog 显示内容与“已截断”提示。
  - `TestcaseGeneratorConfirmAndDiscard`：ready 时确认调用 API，成功 Toast 并跳转 `problem-edit`；丢弃二次确认后调用 DELETE 并清空；failed 只显示丢弃。
  - 轮询在 ready/failed 后停止，组件卸载清理定时器。

  `TestcaseManager.spec.ts`：
  - 传统题显示“AI 造数据”按钮，点击 push `problem-testcase-generator`；special 不显示。

- [ ] **Step 3: 实现**

  - `TestcaseGeneratorView.vue`：
    - `AppPageHeader`、`AppSurface`；表单 `Select`（c/cpp/python3/java/go）、`CodeEditor`（语言映射 python3→python）、`InputNumber` 1–10、说明文字。
    - `create`：校验源码 trim 非空且 ≤64 KiB；`creating` 状态；成功后保存 generation、启动 2s 轮询、10 分钟超时。
    - 轮询：`GET`；`pending/running` 继续；`ready/failed/confirmed/discarded` 停止；失败最多 10 分钟提示“生成超时，请稍后重试/重新生成”。
    - 状态 Tag 文案：排队中/生成中/待确认/生成失败/已入库/已丢弃；错误 `Message`；失败显示 `errorMessage`。
    - ready：cases `DataTable`（序号/输入大小/输出大小/预览按钮）、生成脚本 `<details>` 只读 `<pre>`、确认/丢弃按钮；confirmed 显示已入库；failed 显示丢弃。
    - 预览 Dialog：`previewTestcaseGeneration`，loading，`<pre>` 内容，truncated 时提示“仅显示前 64 KiB”。
    - 错误映射：`ApiError.code===40002` → 管理员未配置 AI 密钥；`40001` → 显示 message 并提供 `/settings/ai` 链接；其余显示 message。
    - 确认/丢弃用 PrimeVue `useConfirm`；成功后 Toast；确认成功 `router.push({ name:'problem-edit', params:{ id }})`。
    - `onBeforeUnmount` 清理轮询；移动端单列。
  - `router/index.ts`：新增路由，放在 problem-edit 附近。
  - `TestcaseManager.vue`：传统题在“测试点”标题旁加 Element Plus 按钮“AI 造数据”，`useRouter().push({name:'problem-testcase-generator',params:{id:props.problemId}})`；不加额外依赖。
  - PrimeVue 迁移文档：新增 UI-39 行，说明 `TestcaseGeneratorView` 使用 PrimeVue，`TestcaseManager` 入口仍属 UI-19 legacy。

- [ ] **Step 4: 运行前端测试、lint、build**

  ```bash
  cd /Users/admin/codespace/kapoj
  corepack pnpm gen:api
  corepack pnpm test:web
  corepack pnpm lint:web
  corepack pnpm build:web
  ```
  Expected: PASS（现有 537+ 测试全部通过，新增测试通过；build 成功）。

- [ ] **Step 5: 提交**

  ```bash
  git add packages/api/openapi.yaml apps/web/src/api apps/web/src/views/TestcaseGeneratorView.vue \
    apps/web/src/views/TestcaseGeneratorView.spec.ts apps/web/src/router/index.ts \
    apps/web/src/components/TestcaseManager.vue apps/web/src/components/TestcaseManager.spec.ts \
    docs/primevue-migration.md
  git commit -m "feat(web): add AI testcase generator page"
  ```

---

### Task 5: 文档、全量检查与真实 judge/Docker 冒烟（GEN-E）

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`（如需补充）
- Test: 全量后端、judge、前端、真实 Docker

**Interfaces:**
- Consumes: Task 1–4。
- Produces: 文档、检查记录、真实冒烟证据。

- [ ] **Step 1: 文档**

  - `DEVELOPMENT.md` §6.4：替换 cyaron 旧设想，记录纯 Python 3 脚本、stdin seed/stdout 输入、judge 任务、状态机、文件 key、确认/丢弃语义、沙盒限制、AI 错误码、权限复用。
  - `docs/任务清单.md`：M5-4 标记完成，列出 API/页面/任务。
  - `docs/开发日志.md`：新增 2026-11-08 M5-4 条目（AI 脚本生成、judge 沙盒、预览确认、测试）。
  - `README.md` / `运行手册.md`：新增 AI 造数据使用步骤、所需 judge/Python 镜像、失败排查（AI 密钥、用户配置、Docker、storage、队列）。
  - `docs/primevue-migration.md`：UI-39 完成说明；`TestcaseManager` 入口仍 legacy。

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
    go test -count=1 ./...

  cd /Users/admin/codespace/kapoj
  corepack pnpm test:web
  corepack pnpm lint:web
  corepack pnpm build:web
  git diff --check
  ```
  Expected: 全部 PASS；lint 0 issues；diff 无空白错误。

- [ ] **Step 3: 真实 judge/Docker 冒烟**

  - 使用临时数据库/存储，不碰现有数据：
    - 用 `KAPOJ_TEST_DATABASE_URL` 的独立测试库（或新建临时库）与 `mktemp -d` storage；Redis 用本地 `6380`。
  - 插入临时用户、传统题（time 1000ms/memory 256MB）、pending generation：generator 脚本为纯标准库（如 `import sys,random; s=int(sys.stdin.readline()); r=random.Random(s); print(s, r.randint(1,100))`），标程为 Python（读入并输出）。
  - 用 Asynq（judgequeue 或临时 Go 程序）投递 `judge:testcase-generation`；启动真实 judge（`KAPOJ_DATABASE_URL`、`KAPOJ_STORAGE_DIR`、`KAPOJ_REDIS_ADDR`、`KAPOJ_JUDGE_INSTANCE=m5-4-smoke`），等待状态 `ready`。
  - 验证：2 组文件存在、内容非空、size 与 DB 一致；`testcase_generation_cases` 2 行；无残留容器。
  - 通过 `testcasegen.Service`（或临时 HTTP server）验证预览截断、确认后 `problem_testcases` 2 行（score 10、sort 接续）、重复确认不重复；丢弃 ready/failed 清理 cases/文件。
  - 失败场景：编译错误、超时、非传统题、AI 未配置 40002；记录结果。
  - 若 Docker/Python 镜像不可用：记录具体错误，至少完成 Go 单测与 fake sandbox 冒烟，并在报告中明确未执行的真实项。
  - 清理临时库/存储/队列/容器/审计；不删除既有数据。

- [ ] **Step 4: 提交**

  ```bash
  git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md
  git commit -m "docs: record AI testcase generation"
  ```

---

## Self-Review

**Spec coverage:**
- 数据模型/迁移 → Task 1（server models + `db.Migrate`）、Task 2（judge 测试表）。
- AI 纯 Python 脚本生成 → Task 1。
- 创建任务/队列契约 → Task 1。
- judge 沙盒生成 input、标程生成 output、storage/DB 写入 → Task 2。
- 预览/确认/丢弃 → Task 3。
- OpenAPI/类型/前端页面/入口 → Task 4。
- 文档/全量检查/真实冒烟 → Task 5。

**Type consistency:**
- `TestcaseScriptInput`/`GenerateTestcaseScript` Task 1，Task 2/3 不直接使用。
- `Generation`/`Status`/`Repository` Task 1，Task 3 扩展 `FindCase/Confirm/Discard`。
- `judgequeue.TypeTestcaseGeneration`/payload Task 1 与 Task 2 同名同字段。
- `TestcaseGenerationView` 字段 Task 4 与 Task 1/3 `View` 对齐（`generatorCode`、`errorMessage`、`cases`）。
- 文件 key 公式 Task 2/3 一致：`problems/<pid>/generated/<gid>/<n>.in|.out`。

**Review Focus mapping:**
1. 沙盒边界 → Task 2 fake sandbox 断言 + `grep` 确认 server 无 `os/exec`；Task 5 真实 Docker。
2. 失败清理 → Task 2 `TestGenerationFailuresCleanUp`。
3. 确认并发/幂等 → Task 3 `TestConfirmInsertsTestcasesAtEnd` + 重复确认。
4. 输入边界 → Task 1 service/handler 表驱动。
5. 预览安全 → Task 3 `TestPreviewTruncatesAndSanitizes`。
6. 丢弃语义 → Task 3 `TestDiscard*`。
