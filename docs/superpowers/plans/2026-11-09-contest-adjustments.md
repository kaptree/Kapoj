# 比赛题目可见性、AI 开关与榜单展示实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让比赛题目按比赛时间窗口派生隐藏，赛后展示本场统计并恢复原可见性；比赛页面只保留本场题目导航；比赛可禁用参赛者 AI；排行榜按首杀/通过/未通过着色并显示 ICPC 错误次数。

**Architecture:** 后端保留 `problems.is_public` 不变，用 `contest_problems + contests.end_time` 派生“锁定”；题目列表、详情、统计、题解、讨论、普通提交和 AI 统一走比赛访问判定。比赛详情在赛后聚合本场提交/通过/个人状态；AI 生成请求新增 `problemId`，由服务端按所属进行中比赛强制校验。前端按比赛状态切换题目列表、右侧资源区和排行榜样式。

**Tech Stack:** Go + Gin + GORM + PostgreSQL；Vue 3 + PrimeVue + Vitest；OpenAPI 生成前端类型。

**Spec:** `docs/superpowers/specs/2026-11-09-contest-adjustments-design.md`

## Global Constraints

- 不修改 `problems.is_public`；锁定完全由比赛关系和时间窗口派生。
- 锁定窗口为 `contests.end_time > now`（upcoming + running）；多场比赛共用题目时以最后一场结束为准。
- 参赛者访问锁定题必须同时满足：已报名、比赛 `start_time <= now < end_time`、题目属于该比赛。
- 锁定期间，统计/题解/讨论对非管理员返回 404；题目不存在与无权访问统一 404。
- 赛后统计仅在 `contest.status == ended` 时返回；统计口径与排行榜一致：报名用户、报名后、比赛时间窗内、排除 `pending`/`judging`。
- AI 禁用只作用于进行中比赛的参赛者；`contest.update`/super 例外；`disableAi=false` 时正常使用。
- `complete/format/optimize` 的 `problemId` 为必填正整数；不存在的题目返回 404。
- 排行榜所有赛制着色；ICPC 已通过格显示 `-attempts`（AC 前错误次数），0 次错误显示 `-0`；总罚时公式不变。
- 所有新增/修改的中文文案保持现有风格；不新增依赖。

## Review Focus

- 参赛者手动去掉 `?contestId` 后仍不能通过列表、详情、统计、题解、讨论或 AI 绕过锁定（Task 3、Task 6）。
- 公开题同时属于多场未结束比赛时，不能因某一场结束提前解锁，也不能永久锁定（Task 1、Task 2、Task 5）。
- 赛后统计与排行榜口径一致：报名前提交、非参赛者管理员测试提交、`pending`/`judging` 不应造成数字不一致（Task 5、Task 7）。
- AI 防护不能被缺失/伪造 `problemId`、非参赛者身份或过宽的管理员例外绕过（Task 6）。
- 首杀在并列时间、OI 赛时自见榜单和 ICPC `-0` 情况下仍稳定、正确（Task 7、Task 13）。

---

### Task 1: 比赛领域锁定/AI 判定与 `disable_ai`

**Files:**
- Modify: `apps/server/internal/contest/model.go`
- Modify: `apps/server/internal/contest/validate.go`
- Modify: `apps/server/internal/contest/service.go`
- Modify: `apps/server/internal/contest/dto.go`
- Modify: `apps/server/internal/contest/repository.go`
- Test: `apps/server/internal/contest/service_test.go`
- Test: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Consumes: existing `Contest`, `Input`, `Repository`, `ContestParticipant`, `ContestProblem`.
- Produces:
  - `Contest.DisableAI bool` (`gorm:"not null;default:true"`)
  - `Input.DisableAI *bool` (`json:"disableAi"`)
  - `ListItem.DisableAI bool` (`json:"disableAi"`)；`toListItem` 填入 `c.DisableAI`
  - `Repository.IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)`
  - `Repository.CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)`（改为仅 running）
  - `Repository.AIBlockedForUser(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)`

- [ ] **Step 1: Write the failing tests**

在 `apps/server/internal/contest/service_test.go` 追加：

```go
func TestProblemLockUsesUnfinishedContests(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()
	p := e.problems[0].ID

	in := baseInput(e)
	in.StartTime, in.EndTime = now.Add(time.Hour), now.Add(2*time.Hour)
	c, err := e.svc.Create(ctx, manager(), in)
	require.NoError(t, err)

	locked, err := e.repo.IsProblemLocked(ctx, p, now)
	require.NoError(t, err)
	assert.True(t, locked, "upcoming contest must lock")

	// 第二场未结束比赛继续锁定
	in2 := baseInput(e)
	in2.StartTime, in2.EndTime = now.Add(-time.Minute), now.Add(3*time.Hour)
	_, err = e.svc.Create(ctx, manager(), in2)
	require.NoError(t, err)
	locked, err = e.repo.IsProblemLocked(ctx, p, now)
	require.NoError(t, err)
	assert.True(t, locked)

	// 两场都结束后解锁
	require.NoError(t, e.db.Exec(
		`UPDATE contests SET end_time = ? WHERE id = ?`, now.Add(-time.Minute), c.ID,
	).Error)
	require.NoError(t, e.db.Exec(
		`UPDATE contests SET end_time = ? WHERE id <> ?`, now.Add(-time.Minute), c.ID,
	).Error)
	locked, err = e.repo.IsProblemLocked(ctx, p, now)
	require.NoError(t, err)
	assert.False(t, locked)
}

func TestCanViewProblemOnlyWhileRunning(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()
	p := e.problems[0].ID
	in := baseInput(e)
	c, err := e.svc.Create(ctx, manager(), in)
	require.NoError(t, err)
	require.NoError(t, e.repo.AddParticipant(ctx, c.ID, userID))

	ok, err := e.repo.CanViewProblem(ctx, userID, p, now)
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, e.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	ok, err = e.repo.CanViewProblem(ctx, userID, p, now)
	require.NoError(t, err)
	assert.False(t, ok, "ended contests must not keep hidden-problem access")
}

func TestAIBlockedForParticipant(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()
	p := e.problems[0].ID
	in := baseInput(e)
	c, err := e.svc.Create(ctx, manager(), in)
	require.NoError(t, err)
	require.NoError(t, e.repo.AddParticipant(ctx, c.ID, userID))

	blocked, err := e.repo.AIBlockedForUser(ctx, userID, p, now)
	require.NoError(t, err)
	assert.True(t, blocked, "default disable_ai must block participants")

	blocked, err = e.repo.AIBlockedForUser(ctx, otherID, p, now)
	require.NoError(t, err)
	assert.False(t, blocked, "non-participants are not blocked by this contest")

	require.NoError(t, e.db.Exec(`UPDATE contests SET disable_ai = false WHERE id = ?`, c.ID).Error)
	blocked, err = e.repo.AIBlockedForUser(ctx, userID, p, now)
	require.NoError(t, err)
	assert.False(t, blocked)
}

func TestContestDisableAIDefaultAndUpdate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, err := e.svc.Create(ctx, manager(), baseInput(e))
	require.NoError(t, err)
	assert.True(t, c.DisableAI)

	in := baseInput(e)
	in.DisableAI = boolPtr(false)
	updated, err := e.svc.Update(ctx, manager(), c.ID, in)
	require.NoError(t, err)
	assert.False(t, updated.DisableAI)

	in.DisableAI = nil
	kept, err := e.svc.Update(ctx, manager(), c.ID, in)
	require.NoError(t, err)
	assert.False(t, kept.DisableAI, "omitted update keeps existing value")
}

func boolPtr(v bool) *bool { return &v }
```

