# 比赛代码重复率检测 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在比赛结束后为管理员提供按题目的代码查重、疑似抄袭列表、左右代码对照和 CSV 导出。

**Architecture:** 在 Go 服务端新增 `similarity` 领域包，使用语言 tokenizer + Winnowing 指纹生成相似度；使用 Asynq `similarity` 队列异步执行每道题的任务；结果写入三张新表。Vue 比赛详情页新增「查重」Tab，复用现有 `DataTable`、`CodeViewer` 和 UI 组件。

**Tech Stack:** Go 1.26、GORM/PostgreSQL、Asynq/Redis、Vue 3、PrimeVue 4、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-11-15-contest-code-similarity-design.md`

## Global Constraints

- 查重引擎固定为自研 Go token + Winnowing，不引入 JPlag、Dolos、MOSS 或新的语言运行时。
- Winnowing 参数固定：k=20、w=10、最小有效 token=80、最小共享 fingerprint=5。
- 默认阈值 80%，允许 1–100 的整数；阈值按题目保存。
- 只比较 `contest_id` 对应比赛、`problem_id` 对应题目的代码提交；排除客观题、空代码、非报名用户的提交和比赛时间窗外的提交。
- 同一用户不互相比对；每对中较早提交为原提交，较晚提交为疑似提交；时间相同时 ID 较小者为原提交。
- 支持每题一个可选公共模板；模板最大 64 KiB；模板语言必须是受支持语言。
- 单题最多处理 1000 份提交；超过时拒绝任务并提示缩小范围。
- 权限复用 `contest.update`；比赛必须已结束；所有接口只对管理员返回代码和结果。
- 只在页面和 CSV 中使用「疑似抄袭」表述，不自动处罚。
- CSV 使用 UTF-8 BOM；以 `= + - @` 开头的字段加 `'` 前缀防公式注入。
- 不暂存或提交未列出的既有工作区改动；每个任务只 `git add` 本任务列出的文件。
- 前端不新增依赖，复用现有 PrimeVue 组件和 `CodeViewer`。

## Review Focus

- 公共模板、固定 I/O、宏和重复代码不能把正常提交大量误报；引擎必须支持模板排除和高频 fingerprint 过滤，并有测试。
- 空代码、客观题、过短代码、非报名用户和比赛时间窗外提交不能被纳入比较；仓库查询和服务层必须有测试。
- 同一用户多次提交、相同时间戳、多个疑似提交不能产生错误方向或重复记录；配对逻辑必须有测试。
- 未结束比赛、无权限用户、跨比赛 match id 不能访问代码或结果；service 和 handler 必须有测试。
- CSV 必须能在 Excel 中正确显示中文，且用户名/题目标题不能触发表格公式；CSV 任务必须有测试。

---

### Task 1: Tokenizer core for C-like languages

**Files:**
- Create: `apps/server/internal/similarity/tokenizer.go`
- Create: `apps/server/internal/similarity/tokenizer_test.go`

**Interfaces:**
- Consumes: `submission.Language` from `apps/server/internal/submission`.
- Produces:
  - `type TokenKind string`
  - `const (TokenIdentifier, TokenKeyword, TokenNumber, TokenString, TokenChar, TokenPunct, TokenPreproc, TokenNewline, TokenIndent, TokenDedent TokenKind)`
  - `type Token struct { Kind TokenKind; Text string; Line int }`
  - `func Tokenize(lang submission.Language, source string) ([]Token, error)`

- [ ] **Step 1: Write the failing tokenizer tests**

Add `tokenizer_test.go` with a helper that compares `(Kind, Text)` only (line numbers are not part of the equality assertion). Include these tests:

```go
func TestTokenizeCNormalizesRenamedVariables(t *testing.T) {
    a := `#include <bits/stdc++.h>
int a = 1;
// same logic
int main() { int b = a + 2; return b; }`
    b := `#include <cstdio>
int x = 1;
int main() { int y = x + 2; return y; }`
    ta, err := Tokenize(submission.LanguageC, a)
    require.NoError(t, err)
    tb, err := Tokenize(submission.LanguageC, b)
    require.NoError(t, err)
    assert.Equal(t, tokenPairs(ta), tokenPairs(tb))
}

func TestTokenizeRemovesCommentsAndNormalizesLiterals(t *testing.T) {
    src := `int main(){ // comment
        char c = 'x'; const char* s = "hello"; double n = 3.14; return 0;
    }`
    tokens, err := Tokenize(submission.LanguageCPP, src)
    require.NoError(t, err)
    assert.Contains(t, tokenTexts(tokens), "CHR")
    assert.Contains(t, tokenTexts(tokens), "STR")
    assert.Contains(t, tokenTexts(tokens), "NUM")
    assert.NotContains(t, tokenTexts(tokens), "hello")
}

func TestTokenizeCppPreprocessorLinesBecomePP(t *testing.T) {
    tokens, err := Tokenize(submission.LanguageCPP, "#include <iostream>\n#define F(x) x\nint a;")
    require.NoError(t, err)
    assert.Equal(t, "PP", tokens[0].Text)
    assert.Equal(t, "PP", tokens[1].Text)
    assert.Contains(t, tokenTexts(tokens), "int")
}

func TestTokenizeJavaAndGoKeepKeywords(t *testing.T) {
    javaTokens, err := Tokenize(submission.LanguageJava, "class A { public static void main(String[] args) {} }")
    require.NoError(t, err)
    assert.Contains(t, tokenTexts(javaTokens), "class")
    assert.Contains(t, tokenTexts(javaTokens), "public")
    goTokens, err := Tokenize(submission.LanguageGo, "package main\nfunc main() { var x int }")
    require.NoError(t, err)
    assert.Contains(t, tokenTexts(goTokens), "package")
    assert.Contains(t, tokenTexts(goTokens), "func")
}

func TestTokenizeRejectsUnknownLanguage(t *testing.T) {
    _, err := Tokenize(submission.Language("ruby"), "puts 1")
    assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarity -run TestTokenize -v`
