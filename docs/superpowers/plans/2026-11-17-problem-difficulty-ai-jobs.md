# 题目难度、平台 AI 模型与定时任务 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把题目难度从 1–5 改为 1–100，支持平台 AI 模型管理、AI 初评、每日通过率重算，并新增通用定时任务后台模块。

**Architecture:** 新增 `difficulty` 领域包负责公式、设置和批量重算；`problem` 包保存难度元数据与历史；`ai` 包新增平台模型 CRUD 和难度评估；`jobs` 包提供调度、Asynq 执行和运行记录；HTTP 层按现有 `internal/<domain>api` 模式挂载。前端按现有 PrimeVue 管理页模式新增平台模型、定时任务和难度编辑 UI。

**Tech Stack:** Go 1.26、Gin 1.12、GORM 1.31 + PostgreSQL、Asynq 0.26、`robfig/cron/v3`；Vue 3.5、PrimeVue 4.5、Vitest、OpenAPI TypeScript 生成。

**Spec:** `docs/superpowers/specs/2026-11-17-problem-difficulty-ai-jobs-design.md`

## Global Constraints

- 难度存储/API 为整数 `1..100`；新题默认 `50`；旧 1–5 迁移为 `10/30/50/70/90`，`source='HydroOJ'` 的旧 1 迁移为 `50`。
- 公式固定为 `p0 = 1-(prior-1)/99`、`p_hat=(a+m*p0)/(n+m)`、`target=1+99*(1-p_hat)^gamma`；默认 `m=10`、`gamma=1.5`、`min_sample=10`、每日 `max_step=5`。
- 通过率按去重用户统计；默认排除比赛提交；默认窗口 365 天。
- `difficulty_auto=false` 的题目不扫描、不写历史、不计入 `progress_total`。
- AI 初评在创建时同步执行，超时默认 15 秒；失败回退 `default_score=50`，不阻塞创建。
- 平台模型 token 只以 AES-GCM 密文存储；任何 API/日志/审计不得返回明文或密文。
- 平台模型只服务系统任务；用户级 AI 仍使用 `user_ai_configs`。
- 新增权限 `system.jobs.manage`；平台模型复用 `ai.settings.manage`；题目重评复用 `problem.update`。
- 定时任务默认 `problem.difficulty.recalculate`，cron `0 3 * * *`，时区 `Asia/Shanghai`；同一 job 同时最多一个 `queued/running` run。
- 题包导出同时写 `difficulty`（旧 1–5 档位）、`difficultyScore`（1–100）、`difficultyScale`；旧版本应用仍可导入，精度降为档位。
- 所有新错误文案用中文；OpenAPI 先改，运行 `corepack pnpm gen:api` 生成类型。
- 后端测试使用 `KAPOJ_TEST_DATABASE_URL`；前端测试用 Vitest；提交前跑对应包检查。

## Review Focus

1. **小样本/冷启动**：少于 10 个去重用户时不得改分；测试 `TestRecalculateSkipsBelowMinSample`（Task 10）。
2. **人工锁定**：`difficulty_auto=false` 的题目不得被每日/一键任务改动；测试 `TestRecalculateSkipsManualProblems`（Task 10）和 `TestUpdateProblemManualLock`（Task 8）。
3. **AI 失败不阻塞创建**：上游超时/格式错误/未配置时必须创建成功并落 50；测试 `TestCreateProblemAutoFallbackOnAIError`（Task 8）。
4. **题包刻度兼容**：旧 1–5 包导入要映射，新包导出要双字段，旧版本应用能读取 1–5 字段；测试 `TestPackageImportOldScale`、`TestPackageExportDualDifficultyFields`（Task 17）。
5. **密钥轮换/重置后的平台 token**：轮换必须重加密平台 token，重置必须清空 token/选择但保留模型元数据；测试 `TestRotateReencryptsPlatformModels`、`TestResetClearsPlatformTokensAndSelection`（Task 5）。

---

## File Structure

**新增后端文件**

- `apps/server/internal/difficulty/formula.go` — 纯公式/步长。
- `apps/server/internal/difficulty/model.go` — `difficulty_settings` 单例。
- `apps/server/internal/difficulty/settings.go` — 设置读取/校验/审计。
- `apps/server/internal/difficulty/recalculate.go` — 全站批量重算。
- `apps/server/internal/difficulty/job.go` — 注册 `problem.difficulty.recalculate`。
- `apps/server/internal/problem/difficulty.go` — 难度决策、历史模型/DTO、来源常量。
- `apps/server/internal/ai/platform.go` — 平台模型 CRUD/测试/选择。
- `apps/server/internal/ai/difficulty.go` — AI 初评 prompt/解析。
- `apps/server/internal/jobs/model.go` — `scheduled_jobs` / `scheduled_job_runs`。
- `apps/server/internal/jobs/service.go` — 任务列表/更新/手动执行/历史。
- `apps/server/internal/jobs/scheduler.go` — cron 计算、到期认领、调度循环。
- `apps/server/internal/jobs/queue.go` — Asynq `jobs` 队列与 handler。
- `apps/server/internal/jobsapi/handler.go` — `/api/admin/jobs` 等接口。
- `apps/server/internal/adminapi/ai_platform_handler.go` — `/api/admin/ai/models` 等接口。
- `apps/server/internal/problemapi/difficulty.go` — 重评/历史接口。

**修改的后端文件**

- `problem/model.go`、`validate.go`、`service.go`、`repository.go`、`dto.go`、`package.go`、`package_import.go`、`package_export.go`、`import_hydro.go`
- `ai/model.go`、`settings.go`、`key_service.go`
- `adminapi/coin_handler.go`、`adminapi/role_handler.go`
- `rbac/catalog.go`、`rbac/audit.go`
- `db/db.go`
- `router/router.go`
- `cmd/server/main.go`
- `packages/api/openapi.yaml`

**新增/修改的前端文件**

- `apps/web/src/utils/difficulty.ts`
- `apps/web/src/components/ui/AppDifficultyTag.vue`
- `apps/web/src/views/ProblemListView.vue`
- `apps/web/src/views/ProblemEditView.vue`
- `apps/web/src/views/ProblemDetailView.vue`
- `apps/web/src/views/AdminAiView.vue`
- `apps/web/src/views/AdminJobsView.vue`
- `apps/web/src/components/layout/AdminNav.vue`
- `apps/web/src/router/index.ts`
- `apps/web/src/permissions.ts`
- `apps/web/src/api/admin.ts`、`apps/web/src/api/problems.ts`、`apps/web/src/api/types.ts`
- 对应 `*.spec.ts`

---

### Task 1: 难度公式纯函数

**Files:**
- Create: `apps/server/internal/difficulty/formula.go`
- Test: `apps/server/internal/difficulty/formula_test.go`

**Interfaces:**
- Produces:
  ```go
  type FormulaInput struct {
      Prior, SampleUsers, AcceptedUsers, PriorStrength int
      Gamma float64
  }
  type FormulaResult struct {
      PriorPassRate, SmoothedPassRate float64
      Target int
  }
  func Compute(in FormulaInput) FormulaResult
  func Step(current, target, maxStep int) int
  ```

- [ ] **Step 1: 写失败测试**

```go
func TestComputeMatchesSpecExamples(t *testing.T) {
    cases := []struct {
        n, a int
        want int
    }{
        {10, 9, 17},
        {30, 12, 44},
        {100, 90, 6},
        {100, 5, 87},
        {10, 0, 65},
    }
    for _, tc := range cases {
        got := difficulty.Compute(difficulty.FormulaInput{
            Prior: 50, SampleUsers: tc.n, AcceptedUsers: tc.a,
            PriorStrength: 10, Gamma: 1.5,
        })
        if got.Target != tc.want {
            t.Fatalf("n=%d a=%d: want %d, got %d", tc.n, tc.a, tc.want, got.Target)
        }
    }
}

func TestStepClampsAndMovesTowardTarget(t *testing.T) {
    assert.Equal(t, 45, difficulty.Step(50, 17, 5))
    assert.Equal(t, 55, difficulty.Step(50, 87, 5))
    assert.Equal(t, 17, difficulty.Step(50, 17, 100))
    assert.Equal(t, 1, difficulty.Step(3, 1, 100))
    assert.Equal(t, 100, difficulty.Step(97, 100, 100))
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/difficulty -run 'TestCompute|TestStep' -v`
Expected: FAIL，`undefined: difficulty.Compute` / `difficulty.Step`。