在 `apps/server/internal/db/db_test.go` 追加迁移断言：

```go
assert.True(t, g.Migrator().HasColumn(&contest.Contest{}, "disable_ai"))
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/contest/ ./internal/db/`
Expected: FAIL，提示 `DisableAI`/`IsProblemLocked`/`AIBlockedForUser` 未定义。

- [ ] **Step 3: Implement**

`Contest` 增加：

```go
DisableAI bool `gorm:"column:disable_ai;not null;default:true"`
```

`Input` 增加：

```go
DisableAI *bool `json:"disableAi"`
```

`ListItem` 增加 `DisableAI bool`，`toListItem` 返回 `DisableAI: c.DisableAI`。

`Service.Create`：缺省 true：

```go
disableAI := true
if in.DisableAI != nil { disableAI = *in.DisableAI }
c := &Contest{..., DisableAI: disableAI}
```

`Service.Update`：只在显式提供时修改：

```go
if in.DisableAI != nil { c.DisableAI = *in.DisableAI }
```

`Clone` 使用 `c := *original`，天然保留 `DisableAI`；不要覆盖。

`Repository` 增加三个方法；`CanViewProblem` 改为：

```go
WHERE cp.user_id = ? AND cpb.problem_id = ?
  AND c.start_time <= ? AND c.end_time > ?
```

`IsProblemLocked` 用 `c.end_time > ?`；`AIBlockedForUser` 用 `c.disable_ai = true AND c.start_time <= ? AND c.end_time > ?`，并 join `contest_participants`。

`db.Migrate` 已包含 `&contest.Contest{}`，无需额外注册。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/contest/ ./internal/db/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/contest apps/server/internal/db
git commit -m "feat(contest): add lock and AI access checks"
```

---

### Task 2: 题库列表隐藏锁定题

**Files:**
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Test: `apps/server/internal/problem/service_test.go`

**Interfaces:**
- Consumes: `contest_problems`, `contests`.
- Produces:
  - `problem.Viewer.CanViewLocked bool`
  - `problem.Filter.IncludeLocked bool`
  - `problem.Filter.Now time.Time`
  - `problemapi.viewer(c)` 为 `CanViewLocked` 派生 `problem.view_hidden || contest.update`

- [ ] **Step 1: Write the failing test**

在 `apps/server/internal/problem/service_test.go` 追加：

```go
func TestListHidesContestLockedProblems(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("locked"))
	e.addCase(t, p.ID)
	in := input("locked")
	in.IsPublic = true
	_, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)

	repo := contest.NewGormRepository(e.db)
	now := time.Now()
	c := &contest.Contest{
		Title: "Lock", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, repo.Create(ctx, c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))

	page, err := e.svc.List(ctx, problem.Viewer{}, problem.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.Zero(t, page.Total, "locked problem must be hidden from the library")

	page, err = e.svc.List(ctx, problem.Viewer{CanViewLocked: true}, problem.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total, "managers can still see public locked problems")

	require.NoError(t, e.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	page, err = e.svc.List(ctx, problem.Viewer{}, problem.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total, "ended contest unlocks a public problem")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/problem/ -run TestListHidesContestLockedProblems`
Expected: FAIL，`page.Total` 仍为 1。

- [ ] **Step 3: Implement**

`problem.Viewer` 增加 `CanViewLocked bool`；`Filter` 增加 `IncludeLocked bool`、`Now time.Time`。

`Service.List`：

```go
now := time.Now()
items, total, err := s.repo.List(ctx, Filter{
    ..., IncludeHidden: q.IncludeHidden && v.CanViewHidden,
    IncludeLocked: v.CanViewLocked, Now: now,
}, ...)
```

`Repository.List` 在 `!f.IncludeLocked` 时追加：

```go
now := f.Now
if now.IsZero() { now = time.Now() }
q = q.Where(`NOT EXISTS (
    SELECT 1 FROM contest_problems cp
    JOIN contests c ON c.id = cp.contest_id
    WHERE cp.problem_id = problems.id AND c.end_time > ?
)`, now)
```

`problemapi.viewer(c)` 增加：

```go
CanViewLocked: access.Has(string(rbac.PermProblemViewHidden)) ||
    access.Has(string(rbac.PermContestUpdate)),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/problem/ ./internal/problemapi/`
Expected: PASS（现有题库列表测试不受影响）。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi/handler.go
git commit -m "feat(problem): hide contest-locked problems from library"
```

---

### Task 3: 题目详情、统计、题解与讨论的比赛保护

**Files:**
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problemapi/official_solution_handler.go`
- Modify: `apps/server/internal/problemapi/discussion_handler.go`
- Test: `apps/server/internal/problemapi/handler_test.go`

**Interfaces:**
- Consumes: `contest.Repository.IsProblemLocked`, `CanViewProblem`; RBAC `problem.view_hidden`, `contest.update`, `problem.update`/`CanViewSolution`.
- Produces:
  - `ContestAccess` 增加 `IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)`
  - handler helper `lockedProblemAccess(c *gin.Context, problemID int64) (locked, participantAllowed, manager bool, err error)`
  - handler helper `lockedResourceBlocked(c *gin.Context, problemID int64) (bool, error)`

- [ ] **Step 1: Write the failing tests**

在 `apps/server/internal/problemapi/handler_test.go` 追加：

```go
func TestLockedContestProblemDetailAndResources(t *testing.T) {
	f := newFx(t)
	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Locked", Slug: "locked-resource", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: 1,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "a.in", OutputFile: "a.out", Score: 10,
	}))

	w, reg := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": "lockeduser", "email": "locked@example.com", "password": "correct-horse",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	token := reg["data"].(map[string]any)["accessToken"].(string)
	var userID int64
	require.NoError(t, f.db.Raw(`SELECT id FROM users WHERE username = 'lockeduser'`).Scan(&userID).Error)

	now := time.Now()
	contestRepo := contest.NewGormRepository(f.db)
	c := &contest.Contest{
		Title: "Locked contest", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, contestRepo.Create(context.Background(), c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))
	require.NoError(t, contestRepo.AddParticipant(context.Background(), c.ID, userID))

	// 游客与参赛者：赛前/赛中详情行为
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/statistics", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/official-solution", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/solutions", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), token, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// 赛后：公开题恢复；隐藏题仍 404
	require.NoError(t, f.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), "", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}
