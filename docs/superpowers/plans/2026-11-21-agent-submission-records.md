# Agent 提交测评与 Markdown 简介实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Agent 的全部提交进入公开测评记录并带 Agent 标签，Rating 历史可打开关联提交详情，Agent 简介支持 Markdown，同时保持人类统计与代码可见性隔离。

**Architecture:** 复用现有 `submissions` 列表/详情链路，增加 Agent 元数据与 Rating 历史的 `submissionId`；前端抽出共享测评详情抽屉，Agent 页面复用同一组件；Agent 简介复用现有 `MarkdownEditor`/`MarkdownView`，简介上限扩到 2000 字符。

**Tech Stack:** Go 1.26 + Gin + GORM + PostgreSQL + golang-migrate；Vue 3 + TypeScript + PrimeVue + Vitest + md-editor-v3。

**Spec:** `docs/superpowers/specs/2026-11-21-agent-submission-records-design.md`

## Global Constraints

- Agent 的全部提交（pending、WA、编译错误、系统错误等）都进入公开 `/submissions`，按 `id DESC` 与人类提交混排；不新增默认隐藏或独立页面。
- 人类统计继续排除 Agent：`HasAccepted`、`CountUserActivity`、`ListCodeSubmissionTimes`、题目 `submit_count/accept_count`、学习/游戏化/难度计算都不变。
- 代码/答案可见性只由服务端 `submission.Service.Detail` 决定：Agent 所属账号、管理员/特权用户、已有人类 AC 的查看者可见；Agent AC 不等于用户本人通过；其他人只看到指标。
- Rating 历史沿用现有题目标题脱敏条件；非公开题/未结束比赛题上，非 Agent 所有者得到 `submissionId = null`，所有者仍得到真实 ID。
- Agent 提交禁止重测；前端隐藏按钮，服务端返回 `40001` + `Agent 提交不支持重测`。
- Agent 简介保存原始 Markdown；上限 2000 个 Unicode 字符；前端渲染必须继续走 `sanitizeHtml`，不新增图片上传或后端 Markdown 渲染。
- 不新增权限码；不改变 Agent Rating 算法、随机选题、20 次上限、token 规则。
- 迁移使用版本化 SQL，版本号接在 `0004` 之后；不要新增 `devAutoMigrate` DDL。
- 所有新增用户可见文案使用中文，与现有页面风格一致。
- 后端命令在 `apps/server` 下执行；数据库测试需要按 `README.md` 设置 `KAPOJ_TEST_DATABASE_URL`，完整测试使用 `go test -p 1 -count=1 ./...`。

## Review Focus

- 非所有者查看脱敏 Rating 历史行时，接口不得返回 `submissionId`；Agent 所有者登录后必须仍能拿到自己的 ID。
- 非 AC 访客打开 Agent 提交详情时必须看不到代码/答案；Agent 所属账号和已有人类 AC 的用户必须看得到。
- Agent 提交不得改变用户 AC、活跃度、题目计数或难度；只改变公开测评记录与详情可见性。
- Agent 提交重测必须在服务端被明确拒绝，且不能回退人类 `accept_count`。
- Markdown 简介在 2000/2001 字符边界、脚本/iframe 等危险内容上必须安全。

## File Structure

- `apps/server/internal/submission/repository.go`：`List`/`FindDetail` 纳入 Agent 行，join `agents` 取名称。
- `apps/server/internal/submission/dto.go`：`ListItem` 暴露 `agentId`/`agentName`。
- `apps/server/internal/submission/service.go`：Agent 提交重测的明确拒绝。
- `apps/server/internal/agent/service.go`：Rating 历史 `submissionId`、viewer-aware 脱敏、`Publisher` 选项、简介上限。
- `apps/server/internal/agent/arena.go`：提交成功后发布 SSE 事件。
- `apps/server/internal/agentapi/handler.go`：Rating 历史传入可选登录用户。
- `apps/server/internal/agent/model.go`：简介列宽 2000。
- `apps/server/cmd/server/main.go`：注入 Redis SSE publisher。
- `apps/server/migrations/0005_agent_intro_length.*.sql`：简介列宽迁移。
- `packages/api/openapi.yaml` + `apps/web/src/api/schema.d.ts`：公开契约与生成类型。
- `apps/web/src/components/SubmissionDetailDrawer.vue`：共享测评详情抽屉（代码、测试点、重测、Agent 标记）。
- `apps/web/src/views/SubmissionsView.vue`：公开测评记录列表、Agent 标签、复用详情抽屉。
- `apps/web/src/views/AgentDetailView.vue`：Agent 公开详情 Markdown 简介、Rating 历史提交入口。
- `apps/web/src/views/AgentSettingsView.vue`：Markdown 简介编辑、Rating 历史提交入口。
- `apps/web/src/views/HomeView.vue`：最近练习的 Agent 小标签。
- 各文件的 `*.spec.ts`/`*_test.go`：对应回归覆盖。

---

## Task 1: Agent 简介上限扩到 2000

**Files:**
- Modify: `apps/server/internal/agent/model.go`
- Modify: `apps/server/internal/agent/service.go`
- Modify: `apps/server/internal/agent/service_test.go`
- Modify: `apps/server/internal/db/db_test.go`
- Create: `apps/server/migrations/0005_agent_intro_length.up.sql`
- Create: `apps/server/migrations/0005_agent_intro_length.down.sql`

**Interfaces:**
- Consumes: existing `agent.AgentInput` and `normalizeAgentInput`.
- Produces: `agentInputMaxIntro = 2000`; `agents.model_intro` is `varchar(2000)` after migration.

- [ ] **Step 1: Write the failing boundary test**

Add to `apps/server/internal/agent/service_test.go`:

```go
func TestAgentIntroSupportsMarkdownUpTo2000(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	markdown := "## 简介\n\n" + strings.Repeat("字", 1993)
	owner := f.user("intro-long", "active")
	view, err := f.svc.CreateAgent(ctx, owner, agent.AgentInput{
		Name: "Intro Agent", ModelName: "model", ModelIntro: markdown,
	})
	require.NoError(t, err)
	assert.Equal(t, markdown, view.ModelIntro)

	tooLong := f.user("intro-too-long", "active")
	_, err = f.svc.CreateAgent(ctx, tooLong, agent.AgentInput{
		Name: "Too Long", ModelName: "model", ModelIntro: strings.Repeat("字", 2001),
	})
	requireAppError(t, err, 400, 40001)
}
```

Also change the existing `strings.Repeat("字", 501)` in `TestCreateAgentOncePerUserAndUniqueName` to `strings.Repeat("字", 2001)`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/server && go test -count=1 ./internal/agent -run 'TestAgentIntroSupportsMarkdownUpTo2000|TestCreateAgentOncePerUserAndUniqueName'`

Expected: FAIL because 2000 characters are rejected by the current 500 limit.

- [ ] **Step 3: Implement the limit and migration**

In `apps/server/internal/agent/service.go`, change:

```go
agentInputMaxIntro = 2000
```

In `apps/server/internal/agent/model.go`, change `ModelIntro` to:

```go
ModelIntro string `gorm:"size:2000;not null;default:''"`
```

Create `apps/server/migrations/0005_agent_intro_length.up.sql`:

```sql
ALTER TABLE public.agents
    ALTER COLUMN model_intro TYPE character varying(2000);
