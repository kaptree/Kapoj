# 客观题（答案题）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 `objective` 题型：题目编辑可配置多个可接受答案；用户直接提交文本答案；judge 跳过沙盒按 trim+忽略大小写判定；正确答案仅题目编辑者可见。

**Architecture:** `problems.answers_json` 保存可接受答案，`submissions.answer` 保存用户答案；后端按题型做提交校验，judge `Run` 在沙盒分支前处理客观题；前端题目编辑/详情/提交/测评记录按题型切换 UI。提交详情中的答案使用独立可见性，只有本人或 `CanViewLiveContest` 管理员可读。

**Tech Stack:** Go + Gin + GORM + PostgreSQL；judge（独立 Go 模块 + pgx）；Vue 3 + PrimeVue 4；OpenAPI 生成前端类型。

**Spec:** `docs/superpowers/specs/2026-11-09-objective-problem-design.md`

## Global Constraints

- 新增题型值固定为 `objective`；非客观题行为不变。
- 正确答案仅 `problem.update`/super 的题目编辑者可见；不沿用包含 testcase/checker 管理的 `CanViewSolution`，不因 AC 放开。
- 用户提交的答案只对提交者本人或 `CanViewLiveContest` 管理员可见；其他用户任何阶段都不可见。
- 答案匹配：`EqualFold(TrimSpace(user), TrimSpace(accepted))`；每题 1–20 个可接受答案，每个 1–2000 字符。
- 客观题不要求测试点、不编译、不跑 sandbox；AC 固定 100 分，耗时/内存 0，judge cases 为空。
- 代码题仍要求语言+代码、至少 1 个测试点；特判题仍要求 checker。
- 客观题提交使用 `language=answer`、`code=""`、`answer=<用户答案>`；前端代码语言列表不加入 `answer`。
- `contest_problems.score` 不参与现有榜单计分，客观题 AC 固定 100；不新增按题自定义计分。
- 比赛/锁定/报名/提交冷却/SSE/AC 事件/学习路线/成就任务全部沿用现有链路。
- 不新增权限；中文错误文案、审计和现有 UI 风格保持一致。

## Review Focus

- 正确答案不能通过公开题目详情、AC 用户详情、比赛页面或测评记录泄露（Task 1、Task 2、Task 6）。
- 客观题无测试点必须能发布/提交/评测，代码题和特判题原有发布与评测约束不能被放宽（Task 1、Task 2、Task 3）。
- 代码题提交不能被 `answer` 字段绕过，客观题也不能误要求 language/code（Task 2）。
- judge 客观题分支必须不调用 sandbox，同时保留 AC 计数与游戏化事件（Task 3）。
- 前端题型切换后答案/测试点/代码编辑器状态不能串型（Task 5、Task 6）。

---

### Task 1: 后端题目类型、答案存储、发布与答案可见性

**Files:**
- Modify: `apps/server/internal/problem/model.go`
- Modify: `apps/server/internal/problem/validate.go`
- Modify: `apps/server/internal/problem/dto.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/contest/dto.go`
- Test: `apps/server/internal/problem/validate_test.go`
- Test: `apps/server/internal/problem/service_test.go`
- Test: `apps/server/internal/problem/repository_test.go`
- Test: `apps/server/internal/problemapi/handler_test.go`
- Test: `apps/server/internal/contest/service_test.go`

**Interfaces:**
- Produces:
  - `problem.TypeObjective Type = "objective"`；`Type.Valid()` 包含它
  - `Problem.Answers dbtypes.List[string]`（`gorm:"column:answers_json;type:jsonb;not null;default:'[]'"`）
  - `problem.Input.Answers []string`（`json:"answers"`）
  - `DetailDTO.Answers *[]string`（`json:"answers,omitempty"`）
  - `ToDetail(p *Problem, includeSolution, includeAnswer bool, author *AuthorSummary) DetailDTO`
  - `contest.ProblemRow.ProblemType string`；`contest.ProblemItem.Type problem.Type`（`json:"type"`）

- [ ] **Step 1: Write failing tests**

在 `problem/validate_test.go` 添加：