```

在 `TestHiddenContestProblemVisibleToParticipant` 末尾补赛后断言：

```go
require.NoError(t, f.db.Exec(
	`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
	now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
).Error)
w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), token, nil)
assert.Equal(t, http.StatusNotFound, w.Code, "hidden problem stays hidden after contest")
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/problemapi/ -run 'TestLockedContestProblemDetailAndResources|TestHiddenContestProblemVisibleToParticipant'`
Expected: FAIL，统计/题解/讨论仍返回 200，或隐藏题赛后仍可访问。

- [ ] **Step 3: Implement**

`problemapi.ContestAccess` 增加 `IsProblemLocked`。`problemapi.handler` 增加：

```go
func (h *handler) lockedProblemAccess(c *gin.Context, problemID int64) (locked, participantAllowed, manager bool, err error) {
	if h.contests == nil {
		return false, true, false, nil
	}
	locked, err = h.contests.IsProblemLocked(c.Request.Context(), problemID, time.Now())
	if err != nil || !locked {
		return locked, !locked, false, err
	}
	manager = viewer(c).CanViewHidden || currentCanManageContest(c) || currentIsSuper(c)
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return true, false, manager, nil
	}
	participantAllowed, err = h.contests.CanViewProblem(c.Request.Context(), claims.UserID, problemID, time.Now())
	return true, participantAllowed, manager, err
}
```

`get`：

```go
locked, participantAllowed, manager, err := h.lockedProblemAccess(c, id)
if err != nil { httpx.Fail(c, err); return }
if locked && !manager && !participantAllowed { httpx.Fail(c, httpx.ErrNotFound); return }
// 只有真正的参赛者访问才临时放开隐藏题；contest.update 管理员不能借此看原本隐藏题。
if locked && participantAllowed && !manager && !v.CanViewHidden { v.CanViewHidden = true }
```

锁定且非题解资源管理员时强制 `includeSolution = false`。可以在 `includeSolution` 计算后加：

```go
if locked && !v.CanViewSolution && !currentIsSuper(c) {
	includeSolution = false
}
```

`statistics`：

```go
locked, _, _, err := h.lockedProblemAccess(c, id)
if err != nil { httpx.Fail(c, err); return }
if locked && !currentCanViewLockedStats(c) {
	httpx.Fail(c, httpx.ErrNotFound)
	return
}
```

其中 `currentCanViewLockedStats` 为 `problem.view_hidden || contest.update || super`。

新增 helper：

```go
func currentHas(c *gin.Context, code string) bool {
	access, ok := middleware.CurrentAccess(c)
	return ok && access != nil && access.Has(code)
}

func currentCanManageContest(c *gin.Context) bool {
	return currentHas(c, string(rbac.PermContestUpdate))
}

func currentCanViewLockedStats(c *gin.Context) bool {
	return currentHas(c, string(rbac.PermProblemViewHidden)) ||
		currentHas(c, string(rbac.PermContestUpdate)) || currentIsSuper(c)
}
```

`lockedResourceBlocked`：锁定且 `!viewer(c).CanViewSolution && !currentIsSuper(c)` 时返回 true。把该检查加到：

- `getOfficialSolution`、`updateOfficialSolution`、`purchaseOfficialSolution`。
- `listPosts`、`createPost`、`getPost`、`updatePost`、`deletePost`、`likePost`、`unlikePost`、`createReply`、`updateReply`、`deleteReply`。

不通过时 `httpx.Fail(c, httpx.ErrNotFound); return`。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/problemapi/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/problemapi
git commit -m "feat(problemapi): guard locked contest resources"
```

---

### Task 4: 普通提交拦截锁定题

**Files:**
- Modify: `apps/server/internal/submission/service.go`
- Test: `apps/server/internal/submission/service_test.go`

**Interfaces:**
- Consumes: `contest.Repository.IsProblemLocked`.
- Produces: `ContestLookup` 增加 `IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)`。

- [ ] **Step 1: Write the failing test**

在 `apps/server/internal/submission/service_test.go` 追加：

```go
func TestLockedProblemRequiresContestSubmission(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	createUser(t, e.db, 2)
	p := e.createProblem(t, publicProblem("locked-submit"), 1)

	now := time.Now()
	c := &contest.Contest{
		Title: "Locked submit", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, e.contests.Create(ctx, c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))
	require.NoError(t, e.contests.AddParticipant(ctx, c.ID, 2))

	_, err := e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, codeInput(p.ID, "int main(){}"))
	assertCode(t, err, 40400)

	in := codeInput(p.ID, "int main(){}")
	in.ContestID = c.ID
	_, err = e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, in)
	require.NoError(t, err)

	require.NoError(t, e.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	_, err = e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, codeInput(p.ID, "int main(){}"))
	require.NoError(t, err, "ended contest unlocks ordinary submission")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/submission/ -run TestLockedProblemRequiresContestSubmission`
Expected: FAIL，普通提交成功而不是 404。

- [ ] **Step 3: Implement**

`ContestLookup` 增加 `IsProblemLocked`。`Create` 在 `contestID == nil` 分支、现有 hidden 检查之前：

```go
if contestID == nil && !v.CanViewHiddenProblems && !v.CanManageContest {
	locked, err := s.contests.IsProblemLocked(ctx, p.ID, time.Now())
	if err != nil { return nil, err }
	if locked { return nil, httpx.ErrNotFound }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/submission/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/submission
git commit -m "feat(submission): block ordinary submissions to locked problems"
```

---

### Task 5: 赛后统计、时间/空间标签与 `locked`

**Files:**
- Modify: `apps/server/internal/contest/dto.go`
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/contest/service.go`
- Test: `apps/server/internal/contest/service_test.go`

**Interfaces:**
- Consumes: `submissions`, `contest_participants`, `contests`.
- Produces:
  - `ProblemRow.ProblemTimeLimitMS int`, `ProblemMemoryLimitMB int`, `ProblemIsPublic bool`
  - `ProblemItem` 新增（JSON 字段名必须与 OpenAPI 一致）：
    - `TimeLimitMS int` (`json:"timeLimitMs"`)
    - `MemoryLimitMB int` (`json:"memoryLimitMb"`)
    - `IsPublic bool` (`json:"isPublic"`)
    - `Locked bool` (`json:"locked"`)
    - `SubmitCount int` (`json:"submitCount"`)
    - `AcceptCount int` (`json:"acceptCount"`)
    - `MyStatus string` (`json:"myStatus"`)
    - `MyAttempts int` (`json:"myAttempts"`)
  - `Repository.ProblemStats(ctx, contestID int64) (map[int64]ProblemStat, error)`
  - `Repository.UserProblemStats(ctx, contestID, userID int64) (map[int64]UserProblemStat, error)`
  - `Repository.LockedProblemIDs(ctx context.Context, problemIDs []int64, now time.Time) (map[int64]bool, error)`
  - `type ProblemStat struct { SubmitCount int `gorm:"column:submit_count"`; AcceptCount int `gorm:"column:accept_count"` }`
  - `type UserProblemStat struct { Attempts int; Accepted bool; LastStatus string }`

- [ ] **Step 1: Write the failing tests**

在 `apps/server/internal/contest/service_test.go` 追加：

```go
func TestContestDetailPostContestStats(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()
	start := now.Add(-2 * time.Hour)
	in := baseInput(e)
	in.StartTime, in.EndTime = start, now.Add(-time.Hour)
	c, err := e.svc.Create(ctx, manager(), in)
	require.NoError(t, err)
	require.NoError(t, e.repo.AddParticipant(ctx, c.ID, userID))
	require.NoError(t, e.repo.AddParticipant(ctx, c.ID, otherID))
	require.NoError(t, e.db.Exec(
		`UPDATE contest_participants SET register_time = ? WHERE contest_id = ?`,
		start.Add(-time.Minute), c.ID,
	).Error)

	p := e.problems[0].ID
	insertRankSubmission(t, e, c.ID, userID, p, "wrong_answer", 0, start.Add(5*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p, "accepted", 100, start.Add(10*time.Minute))
	insertRankSubmission(t, e, c.ID, otherID, p, "wrong_answer", 0, start.Add(6*time.Minute))
	// pending 不计入
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, contest_id, language, code, status, created_at)
		 VALUES (?, ?, ?, 'cpp', 'x', 'pending', ?)`, otherID, p, c.ID, start.Add(7*time.Minute),
	).Error)

	detail, err := e.svc.Detail(ctx, user(), c.ID)
	require.NoError(t, err)
	require.Len(t, detail.Problems, 2)
	assert.EqualValues(t, 3, detail.Problems[0].SubmitCount)
	assert.EqualValues(t, 1, detail.Problems[0].AcceptCount)
	assert.Equal(t, "accepted", detail.Problems[0].MyStatus)
	assert.Equal(t, 2, detail.Problems[0].MyAttempts)
	assert.EqualValues(t, 1000, detail.Problems[0].TimeLimitMS)
	assert.EqualValues(t, 256, detail.Problems[0].MemoryLimitMB)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/contest/ -run TestContestDetailPostContestStats`
Expected: FAIL，字段为零值。

- [ ] **Step 3: Implement**

`ProblemRow` 增加带显式列的字段：

```go
ProblemTimeLimitMS   int  `gorm:"column:problem_time_limit_ms"`
ProblemMemoryLimitMB int  `gorm:"column:problem_memory_limit_mb"`
ProblemIsPublic      bool `gorm:"column:problem_is_public"`
```

`FindProblems` 的 select 增加：

```sql
p.time_limit_ms AS problem_time_limit_ms,
p.memory_limit_mb AS problem_memory_limit_mb,
p.is_public AS problem_is_public
```

`toProblemItem` 填入 `TimeLimitMS`、`MemoryLimitMB`、`IsPublic`。

`ProblemStats` SQL：

```sql
SELECT s.problem_id,
       COUNT(*) AS submit_count,
       COUNT(DISTINCT s.user_id) FILTER (WHERE s.status = 'accepted') AS accept_count