```

Create `apps/server/migrations/0005_agent_intro_length.down.sql`:

```sql
ALTER TABLE public.agents
    ALTER COLUMN model_intro TYPE character varying(500)
    USING left(model_intro, 500);
```

- [ ] **Step 4: Pin the migrated column width**

In `apps/server/internal/db/db_test.go`, add inside `TestMigrateCreatesAgentSchema` (after the token hash/prefix checks is fine):

```go
	var modelIntro struct {
		DataType  string
		MaxLength *int64 `gorm:"column:max_length"`
	}
	require.NoError(t, g.Raw(`
		SELECT data_type, character_maximum_length AS max_length
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'agents' AND column_name = 'model_intro'`).Scan(&modelIntro).Error)
	assert.Equal(t, "character varying", modelIntro.DataType)
	require.NotNil(t, modelIntro.MaxLength)
	assert.EqualValues(t, 2000, *modelIntro.MaxLength, "model_intro 应为 varchar(2000)")
```

- [ ] **Step 5: Run the focused tests**

Run: `cd apps/server && go test -count=1 ./internal/agent ./internal/db`

Expected: PASS (db tests require `KAPOJ_TEST_DATABASE_URL`).

- [ ] **Step 6: Commit**

```bash
git add apps/server/internal/agent/model.go apps/server/internal/agent/service.go apps/server/internal/agent/service_test.go apps/server/internal/db/db_test.go apps/server/migrations/0005_agent_intro_length.up.sql apps/server/migrations/0005_agent_intro_length.down.sql
git commit -m "feat(agent): allow 2000-character markdown intro"
```

---

## Task 2: 测评记录列表与详情包含 Agent 提交

**Files:**
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/submission/repository_test.go`
- Modify: `apps/server/internal/submission/records_test.go`

**Interfaces:**
- Produces: `ListRow.AgentName string` (the embedded `submission.Submission` already supplies `AgentID`); `ListItem.AgentID *int64` (`json:"agentId"`), `ListItem.AgentName string` (`json:"agentName"`).
- Consumes: existing `agents` table and `submissions.agent_id`.

- [ ] **Step 1: Update the existing isolation test to expect Agent rows in list/detail**

Rename `TestHumanSubmissionQueriesExcludeAgent` to `TestHumanStatsExcludeAgent`, because the test now pins that list/detail include Agent rows while human statistics still exclude them.

In `apps/server/internal/submission/repository_test.go`, add the `agent` import under an alias to avoid the existing local `agent` submission variable:

```go
agentpkg "github.com/kaptree/kapoj/apps/server/internal/agent"
```

Replace the arbitrary `agentID := int64(11)` setup with a real Agent row:

```go
	now := time.Now()
	agentRow := &agentpkg.Agent{
		UserID: 7, Name: "repo-agent", ModelName: "test-model",
		Status: agentpkg.StatusActive, Rating: 1500, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, g.Create(agentRow).Error)
	agentID := agentRow.ID
```

Change the list assertions to:

```go
	rows, total, err := repo.List(ctx, submission.ListFilter{
		ProblemID: p.ID, ViewerID: 7, CanSeeHidden: true,
	}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total, "the public list includes Agent submissions")
	require.Len(t, rows, 2)
	assert.Equal(t, agent.ID, rows[0].ID)
	require.NotNil(t, rows[0].AgentID)
	assert.Equal(t, agentID, *rows[0].AgentID)
	assert.Equal(t, "repo-agent", rows[0].AgentName)
	assert.Nil(t, rows[1].AgentID)
	assert.Empty(t, rows[1].AgentName)
```

Change the detail assertions to:

```go
	agentDetail, err := repo.FindDetail(ctx, agent.ID)
	require.NoError(t, err)
	require.NotNil(t, agentDetail.AgentID)
	assert.Equal(t, agentID, *agentDetail.AgentID)
	assert.Equal(t, "repo-agent", agentDetail.AgentName)
	assert.Equal(t, "package main", agentDetail.Code)

	detail, err := repo.FindDetail(ctx, human.ID)
	require.NoError(t, err)
	assert.Nil(t, detail.AgentID)
	assert.Equal(t, human.ID, detail.ID)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/server && go test -count=1 ./internal/submission -run TestHumanStatsExcludeAgent`

Expected: FAIL because the list still excludes Agent rows and `FindDetail(agent.ID)` returns not found.

- [ ] **Step 3: Implement Agent metadata in the repository**

In `apps/server/internal/submission/repository.go`:

```go
type ListRow struct {
	Submission
	Username         string
	Nickname         string
	AgentName        string `gorm:"column:agent_name"`
	ProblemTitle     string `gorm:"column:problem_title"`
	// ...
}
```

`AgentID` comes from the embedded `Submission`; do not add a second field with that name.

In `List`, remove `Where("s.agent_id IS NULL")` and add the Agent join:

```go
	q := r.db.WithContext(ctx).Table("submissions AS s").
		Joins("JOIN users AS u ON u.id = s.user_id").
		Joins("JOIN problems AS p ON p.id = s.problem_id").
		Joins("LEFT JOIN contests AS c ON c.id = s.contest_id").
		Joins("LEFT JOIN agents AS ag ON ag.id = s.agent_id")
```

Add `s.agent_id, ag.name AS agent_name` to the `Select` list.

In `FindDetail`, remove `AND s.agent_id IS NULL`, add `LEFT JOIN agents AS ag ON ag.id = s.agent_id`, and select `ag.name AS agent_name` alongside `s.*`.

In `apps/server/internal/submission/dto.go`, add to `ListItem`:

```go
	AgentID   *int64 `json:"agentId"`
	AgentName string `json:"agentName"`
```

and in `toListItem`:

```go
		AgentID: s.AgentID, AgentName: row.AgentName,
```

- [ ] **Step 4: Add detail visibility coverage for Agent submissions**

In `apps/server/internal/submission/records_test.go`, add a helper:

```go
func insertAgentRecord(t *testing.T, g *gorm.DB, userID, problemID, agentID int64, language, code, status string, created time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(
		`INSERT INTO submissions (user_id, problem_id, agent_id, language, code, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		userID, problemID, agentID, language, code, status, created,
	).Scan(&id).Error)
	return id
}
```

Add:

```go
func TestAgentSubmissionDetailVisibility(t *testing.T) {
	e := newRecordsEnv(t)
	ctx := context.Background()
	now := time.Now()

	agentSub := insertAgentRecord(t, e.db, e.alice, e.problem1.ID, 501, "cpp", "agent-secret", "accepted", now)
	insertRecord(t, e.db, e.bob, e.problem1.ID, "cpp", "human-ac", "accepted", nil, now)
	otherAgentSub := insertAgentRecord(t, e.db, e.alice, e.problem2.ID, 501, "cpp", "agent-secret-2", "accepted", now)
	insertAgentRecord(t, e.db, e.bob, e.problem2.ID, 502, "cpp", "bob-agent-ac", "accepted", now)

	owner, err := e.svc.Detail(ctx, submission.Viewer{UserID: e.alice}, agentSub)
	require.NoError(t, err)
	assert.False(t, owner.CodeHidden)
	assert.Equal(t, "agent-secret", owner.Code)

	acViewer, err := e.svc.Detail(ctx, submission.Viewer{UserID: e.bob}, agentSub)
	require.NoError(t, err)
	assert.False(t, acViewer.CodeHidden)
	assert.Equal(t, "agent-secret", acViewer.Code)

	guest, err := e.svc.Detail(ctx, submission.Viewer{}, agentSub)
	require.NoError(t, err)
	assert.True(t, guest.CodeHidden)
	assert.Empty(t, guest.Code)

	// Bob's Agent AC on problem2 does not count as Bob's own human AC.
	blocked, err := e.svc.Detail(ctx, submission.Viewer{UserID: e.bob}, otherAgentSub)
	require.NoError(t, err)
	assert.True(t, blocked.CodeHidden)
	assert.Empty(t, blocked.Code)
}
```

- [ ] **Step 5: Run the focused tests**

Run: `cd apps/server && go test -count=1 ./internal/submission`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/server/internal/submission/repository.go apps/server/internal/submission/dto.go apps/server/internal/submission/repository_test.go apps/server/internal/submission/records_test.go
git commit -m "feat(submission): include agent submissions in public records"
```

---

## Task 3: Agent 提交重测返回明确错误

**Files:**
- Modify: `apps/server/internal/submission/service.go`
- Modify: `apps/server/internal/submission/records_test.go`

**Interfaces:**
- Consumes: `submission.Viewer.CanRejudge`, `submission.Submission.AgentID`.
- Produces: `Rejudge` returns `httpx.ErrValidation("Agent 提交不支持重测")` for Agent rows.

- [ ] **Step 1: Write the failing test**

Add to `apps/server/internal/submission/records_test.go` (add the `httpx` import):

```go
func TestAgentSubmissionRejudgeReturnsClearError(t *testing.T) {
	e := newRecordsEnv(t)
	id := insertAgentRecord(t, e.db, e.alice, e.problem1.ID, 501, "cpp", "code", "accepted", time.Now())

	_, err := e.svc.Rejudge(context.Background(), submission.Viewer{CanRejudge: true}, id)
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 400, appErr.Status)
	assert.Equal(t, 40001, appErr.Code)
	assert.Equal(t, "Agent 提交不支持重测", appErr.Message)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/server && go test -count=1 ./internal/submission -run TestAgentSubmissionRejudgeReturnsClearError`

Expected: FAIL because the current message is the generic “提交仍在评测中，不能重测”.

- [ ] **Step 3: Implement the guard**

In `apps/server/internal/submission/service.go`, after `FindByID` and before the terminal-status check:

```go
	if sub.AgentID != nil {
		return nil, httpx.ErrValidation("Agent 提交不支持重测")
	}
```

- [ ] **Step 4: Run the focused test**

Run: `cd apps/server && go test -count=1 ./internal/submission -run 'TestAgentSubmissionRejudgeReturnsClearError|TestAgentSubmissionCannotBeRejudged'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/submission/service.go apps/server/internal/submission/records_test.go
git commit -m "fix(submission): reject agent rejudge with a clear error"
```

---

## Task 4: Rating 历史返回 submissionId（遵守脱敏）

**Files:**
- Modify: `apps/server/internal/agent/service.go`
- Modify: `apps/server/internal/agentapi/handler.go`
- Modify: `apps/server/internal/agent/service_test.go`
- Modify: `apps/server/internal/agentapi/handler_test.go`

**Interfaces:**
- Produces: `AgentRatingRow.SubmissionID *int64` (`json:"submissionId"`).
- Produces: `Service.RatingHistory(ctx context.Context, agentID, viewerID int64, page, size int) (httpx.Page[AgentRatingRow], error)`.
- Consumes: `agents.user_id` and existing `agent_rating_history.submission_id`.

- [ ] **Step 1: Update tests for the new field and signature**

In `apps/server/internal/agent/service_test.go`, change every `f.svc.RatingHistory(ctx, created.ID, 1, 20)` call to include a viewer: `f.svc.RatingHistory(ctx, created.ID, 0, 1, 20)`. Change the not-found call to `f.svc.RatingHistory(ctx, 999999, 0, 1, 20)`.

In `TestAgentPublicDetailAndHistory`, create a real submission before the solve history row:

```go
	var solveSubID int64
	require.NoError(t, f.db.Raw(
		`INSERT INTO submissions (user_id, problem_id, agent_id, language, code, status, created_at)
		 VALUES (?, ?, ?, 'cpp', 'solution', 'accepted', ?) RETURNING id`,
		owner, firstProblem, created.ID, solveAt,
	).Scan(&solveSubID).Error)
```

Set `SubmissionID: &solveSubID` on the solve `AgentRatingHistory` row and assert:

```go
	require.NotNil(t, solve.SubmissionID)
	assert.Equal(t, solveSubID, *solve.SubmissionID)
	assert.Nil(t, abandon.SubmissionID)
	assert.Nil(t, reset.SubmissionID)
```

In `TestAgentRatingHistoryHidesNonPublicProblems`, replace the existing history-insert loop with one that also creates a real submission per row:

```go
	subIDs := make(map[int64]int64, 3)
	for i, problemID := range []int64{hidden.ID, locked.ID, ended.ID} {
		var subID int64
		require.NoError(t, f.db.Raw(
			`INSERT INTO submissions (user_id, problem_id, agent_id, language, code, status, created_at)
			 VALUES (?, ?, ?, 'cpp', 'solution', 'accepted', ?) RETURNING id`,
			owner, problemID, created.ID, f.clock.now.Add(time.Duration(i)*time.Minute),
		).Scan(&subID).Error)
		subIDs[problemID] = subID
		require.NoError(t, f.db.Create(&agent.AgentRatingHistory{
			AgentID: created.ID, ProblemID: problemID, SubmissionID: &subID,
			Kind: agent.RatingSolve, RatingBefore: 1500, RatingAfter: 1510, Delta: 10,
			CreatedAt: f.clock.now.Add(time.Duration(i) * time.Minute),
		}).Error)
	}
```

Keep the existing title/slug assertions and add:

```go
	publicPage, err := f.svc.RatingHistory(ctx, created.ID, 0, 1, 20)
	require.NoError(t, err)
	for _, row := range publicPage.Items {
		switch row.ProblemID {
		case hidden.ID, locked.ID:
			assert.Nil(t, row.SubmissionID, "non-owner must not receive a redacted submission id")
		case ended.ID:
			require.NotNil(t, row.SubmissionID)
			assert.Equal(t, subIDs[ended.ID], *row.SubmissionID)
		}
	}

	ownerPage, err := f.svc.RatingHistory(ctx, created.ID, owner, 1, 20)
	require.NoError(t, err)
	for _, row := range ownerPage.Items {
		if row.Kind == agent.RatingSolve {
			require.NotNil(t, row.SubmissionID, "the owner must still receive their agent submission id")
		}
	}
```