- [ ] **Step 3: 实现 `Compute` 与 `Step`**

`Compute`：Clamp `Prior` 到 1..100、`SampleUsers` ≥0、`AcceptedUsers` 到 0..SampleUsers、`PriorStrength` ≥1、`Gamma` ≤0 时用 1.5；按设计文档公式计算，`math.Round` 后 clamp 1..100。`Step`：`maxStep<=0` 或 `current==target` 返回 clamp 后的 current；否则向 target 移动最多 `maxStep`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/difficulty -run 'TestCompute|TestStep' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/difficulty/formula.go apps/server/internal/difficulty/formula_test.go
git commit -m "feat(difficulty): add difficulty formula"
```

---

### Task 2: 难度设置单例

**Files:**
- Create: `apps/server/internal/difficulty/model.go`, `apps/server/internal/difficulty/settings.go`
- Test: `apps/server/internal/difficulty/settings_test.go`

**Interfaces:**
- Produces:
  ```go
  type SettingsView struct {
      Enabled bool
      DefaultScore, MinSample, PriorStrength, MaxStep, WindowDays int
      IncludeContest bool
      CurveGamma float64
      AIAssessEnabled bool
      AITimeoutSeconds int
      UpdatedAt time.Time
  }
  type SettingsInput struct { /* 同字段，无 UpdatedAt */ }
  type SettingsService struct { /* db */ }
  func NewSettingsService(db *gorm.DB) *SettingsService
  func (s *SettingsService) Get(ctx context.Context) (*SettingsView, error)
  func (s *SettingsService) Update(ctx context.Context, actorID int64, in SettingsInput, audit func(*gorm.DB) error) (*SettingsView, error)
  ```

- [ ] **Step 1: 写失败测试**

```go
func TestSettingsDefaultsAndLazyCreate(t *testing.T) {
    g := testutil.PG(t)
    svc := difficulty.NewSettingsService(g)
    view, err := svc.Get(context.Background())
    require.NoError(t, err)
    assert.True(t, view.Enabled)
    assert.Equal(t, 50, view.DefaultScore)
    assert.Equal(t, 10, view.MinSample)
    assert.Equal(t, 10, view.PriorStrength)
    assert.Equal(t, 5, view.MaxStep)
    assert.Equal(t, 365, view.WindowDays)
    assert.Equal(t, 1.5, view.CurveGamma)
}