FROM submissions s
JOIN contest_participants p
  ON p.contest_id = s.contest_id AND p.user_id = s.user_id
JOIN contests c ON c.id = s.contest_id
WHERE s.contest_id = ?
  AND s.status NOT IN ('pending', 'judging')
  AND s.created_at >= c.start_time
  AND s.created_at < c.end_time
  AND s.created_at >= p.register_time
GROUP BY s.problem_id
```

`UserProblemStats` SQL 同过滤条件，追加 `AND s.user_id = ?`，按 `created_at, id` 升序读取 `status`，在 Go 中聚合为 attempts/accepted/lastStatus。

`LockedProblemIDs` 用 `WHERE cp.problem_id IN (?) AND c.end_time > ?`；`problemIDs` 为空时直接返回空 map。

`Service.Detail`：

```go
status := StatusAt(c.StartTime, c.EndTime, now)
if status == StatusEnded {
	stats, err := s.repo.ProblemStats(ctx, id)
	...
	mine := map[int64]UserProblemStat{}
	if v.UserID != 0 { mine, err = s.repo.UserProblemStats(ctx, id, v.UserID); ... }
	ids := ...; locked, err := s.repo.LockedProblemIDs(ctx, ids, now)
	for i := range problems {
		problems[i].SubmitCount = stats[row.ProblemID].SubmitCount
		...
		problems[i].Locked = locked[row.ProblemID]
	}
}
```

赛前/赛中 `Locked` 也要填，便于前端；`LockedProblemIDs` 不依赖 `status == ended`。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/contest/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/contest
git commit -m "feat(contest): expose post-contest problem stats"
```

---

### Task 6: AI 比赛上下文与后端强制校验

**Files:**
- Modify: `apps/server/internal/httpx/errors.go`
- Modify: `apps/server/internal/ai/generate.go`
- Modify: `apps/server/internal/aiapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Test: `apps/server/internal/aiapi/handler_test.go`

**Interfaces:**
- Consumes: `contest.Repository.IsProblemLocked`, `CanViewProblem`, `AIBlockedForUser`; `problem.Repository.FindByID`.
- Produces:
  - `httpx.ErrContestAIDisabled`（40302，文案“比赛期间已禁用 AI”）
  - `ai.GenerateInput.ProblemID int64`（`json:"problemId"`）
  - `aiapi.Options{ContestAccess ContestAccess, Problems ProblemLookup}`
  - `aiapi.ContestAccess` 三个方法
  - `aiapi.ProblemLookup`：`FindByID(ctx context.Context, id int64) (*problem.Problem, error)`

- [ ] **Step 1: Write the failing tests**

在 `apps/server/internal/aiapi/handler_test.go` 追加：

```go
func newAIContestHandler(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	g := testutil.PG(t)
	require.NoError(t, ai.SeedProviders(context.Background(), g))
	km, err := ai.NewKeyManager(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "")
	require.NoError(t, err)
	svc := ai.NewService(g, km, nil, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	issuer := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute)
	api := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(1<<20))
	aiapi.RegisterRoutes(api, svc, aiapi.Options{
		ContestAccess: contest.NewGormRepository(g),
		Problems:      problem.NewGormRepository(g),
	})
	return r, g
}