In `apps/server/internal/agentapi/handler_test.go`, set a `SubmissionID` on the solve history row and assert the JSON:

```go
	assert.EqualValues(t, subID, solveRow["submissionId"])
	assert.Nil(t, resetRow["submissionId"])
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/server && go test -count=1 ./internal/agent ./internal/agentapi -run 'TestAgentPublicDetailAndHistory|TestAgentRatingHistoryHidesNonPublicProblems|TestPublicAgentEndpoints'`

Expected: FAIL to compile (`SubmissionID` and the new `RatingHistory` signature do not exist).

- [ ] **Step 3: Implement the row field and viewer-aware query**

In `apps/server/internal/agent/service.go`:

```go
type AgentRatingRow struct {
	// existing fields...
	SubmissionID *int64 `json:"submissionId"`
}
```

Change the service methods:

```go
func (s *Service) RatingHistory(
	ctx context.Context, agentID, viewerID int64, page, size int,
) (httpx.Page[AgentRatingRow], error) {
	return s.ratingHistoryPage(ctx, agentID, viewerID, page, size)
}

func (s *Service) RatingHistoryForUser(
	ctx context.Context, userID int64, page, size int,
) (httpx.Page[AgentRatingRow], error) {
	var a Agent
	err := s.db.WithContext(ctx).Select("id").Where("user_id = ?", userID).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.Page[AgentRatingRow]{}, httpx.ErrNotFound
	}
	if err != nil {
		return httpx.Page[AgentRatingRow]{}, err
	}
	return s.ratingHistoryPage(ctx, a.ID, userID, page, size)
}
```

In `ratingHistoryPage`, load the owner and use a CTE:

```go
	var a Agent
	err := s.db.WithContext(ctx).Select("id, user_id").First(&a, agentID).Error
```

Use this SQL (the CTE computes visibility once):

```sql
WITH history AS (
	SELECT h.*, p.title, p.slug, p.is_public,
	       (p.id IS NOT NULL AND p.is_public AND NOT EXISTS (
	           SELECT 1 FROM contest_problems cp
	           JOIN contests c ON c.id = cp.contest_id
	           WHERE cp.problem_id = p.id AND c.end_time > $2
	       )) AS problem_visible
	FROM agent_rating_history h
	LEFT JOIN problems p ON p.id = h.problem_id
	WHERE h.agent_id = $1
)
SELECT id, COALESCE(problem_id, 0) AS problem_id,
       CASE WHEN problem_visible THEN title ELSE '' END AS problem_title,
       CASE WHEN problem_visible THEN slug ELSE '' END AS problem_slug,
       problem_visible, submission_id, kind, difficulty, rating_before,
       rating_after, delta, solve_time_ms, created_at
FROM history
ORDER BY created_at DESC, id DESC
LIMIT $3 OFFSET $4
```

Add to `ratingHistoryRow`:

```go
	SubmissionID   *int64 `gorm:"column:submission_id"`
	ProblemVisible bool   `gorm:"column:problem_visible"`
```

Change the row projection so it takes the viewer's ownership into account:

```go
func (r ratingHistoryRow) view(ownerViewer bool) AgentRatingRow {
	submissionID := r.SubmissionID
	if !r.ProblemVisible && !ownerViewer {
		submissionID = nil
	}
	return AgentRatingRow{
		// existing fields...
		SubmissionID: submissionID,
	}
}
```

When building items:

```go
	items = append(items, row.view(viewerID == a.UserID))
```

In `apps/server/internal/agentapi/handler.go`, read optional claims in `ratingHistory`:

```go
	viewerID := int64(0)
	if claims, ok := middleware.CurrentClaims(c); ok && claims != nil {
		viewerID = claims.UserID
	}
	result, err := h.svc.RatingHistory(c.Request.Context(), id, viewerID, page, size)
```

- [ ] **Step 4: Run the focused tests**

Run: `cd apps/server && go test -count=1 ./internal/agent ./internal/agentapi`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/agent/service.go apps/server/internal/agentapi/handler.go apps/server/internal/agent/service_test.go apps/server/internal/agentapi/handler_test.go
git commit -m "feat(agent): expose submission ids in rating history"
```

---

## Task 5: Agent 提交发布 SSE 事件

**Files:**
- Modify: `apps/server/internal/agent/service.go`
- Modify: `apps/server/internal/agent/arena.go`
- Modify: `apps/server/internal/agent/arena_test.go`
- Modify: `apps/server/cmd/server/main.go`

**Interfaces:**
- Produces: `type Publisher interface { Publish(ctx context.Context, submissionID int64) }`, `func WithPublisher(p Publisher) Option`.
- Consumes: `submissionevents.NewRedisPublisher(rdb)`.

- [ ] **Step 1: Write failing publisher tests**

In `apps/server/internal/agent/arena_test.go`, add:

```go
type recordingPublisher struct {
	ids []int64
}

func (p *recordingPublisher) Publish(_ context.Context, submissionID int64) {
	p.ids = append(p.ids, submissionID)
}