```go
func TestObjectiveAnswersNormalize(t *testing.T) {
	in := input("objective")
	in.Type = TypeObjective
	in.Answers = []string{"  A  ", "b", " C "}
	got, err := in.Normalize()
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "b", "C"}, got.Answers)

	in.Answers = []string{"A", " a "}
	_, err = in.Normalize()
	assertCode(t, err, 40001)
}

func TestNonObjectiveClearsAnswers(t *testing.T) {
	in := input("traditional")
	in.Answers = []string{"A"}
	got, err := in.Normalize()
	require.NoError(t, err)
	assert.Empty(t, got.Answers)
}
```

在 `problem/service_test.go` 添加：

```go
func TestObjectivePublishesWithoutTestcase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := input("objective")
	in.Type = problem.TypeObjective
	in.Answers = []string{"A", "B"}
	in.IsPublic = true
	p, err := e.svc.Create(ctx, 1, in)
	require.NoError(t, err)
	// 新题创建时 isPublic 被忽略；通过 Update 发布
	in.IsPublic = true
	_, err = e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)

	got, err := e.svc.Get(ctx, problem.Viewer{}, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, []string(got.Answers))
}
```

并在 `TestPublishRequiresTestcase` 旁加一个断言：传统题无测试点仍报 40001（已有测试保留）。

在 `problem/dto_test.go` 或 `problemapi/handler_test.go` 添加答案可见性测试：

```go
func TestObjectiveAnswersOnlyForEditors(t *testing.T) {
	f := newFx(t)
	id := f.createObjectiveProblem(t, "objective-answer", []string{"A", "B"})
	admin := f.token("problem_admin")

	w, resp := f.do(http.MethodGet, path("/api/problems/%d", id), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, resp["data"].(map[string]any), "answers")

	w, resp = f.do(http.MethodGet, path("/api/problems/%d", id), admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []any{"A", "B"}, resp["data"].(map[string]any)["answers"])
}
```

在 `contest/service_test.go` 添加题目列表 `type` 字段断言：

```go
detail, err := e.svc.Detail(ctx, user(), c.ID)
require.NoError(t, err)
assert.Equal(t, problem.TypeObjective, detail.Problems[0].Type)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 -count=1 ./internal/problem/ ./internal/problemapi/ ./internal/contest/`
Expected: FAIL，`TypeObjective`/`Answers`/`includeAnswer` 未定义。

- [ ] **Step 3: Implement**

`problem/model.go`：

```go
const (
	TypeTraditional Type = "traditional"
	TypeSpecial     Type = "special"
	TypeInteractive Type = "interactive"
	TypeObjective   Type = "objective"
)

func (t Type) Valid() bool {
	return t == TypeTraditional || t == TypeSpecial || t == TypeInteractive || t == TypeObjective
}

type Problem struct {
	...
	Answers dbtypes.List[string] `gorm:"column:answers_json;type:jsonb;not null;default:'[]'"`
}
```

`problem/validate.go`：

```go
type Input struct {
	...
	Answers []string `json:"answers"`
}
```

常量：

```go
const (
	maxObjectiveAnswers = 20
	maxAnswerChars      = 2000
)
```

`Normalize`：

- 类型错误文案改为 `traditional、special、interactive 或 objective`。
- `if in.Type == TypeObjective { normalizeObjectiveAnswers } else { in.Answers = []string{} }`。
- `normalizeObjectiveAnswers`：trim；长度 1–2000 runes；拒绝控制字符；按 `strings.EqualFold` 去重；1–20 条；空列表报错。

`problem/service.go`：

- `apply` 增加 `p.Answers = in.Answers`。
- `UploadChecker` 仍只允许 special；objective 返回“只有特判题可以上传 checker”。

`problem/repository.go`：

- `editableColumns` 增加 `"answers_json"`。
- `Update` 发布校验改为：

```go
if p.IsPublic {
	if p.Type != TypeObjective {
		var n int64
		if err := tx.Model(&Testcase{}).Where("problem_id = ?", p.ID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 { return ErrPublishNeedsTestcase }
	}
	if p.Type == TypeSpecial && current.CheckerPath == "" {
		return ErrPublishNeedsChecker
	}
}
```

- `DeleteTestcase` 的“公开题至少保留 1 个测试点”守卫在 `p.Type == TypeObjective` 时跳过。

`problem/dto.go`：

```go
type DetailDTO struct {
	...
	Answers *[]string `json:"answers,omitempty"`
}

func ToDetail(p *Problem, includeSolution, includeAnswer bool, author *AuthorSummary) DetailDTO {
	...
	if includeAnswer {
		answers := []string(p.Answers)
		d.Answers = &answers
	}
	...
}
```