func TestAIContestGuard(t *testing.T) {
	h, g := newAIContestHandler(t)
	uid := createUser(t, g, "aicontest")
	token := tokenFor(t, uid)
	prepo := problem.NewGormRepository(g)
	p := &problem.Problem{
		Title: "AI Locked", Slug: "ai-locked", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: 1,
	}
	require.NoError(t, prepo.Create(context.Background(), p))

	now := time.Now()
	crepo := contest.NewGormRepository(g)
	c := &contest.Contest{
		Title: "AI contest", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, crepo.Create(context.Background(), c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))
	require.NoError(t, crepo.AddParticipant(context.Background(), c.ID, uid))

	w, resp := do(t, h, http.MethodPost, "/api/ai/complete", token,
		fmt.Sprintf(`{"code":"int main(){}","language":"cpp","cursor":0,"problemId":%d}`, p.ID))
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.EqualValues(t, 40302, resp["code"])

	// 关闭开关后不再 40302（后续因未配置模型返回 40001，证明已通过比赛校验）
	require.NoError(t, g.Exec(`UPDATE contests SET disable_ai = false WHERE id = ?`, c.ID).Error)
	w, resp = do(t, h, http.MethodPost, "/api/ai/complete", token,
		fmt.Sprintf(`{"code":"int main(){}","language":"cpp","cursor":0,"problemId":%d}`, p.ID))
	assert.NotEqual(t, http.StatusForbidden, w.Code)
	assert.NotEqualValues(t, 40302, resp["code"])

	// 缺少 problemId -> 40001
	w, resp = do(t, h, http.MethodPost, "/api/ai/format", token, `{"code":"x","language":"cpp"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}
```

同时补一个“锁定题未授权 404”的用例：未报名用户使用同一 `problemId` 调 `/api/ai/hint`，期望 404。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/aiapi/ -run TestAIContestGuard`
Expected: FAIL，缺少字段/40302 未定义。

- [ ] **Step 3: Implement**

`httpx` 增加：

```go
ErrContestAIDisabled = newErr(http.StatusForbidden, 40302, "比赛期间已禁用 AI")
```

`ai.GenerateInput` 只增加 `ProblemID int64`；不要在 `validateGenerateInput` 里加校验，避免改动 19 处直接调用 `ai.Service` 的既有测试。请求层在 `aiapi.checkContestAI` 校验：

```go
if problemID <= 0 { httpx.Fail(c, httpx.ErrValidation("缺少题目 ID")); return false }
```

`aiapi` 定义：

```go
type ContestAccess interface {
	IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)
	CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
	AIBlockedForUser(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
}

type ProblemLookup interface {
	FindByID(ctx context.Context, id int64) (*problem.Problem, error)
}
```

`Options` 增加 `ContestAccess`、`Problems`；`handler` 保存它们。

新增：

```go
func (h *handler) checkContestAI(c *gin.Context, problemID int64) bool {
	if problemID <= 0 {
		httpx.Fail(c, httpx.ErrValidation("缺少题目 ID"))
		return false
	}
	claims, ok := middleware.CurrentClaims(c)
	if !ok { httpx.Fail(c, httpx.ErrUnauthorized); return false }
	if h.problems != nil {
		if _, err := h.problems.FindByID(c.Request.Context(), problemID); err != nil {
			if errors.Is(err, problem.ErrNotFound) { httpx.Fail(c, httpx.ErrNotFound); return false }
			httpx.Fail(c, err); return false
		}
	}
	if h.contests == nil { return true }
	now := time.Now()
	locked, err := h.contests.IsProblemLocked(c.Request.Context(), problemID, now)
	if err != nil { httpx.Fail(c, err); return false }
	if locked {
		allowed, err := h.contests.CanViewProblem(c.Request.Context(), claims.UserID, problemID, now)
		if err != nil { httpx.Fail(c, err); return false }
		if !allowed && !currentCanViewHidden(c) && !currentCanManageContest(c) && !currentIsSuper(c) {
			httpx.Fail(c, httpx.ErrNotFound); return false
		}
	}
	blocked, err := h.contests.AIBlockedForUser(c.Request.Context(), claims.UserID, problemID, now)
	if err != nil { httpx.Fail(c, err); return false }
	if blocked && !currentCanManageContest(c) && !currentIsSuper(c) {
		httpx.Fail(c, httpx.ErrContestAIDisabled); return false
	}
	return true
}
```

`complete/format/optimize` 绑定输入后调用 `checkContestAI(c, in.ProblemID)`；`hint` 绑定后调用 `checkContestAI(c, in.ProblemID)`。

`router.New`：把 `contestRepo := contest.NewGormRepository(d.DB)` 和 `problemRepo := problem.NewGormRepository(d.DB)` 提到 AI 注册之前，并复用：

```go
aiapi.RegisterRoutes(api, d.AI, aiapi.Options{
	RBAC: d.RBAC, Settings: d.AISettings,
	ContestAccess: contestRepo, Problems: problemRepo,
})
```

删除后面重复的 `contestRepo`/`problemRepo` 声明。

`aiapi` 需要 `currentCanViewHidden`（`problem.view_hidden`）、`currentCanManageContest`（`contest.update`）/`currentIsSuper` 的小 helper（aiapi 已有 rbac，可用 `middleware.CurrentAccess`）。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/aiapi/ ./internal/router/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/httpx apps/server/internal/ai apps/server/internal/aiapi apps/server/internal/router
git commit -m "feat(ai): enforce contest AI disable by problem context"
```

---

### Task 7: 排行榜首杀标记

**Files:**
- Modify: `apps/server/internal/contest/ranklist.go`
- Test: `apps/server/internal/contest/ranklist_test.go`

**Interfaces:**
- Consumes: existing `RankCell`, `RankRow`, `computeICPC`, `computeScoreBased`.
- Produces: `RankCell.FirstBlood bool`（`json:"firstBlood"`）。

- [ ] **Step 1: Write the failing tests**

在 `apps/server/internal/contest/ranklist_test.go` 追加：

```go
func TestRanklistMarksFirstBlood(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeICPC, start, now.Add(time.Hour), userID, otherID)
	p := e.problems[0].ID

	insertRankSubmission(t, e, c.ID, userID, p, "wrong_answer", 0, start.Add(5*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p, "accepted", 100, start.Add(10*time.Minute))
	insertRankSubmission(t, e, c.ID, otherID, p, "accepted", 100, start.Add(15*time.Minute))

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	byUser := map[int64]contest.RankRow{}
	for _, row := range rank.Rows { byUser[row.UserID] = row }
	assert.True(t, byUser[userID].Problems[0].FirstBlood)
	assert.False(t, byUser[otherID].Problems[0].FirstBlood)
	assert.Equal(t, 1, byUser[userID].Problems[0].Attempts)
	assert.Equal(t, 0, byUser[otherID].Problems[0].Attempts)
}

func TestRanklistFirstBloodTieUsesUserID(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeICPC, start, now.Add(time.Hour), userID, otherID)
	p := e.problems[0].ID
	at := start.Add(10 * time.Minute)
	insertRankSubmission(t, e, c.ID, otherID, p, "accepted", 100, at)
	insertRankSubmission(t, e, c.ID, userID, p, "accepted", 100, at)

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	byUser := map[int64]contest.RankRow{}
	for _, row := range rank.Rows { byUser[row.UserID] = row }
	assert.False(t, byUser[otherID].Problems[0].FirstBlood, "lower user ID wins the tie")
	assert.True(t, byUser[userID].Problems[0].FirstBlood)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/contest/ -run 'TestRanklistMarksFirstBlood|TestRanklistFirstBloodTieUsesUserID'`
Expected: FAIL，`FirstBlood` 字段未定义或为 false。

- [ ] **Step 3: Implement**

`RankCell` 增加 `FirstBlood bool`。新增：

```go
func markFirstBlood(rows []RankRow) {
	if len(rows) == 0 { return }
	for idx := range rows[0].Problems {
		var winner *RankCell
		var winnerID int64
		for i := range rows {
			cell := &rows[i].Problems[idx]
			if cell.Status != "accepted" || cell.AcceptedAt == nil { continue }
			if winner == nil || cell.AcceptedAt.Before(*winner.AcceptedAt) ||
				(cell.AcceptedAt.Equal(*winner.AcceptedAt) && rows[i].UserID < winnerID) {
				winner, winnerID = cell, rows[i].UserID
			}
		}
		if winner != nil { winner.FirstBlood = true }
	}
}
```

在 `Ranklist` 构造 `rows` 后、`sortRankRows` 前调用 `markFirstBlood(rows)`；OI 自见过滤在其后。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/contest/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/contest/ranklist.go apps/server/internal/contest/ranklist_test.go
git commit -m "feat(contest): mark first blood in ranklist"
```

---

### Task 8: OpenAPI 契约与前端生成类型

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify if needed: `apps/web/src/api/types.ts`

**Interfaces:**
- Consumes: backend DTO field names from Tasks 1/5/6/7.
- Produces: generated types for `disableAi`, `firstBlood`, `locked`, stats fields, AI `problemId`.

- [ ] **Step 1: Update OpenAPI schemas**

- `ContestListItem.required` 加 `disableAi`；properties 加 `disableAi: { type: boolean, default: true }`。
- `ContestInput.properties` 加 `disableAi: { type: boolean, default: true, description: 创建缺省 true；更新缺省保留原值 }`。
- `ContestProblemItem.required` 加 `timeLimitMs, memoryLimitMb, isPublic, locked, submitCount, acceptCount, myStatus, myAttempts`；properties 加这些字段。
- `ContestRankCell.required` 加 `firstBlood`；properties 加 `firstBlood: { type: boolean }`。
- `AICompleteInput`、`AIFormatInput`、`AIOptimizeInput`：`required` 加 `problemId`；properties 加 `problemId: { type: integer, format: int64, minimum: 1 }`。
- `/ai/*` description 补充：锁定题 404；比赛禁用 AI 40302；管理员例外。

- [ ] **Step 2: Regenerate types**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm gen:api`
Expected: 命令成功，`apps/web/src/api/schema.d.ts` 更新。

- [ ] **Step 3: Verify typecheck and fix fixtures**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vue-tsc -b --force`
Expected: PASS。若 `ContestListItem` 新增必填 `disableAi` 导致 `ContestListView.spec.ts`、`ProblemContestsView.spec.ts` fixture 报错，在本步补齐 `disableAi: true`；`ContestProblemItem`/`ContestRankCell`/AI 输入的 fixture 缺字段留到对应前端任务修复。

- [ ] **Step 4: Commit**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts
git commit -m "docs(api): add contest visibility and AI fields"
```

---

### Task 9: 比赛详情页时间/空间标签、赛后统计与跳转

**Files:**
- Modify: `apps/web/src/views/ContestDetailView.vue`
- Test: `apps/web/src/views/ContestDetailView.spec.ts`

**Interfaces:**
- Consumes: `ContestProblemItem.timeLimitMs`, `memoryLimitMb`, `isPublic`, `locked`, `submitCount`, `acceptCount`, `myStatus`, `myAttempts`; `ContestDetail.status`.
- Produces: UI columns/tags and jump behavior.

- [ ] **Step 1: Update fixtures and write failing tests**

在 `ContestDetailView.spec.ts` 的 `detail()` 中补齐 `disableAi` 和每个 problem 的新字段：

```ts
disableAi: true,
problems: [{
  problemId: 1, title: 'Two Sum', slug: 'two-sum', displayId: 'A', sortOrder: 1, score: 100,
  timeLimitMs: 1000, memoryLimitMb: 256, isPublic: true, locked: false,
  submitCount: 0, acceptCount: 0, myStatus: '', myAttempts: 0,
}],
```

新增：

```ts
it('shows limits and post-contest stats with problem-bank links', async () => {
  vi.mocked(getContest).mockResolvedValue(detail({
    status: 'ended',
    problems: [{
      problemId: 1, title: 'Two Sum', slug: 'two-sum', displayId: 'A', sortOrder: 1, score: 100,
      timeLimitMs: 1000, memoryLimitMb: 256, isPublic: true, locked: false,
      submitCount: 12, acceptCount: 3, myStatus: 'accepted', myAttempts: 2,
    }],
  }))
  const wrapper = await mountReady()
  expect(wrapper.text()).toContain('1000 ms')
  expect(wrapper.text()).toContain('256 MB')
  expect(wrapper.text()).toContain('提交 12 / 通过 3')
  expect(wrapper.text()).toContain('已通过 · 2 次')
  expect(wrapper.find('[data-test="problem-bank-link"]').attributes('href')).toContain('/problems/1')
})
```

再加：`status: 'running'` 时不渲染“本场”/“我的”列；`isPublic:false` 或 `locked:true` 时显示“未公开”/“比赛中”且无跳转。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ContestDetailView.spec.ts`
Expected: FAIL，缺少标签/列/按钮。

- [ ] **Step 3: Implement**

- `isEnded = computed(() => contest.value?.status === 'ended')`。
- 表格增加“限制”列：

```vue
<Column header="限制" style="width: 150px">
  <template #body="{ data }">
    <div class="limit-tags">
      <Tag :value="`${data.timeLimitMs} ms`" severity="secondary" />
      <Tag :value="`${data.memoryLimitMb} MB`" severity="secondary" />
    </div>
  </template>
</Column>
```

- `v-if="isEnded"` 增加“本场”“我的”列；我的状态用 `Tag`，`data.myStatus === 'accepted'` success，`data.myAttempts > 0` danger，否则 secondary。
- 标题链接：`status === 'ended'` 时不带 `contestId`；若 `!data.isPublic || data.locked`，改为纯文本，不渲染 `RouterLink`。
- 操作列：赛后渲染 `Button as="router-link"`，`data-test="problem-bank-link"`；`!data.isPublic` 显示 `Tag value="未公开"`，`data.locked` 显示 `Tag value="比赛中"`，不渲染按钮。
- 增加 `.limit-tags` 样式（flex + gap）。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ContestDetailView.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/ContestDetailView.vue apps/web/src/views/ContestDetailView.spec.ts
git commit -m "feat(web): show contest limits and post-contest stats"
```

---

### Task 10: 题目详情页比赛模式

**Files:**
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Test: `apps/web/src/views/ProblemDetailView.spec.ts`

**Interfaces:**
- Consumes: `getContest`, `ContestDetail`, `auth.can(PERM.contest.update)`, `PERM`.
- Produces:
  - `contestMode` computed
  - `contestProblems` list
  - AI hint visibility condition

- [ ] **Step 1: Write failing tests**

在 `ProblemDetailView.spec.ts` 增加 `getContest` mock 和用例：

```ts
vi.mock('@/api/contests', () => ({ getContest: vi.fn() }))

it('uses contest problem navigation and hides resources during a running contest', async () => {
  route.query = { contestId: '5' }
  vi.mocked(getProblem).mockResolvedValue(detail(1))
  vi.mocked(getContest).mockResolvedValue(contest({
    status: 'running', disableAi: true,
    problems: [problemItem(1, 'A'), problemItem(2, 'B')],
  }))
  const wrapper = await mountView()
  expect(wrapper.find('[data-test="contest-problem-list"]').exists()).toBe(true)
  expect(wrapper.text()).not.toContain('官方题解')
  expect(wrapper.text()).not.toContain('解题思路')
  expect(wrapper.text()).not.toContain('提交统计')
  expect(wrapper.text()).not.toContain('关联比赛')
  expect(wrapper.find('[data-test="ai-hint-button"]').exists()).toBe(false)
})

it('keeps AI hint for contest admins', async () => {
  auth.can.mockReturnValue(true)
  // running + disableAi true + contest.update -> ai-hint-button exists
})
```

补齐 `contest()`/`problemItem()` fixture 的新字段。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ProblemDetailView.spec.ts`
Expected: FAIL，资源卡仍存在/没有本场题目列表。

- [ ] **Step 3: Implement**

- 读取 `getContest(contestId)`；保存 `contest`。
- `contestMode = computed(() => contestId.value > 0 && contest.value?.status === 'running')`。
- `contestMode` 时 `load` 不请求 `getStatistics`。
- `showSolutionResources` 在 `contestMode` 时返回 false。
- 右侧 `aside` 用 `v-if="contestMode"` 渲染 `data-test="contest-problem-list"`，每项 `RouterLink` 到 `{ name: 'problem-detail', params: { id: p.problemId }, query: { contestId: String(contestId) } }`，当前题加 `active` class。
- 非比赛模式保留原资源卡。
- AI 提示按钮加 `data-test="ai-hint-button"`，条件：
  ```ts
  const aiHintVisible = computed(() =>
    !(contestMode.value && contest.value?.disableAi && !auth.can(PERM.contest.update)),
  )
  ```
- 增加右侧列表样式。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ProblemDetailView.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts
git commit -m "feat(web): add contest mode to problem detail"
```

---

### Task 11: 提交页 AI 禁用与 `problemId`

**Files:**
- Modify: `apps/web/src/views/SubmitView.vue`
- Modify: `apps/web/src/api/ai.ts` if input types need explicit `problemId`
- Test: `apps/web/src/views/SubmitView.spec.ts`

**Interfaces:**
- Consumes: `ContestDetail.disableAi`, `ContestDetail.status`, `auth.can(PERM.contest.update)`, AI input types.
- Produces: `contestAIDisabled` computed; all AI calls include `problemId`.

- [ ] **Step 1: Write failing tests**

在 `SubmitView.spec.ts` 增加：

```ts
it('disables AI during a contest and sends problemId', async () => {
  vi.mocked(getContest).mockResolvedValue(contestDetail({ status: 'running', disableAi: true }))
  const wrapper = await mountReady()
  expect(wrapper.find('[data-test="contest-ai-banner"]').exists()).toBe(true)
  expect(wrapper.find('[data-test="ai-format-button"]').attributes('disabled')).toBeDefined()
  // 触发补全（或直接断言 disabled）
})

it('sends problemId to AI endpoints', async () => {
  vi.mocked(completeCode).mockResolvedValue({ completion: 'x' })
  // 触发补全后
  expect(completeCode).toHaveBeenCalledWith(expect.objectContaining({ problemId: 1 }))
  expect(formatCode).toHaveBeenCalledWith(expect.objectContaining({ problemId: 1 }))
  expect(optimizeCode).toHaveBeenCalledWith(expect.objectContaining({ problemId: 1 }))
})
```

补一个 40302 错误提示用例。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/SubmitView.spec.ts`
Expected: FAIL，AI 按钮仍可用/调用缺少 `problemId`。

- [ ] **Step 3: Implement**

- 新增 `useAuthStore`/`PERM` import 和 `const auth = useAuthStore()`。
- `contest = ref<ContestDetail | null>(null)`；`getContest` 结果保存，`contestTitle = contest.title`。
- `contestAIDisabled = computed(() => contest.value?.status === 'running' && contest.value.disableAi && !auth.can(PERM.contest.update))`。
- `aiButtonsDisabled` 增加 `contestAIDisabled.value`。
- `completeCode({ code, language, cursor, problemId: problemId.value })`；`formatCode`/`optimizeCode` 同样。
- 在工具栏上方加 `Message data-test="contest-ai-banner"`，文案“比赛期间已禁用 AI，管理员可在比赛设置中调整。”
- `handleAIError` 处理 `e.code === 40302`，显示“比赛期间已禁用 AI”。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/SubmitView.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/SubmitView.vue apps/web/src/views/SubmitView.spec.ts apps/web/src/api/ai.ts
git commit -m "feat(web): disable AI in contest submit page"
```

---

### Task 12: 比赛编辑页 AI 开关

**Files:**
- Modify: `apps/web/src/views/ContestEditView.vue`
- Test: `apps/web/src/views/ContestEditView.spec.ts`

**Interfaces:**
- Consumes: `ContestDetail.disableAi`, `ContestInput.disableAi`.
- Produces: `FormState.disableAi`, ToggleSwitch, load/save mapping.

- [ ] **Step 1: Write failing tests**

在 `ContestEditView.spec.ts` 增加：

```ts
it('defaults disableAi to true and saves the toggle', async () => {
  const wrapper = await mountNew()
  const toggle = wrapper.find('[data-test="contest-disable-ai"] input')
  expect((toggle.element as HTMLInputElement).checked).toBe(true)
})

it('loads and saves disableAi=false', async () => {
  vi.mocked(getContest).mockResolvedValue(contest({ disableAi: false }))
  // mount edit, assert unchecked; save -> updateContest called with disableAi:false
})
```

补齐 `contest()` fixture 的 `disableAi`。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ContestEditView.spec.ts`
Expected: FAIL，字段不存在。

- [ ] **Step 3: Implement**

- `FormState` 加 `disableAi: boolean`；`emptyForm` 默认 true。
- `loadContest` 映射 `disableAi: c.disableAi`。
- `save` 的 `ContestInput` 加 `disableAi: current.disableAi`。
- 在比赛选项区域加：

```vue
<AppFormField label="比赛期间禁用 AI" hint="仅对从本场比赛进入题目的参赛者生效">
  <ToggleSwitch v-model="form.disableAi" data-test="contest-disable-ai" />
</AppFormField>
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/ContestEditView.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/ContestEditView.vue apps/web/src/views/ContestEditView.spec.ts
git commit -m "feat(web): add contest AI toggle"
```

---

### Task 13: 排行榜颜色、图例与 ICPC 错误次数

**Files:**
- Modify: `apps/web/src/components/ContestRanklist.vue`
- Test: `apps/web/src/components/ContestRanklist.spec.ts`

**Interfaces:**
- Consumes: `ContestRankCell.firstBlood`.
- Produces: CSS classes `cell--first`, `cell--accepted`, `cell--failed`; `cellText` `-attempts`.

- [ ] **Step 1: Write failing tests**

在 `ContestRanklist.spec.ts` 增加：

```ts
it('colors first blood, later accepts and failures', async () => {
  vi.mocked(getContestRanklist).mockResolvedValue(ranklist({
    mode: 'icpc',
    problems: [{ problemId: 1, displayId: 'A', title: 'A' }],
    rows: [
      row({ userId: 1, problems: [cell({ status: 'accepted', attempts: 2, firstBlood: true })] }),
      row({ userId: 2, problems: [cell({ status: 'accepted', attempts: 0, firstBlood: false })] }),
      row({ userId: 3, problems: [cell({ status: 'wrong_answer', attempts: 1, firstBlood: false })] }),
    ],
  }))
  const wrapper = await mountReady()
  expect(wrapper.find('.cell--first').text()).toBe('-2')
  expect(wrapper.find('.cell--accepted').text()).toBe('-0')
  expect(wrapper.find('.cell--failed').exists()).toBe(true)
  expect(wrapper.text()).toContain('首杀')
})
```

补一个 IOI 用例：accepted 格有颜色，未通过格红色。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/components/ContestRanklist.spec.ts`
Expected: FAIL，class 不存在、文本仍为 `AC (2)`。

- [ ] **Step 3: Implement**

- `cellText` ICPC：

```ts
if (cell.status === 'accepted') return `-${cell.attempts}`
return cell.attempts > 0 ? `-${cell.attempts}` : ''
```

- 模板 `:class`：

```ts
{
  'cell--first': cell?.status === 'accepted' && cell.firstBlood,
  'cell--accepted': cell?.status === 'accepted' && !cell.firstBlood,
  'cell--failed': cell && cell.status !== 'accepted' && cell.attempts > 0,
}
```

- CSS：首杀绿色加粗 + 绿色浅底；通过浅绿；失败红色浅底。
- 榜单 meta 增加图例 span：首杀 / 通过 / 未通过。
- 所有赛制共用 class。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/components/ContestRanklist.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/ContestRanklist.vue apps/web/src/components/ContestRanklist.spec.ts
git commit -m "feat(web): color contest ranklist cells"
```

---

### Task 14: 全量验证、真实冒烟与文档

**Files:**
- Modify if needed: `docs/superpowers/specs/2026-11-09-contest-adjustments-design.md`（仅当实现偏离）
- No new feature files.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: verified branch.

- [ ] **Step 1: Run backend formatting, vet, lint, full tests**

```bash
cd /Users/admin/codespace/kapoj/apps/server
test -z "$(gofmt -l .)"
go vet ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
```

Expected: 全部通过，lint 0 issues。

- [ ] **Step 2: Run frontend tests, lint and build**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过。

在 `build` 前先修复剩余的类型 fixture：`apps/web/src/views/ContestEditView.spec.ts` 的 `ContestProblemItem` 补 `timeLimitMs`、`memoryLimitMb`、`isPublic`、`locked`、`submitCount`、`acceptCount`、`myStatus`、`myAttempts`。这是 Task 8 生成必填字段后遗留的唯一已知类型错误。

- [ ] **Step 3: Real smoke with local server**

1. 重建并重启本地 server（沿用现有 `kapoj-run` 目录和 env），确认 `/health` 200。
2. 用现有测试账号创建一场 running 比赛，加入公开题 A、B，设置 `disableAi=true`。
3. 验证：
   - 游客题库不显示 A/B；`GET /api/problems/:id`、`/statistics`、`/official-solution`、`/solutions` 均为 404。
   - 参赛者开赛后可从比赛页打开 A/B；`GET /statistics` 仍 404。
   - 参赛者不带 `contestId` 提交 A 返回 404；带 `contestId` 成功。
   - `POST /api/ai/complete` 带 A 的 `problemId` 返回 40302；`disableAi=false` 后不再 40302。
   - 结束比赛后 A/B 恢复题库可见；比赛页显示“提交/通过/我的状态”和“题库”按钮。
4. 检查排行榜首杀/通过/未通过颜色与 ICPC `-2`/`-0`。

- [ ] **Step 4: Update docs / commit fixes**

若实现与设计有偏差，更新设计文档的“已知取舍”或实现备注；只提交实际需要的修正。

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "test: verify contest adjustments"
```

---

## Self-Review

- **Spec coverage:** 派生锁定（Task 1、2、3、4）、赛后统计/标签/跳转（Task 5、9）、比赛题目页（Task 10）、AI 开关（Task 6、11、12）、排行榜（Task 7、13）、契约（Task 8）、验证（Task 14）。无遗漏。
- **Type consistency:** `IsProblemLocked`、`CanViewProblem`、`AIBlockedForUser`、`ProblemStats`、`UserProblemStats`、`LockedProblemIDs`、`ProblemItem` 字段、`firstBlood` 在任务间名称一致。
- **Review Focus mapping:** 每个 Review Focus 行都有对应测试：Task 3/6（绕过）、Task 1/2/5（多比赛）、Task 5/7（统计口径）、Task 6（AI 绕过）、Task 7/13（首杀/罚时）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出签名、SQL 语义和测试断言，不逐行抄写实现。