Expected: FAIL because `Tokenize` and token types do not exist.

- [ ] **Step 3: Implement the generic scanner and C-like language specs**

Implement a single scanner in `tokenizer.go`:

- `Tokenize` switches on `c`, `cpp`, `java`, `go`, and returns an error for anything else (Python is added in Task 2).
- The scanner skips whitespace and line/block comments.
- For C/C++, a `#` at the start of a line consumes the rest of the physical line and emits `TokenPreproc` with text `PP`.
- Strings/chars consume escapes and emit `STR`/`CHR` with one Line value.
- Numeric literals emit `NUM`; identifiers not in the language keyword set emit `ID`; keywords keep their source text.
- Operators/punctuation use longest-match against a shared list including `>>=`, `<<=`, `...`, `->`, `::`, `==`, `!=`, `<=`, `>=`, `&&`, `||`, `++`, `--`, `+=`, `-=`, `*=`, `/=`, `%=`, `&^=`.
- Line numbers start at 1 and increment on `\n`.
- Keyword sets:
  - C/C++: `auto break case char const continue default do double else enum extern float for goto if inline int long register return short signed sizeof static struct switch typedef union unsigned void volatile while class namespace public private protected template typename using new delete nullptr true false constexpr`.
  - Java: `abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new package private protected public return short static strictfp super switch synchronized this throw throws transient try void volatile while true false null`.
  - Go: `break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var true false nil`.
- Use `unicode.IsLetter`/`IsDigit` for identifiers (C-like code is ASCII but this keeps the scanner safe).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && go test ./internal/similarity -run TestTokenize -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarity/tokenizer.go apps/server/internal/similarity/tokenizer_test.go
git commit -m "feat: add similarity tokenizer for C-like languages"
```

---

### Task 2: Python tokenizer with indentation

**Files:**
- Modify: `apps/server/internal/similarity/tokenizer.go`
- Modify: `apps/server/internal/similarity/tokenizer_test.go`

**Interfaces:**
- Consumes: `Tokenize` from Task 1.
- Produces: Python support through the same `Tokenize(submission.LanguagePython3, source)` API.

- [ ] **Step 1: Write failing Python tests**

Add:

```go
func TestTokenizePythonNormalizesRenamesAndComments(t *testing.T) {
    a := "def f(x):\n    y = x + 1  # comment\n    return y\n"
    b := "def g(n):\n    value = n + 1\n    return value\n"
    ta, err := Tokenize(submission.LanguagePython3, a)
    require.NoError(t, err)
    tb, err := Tokenize(submission.LanguagePython3, b)
    require.NoError(t, err)
    assert.Equal(t, tokenPairs(ta), tokenPairs(tb))
}

func TestTokenizePythonKeepsIndentationStructure(t *testing.T) {
    a, err := Tokenize(submission.LanguagePython3, "if x:\n    y = 1\nz = 2\n")
    require.NoError(t, err)
    b, err := Tokenize(submission.LanguagePython3, "if x:\n    y = 1\n    z = 2\n")
    require.NoError(t, err)
    assert.NotEqual(t, tokenPairs(a), tokenPairs(b))
    assert.Contains(t, tokenKinds(a), TokenIndent)
    assert.Contains(t, tokenKinds(a), TokenDedent)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarity -run TestTokenizePython -v`
Expected: FAIL with unsupported language.

- [ ] **Step 3: Implement Python mode**

Add `python3` to the switch. In Python mode:

- `#` starts a line comment.
- Support single/double quoted strings and triple-quoted strings across lines.
- At the start of each logical line, compute indentation (spaces; tabs count as 4 spaces).
- Emit `TokenIndent`/`TokenDedent` when the indent level changes; emit `TokenNewline` for non-empty logical lines.
- Normalize identifiers/numbers/strings using the same rules.
- Keyword set: `False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield match case`.
- Empty lines and comment-only lines do not change indentation.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && go test ./internal/similarity -run TestTokenize -v`
Expected: PASS, including all Task 1 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarity/tokenizer.go apps/server/internal/similarity/tokenizer_test.go
git commit -m "feat: add Python tokenizer for similarity"
```

---

### Task 3: Winnowing fingerprints

**Files:**
- Create: `apps/server/internal/similarity/fingerprint.go`
- Create: `apps/server/internal/similarity/fingerprint_test.go`

**Interfaces:**
- Consumes: `[]Token` from Task 1/2.
- Produces:
  - `const KGramSize = 20`
  - `const WindowSize = 10`
  - `type Fingerprint uint64`
  - `func Fingerprints(tokens []Token) []Fingerprint` (unique, order preserved)

- [ ] **Step 1: Write failing tests**

```go
func TestFingerprintsDeterministic(t *testing.T) {
    tokens := []Token{{Kind: TokenKeyword, Text: "int"}, {Kind: TokenIdentifier, Text: "ID"}}
    require.Equal(t, Fingerprints(tokens), Fingerprints(tokens))
}

func TestFingerprintsIgnoreShortInput(t *testing.T) {
    assert.Empty(t, Fingerprints([]Token{{Kind: TokenKeyword, Text: "int"}}))
}

func TestFingerprintsChangeWhenCodeChanges(t *testing.T) {
    base := make([]Token, 0, 80)
    for i := 0; i < 80; i++ { base = append(base, Token{Kind: TokenPunct, Text: ";"}) }
    changed := append([]Token{}, base...)
    changed[40].Text = "{}"
    assert.NotEqual(t, Fingerprints(base), Fingerprints(changed))
}

func TestWinnowSelectsRightmostMinimum(t *testing.T) {
    got := winnow([]uint64{5, 1, 1, 9}, 2)
    assert.Equal(t, []Fingerprint{1, 1}, got)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarity -run 'TestFingerprints|TestWinnow' -v`
Expected: FAIL because fingerprint code does not exist.

- [ ] **Step 3: Implement rolling hashing and winnowing**

- Map each normalized token text to a stable 64-bit FNV-1a hash cached in a package-level `sync.Map`.
- Compute k-gram hashes with `hash = hash*1000003 + tokenHash` (uint64 overflow is intentional).
- `winnow(hashes []uint64, w int)` selects the minimum hash in every window of size `w`; on ties select the rightmost position; preserve one copy of repeated selected hashes.
- `Fingerprints` returns an empty slice when `len(tokens) < KGramSize`; otherwise winnows the k-gram hashes and removes duplicate fingerprint values while preserving order.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && go test ./internal/similarity -run 'TestFingerprints|TestWinnow' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarity/fingerprint.go apps/server/internal/similarity/fingerprint_test.go
git commit -m "feat: add winnowing fingerprints"
```

---

### Task 4: Similarity engine and pair attribution

**Files:**
- Create: `apps/server/internal/similarity/engine.go`
- Create: `apps/server/internal/similarity/engine_test.go`

**Interfaces:**
- Consumes: `Fingerprints` from Task 3.
- Produces:
  - `const MinTokens = 80`
  - `const MinSharedFingerprints = 5`
  - `const MaxSubmissionsPerProblem = 1000`
  - `type Candidate struct { ID, UserID int64; Username, Nickname string; Language submission.Language; Code string; CreatedAt time.Time }`
  - `type EngineRequest struct { Candidates []Candidate; ThresholdPercent float64; BaseCode string; BaseLanguage submission.Language; IgnoreCommonCode bool }`
  - `type Pair struct { OriginalSubmissionID, SuspectSubmissionID, OriginalUserID, SuspectUserID int64; Language submission.Language; SimilarityPercent, DicePercent float64; SharedFingerprints, OriginalFingerprints, SuspectFingerprints int; OriginalCreatedAt, SuspectCreatedAt time.Time }`
  - `type EngineResult struct { Pairs []Pair; SubmissionCount, SkippedCount int }`
  - `type Engine interface { Compare(ctx context.Context, req EngineRequest) (EngineResult, error) }`
  - `func NewWinnowingEngine() *WinnowingEngine`
  - `func (e *WinnowingEngine) Compare(ctx context.Context, req EngineRequest) (EngineResult, error)`

- [ ] **Step 1: Write failing engine tests**

Use code strings with at least 100 normalized tokens. Include:

```go
func TestEngineDetectsRenamedCopy(t *testing.T) {
    original := Candidate{ID: 1, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(0), Code: cppTemplate("a")}
    copy := Candidate{ID: 2, UserID: 20, Language: submission.LanguageCPP, CreatedAt: at(time.Minute), Code: cppTemplate("b")}
    result, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
        Candidates: []Candidate{original, copy}, ThresholdPercent: 80,
    })
    require.NoError(t, err)
    require.Len(t, result.Pairs, 1)
    assert.EqualValues(t, 1, result.Pairs[0].OriginalSubmissionID)
    assert.EqualValues(t, 2, result.Pairs[0].SuspectSubmissionID)
    assert.GreaterOrEqual(t, result.Pairs[0].SimilarityPercent, 90)
}