`problemapi/handler.go`：

- `viewer(c)` 已能取 RBAC；在 `get` 中计算：

```go
includeAnswer := false
if access, ok := middleware.CurrentAccess(c); ok && access != nil {
	includeAnswer = access.IsSuper || access.Has(string(rbac.PermProblemUpdate))
}
```

- `problem.ToDetail(p, includeSolution, includeAnswer, author)`。
- 其他调用 `ToDetail` 的地方（create/update 返回）传 `true, true` 或按需；创建/更新响应只有管理员能调用。

`contest/repository.go`：

- `ProblemRow` 增加 `ProblemType string gorm:"column:problem_type"`。
- `FindProblems` select `p.type AS problem_type`。

`contest/dto.go`：

- `ProblemItem` 增加 `Type problem.Type json:"type"`；`toProblemItem` 填入 `problem.Type(row.ProblemType)`。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 -count=1 ./internal/problem/ ./internal/problemapi/ ./internal/contest/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/server/internal/contest
git commit -m "feat(problem): add objective answer type"
```

---

### Task 2: 后端提交协议、答案可见性与类型校验

**Files:**
- Modify: `apps/server/internal/submission/model.go`
- Modify: `apps/server/internal/submission/validate.go`
- Modify: `apps/server/internal/submission/service.go`
- Modify: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/submission/status.go`
- Test: `apps/server/internal/submission/validate_test.go`
- Test: `apps/server/internal/submission/service_test.go`
- Test: `apps/server/internal/submission/records_test.go`

**Interfaces:**
- Produces:
  - `Submission.Answer string`（`gorm:"type:text;not null;default:''"`）
  - `LanguageAnswer Language = "answer"`；`Language.Valid()` 包含
  - `Input.Answer string`（`json:"answer"`）
  - `Input.Normalize()` 只做公共字段校验
  - `Input.ValidateForProblem(t problem.Type) (Input, error)`
  - `Detail.Answer string`（`json:"answer,omitempty"`）
  - `Detail.AnswerHidden bool`（`json:"answerHidden"`）

- [ ] **Step 1: Write failing tests**

`submission/validate_test.go` 改为按题型校验：

```go
func TestInputValidateForObjective(t *testing.T) {
	in := submission.Input{ProblemID: 1, Answer: "  A  "}
	got, err := in.Normalize()
	require.NoError(t, err)
	got, err = got.ValidateForProblem(problem.TypeObjective)
	require.NoError(t, err)
	assert.Equal(t, "A", got.Answer)
	assert.Equal(t, submission.LanguageAnswer, got.Language)
	assert.Empty(t, got.Code)
}

func TestObjectiveRejectsEmptyAnswer(t *testing.T) {
	in := submission.Input{ProblemID: 1, Answer: "  "}
	in, err := in.Normalize()
	require.NoError(t, err)
	_, err = in.ValidateForProblem(problem.TypeObjective)
	assertCode(t, err, 40001)
}
```

代码题测试改为 `in.ValidateForProblem(problem.TypeTraditional)` 并保留原断言。

`submission/service_test.go` 添加：

```go
func TestCreateObjectiveSubmission(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	createUser(t, e.db, 2)
	p := e.createProblem(t, &problem.Problem{
		Title: "Objective", Slug: "objective", StatementMD: "s",
		Type: problem.TypeObjective, Answers: dbtypes.List[string]{"A", "B"},
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true, CreatedBy: 1,
	}, 0)

	sub, err := e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, submission.Input{
		ProblemID: p.ID, Answer: " a ",
	})
	require.NoError(t, err)
	assert.Equal(t, submission.LanguageAnswer, sub.Language)
	assert.Equal(t, "a", sub.Answer)
	assert.Empty(t, sub.Code)
	assert.Contains(t, e.queue.ids, sub.ID)
}
```

`records_test.go` 添加答案可见性测试：本人详情 `answer` 存在；其他用户 `answer=""`、`answerHidden=true`；`CanViewLiveContest` 管理员可读。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 -count=1 ./internal/submission/`
Expected: FAIL，`Answer`/`ValidateForProblem` 未定义。

- [ ] **Step 3: Implement**

`status.go`：

```go
const LanguageAnswer Language = "answer"