func TestSettingsUpdateValidatesAndAudits(t *testing.T) {
    g := testutil.PG(t)
    svc := difficulty.NewSettingsService(g)
    audited := false
    in := difficulty.SettingsInput{
        Enabled: true, DefaultScore: 50, MinSample: 10, PriorStrength: 10,
        MaxStep: 5, WindowDays: 365, IncludeContest: false,
        CurveGamma: 1.5, AIAssessEnabled: true, AITimeoutSeconds: 15,
    }
    _, err := svc.Update(context.Background(), 7, in, func(tx *gorm.DB) error {
        audited = true
        return nil
    })
    require.NoError(t, err)
    assert.True(t, audited)

    in.DefaultScore = 101
    _, err = svc.Update(context.Background(), 7, in, nil)
    assertCode(t, err, 40001)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/difficulty -run 'TestSettings' -v`
Expected: FAIL，`undefined: difficulty.NewSettingsService`。

- [ ] **Step 3: 实现模型与服务**

`Settings` 表 `difficulty_settings`，固定 `id=1`；`Get` 在事务中 `ensureSettings` 后读取；`Update` 校验范围（`default_score 1..100`、`min_sample 1..1000`、`prior_strength 1..100`、`max_step 1..100`、`window_days 0..3650`、`curve_gamma 1.0..3.0`、`ai_timeout_seconds 5..60`），写 `updated_by/updated_at` 并在同一事务调用审计回调。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/difficulty -run 'TestSettings' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/difficulty/model.go apps/server/internal/difficulty/settings.go apps/server/internal/difficulty/settings_test.go
git commit -m "feat(difficulty): add difficulty settings"
```

---

### Task 3: 题目难度字段、历史表与迁移

**Files:**
- Modify: `apps/server/internal/problem/model.go`
- Create: `apps/server/internal/problem/difficulty.go`
- Modify: `apps/server/internal/db/db.go`
- Test: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Produces:
  ```go
  const (
      DifficultySourceAIInitial = "ai_initial"
      DifficultySourceAIReassess = "ai_reassess"
      DifficultySourcePassRate = "pass_rate"
      DifficultySourceManual = "manual"
      DifficultySourceMigration = "migration"
      DifficultySourceImport = "import"
      DifficultyPriorSourceAI = "ai"
      DifficultyPriorSourceDefault = "default"
      DifficultyPriorSourceMigrated = "migrated"
      DifficultyPriorSourceManual = "manual"
      DifficultyPriorSourceImport = "import"
  )

  type DifficultyHistory struct {
      ID int64
      ProblemID int64
      OldScore, NewScore, Prior int
      Source string
      SampleUsers, AcceptedUsers int
      PassRate, SmoothedPassRate float64
      TargetScore int
      Confidence float64
      Reason string
      RunID *int64
      CreatedBy int64
      CreatedAt time.Time
  }
  func (DifficultyHistory) TableName() string { return "problem_difficulty_history" }
  ```

- [ ] **Step 1: 写失败测试**

在 `db/db_test.go` 增加（该文件在 `package db` 内，不能 import `testutil`，因为它 import `db` 会形成循环；直接用 `open`）：

```go
func TestMigrateCreatesDifficultySchema(t *testing.T) {
    dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
    if dsn == "" { t.Skip("KAPOJ_TEST_DATABASE_URL not set; skipping database test") }
    g, err := open(dsn, io.Discard)
    require.NoError(t, err)
    require.NoError(t, Migrate(g))
    assert.True(t, g.Migrator().HasTable("difficulty_settings"))
    assert.True(t, g.Migrator().HasTable("problem_difficulty_history"))
    for _, field := range []string{
        "DifficultyPrior", "DifficultyPriorSource",
        "DifficultyAuto", "DifficultySource",
        "DifficultyReason", "DifficultyUpdatedAt",
    } {
        assert.True(t, g.Migrator().HasColumn(&problem.Problem{}, field), "缺少列 %s", field)
    }
}

func TestBackfillProblemDifficultyScale(t *testing.T) {
    dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
    if dsn == "" { t.Skip("KAPOJ_TEST_DATABASE_URL not set; skipping database test") }
    g, err := open(dsn, io.Discard)
    require.NoError(t, err)
    require.NoError(t, Migrate(g))
    rows := []problem.Problem{
        {Title: "a", Slug: "diff-a", StatementMD: "s", Difficulty: 1, Source: "manual"},
        {Title: "b", Slug: "diff-b", StatementMD: "s", Difficulty: 3, Source: "manual"},
        {Title: "c", Slug: "diff-c", StatementMD: "s", Difficulty: 1, Source: "HydroOJ"},
    }
    require.NoError(t, g.Create(&rows).Error)
    require.NoError(t, g.Where("key = ?", "problem_difficulty_1_100_v1").Delete(&Migration{}).Error)
    require.NoError(t, Migrate(g))
    var got []problem.Problem
    require.NoError(t, g.Where("slug IN ?", []string{"diff-a", "diff-b", "diff-c"}).Order("slug").Find(&got).Error)
    require.Len(t, got, 3)
    assert.Equal(t, 10, got[0].Difficulty)
    assert.Equal(t, 50, got[1].Difficulty)
    assert.Equal(t, 50, got[2].Difficulty, "HydroOJ 的默认 1 映射 50")
    assert.Equal(t, "migrated", got[0].DifficultyPriorSource)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/db -run 'TestMigrateCreatesDifficultySchema|TestBackfillProblemDifficultyScale' -v`
Expected: FAIL，缺少表/列或 `db.Migrate` 未回填。

- [ ] **Step 3: 实现字段、历史模型与回填**

`problem.Problem` 增加设计文档 §4.1 的 7 个字段；`DifficultyHistory` 按 §4.2；`db.Migrate` 的 `AutoMigrate` 加入 `&difficulty.Settings{}`、`&problem.DifficultyHistory{}`；新增 `backfillProblemDifficulty`（键 `problem_difficulty_1_100_v1`），按 §4.7 的 CASE 映射并写迁移标记。`difficulty` 的 GORM default 改为 50。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/db -run 'TestMigrateCreatesDifficultySchema|TestBackfillProblemDifficultyScale' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem/model.go apps/server/internal/problem/difficulty.go apps/server/internal/db/db.go apps/server/internal/db/db_test.go
git commit -m "feat(problem): add difficulty scale schema and migration"
```

---

### Task 4: 题目难度决策、历史与仓储

**Files:**
- Modify: `apps/server/internal/problem/validate.go`, `service.go`, `repository.go`, `model.go`
- Create/Modify: `apps/server/internal/problem/difficulty.go`
- Test: `apps/server/internal/problem/difficulty_test.go`, `apps/server/internal/problem/service_test.go`

**Interfaces:**
- Consumes: `DifficultyHistory`, source constants from Task 3.
- Produces:
  ```go
  type DifficultyDecision struct {
      Score, Prior int
      PriorSource, Source string
      Auto bool
      Reason string
      Confidence float64
      ActorID int64
  }
  func (s *Service) CreateWithDifficulty(ctx context.Context, createdBy int64, in Input, d DifficultyDecision) (*Problem, error)
  func (s *Service) UpdateWithDifficulty(ctx context.Context, id int64, in Input, d DifficultyDecision) (*Problem, error)
  func (s *Service) UpdateDifficultyMeta(ctx context.Context, id int64, d DifficultyDecision) (*Problem, error)
  func (s *Service) DifficultyHistory(ctx context.Context, problemID int64, page, pageSize int) ([]DifficultyHistory, int64, error)
  ```
- `Input` 增加 `DifficultyAuto *bool json:"difficultyAuto"`；`Normalize` 校验 `difficulty 1..100`。
- Repository 增加 `CreateWithHistory`、`UpdateWithHistory`、`UpdateDifficultyWithHistory`、`ListDifficultyHistory`。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateWithManualDifficultyWritesHistory(t *testing.T) {
    e := newEnv(t)
    in := input("manual-diff")
    in.Difficulty, in.DifficultyAuto = 80, ptr(false)
    p, err := e.svc.CreateWithDifficulty(context.Background(), 7, in, problem.DifficultyDecision{
        Score: 80, Prior: 80, PriorSource: problem.DifficultyPriorSourceManual,
        Source: problem.DifficultySourceManual, Auto: false, ActorID: 7,
    })
    require.NoError(t, err)
    assert.Equal(t, 80, p.Difficulty)
    assert.False(t, p.DifficultyAuto)
    var h problem.DifficultyHistory
    require.NoError(t, e.db.Where("problem_id = ?", p.ID).First(&h).Error)
    assert.Equal(t, problem.DifficultySourceManual, h.Source)
}

func TestUpdateManualLockAndReenable(t *testing.T) {
    e := newEnv(t)
    p := e.create(t, input("lock-diff"))
    in := input("lock-diff")
    in.Difficulty = 90
    up, err := e.svc.UpdateWithDifficulty(context.Background(), p.ID, in, problem.DifficultyDecision{
        Score: 90, Prior: 90, PriorSource: problem.DifficultyPriorSourceManual,
        Source: problem.DifficultySourceManual, Auto: false, ActorID: 1,
    })
    require.NoError(t, err)
    assert.False(t, up.DifficultyAuto)

    in.DifficultyAuto = ptr(true)
    up, err = e.svc.UpdateWithDifficulty(context.Background(), p.ID, in, problem.DifficultyDecision{
        Score: 90, Prior: 90, PriorSource: problem.DifficultyPriorSourceManual,
        Source: problem.DifficultySourceManual, Auto: true, ActorID: 1,
    })
    require.NoError(t, err)
    assert.True(t, up.DifficultyAuto)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/problem -run 'TestCreateWithManualDifficulty|TestUpdateManualLockAndReenable' -v`
Expected: FAIL，`undefined: CreateWithDifficulty` 或校验仍为 1–5。

- [ ] **Step 3: 实现服务与仓储**

- `Normalize` 把难度上限改为 100，错误文案“难度需为 1–100”。
- `CreateWithDifficulty`：`Normalize` → 校验 decision.Score 1..100 → `apply` → 设置难度元数据 → `repo.CreateWithHistory`（事务插入题目 + 历史，`old_score=0`）。
- `Create` 包装为 `decision{Score:in.Difficulty, Prior:in.Difficulty, PriorSource:manual, Source:manual, Auto:true, ActorID:createdBy}`。
- `UpdateWithDifficulty`：加载题目 → `Normalize` → `apply` → 设置 decision 的难度元数据 → `repo.UpdateWithHistory`；只有分数变化时写历史。
- `Update` 包装：`DifficultyAuto` 缺省时，分值变化按 manual lock，分值不变保留原 `auto/prior/source`；显式 false 按 manual；显式 true 时若分值变化则 `prior=新分/prior_source=manual`，否则保留先验。
- `UpdateDifficultyMeta`：只更新难度字段，写历史。
- 仓储用事务实现；`ListDifficultyHistory` 按 `created_at desc, id desc` 分页。
- 旧测试中 `Difficulty: 9` 的“非法”断言改为 101。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/problem -run 'TestCreateWithManualDifficulty|TestUpdateManualLockAndReenable|TestCreateValidates|TestDifficultyHistory' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem
git commit -m "feat(problem): add difficulty decisions and history"
```

---

### Task 5: 平台模型服务与密钥轮换

**Files:**
- Modify: `apps/server/internal/ai/model.go`, `settings.go`, `key_service.go`
- Create: `apps/server/internal/ai/platform.go`
- Test: `apps/server/internal/ai/platform_test.go`, `key_service_test.go`, `settings_test.go`

**Interfaces:**
- Produces:
  ```go
  type PlatformModel struct { /* 设计文档 §4.4 字段 */ }
  func (PlatformModel) TableName() string { return "ai_platform_models" }

  type PlatformModelView struct {
      ID int64
      Name, BaseURL, ModelName string
      Enabled bool
      APIKeyMasked string
      LastTestAt *time.Time
      LastTestOK bool
      LastTestError string
      LastTestLatencyMS int
      CreatedAt, UpdatedAt time.Time
  }
  type PlatformModelInput struct {
      Name, BaseURL, ModelName string
      APIKey string
      Enabled bool
  }
  type PlatformTestResult struct {
      OK bool
      LatencyMS int
      Message string
  }

  func NewPlatformService(db *gorm.DB, keys *KeyManager, client ChatClient, settings *SettingsService) *PlatformService
  func (s *PlatformService) List(ctx context.Context) ([]PlatformModelView, error)
  func (s *PlatformService) Create(ctx context.Context, actorID int64, in PlatformModelInput, audit func(*gorm.DB) error) (*PlatformModelView, error)
  func (s *PlatformService) Update(ctx context.Context, actorID, id int64, in PlatformModelInput, audit func(*gorm.DB) error) (*PlatformModelView, error)
  func (s *PlatformService) Delete(ctx context.Context, actorID, id int64, audit func(*gorm.DB) error) error
  func (s *PlatformService) Test(ctx context.Context, id int64) (*PlatformTestResult, error)
  func (s *PlatformService) Select(ctx context.Context, actorID int64, modelID *int64, audit func(*gorm.DB) error) (*SettingsView, error)
  ```

- [ ] **Step 1: 写失败测试**

```go
func TestPlatformModelCRUDAndMasking(t *testing.T) {
    g := testutil.PG(t)
    keys := newKeyManager(t)
    svc := ai.NewPlatformService(g, keys, fakeChat{}, ai.NewSettingsService(g, keys, ""))
    view, err := svc.Create(context.Background(), 7, ai.PlatformModelInput{
        Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1",
        ModelName: "deepseek-chat", APIKey: "sk-1234567890", Enabled: true,
    }, nil)
    require.NoError(t, err)
    assert.Equal(t, "sk-…7890", view.APIKeyMasked)
    list, err := svc.List(context.Background())
    require.NoError(t, err)
    assert.Len(t, list, 1)
}

func TestRotateReencryptsPlatformModels(t *testing.T) {
    // 创建模型、轮换密钥、断言 Test/List 仍能解密（用 fake client 捕获 key）。
}

func TestResetClearsPlatformTokensAndSelection(t *testing.T) {
    // 选择模型后 ResetKey；断言 token 为空、enabled=false、platform_model_id null。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/ai -run 'TestPlatform|TestRotateReencrypts|TestResetClears' -v`
Expected: FAIL，`undefined: ai.NewPlatformService`。

- [ ] **Step 3: 实现平台模型服务**

- `PlatformModel` 表与字段按 §4.4；`name` 唯一；`api_key_encrypted` 用 `text`。
- 校验：名称/模型名 1–100；base URL 仅 http/https、无 userinfo、不能以 `/chat/completions` 结尾；token 创建必填、更新可空保留；`keyConfigured=false` 返回 40002。
- `Create/Update/Delete/Select` 在事务内写审计回调；`Select` 校验模型存在且 enabled，更新 `ai_settings.platform_model_id`。
- `Test` 解密 token，调用 `{baseURL}/chat/completions` 的轻量请求；更新 `last_test_*`；返回 `OK/LatencyMS/Message`，不泄露 token/上游 body。
- `SettingsView` 增加 `PlatformModelID *int64`、`PlatformModelName string`；`SettingsService.Get` 查询模型名。
- `key_service.rotate`：统计平台模型密文；keep 模式无旧密钥但有密文时拒绝；`reencryptPlatformModels` 用 `SELECT FOR UPDATE` 逐行重加密；reset 模式清空 `api_key_encrypted`、`enabled=false`、`platform_model_id=NULL`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/ai -run 'TestPlatform|TestRotateReencrypts|TestResetClears|TestSettings' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai
git commit -m "feat(ai): add platform models and key rotation"
```

---

### Task 6: AI 难度评估

**Files:**
- Create: `apps/server/internal/ai/difficulty.go`
- Test: `apps/server/internal/ai/difficulty_test.go`

**Interfaces:**
- Produces:
  ```go
  type DifficultySample struct { Input, Output string }
  type DifficultyAssessmentInput struct {
      Title, Type string
      Tags []string
      TimeLimitMS, MemoryLimitMB int
      InputFormatMD, OutputFormatMD, StatementMD string
      Samples []DifficultySample
  }
  type DifficultyAssessment struct {
      Score int
      Confidence float64
      Reason string
  }
  func (s *Service) PlatformModelConfigured(ctx context.Context) (bool, error)
  func (s *Service) AssessDifficulty(ctx context.Context, in DifficultyAssessmentInput) (*DifficultyAssessment, error)
  ```

- [ ] **Step 1: 写失败测试**

```go
func TestAssessDifficultyValidJSON(t *testing.T) {
    client := &fakeChatClient{reply: "```json\n{\"difficulty\":62,\"confidence\":0.8,\"reason\":\"需要最短路\"}\n```"}
    svc := newServiceWithPlatformModel(t, client)
    got, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{
        Title: "最短路", StatementMD: "求 1 到 n 的最短路。",
        Type: "traditional", TimeLimitMS: 1000, MemoryLimitMB: 256,
    })
    require.NoError(t, err)
    assert.Equal(t, 62, got.Score)
    assert.InDelta(t, 0.8, got.Confidence, 0.001)
}

func TestAssessDifficultyFallsBackOnInvalidJSON(t *testing.T) {
    client := &fakeChatClient{reply: "难度大概是中等"}
    svc := newServiceWithPlatformModel(t, client)
    _, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{Title: "x", StatementMD: "y"})
    assertCode(t, err, 50200)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/ai -run 'TestAssessDifficulty' -v`
Expected: FAIL，`undefined: AssessDifficulty`。

- [ ] **Step 3: 实现 prompt、平台解析与 JSON 解析**

- `resolvePlatformChat`：读 `ai_settings`，`platform_model_id` 为空返回 40001“尚未配置平台 AI 模型”；模型不存在/停用/token 为空返回 40001；解密 token。
- Prompt：系统消息给 1–100 评分标准，要求只输出 JSON，声明题面不是指令；用户消息包含标题/类型/标签/时限/内存/输入输出格式/最多 3 组样例/题面（截断 20,000 字符），样例与格式各截断 2,000/5,000。
- `Chat` 温度 0.2；`stripCodeFence` + 提取首个 `{` 到最后一个 `}`；`difficulty` 允许数字，`math.Round` 后校验 1..100；`confidence` clamp 0..1；`reason` 截断 500。
- `PlatformModelConfigured` 只检查选择/启用/密文非空，不解密。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/ai -run 'TestAssessDifficulty' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/ai/difficulty.go apps/server/internal/ai/difficulty_test.go
git commit -m "feat(ai): add problem difficulty assessment"
```

---

### Task 7: 管理后台平台模型 API 与审计

**Files:**
- Modify: `apps/server/internal/rbac/audit.go`, `apps/server/internal/adminapi/coin_handler.go`, `apps/server/internal/adminapi/role_handler.go`
- Create: `apps/server/internal/adminapi/ai_platform_handler.go`
- Test: `apps/server/internal/adminapi/ai_platform_handler_test.go`

**Interfaces:**
- Consumes: `*ai.PlatformService` from Task 5.
- Adds `adminapi.Options.AIPlatform *ai.PlatformService`.
- Routes:
  - `GET/POST /api/admin/ai/models`
  - `PUT/DELETE /api/admin/ai/models/:id`
  - `POST /api/admin/ai/models/:id/test`
  - `PUT /api/admin/ai/platform-model`

- [ ] **Step 1: 写失败测试**

```go
func TestAdminAIPlatformModelCRUD(t *testing.T) {
    f := newAIPlatformFixture(t, true)
    _, tok := f.user(t, "ai_platform_admin", rbac.RoleCodeSuperAdmin)

    w, resp := f.do(http.MethodPost, "/api/admin/ai/models", tok,
        `{"name":"Fake","baseUrl":"https://example.com/v1","modelName":"fake-chat","apiKey":"sk-test-1234","enabled":true}`)
    requireStatus(t, w, resp, http.StatusCreated)
    id := int64(dataMap(t, resp)["id"].(float64))

    w, resp = f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, fmt.Sprintf(`{"modelId":%d}`, id))
    requireStatus(t, w, resp, http.StatusOK)
    assert.EqualValues(t, id, dataMap(t, resp)["platformModelId"])
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/adminapi -run 'TestAdminAIPlatformModel' -v`
Expected: FAIL，路由 404。

- [ ] **Step 3: 实现 handler、路由与审计**

- `ai_platform_handler.go`：`listAIPlatformModels`、`createAIPlatformModel`、`updateAIPlatformModel`、`deleteAIPlatformModel`、`testAIPlatformModel`、`selectAIPlatformModel`。
- 使用 `httpx.BindJSON[ai.PlatformModelInput]`、`pathID`、`auditActor`；审计 action/target 按设计文档 §7.2。
- `role_handler.go` 的 `handler` 增加 `aiPlatform`，`Options` 增加字段，注册路由（`ai.settings.manage`）。
- 不返回 token/密文；测试失败返回 200 `{ok:false,...}`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/adminapi -run 'TestAdminAIPlatformModel' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/rbac/audit.go apps/server/internal/adminapi
git commit -m "feat(admin): add AI platform model API"
```

---

### Task 8: 题目 API 难度契约、AI 初评与历史

**Files:**
- Modify: `apps/server/internal/problem/dto.go`, `repository.go`, `service.go`, `apps/server/internal/problemapi/handler.go`
- Create: `apps/server/internal/problemapi/difficulty.go`
- Test: `apps/server/internal/problemapi/difficulty_test.go`, `apps/server/internal/problem/repository_test.go`

**Interfaces:**
- `problemapi.Options` 增加：
  ```go
  Difficulty *difficulty.SettingsService
  AI         DifficultyAssessor
  AISettings interface{ Enabled(ctx context.Context) (bool, error) }
  ```
  `DifficultyAssessor` 接口：
  ```go
  type DifficultyAssessor interface {
      PlatformModelConfigured(ctx context.Context) (bool, error)
      AssessDifficulty(ctx context.Context, in ai.DifficultyAssessmentInput) (*ai.DifficultyAssessment, error)
  }
  ```
- `problem.Filter` 增加 `DifficultyMin, DifficultyMax int`；`repository.List` 追加范围条件。
- `problem.DetailDTO` 增加管理端可选字段：
  ```go
  DifficultyAuto *bool
  DifficultySource *string
  DifficultyPrior *int
  DifficultyPriorSource *string
  DifficultyReason *string
  DifficultyUpdatedAt *time.Time
  ```
- New routes:
  - `POST /api/problems/:id/difficulty/assess`（`problem.update` + manage 检查）
  - `GET /api/problems/:id/difficulty/history`（同）

- [ ] **Step 1: 写失败测试**

```go
func TestCreateProblemAutoDifficulty(t *testing.T) {
    f := newProblemFixture(t, fakeAssessor{score: 62})
    _, tok := f.userWithProblemCreate(t, "author_auto")
    w, resp := f.do(http.MethodPost, "/api/problems", tok, `{
        "title":"最短路","slug":"shortest-path","statementMd":"求最短路",
        "difficulty":50,"difficultyAuto":true,"type":"traditional",
        "timeLimitMs":1000,"memoryLimitMb":256
    }`)
    requireStatus(t, w, resp, http.StatusCreated)
    data := dataMap(t, resp)
    assert.EqualValues(t, 62, data["difficulty"])
    assert.Equal(t, "ai", data["difficultySource"])
}

func TestCreateProblemAutoFallbackOnAIError(t *testing.T) {
    f := newProblemFixture(t, fakeAssessor{err: errors.New("timeout")})
    // 同请求；断言 201、difficulty=50、difficultySource="default"
}

func TestCreateProblemManualSkipsAI(t *testing.T) {
    f := newProblemFixture(t, fakeAssessor{score: 62})
    // difficultyAuto=false, difficulty=80；断言 80/ manual，assessor 调用次数 0
}

func TestUpdateProblemManualLock(t *testing.T) {
    // 先创建 auto 题，再 PUT difficulty=75, difficultyAuto=false；断言 auto=false
}

func TestAssessProblemDifficulty(t *testing.T) {
    // POST /difficulty/assess；断言返回 62、history ai_reassess、auto 状态不变
}

func TestListProblemsDifficultyRange(t *testing.T) {
    // 创建 10/50/90 三题；GET /problems?difficultyMin=40&difficultyMax=80 只返回 50
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/problemapi -run 'TestCreateProblemAuto|TestCreateProblemManual|TestAssessProblemDifficulty|TestListProblemsDifficultyRange' -v`
Expected: FAIL，字段/路由不存在或仍校验 1–5。

- [ ] **Step 3: 实现 API 编排**

- `handler.go` 的 `Options`/`handler` 接入 `Difficulty`、`AI`、`AISettings`。
- `create`：
  1. `BindJSON[problem.Input]`。
  2. `auto := in.DifficultyAuto == nil || *in.DifficultyAuto`。
  3. `auto` 时读取 `Difficulty.Get`，默认 `decision{Score: settings.DefaultScore, Prior: ..., Source: "default", Auto: true}`；若 `settings.AIAssessEnabled && aiSettings.Enabled && ai.PlatformModelConfigured`，用 `context.WithTimeout(settings.AITimeoutSeconds)` 调 `AssessDifficulty`，成功覆盖 decision。
  4. `auto=false` 时用请求分值构造 manual decision，不调用 AI。
  5. `svc.CreateWithDifficulty`。
- `update`：加载 existing，按设计文档 §5.3 计算 decision，调用 `svc.UpdateWithDifficulty`。
- `assess`：加载并检查 manage 权限；调 AI；构造 `decision{Auto: existing.DifficultyAuto}`；`svc.UpdateDifficultyMeta`；返回 `ToDetail`。
- `history`：分页返回 `problem.DifficultyHistory`。
- `list`：解析 `difficultyMin/difficultyMax`，校验 1..100 且 min<=max，写入 `problem.Filter`。
- `ToDetail` 在 `includeSolution=true` 时填充难度元数据指针。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/problemapi -run 'TestCreateProblemAuto|TestCreateProblemManual|TestUpdateProblemManualLock|TestAssessProblemDifficulty|TestListProblemsDifficultyRange' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi
git commit -m "feat(problem): add AI difficulty assessment endpoints"
```

---

### Task 9: 通用定时任务核心

**Files:**
- Create: `apps/server/internal/jobs/model.go`, `service.go`, `scheduler.go`, `queue.go`
- Test: `apps/server/internal/jobs/service_test.go`, `scheduler_test.go`

**Interfaces:**
- Produces:
  ```go
  const (
      TriggerSchedule = "schedule"
      TriggerManual = "manual"
      StatusQueued = "queued"
      StatusRunning = "running"
      StatusSucceeded = "succeeded"
      StatusFailed = "failed"
      StatusCanceled = "canceled"
  )
  type Job struct { /* scheduled_jobs */ }
  type Run struct { /* scheduled_job_runs */ }
  type JobView struct { /* 设计文档 §6.3 */ }
  type JobInput struct { Enabled bool; CronExpr, Timezone string }
  type RunView struct { /* 设计文档 §6.3 */ }
  type RunFilter struct { JobKey, Status string }
  type Reporter interface {
      SetProgress(current, total int)
      SetStats(stats map[string]any)
  }
  type Handler func(ctx context.Context, run *Run, reporter Reporter) error
  type Definition struct {
      Key, Name, Description, DefaultCron, Timezone string
      Handler Handler
  }
  func NewService(db *gorm.DB, queue *Queue) *Service
  func (s *Service) Register(def Definition)
  func (s *Service) Ensure(ctx context.Context) error
  func (s *Service) RunScheduler(ctx context.Context, interval time.Duration)
  func (s *Service) ClaimDue(ctx context.Context, now time.Time) ([]Run, error)
  func (s *Service) List(ctx context.Context) ([]JobView, error)
  func (s *Service) Update(ctx context.Context, actorID int64, key string, in JobInput, audit func(*gorm.DB) error) (*JobView, error)
  func (s *Service) CreateManualRun(ctx context.Context, actorID int64, key string, audit func(*gorm.DB) error) (*RunView, error)
  func (s *Service) ListRuns(ctx context.Context, f RunFilter, page, pageSize int) (httpx.Page[RunView], error)
  func (s *Service) GetRun(ctx context.Context, id int64) (*RunView, error)
  func (s *Service) ReconcileStale(ctx context.Context, olderThan time.Duration) error
  func NextRun(cronExpr, timezone string, after time.Time) (time.Time, error)
  ```
- `Queue`：
  ```go
  const QueueName = "jobs"
  const TypeRun = "jobs.run"
  func NewQueue(redisAddr string) *Queue
  func (q *Queue) Enqueue(ctx context.Context, runID int64) error
  func Register(mux *asynq.ServeMux, svc *Service)
  ```

- [ ] **Step 1: 写失败测试**

```go
func TestNextRun(t *testing.T) {
    loc, _ := time.LoadLocation("Asia/Shanghai")
    now := time.Date(2026, 11, 17, 4, 0, 0, 0, loc)
    next, err := jobs.NextRun("0 3 * * *", "Asia/Shanghai", now)
    require.NoError(t, err)
    assert.Equal(t, time.Date(2026, 11, 18, 3, 0, 0, 0, loc), next)
}

func TestClaimDueCreatesRunAndAdvancesNext(t *testing.T) {
    // Ensure + 把 next_run_at 改到过去 + ClaimDue；断言 run queued、next 前进。
}

func TestManualRunDuplicate(t *testing.T) {
    // 第一次 202；第二次 40904。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/jobs -run 'TestNextRun|TestClaimDue|TestManualRun' -v`
Expected: FAIL，包/类型不存在。

- [ ] **Step 3: 实现任务核心**

- 模型按设计文档 §4.6；`Job.Key` 唯一；`Run` 索引 `(job_key, created_at desc)`；部分唯一索引在 `db.Migrate` 或 `Ensure` 用原生 SQL 创建（选择其一并写测试）。
- `Ensure`：按注册的 Definition `ON CONFLICT (key) DO NOTHING`；为 `enabled && next_run_at is null` 计算下次。
- `NextRun`：`cron/v3` 标准 5 段 + `time.LoadLocation`；返回 after 之后的下一次。
- `ClaimDue`：事务内 `FOR UPDATE SKIP LOCKED` 取到期任务，推进 `next_run_at/last_run_at/last_status`，为每个创建 `Run{status:queued, trigger:schedule}`；提交后由 scheduler 调 `queue.Enqueue`。
- `CreateManualRun`：校验 job/key；用部分唯一索引/查询检测 active run，冲突返回 `httpx` 40904（新增 `httpx.ErrJobRunning`）；创建 run 并入队。
- `Run` handler：仅 queued 执行；置 running/started；调 Definition.Handler；成功/失败写终态、stats、错误；更新 job last_status。
- `Reporter`：更新 run 的 progress/stats（可每 500ms 节流）。
- `ReconcileStale`：把超过 `olderThan` 的 queued/running 标记 failed。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/jobs -run 'TestNextRun|TestClaimDue|TestManualRun|TestRun' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/jobs apps/server/internal/httpx/errors.go
git commit -m "feat(jobs): add scheduled job framework"
```

---

### Task 10: 难度重算服务与 job handler

**Files:**
- Create: `apps/server/internal/difficulty/recalculate.go`, `recalculate_test.go`, `job.go`
- Test: `apps/server/internal/difficulty/recalculate_test.go`

**Interfaces:**
- Consumes: `SettingsService`, `FormulaInput/Compute/Step`, `problem.DifficultyHistory`, `jobs.Definition/Reporter`.
- Produces:
  ```go
  type RecalculateOptions struct {
      RunID int64
      FullStep bool
  }
  type Stats struct {
      Total, Processed, Changed, SkippedNoSample, SkippedUnchanged, Failed int
  }
  type Reporter interface {
      SetProgress(current, total int)
      SetStats(stats map[string]any)
  }
  type RecalculateService struct { /* db, settings, now */ }
  func NewRecalculateService(db *gorm.DB, settings *SettingsService) *RecalculateService
  func (s *RecalculateService) RecalculateAll(ctx context.Context, opts RecalculateOptions, reporter Reporter) (Stats, error)
  func RegisterJob(reg *jobs.Service, svc *RecalculateService)
  ```

- [ ] **Step 1: 写失败测试**

```go
func TestRecalculateUsesDistinctUsersAndWritesHistory(t *testing.T) {
    // 创建 1 题，10 个用户提交：8 个 AC，2 个 WA（其中一个用户先 WA 再 AC）。
    // prior=50, n=10, a=8；断言 difficulty 变化且 history sample_users=10/accepted_users=8。
}

func TestRecalculateSkipsBelowMinSample(t *testing.T) {
    // 9 个用户；断言难度不变、Stats.SkippedNoSample=1、无 history。
}

func TestRecalculateSkipsManualProblems(t *testing.T) {
    // difficulty_auto=false；断言难度不变、Stats.Total=0。
}

func TestRecalculateFullStepOnManualRun(t *testing.T) {
    // 目标 17、当前 50、FullStep=true；断言直接 17；FullStep=false 只到 45。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/difficulty -run 'TestRecalculate' -v`
Expected: FAIL，`undefined: NewRecalculateService`。

- [ ] **Step 3: 实现重算**

- 读取 settings；`enabled=false` 返回 40001。
- keyset 分页每批 200 道 `difficulty_auto=true` 的题。
- 聚合 SQL：
  ```sql
  SELECT problem_id, COUNT(*) AS submitters,
         COUNT(*) FILTER (WHERE solved) AS solvers
  FROM (
    SELECT problem_id, user_id, bool_or(status = 'accepted') AS solved
    FROM submissions
    WHERE problem_id IN ? AND status IN (?)
      [AND contest_id IS NULL]
      [AND created_at >= ?]
    GROUP BY problem_id, user_id
  ) t
  GROUP BY problem_id
  ```
- 对每题 `Compute`；`n<min_sample` 记 `SkippedNoSample`；`target==current` 记 `SkippedUnchanged`；`Step` 的 maxStep：manual run 100，否则 settings.MaxStep。
- 分数变化时事务更新 `problems`（`difficulty`, `difficulty_source='pass_rate'`, `difficulty_reason`, `difficulty_updated_at`）并插入 `problem.DifficultyHistory`（`RunID`、created_by=0、reason 含 n/a/p_hat）。
- 每批更新 reporter progress/stats。
- `RegisterJob` 注册 key/name/description/default cron/timezone；handler 调 `RecalculateAll`，`FullStep: run.Trigger == jobs.TriggerManual`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/difficulty -run 'TestRecalculate' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/difficulty/recalculate.go apps/server/internal/difficulty/recalculate_test.go apps/server/internal/difficulty/job.go
git commit -m "feat(difficulty): add scheduled recalculation job"
```

---

### Task 11: 定时任务 API、权限与接线

**Files:**
- Modify: `apps/server/internal/rbac/catalog.go`, `rbac/audit.go`, `rbac/seed_test.go`
- Create: `apps/server/internal/jobsapi/handler.go`
- Test: `apps/server/internal/jobsapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`, `apps/server/cmd/server/main.go`

**Interfaces:**
- Adds `rbac.PermSystemJobsManage = "system.jobs.manage"` and audit constants.
- `jobsapi.RegisterRoutes(api gin.IRouter, svc *jobs.Service, rbacSvc *rbac.Service)`.
- Routes per design §6.3.

- [ ] **Step 1: 写失败测试**

```go
func TestAdminJobsPermissionAndRun(t *testing.T) {
    // 未登录 401；content_admin 403；super_admin GET 200；POST run 202。
}

func TestAdminJobsUpdateCron(t *testing.T) {
    // PUT cron 非法 40001；合法 200 且 nextRunAt 变化。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/jobsapi -run 'TestAdminJobs' -v`
Expected: FAIL，路由不存在/权限目录缺少。

- [ ] **Step 3: 实现 API、权限、接线**

- `rbac/catalog.go` 增加权限定义（`PermissionGroupAdmin`），`rbac/audit.go` 增加 `AuditActionSystemJobsManage`、`AuditActionAIPlatformModel*`、target 常量；更新 seed 测试权限数 43→44。
- `jobsapi`：list/update/run/list runs/get run/all runs；使用 `requirePermission(rbac.PermSystemJobsManage)`；`run` 成功 `202`，冲突 `40904`。
- `router.Deps` 增加 `Difficulty *difficulty.SettingsService`、`AIPlatform *ai.PlatformService`、`Jobs *jobs.Service`；`problemapi.Options` 注入 Difficulty/AI/AISettings；`adminapi.Options` 注入 AIPlatform；`jobsapi.RegisterRoutes`。
- `main.go`：
  - 抽出 `aiClient := ai.NewOpenAIClient(cfg.AIRequestTimeout)`。
  - `aiSettings := ai.NewSettingsService(...)`；`platformSvc := ai.NewPlatformService(g, keyMgr, aiClient, aiSettings)`。
  - `difficultySettings := difficulty.NewSettingsService(g)`；`difficultyRecalc := difficulty.NewRecalculateService(g, difficultySettings)`。
  - `jobsQueue := jobs.NewQueue(cfg.RedisAddr)`；`jobsSvc := jobs.NewService(g, jobsQueue)`；`difficulty.RegisterJob(jobsSvc, difficultyRecalc)`；`jobsSvc.Ensure`；`jobsSvc.ReconcileStale(2h)`。
  - 启动 `jobs` Asynq server + `jobs.Register`，`go jobsSvc.RunScheduler(workCtx, 30*time.Second)`。
  - `router.Deps` 传入新服务。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd apps/server && go test ./internal/jobsapi ./internal/rbac ./internal/router -v`
Expected: PASS（router 可能无测试，至少编译）。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/rbac apps/server/internal/jobsapi apps/server/internal/router apps/server/cmd/server/main.go
git commit -m "feat(admin): add scheduled jobs API and wiring"
```

---

### Task 12: OpenAPI 与生成类型

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`

**Interfaces:**
- Adds paths/schemas per design §6.1–6.4.
- Generated aliases used by frontend tasks:
  `PlatformModelView`, `PlatformModelInput`, `PlatformModelTestResult`, `AdminAISettingsView`, `DifficultySettingsView`, `DifficultySettingsInput`, `JobView`, `JobInput`, `JobRunView`, `DifficultyHistoryItem`, `ProblemDetail` fields.

- [ ] **Step 1: 写失败检查**

Run: `corepack pnpm --filter web build`
Expected: 通过（当前代码）；记录基线。

- [ ] **Step 2: 修改 OpenAPI**

按设计文档补齐：
- `/admin/ai/models`、`/admin/ai/models/{id}`、`/admin/ai/models/{id}/test`、`/admin/ai/platform-model`
- `/admin/difficulty/settings`
- `/admin/jobs`、`/admin/jobs/{key}`、`/admin/jobs/{key}/run`、`/admin/jobs/{key}/runs`、`/admin/job-runs`、`/admin/job-runs/{id}`
- `/problems` 的 `difficultyMin/difficultyMax`
- `/problems/{id}/difficulty/assess`、`/problems/{id}/difficulty/history`
- `ProblemInput/ProblemUpdateInput.difficulty` 1–100、`difficultyAuto`
- `ProblemDetail` 可选难度元数据
- 新 schemas：`PlatformModelView/Input/TestResult`、`DifficultySettingsView/Input`、`JobView/Input`、`JobRunView`、`DifficultyHistoryItem` 等。

- [ ] **Step 3: 生成类型并修正别名**

Run: `corepack pnpm gen:api`
Expected: `apps/web/src/api/schema.d.ts` 更新；在 `types.ts` 增加别名。

- [ ] **Step 4: 运行构建确认通过**

Run: `corepack pnpm --filter web build`
Expected: PASS（新字段可选，不破坏现有页面）。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts
git commit -m "feat(api): add difficulty, AI platform and jobs contracts"
```

---

### Task 13: 前端难度工具、标签与题库筛选

**Files:**
- Modify: `apps/web/src/utils/difficulty.ts`, `apps/web/src/utils/difficulty.spec.ts`
- Modify: `apps/web/src/components/ui/AppDifficultyTag.vue`, `.spec.ts`
- Modify: `apps/web/src/views/ProblemListView.vue`, `.spec.ts`
- Modify: `apps/web/src/utils/problemQuery.ts`, `.spec.ts`
- Modify: `apps/web/src/api/problems.ts`

**Interfaces:**
- `DIFFICULTY_BANDS` 五档；`difficultyBand(d): number`；`difficultyLabel(d): string`；`difficultyText(d): string`。
- `ProblemQuery` 改为 `difficultyMin/difficultyMax`。
- `AppDifficultyTag` props `{ difficulty: number; showLabel?: boolean }`。

- [ ] **Step 1: 写失败测试**

```ts
it('maps 1-100 to five bands', () => {
  expect(difficultyBand(1)).toBe(1)
  expect(difficultyBand(20)).toBe(1)
  expect(difficultyBand(21)).toBe(2)
  expect(difficultyBand(62)).toBe(4)
  expect(difficultyBand(100)).toBe(5)
  expect(difficultyText(62)).toBe('62 · 省选')
})

it('renders the score with band color', () => {
  const wrapper = mount(AppDifficultyTag, { props: { difficulty: 62 } })
  expect(wrapper.text()).toContain('62')
  expect(wrapper.classes()).toContain('app-difficulty-tag--4')
})

it('parses range query', () => {
  expect(parseProblemQuery({ difficultyMin: '40', difficultyMax: '80' })).toMatchObject({
    difficultyMin: 40, difficultyMax: 80,
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/utils/difficulty.spec.ts src/components/ui/AppDifficultyTag.spec.ts src/utils/problemQuery.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现工具、标签、筛选**

- 五档区间按设计文档；非法值返回 `0`/“未知”。
- `AppDifficultyTag` 按 band 加 class，显示数字；`showLabel` 为 true 时显示 `difficultyText`。
- `ProblemListView` 用两个 `InputNumber`（最低/最高）替换原 `Select`，保留 `canManageOwn`、搜索/重置/URL 同步；列表标签显示数字。
- `problemQuery` 解析/序列化 `difficultyMin/difficultyMax`，范围 1–100，min>max 时交换。
- `api/problems.ts` 查询参数增加 min/max。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web exec vitest run src/utils/difficulty.spec.ts src/components/ui/AppDifficultyTag.spec.ts src/utils/problemQuery.spec.ts src/views/ProblemListView.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/utils apps/web/src/components/ui/AppDifficultyTag.vue apps/web/src/components/ui/AppDifficultyTag.spec.ts apps/web/src/views/ProblemListView.vue apps/web/src/views/ProblemListView.spec.ts apps/web/src/api/problems.ts
git commit -m "feat(web): show 1-100 difficulty and range filter"
```

---

### Task 14: 前端题目编辑、重评与历史

**Files:**
- Modify: `apps/web/src/views/ProblemEditView.vue`, `.spec.ts`
- Modify: `apps/web/src/views/ProblemDetailView.vue`, `.spec.ts`
- Modify: `apps/web/src/views/SubmitView.vue`（仅 Tag 自动生效）
- Modify: `apps/web/src/components/learning/LearningNodePanel.vue`
- Modify: `apps/web/src/api/problems.ts`

**Interfaces:**
- `ProblemEditView` 表单增加 `difficultyAuto: boolean`，默认 true；`ProblemInput` 发送 `difficultyAuto`。
- API：
  ```ts
  assessProblemDifficulty(id): Promise<ProblemDetail>
  getProblemDifficultyHistory(id, page, pageSize): Promise<Page<DifficultyHistoryItem>>
  ```

- [ ] **Step 1: 写失败测试**

```ts
it('disables difficulty input while auto assessment is on', async () => {
  const { wrapper } = await mountCreate()
  expect(wrapper.find('[data-test="difficulty-input"] input').attributes('disabled')).toBeDefined()
})

it('unlocks difficulty when auto adjustment is turned off', async () => {
  // 切换 ToggleSwitch；断言输入可编辑，且保存 payload difficultyAuto=false
})

it('calls AI reassessment and updates the score', async () => {
  // mock assessProblemDifficulty -> { difficulty: 72, ... }
  // 点击“AI 重新评估”，确认 Dialog，断言显示 72
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/ProblemEditView.spec.ts src/views/ProblemDetailView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现编辑页/详情页**

- `FormState`/`emptyForm`/`fill` 增加 `difficultyAuto`；新建默认 true；`fill` 用 `p.difficultyAuto ?? true`。
- 难度行：`InputNumber` 1–100、`ToggleSwitch` 自动调整、来源 `Tag`、`difficultyUpdatedAt`/reason 文案；自动时输入禁用。
- 人工改分值自动把 `difficultyAuto=false`；重新打开时保留当前分作为先验（提交 `difficultyAuto=true`）。
- 编辑模式显示“AI 重新评估”和“查看难度历史”按钮；重评前 `useConfirm` 确认，成功 `fill(updated)`；历史用 `Dialog` + `DataTable`。
- 详情页/提交页使用 `AppDifficultyTag` 的 `showLabel`；学习路线显示 `difficultyText`。
- 更新 `ProblemEditView.spec.ts` 的 `difficulty` fixture 为 1–100 合法值。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web exec vitest run src/views/ProblemEditView.spec.ts src/views/ProblemDetailView.spec.ts src/views/SubmitView.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/ProblemEditView.vue apps/web/src/views/ProblemEditView.spec.ts apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts apps/web/src/views/SubmitView.vue apps/web/src/components/learning/LearningNodePanel.vue apps/web/src/api/problems.ts
git commit -m "feat(web): add difficulty auto mode, AI reassessment and history"
```

---

### Task 15: 前端 AI 设置平台模型

**Files:**
- Modify: `apps/web/src/views/AdminAiView.vue`, `.spec.ts`
- Modify: `apps/web/src/api/admin.ts`

**Interfaces:**
- API functions:
  ```ts
  listAdminAIPlatformModels()
  createAdminAIPlatformModel(input)
  updateAdminAIPlatformModel(id, input)
  deleteAdminAIPlatformModel(id)
  testAdminAIPlatformModel(id)
  selectAdminAIPlatformModel(modelId: number | null)
  ```

- [ ] **Step 1: 写失败测试**

```ts
it('lists platform models and selects the current one', async () => {
  // mock list -> 两个模型；渲染表格；选择第二个并保存；断言 select API 调用
})

it('disables adding models when the encryption key is missing', async () => {
  // getAdminAISettings -> keyConfigured=false；断言警告和禁用
})

it('tests a model and shows the result', async () => {
  // mock test -> {ok:true, latencyMs:812}；断言 Toast/Tag
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AdminAiView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现平台模型板块**

- 在现有“功能开关”“加密密钥”之间/之后新增“平台模型”`AppSurface`。
- `Select` 当前模型 + 保存；`DataTable` 列表；新增/编辑 `Dialog`（名称/接口地址/模型名/API Key/启用）；测试/删除按钮；选中模型删除禁用。
- API 客户端按接口添加；错误 Toast 中文；无密钥禁用新增并提示。
- 现有测试的 `getAdminAISettings` fixture 增加 `platformModelId: null`、`platformModelName: ''`。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web exec vitest run src/views/AdminAiView.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminAiView.vue apps/web/src/views/AdminAiView.spec.ts apps/web/src/api/admin.ts
git commit -m "feat(web): manage platform AI models"
```

---

### Task 16: 前端定时任务页、导航与权限

**Files:**
- Create: `apps/web/src/views/AdminJobsView.vue`, `.spec.ts`
- Modify: `apps/web/src/router/index.ts`, `apps/web/src/components/layout/AdminNav.vue`, `.spec.ts`
- Modify: `apps/web/src/permissions.ts`, `.spec.ts`
- Modify: `apps/web/src/api/admin.ts`

**Interfaces:**
- API functions:
  ```ts
  listAdminJobs()
  updateAdminJob(key, input)
  runAdminJob(key)
  listAdminJobRuns(query)
  getAdminJobRun(id)
  getAdminDifficultySettings()
  updateAdminDifficultySettings(input)
  ```
- `PERM.system.jobs.manage`。

- [ ] **Step 1: 写失败测试**

```ts
it('renders the three job tabs and the difficulty job', async () => {
  const { wrapper } = await mountPage()
  expect(wrapper.text()).toContain('定时任务')
  expect(wrapper.text()).toContain('全站题目难度重算')
})

it('runs the difficulty job and shows the run status', async () => {
  // mock runAdminJob -> queued run；点击“立即执行”；断言 202 结果渲染
})

it('shows the rating placeholder', async () => {
  expect(wrapper.text()).toContain('规划中')
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AdminJobsView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现页面与路由**

- 新页面三个 Tab：任务列表/题目难度/运行历史；任务卡片、cron 编辑 Dialog、立即执行、历史表格、运行中轮询 2–3 秒。
- 难度设置表单按 `DifficultySettingsView`；保存 Toast；立即重算二次确认。
- 路由 `/admin/jobs`，权限 `system.jobs.manage`；AdminNav 系统组新增“定时任务”（`pi pi-clock`）。
- `permissions.ts`/`ADMIN_ACCESS_PERMISSIONS`/spec 同步。
- API 客户端按接口添加。

- [ ] **Step 4: 运行测试确认通过**

Run: `corepack pnpm --filter web exec vitest run src/views/AdminJobsView.spec.ts src/components/layout/AdminNav.spec.ts src/permissions.spec.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminJobsView.vue apps/web/src/views/AdminJobsView.spec.ts apps/web/src/router/index.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/permissions.ts apps/web/src/permissions.spec.ts apps/web/src/api/admin.ts
git commit -m "feat(web): add scheduled jobs admin page"
```

---

### Task 17: 题包/HydroOJ 兼容、文档与全量验证

**Files:**
- Modify: `apps/server/internal/problem/package.go`, `package_import.go`, `package_export.go`, `import_hydro.go`
- Test: `apps/server/internal/problem/package_import_test.go`, `package_export_test.go`, `import_hydro_test.go`
- Modify: `DEVELOPMENT.md`, `docs/题库批量管理与题包.md`, `docs/任务清单.md`, `docs/开发日志.md`, `README.md`/`运行手册.md` 相关段落

**Interfaces:**
- `PackageProblem` 增加 `DifficultyScore *int json:"difficultyScore,omitempty"`、`DifficultyScale string json:"difficultyScale,omitempty"`；`Difficulty` 保持 1–5。
- 导入 helper：`resolvePackageDifficulty(manifest PackageProblem) (int, error)`。

- [ ] **Step 1: 写失败测试**

```go
func TestPackageExportDualDifficultyFields(t *testing.T) {
    // 题目 difficulty=62；导出 manifest：difficulty=4、difficultyScore=62、difficultyScale="1-100"。
}

func TestPackageImportOldScale(t *testing.T) {
    // 旧 manifest difficulty=3 无 score；导入后题目 difficulty=50、prior_source=import、history source=import。
}

func TestLegacyPackageImportCompatibility(t *testing.T) {
    // 断言新导出的 difficulty 字段始终 1..5，旧版 Input.Normalize 可接受。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/problem -run 'TestPackageExportDual|TestPackageImportOldScale|TestLegacyPackage' -v`
Expected: FAIL。

- [ ] **Step 3: 实现题包兼容**

- 导出：按精确分计算 legacy band（1–20→1、21–40→2、41–60→3、61–80→4、81–100→5）；写 `difficultyScore`/`difficultyScale`。
- 导入：有 `difficultyScore` 用它；否则按 `difficulty` 1–5 映射；未知 `difficultyScale` 返回 40001。
- 导入/覆盖调用 `CreateWithDifficulty`/`UpdateWithDifficulty`，`Source=import`、`PriorSource=import`、`Auto=true`，写历史（`old_score` 覆盖前分数，`created_by` 导入者）。
- Hydro 导入默认 50，`Source=import`/`PriorSource=default`/`Auto=true`，写历史。
- 更新文档：DEVELOPMENT、题包文档、任务清单、开发日志、README/运行手册相关段落。

- [ ] **Step 4: 全量验证**

Run:
```bash
cd apps/server && go test ./...
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```
Expected: 全部 PASS。真实冒烟按设计文档 §10.3 用本地假 OpenAI 服务执行；清理临时数据。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem/package.go apps/server/internal/problem/package_import.go apps/server/internal/problem/package_export.go apps/server/internal/problem/import_hydro.go apps/server/internal/problem/package_import_test.go apps/server/internal/problem/package_export_test.go apps/server/internal/problem/import_hydro_test.go DEVELOPMENT.md docs/题库批量管理与题包.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "feat(problem): support 1-100 difficulty in packages"
```

（全量验证中发现的其他修复按所属任务单独提交，不混入本提交。）

---

## Self-Review

**Spec coverage:**
- 1–100 刻度/展示/筛选：Task 3/4/8/13/14。
- 公式 B、先验、min_sample、step、窗口、比赛过滤：Task 1/10。
- AI 初评、失败回退、手动重评、历史：Task 6/8/14。
- 平台模型多模型、token 加密、选择、测试、轮换/reset：Task 5/7/15。
- 通用定时任务、cron、立即执行、历史、并发：Task 9/11/16。
- 权限/审计：Task 7/11。
- 迁移/旧数据/题包/Hydro：Task 3/17。
- OpenAPI/类型：Task 12。
- 测试/验收/文档：各任务 + Task 17。

**Type consistency:** `DifficultyDecision` 在 Task 4 定义，Task 8/17 使用；`DifficultyHistory` 在 Task 3 定义，Task 4/10/17 使用；`jobs.Definition/Reporter/Run` 在 Task 9 定义，Task 10/11 使用；`ai.PlatformService` 在 Task 5 定义，Task 7/11 使用。

**Review Focus tests:** 小样本/人工锁定（Task 10）、AI 失败回退（Task 8）、题包兼容（Task 17）、密钥轮换/reset（Task 5）。

**Proportion:** 计划按任务给出签名、测试断言和命令；实现体留给执行者。若执行中发现接口与现有测试冲突，按现有代码模式调整并在计划中更新。

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-11-17-problem-difficulty-ai-jobs.md`.