func TestEngineIgnoresUnrelatedAndSameUser(t *testing.T) {
    // unrelated submissions below threshold + two submissions from the same user
}

func TestEngineRemovesBaseTemplate(t *testing.T) {
    // both candidates share a large template and differ in the body;
    // without BaseCode similarity >= 80, with BaseCode < 80
}

func TestEngineIgnoresHighFrequencyFingerprints(t *testing.T) {
    // five candidates share a common prefix; with IgnoreCommonCode=true
    // the shared prefix must not create a pair between unrelated bodies
}

func TestEngineUsesEarlierSubmissionAsOriginal(t *testing.T) {
    // later submission passed first in the slice; result must still point to the earlier one
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarity -run TestEngine -v`
Expected: FAIL because engine code does not exist.

- [ ] **Step 3: Implement the engine**

Algorithm:

1. If `len(Candidates) > MaxSubmissionsPerProblem`, return an error.
2. Group candidates by language; skip `LanguageAnswer` and unsupported languages.
3. Tokenize each candidate. Skip submissions with fewer than `MinTokens` tokens or zero fingerprints; increment `SkippedCount`.
4. If `BaseCode != ""` and `BaseLanguage` matches a candidate language, tokenize the base code and remove its fingerprints from every candidate fingerprint set of that language.
5. If `IgnoreCommonCode` is true and a language has at least 5 candidates, compute fingerprint document frequencies and remove fingerprints appearing in more than 50% of that language's candidates.
6. Build a `map[[2]int64]int` of shared fingerprint counts using an inverted index: fingerprint -> submission IDs; for every pair in a bucket increment the shared count.
7. For each pair with both submissions in the same language, ignore same-user pairs; compute:
   - `similarity = max(shared/len(A), shared/len(B)) * 100`
   - `dice = 2*shared/(len(A)+len(B)) * 100`
   - skip if `shared < MinSharedFingerprints` or `similarity < ThresholdPercent`.
8. Order each pair by `(CreatedAt, ID)` and emit a `Pair`.
9. Sort pairs by `SimilarityPercent` descending, then original ID, then suspect ID.
10. Return `SubmissionCount` as the number of candidates that produced fingerprints.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && go test ./internal/similarity -run TestEngine -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarity/engine.go apps/server/internal/similarity/engine_test.go
git commit -m "feat: add winnowing similarity engine"
```

---

### Task 5: Persistence models and repository

**Files:**
- Create: `apps/server/internal/similarity/model.go`
- Create: `apps/server/internal/similarity/repository.go`
- Create: `apps/server/internal/similarity/repository_test.go`
- Modify: `apps/server/internal/db/db.go`

**Interfaces:**
- Consumes: `submission.Language`, GORM DB.
- Produces:
  - Models `Settings`, `Run`, `Match`.
  - `type SubmissionRow struct { ID, UserID int64; Username, Nickname string; Language submission.Language; Code string; CreatedAt time.Time }`
  - `type MatchFilter struct { ContestID, ProblemID, RunID int64; Page, PageSize int }`
  - `type Repository interface { ... }` with:
    - `GetSettings(ctx, contestID, problemID int64) (*Settings, error)`
    - `ListSettings(ctx, contestID int64) ([]Settings, error)`
    - `UpsertSettings(ctx, *Settings) error`
    - `CreateRun(ctx, *Run) error`
    - `FindRun(ctx, id int64) (*Run, error)`
    - `FindActiveRun(ctx, contestID, problemID int64) (*Run, error)`
    - `UpdateRun(ctx, *Run) error`
    - `ReplaceMatches(ctx, runID int64, matches []Match) error`
    - `LatestCompletedRuns(ctx, contestID int64) (map[int64]*Run, error)`
    - `ListMatches(ctx, MatchFilter) ([]MatchRow, int64, error)`
    - `FindMatch(ctx, id int64) (*MatchDetailRow, error)`
    - `ListProblemSubmissions(ctx, contestID, problemID int64) ([]SubmissionRow, error)`
    - `MarkStaleRunsFailed(ctx, before time.Time) error`

- [ ] **Step 1: Write failing repository tests**

Use `testutil.PG(t)` and seed one contest, two problems, two participants, one non-participant, one objective submission, one submission outside the contest window, and code submissions. Assert:

```go
func TestListProblemSubmissionsOnlyParticipantsInWindow(t *testing.T) {
    // returns only participant code submissions inside [start,end)
    // excludes language=answer, non-participant, and outside-window rows
}

func TestSettingsUpsertAndLatestCompletedRun(t *testing.T) {
    // insert settings; update threshold; read back
    // create two runs for same problem; only latest completed is returned
}

func TestReplaceMatchesIsIdempotent(t *testing.T) {
    // replace with 2 rows then 1 row; count is 1 and unique constraint holds
}

func TestFindMatchIncludesBothCodes(t *testing.T) {
    // detail has originalCode, suspectCode and user metadata
}

func TestMarkStaleRunsFailed(t *testing.T) {
    // running run older than cutoff becomes failed; queued stays queued
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarity -run 'TestListProblem|TestSettingsUpsert|TestReplaceMatches|TestFindMatch|TestMarkStale' -v`
Expected: FAIL because models/repository do not exist.

- [ ] **Step 3: Implement models and repository**

Use these GORM fields:

```go
type Settings struct {
    ID int64 `gorm:"primaryKey"`
    ContestID int64 `gorm:"not null;uniqueIndex:uniq_similarity_settings_problem,priority:1"`
    ProblemID int64 `gorm:"not null;uniqueIndex:uniq_similarity_settings_problem,priority:2"`
    ThresholdPercent int `gorm:"not null;default:80"`
    IgnoreCommonCode bool `gorm:"not null;default:true"`
    BaseLanguage string `gorm:"size:20;not null;default:''"`
    BaseCode string `gorm:"type:text;not null;default:''"`
    UpdatedBy int64 `gorm:"not null;default:0"`
    UpdatedAt time.Time `gorm:"type:timestamptz;not null"`
}

type Run struct {
    ID int64 `gorm:"primaryKey"`
    ContestID int64 `gorm:"not null;index:idx_similarity_runs_problem,priority:1"`
    ProblemID int64 `gorm:"not null;index:idx_similarity_runs_problem,priority:2"`
    Status string `gorm:"size:20;not null;index"`
    ThresholdPercent int `gorm:"not null"`
    IgnoreCommonCode bool `gorm:"not null;default:true"`
    Engine string `gorm:"size:30;not null"`
    EngineVersion string `gorm:"size:20;not null"`
    RequestedBy int64 `gorm:"not null;default:0"`
    SubmissionCount int `gorm:"not null;default:0"`
    MatchCount int `gorm:"not null;default:0"`
    ErrorMessage string `gorm:"type:text;not null;default:''"`
    StartedAt *time.Time `gorm:"type:timestamptz"`
    FinishedAt *time.Time `gorm:"type:timestamptz"`
    CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}

type Match struct {
    ID int64 `gorm:"primaryKey"`
    RunID int64 `gorm:"not null;uniqueIndex:uniq_similarity_match_pair,priority:1"`
    ContestID int64 `gorm:"not null;index"`
    ProblemID int64 `gorm:"not null;index"`
    OriginalSubmissionID int64 `gorm:"not null;uniqueIndex:uniq_similarity_match_pair,priority:2"`
    SuspectSubmissionID int64 `gorm:"not null;uniqueIndex:uniq_similarity_match_pair,priority:3"`
    OriginalUserID int64 `gorm:"not null"`
    SuspectUserID int64 `gorm:"not null"`
    Language submission.Language `gorm:"size:20;not null"`
    SimilarityPercent float64 `gorm:"type:double precision;not null"`
    DicePercent float64 `gorm:"type:double precision;not null"`
    SharedFingerprints int `gorm:"not null"`
    OriginalFingerprints int `gorm:"not null"`
    SuspectFingerprints int `gorm:"not null"`
    OriginalCreatedAt time.Time `gorm:"type:timestamptz;not null"`
    SuspectCreatedAt time.Time `gorm:"type:timestamptz;not null"`
    CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}
```

Add `&similarity.Settings{}`, `&similarity.Run{}`, `&similarity.Match{}` to the `AutoMigrate` list in `apps/server/internal/db/db.go`.

Repository details:

- `ListProblemSubmissions` joins `submissions`, `users`, `contest_participants` and `contests`; filters `s.contest_id = ?`, `s.problem_id = ?`, `s.language <> 'answer'`, `s.created_at >= c.start_time`, `s.created_at < c.end_time`; orders by `s.created_at, s.id`.
- `LatestCompletedRuns` uses PostgreSQL `DISTINCT ON (problem_id)` ordered by `problem_id, id DESC` and status `completed`.
- `ListMatches` joins `contest_similarity_runs` and includes only latest completed run per problem unless an explicit `RunID` is set.
- `ReplaceMatches` runs in a transaction: delete existing matches for `run_id`, then insert the provided rows.
- `MarkStaleRunsFailed` updates `running` rows with `started_at < before` to `failed` and `error_message = '任务超时'`.
- `FindMatch` joins both submissions and both users; returns code, language, status, user metadata, and created times.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && go test ./internal/similarity -run 'TestListProblem|TestSettingsUpsert|TestReplaceMatches|TestFindMatch|TestMarkStale' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarity/model.go apps/server/internal/similarity/repository.go apps/server/internal/similarity/repository_test.go apps/server/internal/db/db.go
git commit -m "feat: add similarity persistence"
```

---

### Task 6: Similarity service, settings, runs and results

**Files:**
- Create: `apps/server/internal/similarity/service.go`
- Create: `apps/server/internal/similarity/dto.go`
- Create: `apps/server/internal/similarity/service_test.go`

**Interfaces:**
- Consumes: `Repository`, `Engine`, and contest lookup from Task 5/4.
- Produces:
  - `type ContestLookup interface { FindByID(ctx, id int64) (*contest.Contest, error); FindProblems(ctx, contestID int64) ([]contest.ProblemRow, error) }`
  - `type Enqueuer interface { Enqueue(ctx context.Context, runID int64) error }`
  - `type Viewer struct { UserID int64; CanManage bool }`
  - `func NewService(repo Repository, contests ContestLookup, engine Engine, queue Enqueuer) *Service`
  - DTO types `Overview`, `ProblemView`, `SettingsInput`, `RunView`, `MatchView`, `MatchDetail`, `MatchPage`.
  - Methods in `Service`: `Overview`, `SaveSettings`, `StartRuns`, `Run`, `Matches`, `MatchDetail`, `ProcessRun`. CSV export is added in Task 9.

- [ ] **Step 1: Write failing service tests**

Use `testutil.PG(t)` and a fake `Engine`/`Enqueuer`; seed contest/users/problems/submissions. Assert:

```go
func TestOverviewRejectsUnfinishedContest(t *testing.T) {
    // contest end in future -> error
}

func TestSaveSettingsValidatesAndUpserts(t *testing.T) {
    // threshold 0 and 101 rejected; 80 saved; problem not in contest rejected
}

func TestStartRunsCreatesAndEnqueuesOneRunPerProblem(t *testing.T) {
    // problemID=0 creates runs for all problems; each enqueued once
}

func TestStartRunsReturnsExistingActiveRun(t *testing.T) {
    // queued/running run prevents a duplicate
}

func TestProcessRunPersistsMatchesAndCompletes(t *testing.T) {
    // fake engine returns one pair; run becomes completed with match count 1
}

func TestProcessRunMarksFailedOnEngineError(t *testing.T) {
    // status failed and error message persisted
}

func TestMatchesUsesLatestCompletedRunOnly(t *testing.T) {
    // old run matches are hidden after a newer completed run
}

func TestMatchDetailRejectsMatchFromAnotherContest(t *testing.T) {
    // cross-contest match id returns not found/forbidden
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarity -run 'TestOverview|TestSaveSettings|TestStartRuns|TestProcessRun|TestMatches|TestMatchDetail' -v`
Expected: FAIL because service code does not exist.

- [ ] **Step 3: Implement the service**

Rules:

- `requireManage(v)` returns `httpx.ErrForbidden` when false.
- `requireEnded(c)` returns `httpx.ErrValidation("比赛结束后才能查重")` unless `contest.StatusAt(c.StartTime, c.EndTime, time.Now()) == contest.StatusEnded`.
- `Overview`: contest must exist and be ended; list problems from `ContestLookup.FindProblems`; list settings; latest completed runs; return default threshold 80 for missing settings.
- `SaveSettings`: verify problem is in contest; threshold 1–100; if `BaseCode != ""`, require valid `BaseLanguage` and `len(BaseCode) <= 64*1024`; trim; upsert with `UpdatedBy = v.UserID`.
- `StartRuns`: verify ended; if `problemID > 0` validate it belongs to contest; if 0 use all problem IDs. For each problem, if an active run exists return it; otherwise create a queued run with current settings/defaults and call `queue.Enqueue`. Return created/existing run views.
- `ProcessRun`: if run status is not queued/running return nil; mark running + `StartedAt`; load settings (defaults if absent), submissions and build `EngineRequest`; call engine; convert pairs to `Match` rows; `ReplaceMatches`; mark completed with counts; on error mark failed with a 500-char message and return the error.
- `Matches`: require ended; pass filter to repository; map match rows to DTOs; only latest completed runs.
- `MatchDetail`: require ended; load match; verify contest id matches; map both code rows.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && go test ./internal/similarity -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarity/service.go apps/server/internal/similarity/dto.go apps/server/internal/similarity/service_test.go
git commit -m "feat: add similarity service"
```

---

### Task 7: Similarity Asynq queue and worker

**Files:**
- Create: `apps/server/internal/similarityqueue/contract.go`
- Create: `apps/server/internal/similarityqueue/client.go`
- Create: `apps/server/internal/similarityqueue/worker.go`
- Create: `apps/server/internal/similarityqueue/contract_test.go`
- Modify: `apps/server/cmd/server/main.go`

**Interfaces:**
- Consumes: `similarity.Service.ProcessRun(ctx, runID int64) error`.
- Produces:
  - `const QueueName = "similarity"`
  - `const TypeRun = "similarity:run"`
  - `type Client struct{ ... }`, `func NewClient(redisAddr string) *Client`, `func (c *Client) Enqueue(ctx, runID int64) error`
  - `type Handler interface { ProcessRun(ctx context.Context, runID int64) error }`
  - `func Register(mux *asynq.ServeMux, h Handler)`
- Main wiring:
  - Create `similarityRepo`, `similarityQueue`, `similarityService`, and `similarityServer` in `apps/server/cmd/server/main.go`.
  - Register the similarity server with `asynq.Config{Concurrency: 1, Queues: map[string]int{similarityqueue.QueueName: 1}, ShutdownTimeout: 10 * time.Second}`.
  - Before starting the worker, call `similarityRepo.MarkStaleRunsFailed(ctx, time.Now().Add(-15*time.Minute))` and log the count.
  - Do not register the API routes yet; Task 8 adds them.

- [ ] **Step 1: Write failing queue contract tests**

```go
func TestContract(t *testing.T) {
    task := newTask(42)
    assert.Equal(t, TypeRun, task.Type())
    assert.Equal(t, []byte(`{"runId":42}`), task.Payload())
    assert.Equal(t, QueueName, taskOption(task, asynq.QueueOpt))
    assert.Equal(t, 10*time.Minute, taskOption(task, asynq.TimeoutOpt))
    assert.Equal(t, 1, taskOption(task, asynq.MaxRetryOpt))
}

func TestWorkerCallsService(t *testing.T) {
    fake := &fakeHandler{}
    mux := asynq.NewServeMux()
    Register(mux, fake)
    require.NoError(t, mux.ProcessTask(context.Background(), newTask(7)))
    assert.Equal(t, int64(7), fake.runID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarityqueue -v`
Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement queue contract and main worker wiring**

- Payload format: `{"runId":<id>}`.
- Queue: `similarity`, max retry 1, timeout 10 minutes.
- Worker decodes payload and calls `ProcessRun`.
- Add imports and service construction in `main.go`; use `contest.NewGormRepository(g)` as `ContestLookup`.
- The service needs the queue for enqueueing; create `similarityQueue` before `similarityService`.

- [ ] **Step 4: Run tests and compile**

Run: `cd apps/server && go test ./internal/similarityqueue ./cmd/server`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarityqueue apps/server/cmd/server/main.go
git commit -m "feat: add similarity worker queue"
```

---

### Task 8: Similarity HTTP API

**Files:**
- Create: `apps/server/internal/similarityapi/handler.go`
- Create: `apps/server/internal/similarityapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`

**Interfaces:**
- Consumes: `similarity.Service` and RBAC.
- Produces routes:
  - `GET /api/contests/:id/similarity/overview`
  - `PUT /api/contests/:id/similarity/problems/:problemId`
  - `POST /api/contests/:id/similarity/runs`
  - `GET /api/contests/:id/similarity/runs/:runId`
  - `GET /api/contests/:id/similarity/matches`
  - `GET /api/contests/:id/similarity/matches/:matchId`
- `similarityapi.Options{RBAC *rbac.Service}`.
- Router `Deps` gains `Similarity *similarity.Service`; register routes when non-nil.

- [ ] **Step 1: Write failing handler tests**

Use `testutil.PG(t)` and `testutil.SeedRBAC(t)`. Include:

```go
func TestRequiresContestUpdatePermission(t *testing.T) {
    // no token -> 401; ordinary user -> 403
}

func TestOverviewRejectsUnfinishedContest(t *testing.T) {
    // manager token -> 400 with message "比赛结束后才能查重"
}

func TestSaveSettingsAndStartRun(t *testing.T) {
    // PUT threshold 85; GET overview returns 85;
    // POST runs returns 202 and queued run
}

func TestRunStatusAndMatchEndpoints(t *testing.T) {
    // complete a run through the service/fake engine, then list matches and detail
}

func TestCrossContestMatchIsNotFound(t *testing.T) {
    // match id from another contest -> 404
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarityapi -v`
Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement handlers**

- Use `middleware.LoadAccess`, `middleware.RequirePermission(resolver, string(rbac.PermContestUpdate))`.
- Build `similarity.Viewer{UserID: access.UserID, CanManage: access.Has(...)}`.
- Audit PUT/POST with `middleware.AuditAdminAction` target `contest`.
- Parse IDs with a local `pathID` helper.
- Validate JSON with `httpx.BindJSON`.
- Return `202` for `POST /runs`; `200` otherwise.
- Use existing `httpx.Page` response shape for match lists.
- Add `Similarity` to `router.Deps` and register routes in `router.New` after contest routes.
- Ensure nil `Similarity` in tests/other callers does not panic.

- [ ] **Step 4: Run tests and compile**

Run: `cd apps/server && go test ./internal/similarityapi ./internal/router ./cmd/server`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarityapi apps/server/internal/router/router.go apps/server/cmd/server/main.go
git commit -m "feat: expose similarity API"
```

---

### Task 9: CSV export

**Files:**
- Create: `apps/server/internal/similarity/csv.go`
- Create: `apps/server/internal/similarity/csv_test.go`
- Modify: `apps/server/internal/similarity/service.go`
- Modify: `apps/server/internal/similarityapi/handler.go`
- Modify: `apps/server/internal/similarityapi/handler_test.go`

**Interfaces:**
- Consumes: latest match rows.
- Produces:
  - `func (s *Service) ExportCSV(ctx context.Context, v Viewer, contestID, problemID int64) ([]byte, error)`
  - `GET /api/contests/:id/similarity/export.csv`
- CSV headers exactly:

```text
比赛ID,比赛标题,题目ID,题号,题目标题,原提交ID,原提交用户ID,原提交用户名,原提交昵称,原提交时间,原提交语言,疑似提交ID,疑似用户ID,疑似用户名,疑似昵称,疑似提交时间,疑似语言,最高重复率,Dice相似度,阈值,计算完成时间
```

- [ ] **Step 1: Write failing CSV tests**

```go
func TestExportCSVUsesBOMAndHeaders(t *testing.T) {
    // first three bytes are EF BB BF; header contains 最高重复率
}

func TestExportCSVEscapesFormula(t *testing.T) {
    // nickname "=cmd" becomes "'=cmd" in the cell
}

func TestExportCSVEscapesCSVQuotes(t *testing.T) {
    // title with comma/quote is parsed back by encoding/csv as one field
}

func TestExportCSVUsesLatestCompletedRun(t *testing.T) {
    // old run match not present
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && go test ./internal/similarity -run TestExportCSV -v`
Expected: FAIL.

- [ ] **Step 3: Implement CSV export**

- Build rows from `Repository.ListMatches` with latest completed runs.
- Use `encoding/csv` and `bytes.Buffer`.
- Prefix cells starting with `=`, `+`, `-`, or `@` with `'`.
- Write UTF-8 BOM before the header.
- Format percentages with one decimal using `strconv.FormatFloat(value, 'f', 1, 64)`.
- Format times as RFC3339.
- Handler sets:
  - `Content-Type: text/csv; charset=utf-8`
  - `Content-Disposition: attachment; filename="contest-<id>-similarity.csv"`
  - `200` body bytes.
- Handler enforces permission and ended contest like other endpoints.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && go test ./internal/similarity ./internal/similarityapi -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/similarity/csv.go apps/server/internal/similarity/csv_test.go apps/server/internal/similarity/service.go apps/server/internal/similarityapi/handler.go apps/server/internal/similarityapi/handler_test.go
git commit -m "feat: export similarity matches as CSV"
```

---

### Task 10: OpenAPI, generated types and frontend API client

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/api/similarity.ts`
- Create: `apps/web/src/api/similarity.spec.ts`

**Interfaces:**
- Consumes: API routes from Tasks 8/9.
- Produces frontend functions:
  - `getSimilarityOverview(contestId)`
  - `updateSimilaritySettings(contestId, problemId, input)`
  - `startSimilarityRuns(contestId, problemId?)`
  - `getSimilarityRun(contestId, runId)`
  - `listSimilarityMatches(contestId, query)`
  - `getSimilarityMatch(contestId, matchId)`
  - `downloadSimilarityCSV(contestId, problemId?)`

- [ ] **Step 1: Add OpenAPI schemas and paths**

Add schemas:

- `ContestSimilarityOverview`
- `ContestSimilarityProblem`
- `ContestSimilarityRun`
- `ContestSimilaritySettingsInput`
- `ContestSimilarityRunRequest`
- `ContestSimilarityRunList`
- `ContestSimilarityMatch`
- `ContestSimilarityMatchDetail`

Ensure all fields match `similarity/dto.go`; use `format: int64` for IDs, `format: date-time` for times, and percentages as `number`.

Add paths for the six JSON routes plus the CSV route.

- [ ] **Step 2: Regenerate types**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm gen:api`
Expected: `apps/web/src/api/schema.d.ts` updated.

- [ ] **Step 3: Add frontend type aliases and client**

In `types.ts` add aliases for the new schemas. In `similarity.ts` implement the functions above using `http.get`, `http.put`, `http.post`, and `http.getBlob`.

Query params for matches: `problemId`, `runId`, `page`, `pageSize`. Omit zero/empty values.

- [ ] **Step 4: Write failing client tests**

Mock the HTTP adapter like existing API specs and assert exact method/url/params, including CSV using `getBlob`.

Run: `cd apps/web && npx vitest run src/api/similarity.spec.ts`
Expected: FAIL before implementation, PASS after.

- [ ] **Step 5: Run typecheck and commit**

Run: `cd apps/web && npx vue-tsc -b`
Expected: PASS.

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/similarity.ts apps/web/src/api/similarity.spec.ts
git commit -m "feat: add similarity API client"
```

---

### Task 11: Frontend overview and settings panel

**Files:**
- Create: `apps/web/src/components/contest/similarity/ContestSimilarityPanel.vue`
- Create: `apps/web/src/components/contest/similarity/ContestSimilaritySettingsDialog.vue`
- Create: `apps/web/src/components/contest/similarity/ContestSimilarityPanel.spec.ts`

**Interfaces:**
- Consumes: `getSimilarityOverview`, `updateSimilaritySettings`, `startSimilarityRuns`.
- Produces:
  - `ContestSimilarityPanel` prop `contestId: number`.
  - Top section with problem rows, threshold/status, save dialog, run buttons.
  - Emits no public events; results table is added in Task 12.
- `ContestSimilaritySettingsDialog` props `visible`, `problem`, `saving`; emits `update:visible` and `save(payload)`.

- [ ] **Step 1: Write failing panel tests**

Mock `@/api/similarity` and `primevue/usetoast`. Assert:

```ts
it('loads overview and renders problem thresholds', ...)
it('saves settings through the dialog', ...)
it('starts a single problem run', ...)
it('starts all runs', ...)
it('polls while a run is queued or running', ...)
it('shows empty and error states', ...)
```

Use `data-test` selectors: `similarity-problem-row`, `similarity-settings-open`, `similarity-run-problem`, `similarity-run-all`, `similarity-settings-threshold`, `similarity-settings-save`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/web && npx vitest run src/components/contest/similarity/ContestSimilarityPanel.spec.ts`
Expected: FAIL because components do not exist.

- [ ] **Step 3: Implement the panel and settings dialog**

Panel:

- `onMounted(loadOverview)`.
- Render loading/error/empty states.
- Problem table columns: 题号、题目、代码提交数、阈值、最近状态、结果数、操作.
- Buttons: 设置、计算本题; top button 计算全部.
- Save settings emits to `updateSimilaritySettings`; update local overview.
- Run starts calls `startSimilarityRuns`; reload overview; set polling.
- Poll every 2 seconds while any latest run is `queued` or `running`; clear on unmount and when none active.
- Show toast success/error.

Settings dialog:

- `InputNumber` threshold, min 1, max 100.
- `Checkbox` ignore common code.
- `Select` base language with blank option.
- `Textarea` base code, maxlength 65536.
- If base code non-empty and language empty, block save with inline error.
- Emit `{thresholdPercent, ignoreCommonCode, baseLanguage, baseCode}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/web && npx vitest run src/components/contest/similarity/ContestSimilarityPanel.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/contest/similarity/ContestSimilarityPanel.vue apps/web/src/components/contest/similarity/ContestSimilaritySettingsDialog.vue apps/web/src/components/contest/similarity/ContestSimilarityPanel.spec.ts
git commit -m "feat: add contest similarity settings panel"
```

---

### Task 12: Results table and code detail dialog

**Files:**
- Modify: `apps/web/src/components/contest/similarity/ContestSimilarityPanel.vue`
- Create: `apps/web/src/components/contest/similarity/ContestSimilarityDetailDialog.vue`
- Modify/Create: `apps/web/src/components/contest/similarity/ContestSimilarityPanel.spec.ts`
- Create: `apps/web/src/components/contest/similarity/ContestSimilarityDetailDialog.spec.ts`

**Interfaces:**
- Consumes: `listSimilarityMatches`, `getSimilarityMatch`, `downloadSimilarityCSV`.
- Produces:
  - Results table with `data-test="similarity-match-row"`.
  - Detail dialog with `data-test="similarity-detail"`, `similarity-original-code`, `similarity-suspect-code`.
  - CSV download button `data-test="similarity-export"`.

- [ ] **Step 1: Write failing tests**

Panel tests:

```ts
it('loads and renders the latest match list', ...)
it('filters matches by problem', ...)
it('opens detail on row click and fetches the match', ...)
it('downloads CSV with the current problem filter', ...)
it('shows no-match and failure states', ...)
```

Detail tests:

```ts
it('renders both user identities, times, languages and percentages', ...)
it('renders both code panes with CodeViewer', ...)
it('uses a stacked layout on mobile width classes', ...)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/web && npx vitest run src/components/contest/similarity`
Expected: FAIL until results/detail are implemented.

- [ ] **Step 3: Implement matches table and detail dialog**

Panel:

- `loadMatches()` with `problemId`, page, pageSize=20.
- Results table columns: 题目、原提交用户、原提交时间、疑似用户、疑似时间、重复率、操作.
- Similarity tag severity: >=90 danger, >=80 warn, else secondary.
- Filter select by contest problem.
- Export: `downloadSimilarityCSV`; create Blob URL, anchor download, revoke URL.
- Row click fetches detail and opens dialog.

Detail dialog:

- `Dialog` max width `min(96vw, 1440px)`; code area scrollable.
- Two `CodeViewer` components with original/suspect code and respective language.
- Metadata rows with user, time, language, status, score.
- Similarity badges for highest and Dice.
- CSS grid two columns on desktop; single column at max-width 760px with an optional tab switch. If using tabs, keep both panes mounted.
- Include note `相似度仅代表代码相似，需人工复核。`

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/web && npx vitest run src/components/contest/similarity`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/contest/similarity
git commit -m "feat: add similarity results and code detail"
```

---

### Task 13: Integrate the similarity tab into contest detail

**Files:**
- Modify: `apps/web/src/views/ContestDetailView.vue`
- Modify: `apps/web/src/views/ContestDetailView.spec.ts`

- [ ] **Step 1: Write failing integration tests**

Add a test with `auth.can.mockImplementation((code) => code === PERM.contest.update)` and an ended contest. Assert:

- `查重` tab exists.
- Clicking it renders `ContestSimilarityPanel` with the contest id.
- A non-ended contest does not show the tab.
- A manager without `contest.update` does not show it.

Mock the panel in the existing test file to avoid its API requests.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/web && npx vitest run src/views/ContestDetailView.spec.ts`
Expected: FAIL because the tab is missing.

- [ ] **Step 3: Implement the tab**

- Import `ContestSimilarityPanel`.
- Add `canManageSimilarity = computed(() => auth.can(PERM.contest.update))`.
- Add `Tab value="similarity"` only when `isEnded && canManageSimilarity`.
- Add `<TabPanel value="similarity"><ContestSimilarityPanel :contest-id="contest?.id ?? contestId" /></TabPanel>`.
- Keep `Tabs lazy`.
- Do not change existing tabs or fetch behavior when the tab is absent.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/web && npx vitest run src/views/ContestDetailView.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/ContestDetailView.vue apps/web/src/views/ContestDetailView.spec.ts
git commit -m "feat: add contest similarity tab"
```

---

### Task 14: Full verification and manual acceptance

**Files:**
- No production files expected.
- If a verification step exposes a bug, fix the owning task and add a regression test.

- [ ] **Step 1: Backend formatting and tests**

Run:

```bash
cd apps/server
gofmt -w internal/similarity internal/similarityqueue internal/similarityapi
go test ./internal/similarity/... ./internal/similarityqueue/... ./internal/similarityapi/... ./internal/db/... ./internal/router/... ./cmd/server/...
```

Expected: PASS.

- [ ] **Step 2: Frontend tests and typecheck**

Run:

```bash
cd apps/web
npx vitest run
npx vue-tsc -b
npx vite build
npx eslint .
npx prettier --check src
```

Expected: all PASS; no new warnings caused by this feature.

- [ ] **Step 3: Real browser acceptance**

Start the app with a seeded ended contest containing copied submissions. As a contest manager:

- Open `/contests/:id`, confirm `查重` only appears after the contest ended.
- Set a threshold and optional template; save.
- Run one problem and all problems; confirm statuses move `queued -> running -> completed`.
- Confirm the result list shows the earlier user on the left and the later user as suspected.
- Open a result and confirm both code panes render, percentages and metadata are visible.
- Export CSV and open it in Excel/Sheets; confirm Chinese headers and no formula execution.
- Check 390px, 768px, 1024px, 1440px widths: no horizontal page overflow; code panes scroll internally.
- Check console for errors.

- [ ] **Step 4: Commit verification-only fixes if any**

```bash
git add <only files changed by verification>
git commit -m "fix: address similarity acceptance issues"
```

If no files changed, skip this commit.

---

## Self-Review Checklist

- [ ] All spec requirements map to a task.
- [ ] No task leaves a function or type undefined for a later task.
- [ ] Tokenizer/engine tests cover comments, renaming, literals, Python indentation, preprocessor, short code, same-user, template exclusion, high-frequency filtering and threshold boundary.
- [ ] Repository tests cover participant/window/objective filtering and latest completed run.
- [ ] Handler tests cover 401/403, unfinished contest, cross-contest match, CSV BOM/injection.
- [ ] Frontend tests cover tab permission, save, run polling, filters, detail and CSV download.
- [ ] No new dependency or external service is introduced.
- [ ] Every commit stages only files listed in its task.