func (l Language) Valid() bool {
	switch l {
	case LanguageC, LanguageCPP, LanguagePython3, LanguageJava, LanguageGo, LanguageAnswer:
		return true
	default:
		return false
	}
}
```

`model.go`：

```go
Answer string `gorm:"type:text;not null;default:''"`
```

`validate.go`：

```go
const maxAnswerChars = 2000

type Input struct {
	ProblemID int64    `json:"problemId"`
	ContestID int64    `json:"contestId"`
	Language  Language `json:"language"`
	Code      string   `json:"code"`
	Answer    string   `json:"answer"`
}

func (in Input) Normalize() (Input, error) {
	if in.ProblemID <= 0 { return in, httpx.ErrValidation("problemId 必须是正整数") }
	if in.ContestID < 0 { return in, httpx.ErrValidation("contestId 不能为负数") }
	return in, nil
}

func (in Input) ValidateForProblem(t problem.Type) (Input, error) {
	if t == problem.TypeObjective {
		in.Answer = strings.TrimSpace(in.Answer)
		if in.Answer == "" { return in, httpx.ErrValidation("答案不能为空") }
		if utf8.RuneCountInString(in.Answer) > maxAnswerChars {
			return in, httpx.ErrValidation("答案不能超过 2000 个字符")
		}
		in.Language, in.Code = LanguageAnswer, ""
		return in, nil
	}
	if !in.Language.Valid() { return in, httpx.ErrValidation("语言必须是 c、cpp、python3、java 或 go") }
	if strings.TrimSpace(in.Code) == "" { return in, httpx.ErrValidation("代码不能为空") }
	if len(in.Code) > maxCodeBytes { return in, httpx.ErrValidation("代码不能超过 64 KiB") }
	in.Answer = ""
	return in, nil
}
```

`service.go`：

```go
in, err := in.Normalize()
...
p, err := s.problems.FindByID(ctx, in.ProblemID)
...
in, err = in.ValidateForProblem(p.Type)
if err != nil { return nil, err }
```

- 客观题跳过 interactive/checker/testcase 检查；代码题保留：
```go
if p.Type == problem.TypeObjective {
	// no testcase/checker requirements
} else {
	if p.Type == problem.TypeInteractive { return nil, ... }
	if p.Type == problem.TypeSpecial && p.CheckerPath == "" { return nil, ... }
	n, err := s.problems.CountTestcases(ctx, p.ID)
	...
}
```
- 创建：

```go
sub := &Submission{
	UserID: userID, ProblemID: p.ID, ContestID: contestID,
	Language: in.Language, Code: in.Code, Answer: in.Answer,
	Status: StatusPending, JudgeResult: dbtypes.List[JudgeCaseResult]{},
}
```

`dto.go`：

```go
type Detail struct {
	ListItem
	Code         string `json:"code,omitempty"`
	CodeHidden   bool   `json:"codeHidden"`
	Answer       string `json:"answer,omitempty"`
	AnswerHidden bool   `json:"answerHidden"`
	...
}
```

`Service.Detail`：

```go
detail.Answer = row.Answer
if row.Language == LanguageAnswer && v.UserID != row.UserID && !v.CanViewLiveContest {
	detail.Answer = ""
	detail.AnswerHidden = true
}
```

`contestProtected` 仍清空 code/judge 字段；答案的隐藏独立于比赛状态，放在 contestProtected 前后均可，但必须对非本人/非 live-admin 始终生效。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 -count=1 ./internal/submission/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/submission
git commit -m "feat(submission): support objective answers"
```

---

### Task 3: judge 客观题判定

**Files:**
- Modify: `apps/judge/internal/model/model.go`
- Modify: `apps/judge/internal/db/repository.go`
- Modify: `apps/judge/internal/pipeline/pipeline.go`
- Modify: `apps/judge/internal/testutil/pg.go`
- Test: `apps/judge/internal/pipeline/pipeline_test.go`

**Interfaces:**
- Produces:
  - `model.Submission.Answer string`
  - `model.Problem.Answers []string`
  - `pipeline.Pipeline.Run` 对 `prob.Type == "objective"` 不进入 sandbox
  - `testutil.InsertObjectiveProblem(t, pool, answers []string) int64`

- [ ] **Step 1: Write failing tests**