func TestAgentSubmitPublishesSubmissionEvent(t *testing.T) {
	pub := &recordingPublisher{}
	f := newAgentFixtureWithDeps(t, noopQueue{}, nil, agent.WithPublisher(pub))
	ctx := context.Background()
	owner := f.user("publisher-owner", "active")
	view := f.create(owner, "Publisher Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "publisher-problem", Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	result, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{result.SubmissionID}, pub.ids)
}

func TestAgentSubmitEnqueueFailureStillPublishes(t *testing.T) {
	pub := &recordingPublisher{}
	f := newAgentFixtureWithDeps(t, &failingQueue{}, nil, agent.WithPublisher(pub))
	ctx := context.Background()
	owner := f.user("publisher-fail", "active")
	view := f.create(owner, "Publisher Fail Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "publisher-fail-problem", Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	_, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	require.Error(t, err)
	require.Len(t, pub.ids, 1)
}
```

Change `newAgentFixtureWithDeps` to accept optional options:

```go
func newAgentFixtureWithDeps(
	t *testing.T, queue submission.Enqueuer, limiter agent.Limiter,
	opts ...agent.Option,
) *agentFixture {
	t.Helper()
	g := testutil.PG(t)
	clock := &testClock{now: time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)}
	options := []agent.Option{
		agent.WithClock(clock.time), agent.WithClaimTimeout(5 * time.Minute),
	}
	options = append(options, opts...)
	svc := agent.NewService(g,
		problem.NewGormRepository(g), contest.NewGormRepository(g),
		queue, limiter, options...)
	return &agentFixture{t: t, db: g, svc: svc, clock: clock}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/server && go test -count=1 ./internal/agent -run 'TestAgentSubmitPublishesSubmissionEvent|TestAgentSubmitEnqueueFailureStillPublishes'`

Expected: FAIL to compile because `agent.WithPublisher` does not exist.

- [ ] **Step 3: Implement the publisher option and calls**

In `apps/server/internal/agent/service.go`, add near `Limiter`:

```go
type Publisher interface {
	Publish(ctx context.Context, submissionID int64)
}

// WithPublisher publishes new Agent submissions to the shared SSE channel.
func WithPublisher(p Publisher) Option {
	return func(s *Service) {
		s.publisher = p
	}
}
```

Add `publisher Publisher` to `Service`.

Add a helper:

```go
func (s *Service) publish(ctx context.Context, submissionID int64) {
	if s.publisher != nil {
		s.publisher.Publish(ctx, submissionID)
	}
}
```

In `apps/server/internal/agent/arena.go`'s `Submit`, after the transaction:

```go
	if s.queue != nil {
		if err := s.queue.Enqueue(ctx, result.SubmissionID); err != nil {
			s.markSubmissionSystemError(ctx, result.SubmissionID, now)
			s.publish(ctx, result.SubmissionID)
			return nil, err
		}
	}
	s.publish(ctx, result.SubmissionID)
	return result, nil
```

- [ ] **Step 4: Wire the Redis publisher in main**

In `apps/server/cmd/server/main.go`, add the import:

```go
	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
```

Change the Agent service construction:

```go
	agentSvc := agent.NewService(g,
		problem.NewGormRepository(g), contest.NewGormRepository(g),
		queue, agent.NewRedisLimiter(rdb),
		agent.WithPublisher(submissionevents.NewRedisPublisher(rdb)))
```

- [ ] **Step 5: Run the focused tests and build**

Run: `cd apps/server && go test -count=1 ./internal/agent && go build ./cmd/server`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/server/internal/agent/service.go apps/server/internal/agent/arena.go apps/server/internal/agent/arena_test.go apps/server/cmd/server/main.go
git commit -m "feat(agent): publish submissions to the live records stream"
```

---

## Task 6: 同步 OpenAPI 契约与前端类型

**Files:**
- Modify: `packages/api/openapi.yaml`
- Modify: `apps/web/src/api/schema.d.ts` (generated)

**Interfaces:**
- Produces: generated fields `SubmissionListItem.agentId: number | null`, `SubmissionListItem.agentName: string`, `AgentRatingRow.submissionId: number | null`, `AgentInput.modelIntro.maxLength = 2000`.

- [ ] **Step 1: Update the OpenAPI schemas**

In `packages/api/openapi.yaml`:

Under `SubmissionListItem.required`, add `agentId` and `agentName`; add properties:

```yaml
        agentId:
          type: integer
          format: int64
          nullable: true
          description: Agent 提交所属 Agent ID；人类提交为 null
        agentName:
          type: string
          description: Agent 名称；人类提交为空字符串
```

Under `AgentRatingRow.required`, add `submissionId`; add:

```yaml
        submissionId:
          type: integer
          format: int64
          nullable: true
          description: 关联的提交 ID；reset/abandon/timeout、脱敏问题或旧数据为 null
```

Change `AgentInput.modelIntro`:

```yaml
        modelIntro: { type: string, maxLength: 2000, description: 模型简介，支持 Markdown，最多 2000 个字符 }
```

Update route descriptions:

- `/submissions` summary: “测评记录列表（游客可访问）；包含人类与 Agent 提交；隐藏题的提交需要 problem.view_hidden 才能看到他人记录”
- `/submissions/{id}` description: add “Agent 提交沿用同一套代码/答案可见性：所属账号、管理员或已在该题 AC 的用户可见。”
- `/agents/{id}/rating-history` description: add “包含关联 submissionId；非公开题/未结束比赛题仅 Agent 所有者可获得 ID。”

- [ ] **Step 2: Regenerate TypeScript types**

Run: `corepack pnpm gen:api`

Expected: `apps/web/src/api/schema.d.ts` changes only for the fields described above.

- [ ] **Step 3: Add the new required fields to existing test fixtures**

The generated types now require `agentId`/`agentName` on `SubmissionListItem` and `submissionId` on `AgentRatingRow`. Update the factories so the frontend compiles before later tasks:

- `apps/web/src/views/SubmissionsView.spec.ts`: add `agentId: null, agentName: ''` to `item()`.
- `apps/web/src/views/HomeView.spec.ts`: add `agentId: null, agentName: ''` to `recentSubmission`.
- `apps/web/src/views/AgentDetailView.spec.ts`: add `submissionId: null` to `ratingRow()`.
- `apps/web/src/views/AgentSettingsView.spec.ts`: add `submissionId: null` to `ratingRow()`.

Run: `corepack pnpm --filter web test -- src/views/SubmissionsView.spec.ts src/views/HomeView.spec.ts src/views/AgentDetailView.spec.ts src/views/AgentSettingsView.spec.ts`

Expected: PASS with the new fields defaulted to human/no-submission values.

- [ ] **Step 4: Check generated diff**

Run: `git diff --stat -- apps/web/src/api/schema.d.ts packages/api/openapi.yaml`

Expected: both files changed; no unrelated generated churn.

- [ ] **Step 5: Commit**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(api): document agent submissions and rating history ids"
```

---

## Task 7: 抽出共享测评详情抽屉

**Files:**
- Create: `apps/web/src/components/SubmissionDetailDrawer.vue`
- Create: `apps/web/src/components/SubmissionDetailDrawer.spec.ts`
- Modify: `apps/web/src/views/SubmissionsView.vue`
- Modify: `apps/web/src/views/SubmissionsView.spec.ts`

**Interfaces:**
- Produces: `SubmissionDetailDrawer` props `visible: boolean`, `submissionId: number | null`, `canRejudge?: boolean`; emits `update:visible` and `rejudged`.
- Consumes: `getSubmission`, `rejudgeSubmission`, `CodeViewer`, `AppStatusTag`, `UserIdentityName`.

- [ ] **Step 1: Write the component regression test**

Create `apps/web/src/components/SubmissionDetailDrawer.spec.ts` with a minimal `SubmissionDetail` fixture and these cases:

Create `apps/web/src/components/SubmissionDetailDrawer.spec.ts`. Reuse the `detail()` fixture shape from `SubmissionsView.spec.ts` (add `agentId: null, agentName: ''`), mount the drawer with `visible: true`, `submissionId: 1`, a memory router (routes `/`, `/users/:id`, `/problems/:id`) and a mocked `primevue/usetoast`; then pin three cases:

- `getSubmission` is called with `1` and `document.body.textContent` contains `int main() {}`.
- With `codeHidden: true` and `code: undefined`, the body contains `该提交的代码暂不可见。`.
- With `canRejudge: true`, `[data-test="rejudge"]` exists; with `canRejudge: false`, it is null. Clear `document.body.innerHTML` between tests.

- [ ] **Step 2: Run the test to verify it fails**

Run: `corepack pnpm --filter web test -- src/components/SubmissionDetailDrawer.spec.ts`

Expected: FAIL because the component does not exist.

- [ ] **Step 3: Create the component**

Move the existing `<Drawer>` markup and the drawer state/methods out of `SubmissionsView.vue` into `apps/web/src/components/SubmissionDetailDrawer.vue`:

- Use `defineProps<{ visible: boolean; submissionId: number | null; canRejudge?: boolean }>()`.
- Use `defineEmits<{ 'update:visible': [value: boolean]; rejudged: [] }>()`.
- On `visible`/`submissionId` change, load `getSubmission(submissionId)`.
- Keep the existing detail metadata grid, code/answer blocks, hidden banners, testcase table, total score, and `rejudge` button.
- Move the drawer-specific scoped styles (`.detail-meta`, `.hidden-banner`, `.code-block`, `.judge-table`, `.drawer-actions`, `.testcase-total`) with the markup.
- `rejudge` calls `rejudgeSubmission`, shows toast, reloads detail, and emits `rejudged`.
- The Drawer uses `:visible="visible"` and `@update:visible="emit('update:visible', $event)"`.

- [ ] **Step 4: Use the component in SubmissionsView**

In `SubmissionsView.vue`:

- Remove the inline `Drawer`, `getSubmission`/`rejudgeSubmission` imports, `detail` refs, and drawer methods.
- Keep `drawer` and add `selectedSubmissionId = ref<number | null>(null)`.
- On row click:

```ts
function onRowClick(event: { data: SubmissionListItem }) {
  selectedSubmissionId.value = event.data.id
  drawer.value = true
}
```

- Render:

```vue
<SubmissionDetailDrawer
  v-model:visible="drawer"
  :submission-id="selectedSubmissionId"
  :can-rejudge="canRejudge"
  @rejudged="load"
/>
```

- Keep the existing toast only for list-level errors if still used; remove unused imports.

- [ ] **Step 5: Run the component and page tests**

Run: `corepack pnpm --filter web test -- src/components/SubmissionDetailDrawer.spec.ts src/views/SubmissionsView.spec.ts`

Expected: PASS; the existing SubmissionsView detail/rejudge/hidden-code tests continue to pass.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/SubmissionDetailDrawer.vue apps/web/src/components/SubmissionDetailDrawer.spec.ts apps/web/src/views/SubmissionsView.vue apps/web/src/views/SubmissionsView.spec.ts
git commit -m "refactor(web): extract shared submission detail drawer"
```

---

## Task 8: 测评记录显示 Agent 标签并隐藏 Agent 重测

**Files:**
- Modify: `apps/web/src/views/SubmissionsView.vue`
- Modify: `apps/web/src/views/SubmissionsView.spec.ts`
- Modify: `apps/web/src/components/SubmissionDetailDrawer.vue`
- Modify: `apps/web/src/components/SubmissionDetailDrawer.spec.ts`

**Interfaces:**
- Consumes: `SubmissionListItem.agentId`, `SubmissionListItem.agentName`, `SubmissionDetail.agentId`, `SubmissionDetail.agentName`.

- [ ] **Step 1: Add failing UI expectations**

In `apps/web/src/views/SubmissionsView.spec.ts` (the `item()` factory already has the fields from Task 6):

- Add a test:

```ts
it('tags Agent submissions and shows the agent name', async () => {
  vi.mocked(listSubmissions).mockResolvedValue({
    total: 1,
    items: [item({ agentId: 11, agentName: 'Arena Agent' })],
  })
  const { wrapper } = await mountPage()

  expect(wrapper.text()).toContain('Agent')
  expect(wrapper.text()).toContain('Arena Agent')
  wrapper.unmount()
})
```

In `SubmissionDetailDrawer.spec.ts`, add:

```ts
it('shows agent metadata and never offers rejudge for agent submissions', async () => {
  vi.mocked(getSubmission).mockResolvedValue(
    detail({ agentId: 11, agentName: 'Arena Agent' }),
  )
  const wrapper = await mountDrawer({ canRejudge: true })

  expect(document.body.textContent).toContain('Arena Agent')
  expect(document.querySelector('[data-test="rejudge"]')).toBeNull()
  wrapper.unmount()
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `corepack pnpm --filter web test -- src/views/SubmissionsView.spec.ts src/components/SubmissionDetailDrawer.spec.ts`

Expected: FAIL because neither view renders Agent metadata yet.

- [ ] **Step 3: Render the tag and agent name**

In `SubmissionsView.vue`, import `Tag` from `primevue/tag` and extend the submitter cell:

```vue
<div class="user-cell">
  <UserIdentityName
    :nickname="data.nickname"
    :username="data.username"
    :title="data.equippedTitle"
    :badges="data.equippedBadges"
    size="small"
    :link-to="{ name: 'user-profile', params: { id: data.userId } }"
  />
  <Tag v-if="data.agentId != null" value="Agent" severity="info" class="agent-tag" />
  <span v-if="data.agentName" class="agent-name">{{ data.agentName }}</span>
</div>
```

Add scoped styles:

```css
.agent-tag {
  margin-top: 2px;
}

.agent-name {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
}
```

In `SubmissionDetailDrawer.vue`, import `Tag` and add near the user detail:

```vue
<Tag v-if="detail.agentId != null" value="Agent" severity="info" />
<span v-if="detail.agentName" class="agent-name">{{ detail.agentName }}</span>
```

Change the rejudge button condition to:

```vue
<Button
  v-if="canRejudge && detail.agentId == null"
  ...
/>
```

- [ ] **Step 4: Run the focused tests**

Run: `corepack pnpm --filter web test -- src/views/SubmissionsView.spec.ts src/components/SubmissionDetailDrawer.spec.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/SubmissionsView.vue apps/web/src/views/SubmissionsView.spec.ts apps/web/src/components/SubmissionDetailDrawer.vue apps/web/src/components/SubmissionDetailDrawer.spec.ts
git commit -m "feat(web): tag agent submissions in evaluation records"
```

---

## Task 9: Agent 详情页 Rating 历史打开关联提交

**Files:**
- Modify: `apps/web/src/views/AgentDetailView.vue`
- Modify: `apps/web/src/views/AgentDetailView.spec.ts`

**Interfaces:**
- Consumes: `AgentRatingRow.submissionId`, `SubmissionDetailDrawer`.

- [ ] **Step 1: Add failing tests**

In `AgentDetailView.spec.ts` (the `ratingRow()` factory already has `submissionId: null` from Task 6):

- Add the drawer's link routes to the existing `routes` array:

```ts
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
```

- Mock the drawer's API module and toast:

```ts
vi.mock('@/api/submissions', () => ({
  getSubmission: vi.fn(),
  rejudgeSubmission: vi.fn(),
}))
vi.mock('primevue/usetoast', () => ({ useToast: () => ({ add: vi.fn() }) }))
```

- Add `document.body.innerHTML = ''` at the end of `beforeEach` so the teleported drawer from one test does not leak into the next.
- Add a `submissionDetail()` factory with `agentId: null` and `agentName: ''`, and import `getSubmission` from `@/api/submissions`.
- Add:

```ts
it('opens the linked submission from a rating history row', async () => {
  vi.mocked(getAgentRatingHistory).mockResolvedValue(
    historyPage([ratingRow({ submissionId: 42 })]),
  )
  vi.mocked(getSubmission).mockResolvedValue(submissionDetail())
  const { wrapper } = await mountPage()

  const link = wrapper.find('[data-test="rating-submission-link"]')
  expect(link.exists()).toBe(true)
  await link.trigger('click')
  await flushPromises()

  expect(getSubmission).toHaveBeenCalledWith(42)
  expect(document.body.textContent).toContain('int main() {}')
  wrapper.unmount()
})

it('shows no submission action when the row has no linked submission', async () => {
  vi.mocked(getAgentRatingHistory).mockResolvedValue(
    historyPage([ratingRow({ submissionId: null })]),
  )
  const { wrapper } = await mountPage()
  expect(wrapper.find('[data-test="rating-submission-link"]').exists()).toBe(false)
  wrapper.unmount()
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `corepack pnpm --filter web test -- src/views/AgentDetailView.spec.ts`

Expected: FAIL because the action column and drawer do not exist.

- [ ] **Step 3: Implement the row action and drawer**

In `AgentDetailView.vue`:

- Import `SubmissionDetailDrawer`.
- Add state:

```ts
const submissionDrawer = ref(false)
const selectedSubmissionId = ref<number | null>(null)

function openSubmission(id: number) {
  selectedSubmissionId.value = id
  submissionDrawer.value = true
}
```

- Add a column after “题目” (or before “时间”):

```vue
<Column header="提交" style="width: 90px">
  <template #body="{ data }">
    <Button
      v-if="data.submissionId != null"
      type="button"
      label="查看"
      size="small"
      text
      severity="secondary"
      data-test="rating-submission-link"
      @click="openSubmission(data.submissionId)"
    />
    <span v-else class="muted">—</span>
  </template>
</Column>
```

- Add the drawer at the root of the template:

```vue
<SubmissionDetailDrawer
  v-model:visible="submissionDrawer"
  :submission-id="selectedSubmissionId"
  :can-rejudge="false"
/>
```

- [ ] **Step 4: Run the focused tests**

Run: `corepack pnpm --filter web test -- src/views/AgentDetailView.spec.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AgentDetailView.vue apps/web/src/views/AgentDetailView.spec.ts
git commit -m "feat(web): link agent rating history to submission details"
```

---

## Task 10: 我的 Agent Rating 历史打开关联提交

**Files:**
- Modify: `apps/web/src/views/AgentSettingsView.vue`
- Modify: `apps/web/src/views/AgentSettingsView.spec.ts`

**Interfaces:**
- Consumes: `AgentRatingRow.submissionId`, `SubmissionDetailDrawer`.

- [ ] **Step 1: Add failing tests**

In `AgentSettingsView.spec.ts` (the `ratingRow()` factory already has `submissionId: null` from Task 6):

- Install a memory router in `mountPage` because the drawer renders `RouterLink`s:

```ts
import { createMemoryHistory, createRouter } from 'vue-router'

const routes = [
  { path: '/settings/agent', component: { template: '<div />' } },
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/settings/agent')
  await router.isReady()
  const wrapper = mount(AgentSettingsView, {
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return wrapper
}
```

- Mock `@/api/submissions`:

```ts
vi.mock('@/api/submissions', () => ({
  getSubmission: vi.fn(),
  rejudgeSubmission: vi.fn(),
}))
```

- Import `getSubmission`, add a `submissionDetail()` factory, and add:

```ts
it('opens the linked submission from the rating history', async () => {
  vi.mocked(getMyAgent).mockResolvedValue({
    agent: agentView(),
    token: null,
    currentAttempt: null,
  })
  vi.mocked(getMyAgentRatingHistory).mockResolvedValue({
    total: 1,
    items: [ratingRow({ submissionId: 42 })],
  })
  vi.mocked(getSubmission).mockResolvedValue(submissionDetail())

  const wrapper = await mountPage()
  const link = wrapper.find('[data-test="rating-submission-link"]')
  expect(link.exists()).toBe(true)
  await link.trigger('click')
  await flushPromises()

  expect(getSubmission).toHaveBeenCalledWith(42)
  expect(document.body.textContent).toContain('int main() {}')
  wrapper.unmount()
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `corepack pnpm --filter web test -- src/views/AgentSettingsView.spec.ts`

Expected: FAIL because the action column and drawer do not exist.

- [ ] **Step 3: Implement the same action and drawer**

In `AgentSettingsView.vue`:

- Import `SubmissionDetailDrawer`.
- Add `submissionDrawer` and `selectedSubmissionId` refs and `openSubmission(id)`.
- Add the “提交” column to the existing rating `DataTable` with the same `Button` and `data-test="rating-submission-link"`.
- Render `SubmissionDetailDrawer` with `:can-rejudge="false"` at the page root.

- [ ] **Step 4: Run the focused tests**

Run: `corepack pnpm --filter web test -- src/views/AgentSettingsView.spec.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AgentSettingsView.vue apps/web/src/views/AgentSettingsView.spec.ts
git commit -m "feat(web): link own agent rating history to submissions"
```

---

## Task 11: Agent 公开详情渲染 Markdown 简介

**Files:**
- Modify: `apps/web/src/views/AgentDetailView.vue`
- Modify: `apps/web/src/views/AgentDetailView.spec.ts`

**Interfaces:**
- Consumes: `MarkdownView` and `useTheme`.
- Produces: `.profile-intro` contains a Markdown preview instead of plain text.

- [ ] **Step 1: Add a failing rendering test**

In `AgentDetailView.spec.ts`, mock `MarkdownView`:

```ts
vi.mock('@/components/MarkdownView.vue', () => ({
  default: {
    name: 'MarkdownView',
    props: ['source', 'theme'],
    template: '<div data-test="markdown-view">{{ source }}</div>',
  },
}))
```

Add:

```ts
it('renders the agent intro as markdown', async () => {
  vi.mocked(getAgent).mockResolvedValue({
    agent: agentView({ modelIntro: '## 策略\n\n- **图论**\n- 动态规划' }),
  })
  const { wrapper } = await mountPage()

  const intro = wrapper.find('[data-test="agent-intro"]')
  expect(intro.find('[data-test="markdown-view"]').text()).toContain('## 策略')
  expect(intro.find('[data-test="markdown-view"]').text()).toContain('**图论**')
  wrapper.unmount()
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `corepack pnpm --filter web test -- src/views/AgentDetailView.spec.ts`

Expected: FAIL because the intro is still a plain `<p>`.

- [ ] **Step 3: Implement MarkdownView**

In `AgentDetailView.vue`:

```ts
import MarkdownView from '@/components/MarkdownView.vue'
import { useTheme } from '@/theme/theme'

const { isDark } = useTheme()
```

Replace the intro markup:

```vue
<div v-if="agent.modelIntro" class="profile-intro" data-test="agent-intro">
  <MarkdownView :source="agent.modelIntro" :theme="isDark ? 'dark' : 'light'" />
</div>
```

Update the scoped `.profile-intro` rule so it applies to the wrapper (no change to `white-space: pre-wrap`; remove that line because MarkdownView handles whitespace).

No new sanitizer test is needed here: `MarkdownView` already routes every preview through `sanitizeHtml`, and `apps/web/src/utils/sanitize.markdown.spec.ts` pins script/iframe/handler removal. This task only proves the Agent intro goes through that component.

- [ ] **Step 4: Run the focused tests**

Run: `corepack pnpm --filter web test -- src/views/AgentDetailView.spec.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AgentDetailView.vue apps/web/src/views/AgentDetailView.spec.ts
git commit -m "feat(web): render agent intro as markdown"
```

---

## Task 12: Agent 设置页使用 Markdown 编辑器

**Files:**
- Modify: `apps/web/src/views/AgentSettingsView.vue`
- Modify: `apps/web/src/views/AgentSettingsView.spec.ts`

**Interfaces:**
- Consumes: `MarkdownEditor`, `useTheme`.
- Produces: both create/edit intro fields use `MarkdownEditor`; validation limit 2000.

- [ ] **Step 1: Add failing editor tests**

In `AgentSettingsView.spec.ts`, remove the `Textarea` import and add:

```ts
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    name: 'MarkdownEditor',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<textarea data-test="agent-intro-editor" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))
```

Replace every `wrapper.findComponent(Textarea)` usage (create, edit, and validation tests) with the mocked editor:

```ts
await wrapper.find('[data-test="agent-intro-editor"]').setValue('  外部 Agent  ')
```

and `'新的简介'` in the edit test.

Replace the `#agent-model-intro` value assertion with:

```ts
expect(
  (wrapper.find('[data-test="agent-intro-editor"]').element as HTMLTextAreaElement).value,
).toBe('用于测试的外部 Agent')
```

Change the long-intro validation test to:

```ts
await wrapper.find('[data-test="agent-intro-editor"]').setValue('x'.repeat(2001))
await submitForm(wrapper)

expect(createAgent).not.toHaveBeenCalled()
expect(wrapper.text()).toContain('模型简介不能超过 2000 个字符')
```

Add a test that Markdown source is submitted unchanged:

```ts
it('submits markdown intro source unchanged', async () => {
  vi.mocked(getMyAgent).mockResolvedValue({
    agent: agentView(),
    token: null,
    currentAttempt: null,
  })
  vi.mocked(updateMyAgent).mockResolvedValue(agentView({ modelIntro: '## 新简介' }))

  const wrapper = await mountPage()
  await wrapper.find('[data-test="agent-intro-editor"]').setValue('## 新简介\n\n- 列表')
  await submitForm(wrapper)

  expect(updateMyAgent).toHaveBeenCalledWith(
    expect.objectContaining({ modelIntro: '## 新简介\n\n- 列表' }),
  )
  wrapper.unmount()
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `corepack pnpm --filter web test -- src/views/AgentSettingsView.spec.ts`

Expected: FAIL because the page still renders `Textarea` and validates 500.

- [ ] **Step 3: Implement the Markdown editor**

In `AgentSettingsView.vue`:

```ts
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import { useTheme } from '@/theme/theme'

const { isDark } = useTheme()
const INTRO_MAX = 2000
```

Replace both `<Textarea ... />` blocks with:

```vue
<MarkdownEditor
  v-model="form.modelIntro"
  height="240px"
  :theme="isDark ? 'dark' : 'light'"
/>
```

Update the hint and validation:

```vue
hint="最多 2000 个字符，支持 Markdown"
```

```ts
if ([...form.value.modelIntro].length > INTRO_MAX) {
  errors.modelIntro = '模型简介不能超过 2000 个字符'
}
```

Remove the now-unused `Textarea` import.

- [ ] **Step 4: Run the focused tests**

Run: `corepack pnpm --filter web test -- src/views/AgentSettingsView.spec.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AgentSettingsView.vue apps/web/src/views/AgentSettingsView.spec.ts
git commit -m "feat(web): edit agent intro with markdown"
```

---

## Task 13: 首页最近练习标记 Agent 提交

**Files:**
- Modify: `apps/web/src/views/HomeView.vue`
- Modify: `apps/web/src/views/HomeView.spec.ts`

**Interfaces:**
- Consumes: `SubmissionListItem.agentId`.

- [ ] **Step 1: Add a failing test**

In `HomeView.spec.ts` (the `recentSubmission` factory already has the fields from Task 6):

- Add:

```ts
it('marks agent submissions in recent practice', async () => {
  auth.isLoggedIn = true
  auth.user = baseUser
  submissionsApi.listSubmissions.mockResolvedValue({
    total: 1,
    items: [{ ...recentSubmission, agentId: 11, agentName: 'Arena Agent' }],
  })

  const { wrapper } = await mountHome()

  expect(wrapper.text()).toContain('Agent')
  expect(wrapper.text()).toContain('Arena Agent')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `corepack pnpm --filter web test -- src/views/HomeView.spec.ts`

Expected: FAIL because the home list does not render Agent metadata.

- [ ] **Step 3: Implement the tag**

In `HomeView.vue`, import `Tag` from `primevue/tag` and wrap the title and tag in a flex row so the ellipsis on the title still works:

```vue
<span class="recent-list__main">
  <span class="recent-list__title-row">
    <span class="recent-list__title">{{ submission.problemTitle }}</span>
    <Tag
      v-if="submission.agentId != null"
      value="Agent"
      severity="info"
      class="recent-list__agent"
    />
  </span>
  <span class="recent-list__time">{{ formatDateTime(submission.createdAt) }}</span>
</span>
```

Add:

```css
.recent-list__title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.recent-list__title {
  flex: 1 1 auto;
  min-width: 0;
}

.recent-list__agent {
  flex: 0 0 auto;
}
```

- [ ] **Step 4: Run the focused test**

Run: `corepack pnpm --filter web test -- src/views/HomeView.spec.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/HomeView.vue apps/web/src/views/HomeView.spec.ts
git commit -m "feat(web): mark agent submissions in recent practice"
```

---

## Task 14: 文档更新与全量验证

**Files:**
- Modify: `docs/superpowers/specs/2026-11-18-agent-arena-design.md`

**Interfaces:**
- Consumes: all previous tasks.

- [ ] **Step 1: Update the old isolation note**

In `docs/superpowers/specs/2026-11-18-agent-arena-design.md` section 8, add a dated note:

```markdown
> 2026-11-21 更新：Agent 提交现在进入公开测评记录（`/submissions`）与详情，列表显示 Agent 标签；代码/答案仍按现有规则只对 Agent 所属账号、管理员和已通过该题的人类用户可见。人类统计隔离不变。Rating 历史可返回关联 `submissionId`，非公开题/未结束比赛题仅 Agent 所有者可获得。详见 `2026-11-21-agent-submission-records-design.md`。
```

- [ ] **Step 2: Run backend formatting, vet and tests**

```bash
cd apps/server
gofmt -l .
go vet ./...
KAPOJ_TEST_DATABASE_URL="$KAPOJ_TEST_DATABASE_URL" go test -p 1 -count=1 ./...
```

Expected: `gofmt -l .` prints nothing; `go vet` exits 0; all tests pass. If the test DB URL is unset, export it first per `README.md`.

- [ ] **Step 3: Run OpenAPI generation and frontend checks**

```bash
corepack pnpm gen:api
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: `gen:api` produces no unexpected diff; all Vitest tests pass; lint passes; build passes (existing chunk-size warnings are acceptable).

- [ ] **Step 4: Inspect the final diff**

```bash
git status --short
git diff --stat
```

Expected: only intended backend/frontend/docs/migration/OpenAPI files changed; no generated test output or temporary files.

- [ ] **Step 5: Commit the docs update**

```bash
git add docs/superpowers/specs/2026-11-18-agent-arena-design.md
git commit -m "docs(agent): note public submission records and markdown intro"
```