在 `pipeline_test.go` 添加：

```go
func TestObjectiveSubmission(t *testing.T) {
	e := newEnv(t)
	problem := testutil.InsertObjectiveProblem(t, e.pool, []string{"A", "hello world"})
	user := testutil.InsertUser(t, e.pool)

	sub := testutil.InsertSubmission(t, e.pool, user, problem, "answer", "", "pending")
	// 需要一个答案字段；testutil 增加 InsertObjectiveSubmission 或直接在测试中 UPDATE
	require.NoError(t, e.pool.Exec(context.Background(),
		`UPDATE submissions SET answer = $1 WHERE id = $2`, " a ", sub).Err())

	require.NoError(t, e.pipeline.Run(context.Background(), sub))
	status, score := submissionResult(t, e.pool, sub)
	assert.Equal(t, "accepted", status)
	assert.Equal(t, 100, score)
	assert.Equal(t, 1, acceptCount(t, e.pool, problem))
}

func TestObjectiveSubmissionWrongAnswer(t *testing.T) {
	// 同样创建 objective，提交 "B"，断言 wrong_answer/0，accept_count 不变
}
```

`testutil` 的 `submissions` 表需加 `answer text not null default ''`；`problems` 表需加 `answers_json jsonb not null default '[]'`。`InsertSubmission` 保持原签名，新增：

```go
func InsertObjectiveSubmission(t *testing.T, pool *pgxpool.Pool, userID, problemID int64, answer, status string) int64
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/pipeline/`
Expected: FAIL，无 objective 支持/无 answers 字段。

- [ ] **Step 3: Implement**

`model.go`：

```go
type Submission struct {
	...
	Language Language
	Code     string
	Answer   string
	...
}

type Problem struct {
	...
	Answers []string
}
```

`db/repository.go` `Claim`：

```sql
RETURNING id, user_id, problem_id, language, code, answer, status
```

```sql
SELECT id, type, time_limit_ms, memory_limit_mb, checker_path, answers_json
FROM problems WHERE id = $1
```

用 `var answersJSON []byte` 扫描，`json.Unmarshal` 到 `p.Answers`（空/NULL 视为空数组）。

`pipeline.Run`：

```go
p.publish(ctx, sub.ID)

if prob.Type == "objective" {
	accepted := false
	user := strings.TrimSpace(sub.Answer)
	for _, answer := range prob.Answers {
		if strings.EqualFold(user, strings.TrimSpace(answer)) {
			accepted = true
			break
		}
	}
	status := model.StatusWrongAnswer
	score := 0
	if accepted {
		status, score = model.StatusAccepted, 100
	}
	return p.finish(ctx, sub, db.FinishInput{
		Status: status, Score: score, Cases: []model.JudgeCaseResult{},
	})
}

if len(cases) == 0 {
	return errors.New("problem has no testcases")
}
if !sub.Language.Valid() {
	return fmt.Errorf("unsupported language %q", sub.Language)
}
```

注意：`p.publish` 目前位于 cases/language 检查之后；objective 分支必须提前 publish（或在分支内 publish），保证前端看到 judging。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/pipeline/ ./internal/db/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/judge/internal/model apps/judge/internal/db apps/judge/internal/pipeline apps/judge/internal/testutil
git commit -m "feat(judge): judge objective answers without sandbox"
```

---

### Task 4: OpenAPI 契约、生成类型与前端类型

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`

**Interfaces:**
- Produces: `ProblemType` 含 objective；`ProblemInput.answers`；`ProblemDetail.answers`；`SubmissionLanguage` 含 answer；`CreateSubmissionRequest` 支持 answer 且 language/code 可选；`SubmissionDetail.answer`/`answerHidden`；`ContestProblemItem.type`。

- [ ] **Step 1: Update OpenAPI**

- `ProblemType.enum` 增加 `objective`。
- `ProblemInput.properties.answers`: array maxItems 20, items string maxLength 2000。
- `ProblemUpdateInput` required 列表是否包含 answers？`PUT` 是整体替换，答案也应提交；但旧客户端可能不传。为兼容，`answers` 保持可选；后端非客观题清空、客观题缺省报错。若希望编辑页总是提交，可选。保持 optional。
- `ProblemDetail.properties.answers`: array of string, description “仅题目编辑者（problem.update）可见”。
- `SubmissionLanguage.enum` 增加 `answer`。
- `CreateSubmissionRequest.required` 从 `[problemId, language, code]` 改为 `[problemId]`；`language`/`code` 可选；新增 `answer` maxLength 2000，描述“客观题必填；代码题忽略”。
- `SubmissionDetail.properties.answer`（可选）、`answerHidden`（required boolean）。
- `ContestProblemItem.required` 加 `type`；properties 加 `type: $ref ProblemType`。

- [ ] **Step 2: Regenerate and typecheck**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm --filter web exec vue-tsc -b --force`
Expected: 生成成功；现有页面可能因 `CreateSubmissionRequest` 可选字段/`ProblemType` 枚举新增而报 fixture 类型错误，留到 Task 5/6 修复。

- [ ] **Step 3: Update `types.ts` aliases if needed**

通常已有 `ProblemType`、`ProblemInput`、`ProblemDetail`、`SubmissionLanguage`、`CreateSubmissionRequest`、`SubmissionDetail`、`ContestProblemItem` 别名，确认无需新增。

- [ ] **Step 4: Commit**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts
git commit -m "docs(api): add objective problem contract"
```

---

### Task 5: 前端题目编辑、详情与比赛列表适配

**Files:**
- Modify: `apps/web/src/views/ProblemEditView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/views/ContestDetailView.vue`
- Test: `apps/web/src/views/ProblemEditView.spec.ts`
- Test: `apps/web/src/views/ProblemDetailView.spec.ts`
- Test: `apps/web/src/views/ContestDetailView.spec.ts`

**Interfaces:**
- Consumes: `ProblemType`/`ProblemInput`/`ProblemDetail`/`ContestProblemItem.type` from Task 4.
- Produces: objective type option, answers editor, conditional testcase/AI/checker, objective meta display, contest objective tag.

- [ ] **Step 1: Write failing tests**

`ProblemEditView.spec.ts`：

```ts
it('supports objective answers and hides testcase manager', async () => {
  vi.mocked(getProblem).mockResolvedValue(problem({
    type: 'objective',
    answers: ['A', 'B'],
  }))
  const wrapper = await mountEdit()
  expect(wrapper.find('[data-test="objective-answers"]').exists()).toBe(true)
  expect(wrapper.find('.testcases').exists()).toBe(false)
})
```

并测试保存 payload：`answers: ['A','B']`、`type: 'objective'`。

`ProblemDetailView.spec.ts`：

```ts
it('shows objective label and no time/memory limits', async () => {
  vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
  const wrapper = await mountView()
  expect(wrapper.text()).toContain('客观题')
  expect(wrapper.text()).toContain('提交答案')
  expect(wrapper.text()).not.toContain('1000 ms')
  expect(wrapper.text()).not.toContain('256 MB')
  expect(wrapper.text()).not.toContain('answers')
})
```

`ContestDetailView.spec.ts`：objective problem item renders `Tag` “客观题” and not `1000 ms`/`256 MB`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ProblemEditView.spec.ts src/views/ProblemDetailView.spec.ts src/views/ContestDetailView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: Implement**

`ProblemEditView.vue`：

- `TYPE_OPTIONS` 加 `{ label: '客观题（直接提交答案）', value: 'objective' }`。
- `FormState` 加 `answers: string[]`；`emptyForm` 加 `answers: []`；`fill` 加 `answers: [...(p.answers ?? [])]`。
- 增加 `answersText = ref('')`；`fill` 时 `answersText.value = form.answers.join('\n')`；当 type 切换时，如果从 objective 切走，清空 `answersText`/`answers`（可用 watch 或 `onTypeChange`）。
- 模板在类型 Select 下加：

```vue
<AppFormField v-if="form.type === 'objective'" label="正确答案" required
  hint="每行一个可接受答案；判定时忽略首尾空白和大小写">
  <Textarea v-model="answersText" data-test="objective-answers" rows="6" fluid />
</AppFormField>
```

- `save` 时：

```ts
const answers = form.type === 'objective'
  ? answersText.value.split(/\r?\n/).map(s => s.trim()).filter(Boolean)
  : []
input.answers = answers
```

- 时间/空间字段用 `v-if="form.type !== 'objective'"` 包裹（如果模板拆成多个 `AppFormField`，逐个加）。
- `AppSurface v-if="isEdit" class="testcases"` 改为 `v-if="isEdit && form.type !== 'objective'"`。
- `TestcaseManager` 的 AI 造数据入口随之隐藏；如 checker 上传在 `TestcaseManager` 内，隐藏整个组件即可。
- 发布提示文案：objective 时显示“客观题无需测试点；发布前请确认至少 1 个正确答案。”，其他类型保留原文案。

`ProblemDetailView.vue`：

- `typeLabels` 加 `objective: '客观题'`。
- 时间为/空间 meta 加 `v-if="problem?.type !== 'objective'"`。
- 客观题时显示 `<Tag value="直接作答" severity="secondary" />`。
- 提交按钮 `:label="problem?.type === 'objective' ? '提交答案' : '提交代码'"`。
- 不渲染 `answers`（API 对普通用户也不会返回）。

`ContestDetailView.vue`：

- 限制列：`data.type === 'objective'` 时显示 `<Tag value="客观题" severity="secondary" />`，否则现有 ms/MB tags。
- 标题/统计/跳转逻辑不变。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ProblemEditView.spec.ts src/views/ProblemDetailView.spec.ts src/views/ContestDetailView.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/ProblemEditView.vue apps/web/src/views/ProblemEditView.spec.ts apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts apps/web/src/views/ContestDetailView.vue apps/web/src/views/ContestDetailView.spec.ts
git commit -m "feat(web): support objective problems in editor and detail"
```

---

### Task 6: 前端答案提交页与测评记录

**Files:**
- Modify: `apps/web/src/views/SubmitView.vue`
- Modify: `apps/web/src/views/SubmitView.spec.ts`
- Modify: `apps/web/src/views/SubmissionsView.vue`
- Modify: `apps/web/src/views/SubmissionsView.spec.ts`
- Modify: `apps/web/src/utils/submission.ts`
- Modify: `apps/web/src/utils/submission.spec.ts`

**Interfaces:**
- Consumes: `ProblemDetail.type`, `CreateSubmissionRequest.answer`, `SubmissionDetail.answer/answerHidden`, `SubmissionLanguage='answer'`.
- Produces: objective answer form, payload `{problemId, contestId?, answer}`, answer display/filter.

- [ ] **Step 1: Write failing tests**

`SubmitView.spec.ts`：

```ts
it('submits objective answers without code or language', async () => {
  vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
  const wrapper = await mountReady()
  expect(wrapper.find('[data-test="objective-answer"]').exists()).toBe(true)
  expect(wrapper.find('.code-editor-stub').exists()).toBe(false)
  await setAnswer(wrapper, '  A  ')
  await submit(wrapper)
  expect(createSubmission).toHaveBeenCalledWith({ problemId: 1, answer: 'A' })
})
```

`SubmissionsView.spec.ts`：

```ts
it('renders objective answer and hides other users answer', async () => {
  vi.mocked(getSubmission).mockResolvedValue(detail({
    language: 'answer',
    answer: 'A',
    answerHidden: false,
  }))
  ...
  expect(wrapper.text()).toContain('客观题')
  expect(wrapper.text()).toContain('A')
})
```

再测 `answerHidden: true` 显示“答案仅提交者本人可见”。

`utils/submission.spec.ts`：

```ts
expect(languageLabel('answer')).toBe('客观题')
expect(SUBMISSION_LANGUAGES.map(l => l.value)).not.toContain('answer')
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/SubmitView.spec.ts src/views/SubmissionsView.spec.ts src/utils/submission.spec.ts`
Expected: FAIL。

- [ ] **Step 3: Implement**

`utils/submission.ts`：

- `languageLabels` 增加 `answer: '客观题'`（不改 `SUBMISSION_LANGUAGES` 代码列表）。
- 为筛选增加：

```ts
export const SUBMISSION_LANGUAGE_FILTERS = [
  { value: 'answer', label: '客观题' },
  ...SUBMISSION_LANGUAGES,
]
```

`SubmitView.vue`：

- `isObjective = computed(() => problem.value?.type === 'objective')`。
- `answer = ref('')`；可加 `maxlength=2000`、字数提示。
- 模板：
  - 语言选择/代码编辑器/AI 工具栏用 `v-if="!isObjective"`；
  - 客观题显示 `AppSurface` + `Textarea data-test="objective-answer"`；
  - 提交按钮：`label="提交答案"`，点击 `submitAnswer`；代码按钮保持。
- `submitAnswer`：

```ts
const text = answer.value.trim()
if (!text) { toast.warn('答案不能为空'); return }
await createSubmission({
  problemId: problemId.value,
  contestId: contestId.value || undefined,
  answer: text,
})
```

- `onMounted`：先加载题目；若 objective，不加载 AI 配置/不初始代码草稿。可将 `loadAIConfig` 放到 `if (!isObjective.value)`。
- 客观题不渲染比赛 AI 禁用提示（因为没有 AI）。
- 提交成功后沿用现有跳转。

`SubmissionsView.vue`：

- 语言筛选 `:options="SUBMISSION_LANGUAGE_FILTERS"`。
- 列表耗时/内存：`data.language === 'answer' ? '—' : ...`。
- 详情抽屉：
  - `answerHidden` 时显示 `Message` “答案仅提交者本人可见。”；
  - `detail.answer` 存在时显示 `<h3>答案</h3><pre>{{ detail.answer }}</pre>`，不显示代码块；
  - 代码题保持现有代码块/保护提示；
  - 耗时/内存客观题显示 `—`。
- `languageLabel` 支持 answer。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/SubmitView.spec.ts src/views/SubmissionsView.spec.ts src/utils/submission.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/SubmitView.vue apps/web/src/views/SubmitView.spec.ts apps/web/src/views/SubmissionsView.vue apps/web/src/views/SubmissionsView.spec.ts apps/web/src/utils/submission.ts apps/web/src/utils/submission.spec.ts
git commit -m "feat(web): submit and display objective answers"
```

---

### Task 7: 全量验证、真实冒烟与任务清单同步

**Files:**
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`（可选）

- [ ] **Step 1: Backend checks**

```bash
cd /Users/admin/codespace/kapoj/apps/server
test -z "$(gofmt -l .)"
go vet ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -p 1 -count=1 ./...
```

Expected: 全部通过。

- [ ] **Step 2: Judge checks**

```bash
cd /Users/admin/codespace/kapoj/apps/judge
test -z "$(gofmt -l .)"
go vet ./...
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./...
```

Expected: 全部通过。

- [ ] **Step 3: Frontend checks**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm gen:api
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过。

- [ ] **Step 4: Real smoke**

临时库/端口/server/storage：

1. 创建客观题（answers: `A`, `B`），无测试点，直接公开。
2. 游客/普通用户/AC 用户 `GET /api/problems/:id` 无 `answers`；管理员有 `answers`。
3. 提交 ` A ` → judge accepted，score 100，time/memory 0，accept_count +1；提交 `B` → accepted；提交 `C` → wrong_answer，score 0。
4. 提交详情：本人 `answer` 可见；其他用户 `answer=""`、`answerHidden=true`；`CanViewLiveContest` 管理员可见。
5. 加入进行中比赛：参赛者提交客观题答案，榜单/赛后统计正常；他人在赛时看不到答案。
6. 传统题/特判题回归：发布仍需测试点/checker，judge 仍走沙盒。
7. 清理临时数据，停止临时 server，删除临时库。

- [ ] **Step 5: Update docs**

`docs/任务清单.md` 新增 `## M7：客观题（答案题）`，把 OBJ-A～F 标 ✅，写交付摘要与测试结果。`docs/开发日志.md` 追加本次记录。README 可加一句当前进度。

- [ ] **Step 6: Commit**

```bash
git add docs/任务清单.md docs/开发日志.md README.md
git commit -m "docs: record objective problem delivery"
```

---

## Self-Review

- **Spec coverage:** 题型/答案存储/发布（Task 1）、提交/可见性（Task 2）、judge（Task 3）、契约（Task 4）、编辑/详情/比赛（Task 5）、提交/记录（Task 6）、验证/文档（Task 7）。
- **Type consistency:** `TypeObjective`、`Answers`、`Answer`、`answerHidden`、`LanguageAnswer`、`includeAnswer` 在任务间一致。
- **Review Focus mapping:** 答案泄露（Task 1/2/6）、无测试点发布/评测（Task 1/2/3）、代码题回归（Task 2/3）、judge 跳过沙盒与 AC 事件（Task 3）、前端类型切换（Task 5/6）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出签名、校验/判定语义与测试断言，不逐行抄写实现。
