package gamification_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type taskEnv struct {
	svc  *gamification.Service
	repo *gamification.Repository
	db   *gorm.DB
	user *user.User
}

func newTaskEnv(t *testing.T) *taskEnv {
	t.Helper()
	g := testutil.PG(t)
	u := &user.User{
		Username:     "tasker",
		Nickname:     "任务用户",
		Email:        "tasker@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	repo := gamification.NewRepository(g)
	return &taskEnv{svc: gamification.NewService(repo), repo: repo, db: g, user: u}
}

func (e *taskEnv) addProblem(t *testing.T, slug string, public bool) *problem.Problem {
	t.Helper()
	p := &problem.Problem{Title: slug, Slug: slug, IsPublic: public}
	require.NoError(t, e.db.Create(p).Error)
	return p
}

func (e *taskEnv) addAC(t *testing.T, problemID int64, judgedAt time.Time) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at, judged_at)
		 VALUES (?, ?, 'cpp', 'int main() {}', ?, now(), ?)`,
		e.user.ID, problemID, submission.StatusAccepted, judgedAt,
	).Error)
}

func taskInput(title string, problemIDs []int64, reward int64, start, end time.Time) gamification.TaskInput {
	return gamification.TaskInput{
		Title:         title,
		DescriptionMD: "测试任务描述",
		ProblemIDs:    problemIDs,
		RewardCoins:   reward,
		StartTime:     start,
		EndTime:       end,
	}
}

func (e *taskEnv) createTask(t *testing.T, in gamification.TaskInput) *gamification.Task {
	t.Helper()
	task, err := e.svc.CreateTask(context.Background(), adminViewer(e.user.ID), in)
	require.NoError(t, err)
	return task
}

func (e *taskEnv) claim(t *testing.T, taskID int64) *gamification.TaskItem {
	t.Helper()
	item, err := e.svc.ClaimTask(context.Background(), e.user.ID, taskID)
	require.NoError(t, err)
	return item
}

func findTask(t *testing.T, items []gamification.TaskItem, id int64) gamification.TaskItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("task %d not found in %+v", id, items)
	return gamification.TaskItem{}
}

func TestTaskClaimAndAutoComplete(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "task-p1", true)
	p2 := e.addProblem(t, "task-p2", true)
	now := time.Now()
	task := e.createTask(t, taskInput("双题任务", []int64{p1.ID, p2.ID}, 80,
		now.Add(-time.Hour), now.Add(time.Hour)))
	assert.True(t, task.IsActive, "创建时未指定 isActive 应默认上架")

	// 领取前已经 AC 的题目不计入进度。
	e.addAC(t, p1.ID, now.Add(-30*time.Minute))

	item := e.claim(t, task.ID)
	assert.Equal(t, gamification.TaskStatusClaimed, item.Status)
	assert.EqualValues(t, 0, item.Progress)
	assert.EqualValues(t, 2, item.ProblemCount)
	require.NotNil(t, item.ClaimedAt)

	again := e.claim(t, task.ID)
	assert.Equal(t, *item.ClaimedAt, *again.ClaimedAt, "重复领取应幂等返回原记录")

	// 领取后 AC 全部题目，EvaluateUser 应自动完成并发奖。
	e.addAC(t, p1.ID, item.ClaimedAt.Add(time.Second))
	e.addAC(t, p2.ID, item.ClaimedAt.Add(2*time.Second))
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

	items, err := e.svc.ListTasks(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	done := findTask(t, items, task.ID)
	assert.Equal(t, gamification.TaskStatusCompleted, done.Status)
	assert.EqualValues(t, 2, done.Progress)
	assert.NotNil(t, done.CompletedAt)
	assert.EqualValues(t, 80, done.RewardCoins)

	balance, err := e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 80, balance)
	var txCount int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, txCount)
	var tx gamification.CoinTransaction
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).First(&tx).Error)
	assert.Equal(t, gamification.CoinEarn, tx.Type)
	assert.Equal(t, "task", tx.Reason)
	assert.Equal(t, "task", tx.RefType)
	assert.EqualValues(t, task.ID, tx.RefID)

	// 重复补算不得重复发奖。
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	balance, err = e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 80, balance)
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, txCount)

	// 已完成任务不能再次领取。
	_, err = e.svc.ClaimTask(ctx, e.user.ID, task.ID)
	assertCode(t, err, 40001)
}

func TestTaskDeadlineAndPreClaimAC(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "deadline-p1", true)
	p2 := e.addProblem(t, "deadline-p2", true)
	now := time.Now()
	end := now.Add(time.Hour)
	task := e.createTask(t, taskInput("截止任务", []int64{p1.ID, p2.ID}, 50,
		now.Add(-time.Hour), end))

	e.addAC(t, p1.ID, now.Add(-30*time.Minute))
	item := e.claim(t, task.ID)
	assert.EqualValues(t, 0, item.Progress, "领取前的 AC 不计入")

	// 截止时间之后的 AC 也不计入。
	e.addAC(t, p2.ID, end.Add(time.Minute))
	require.NoError(t, e.svc.EvaluateTasks(ctx, e.user.ID))
	items, err := e.svc.ListTasks(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	assert.EqualValues(t, 0, findTask(t, items, task.ID).Progress)

	// 窗口内的 AC 计入进度，但未集齐不完成。
	e.addAC(t, p2.ID, item.ClaimedAt.Add(time.Second))
	require.NoError(t, e.svc.EvaluateTasks(ctx, e.user.ID))
	items, err = e.svc.ListTasks(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	partial := findTask(t, items, task.ID)
	assert.Equal(t, gamification.TaskStatusClaimed, partial.Status)
	assert.EqualValues(t, 1, partial.Progress)

	// 截止前未集齐题目：读取保持 claimed，但展示为 expired。
	require.NoError(t, e.db.Model(&gamification.Task{}).Where("id = ?", task.ID).
		Update("end_time", now.Add(-time.Minute)).Error)
	require.NoError(t, e.svc.EvaluateTasks(ctx, e.user.ID))
	items, err = e.svc.ListTasks(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	expired := findTask(t, items, task.ID)
	assert.Equal(t, gamification.TaskStatusExpired, expired.Status)
	balance, err := e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, balance)

	// 截止后不能再次领取。
	_, err = e.svc.ClaimTask(ctx, e.user.ID, task.ID)
	assertCode(t, err, 40001)

	// 未领取的已结束任务不能领取。
	ended := e.createTask(t, taskInput("已结束任务", []int64{p1.ID}, 10,
		now.Add(-2*time.Hour), now.Add(-time.Hour)))
	_, err = e.svc.ClaimTask(ctx, e.user.ID, ended.ID)
	assertCode(t, err, 40001)

	// 未开始的任务不能领取。
	upcoming := e.createTask(t, taskInput("未开始任务", []int64{p1.ID}, 10,
		now.Add(time.Hour), now.Add(2*time.Hour)))
	_, err = e.svc.ClaimTask(ctx, e.user.ID, upcoming.ID)
	assertCode(t, err, 40001)
}

func TestTaskFreezeAfterClaim(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "freeze-p1", true)
	p2 := e.addProblem(t, "freeze-p2", true)
	p3 := e.addProblem(t, "freeze-p3", true)
	now := time.Now()
	task := e.createTask(t, taskInput("冻结任务", []int64{p1.ID, p2.ID}, 50,
		now.Add(-time.Hour), now.Add(time.Hour)))
	e.claim(t, task.ID)

	frozenProblems := taskInput("冻结任务", []int64{p1.ID, p3.ID}, 50,
		now.Add(-time.Hour), now.Add(time.Hour))
	_, err := e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID, frozenProblems)
	assertCode(t, err, 40001)

	frozenReward := taskInput("冻结任务", []int64{p1.ID, p2.ID}, 60,
		now.Add(-time.Hour), now.Add(time.Hour))
	_, err = e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID, frozenReward)
	assertCode(t, err, 40001)

	// 标题、描述与结束时间仍可修改，上下架也不受影响。
	editable := taskInput("改名后的任务", []int64{p1.ID, p2.ID}, 50,
		now.Add(-time.Hour), now.Add(2*time.Hour))
	editable.DescriptionMD = "新的描述"
	updated, err := e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID, editable)
	require.NoError(t, err)
	assert.Equal(t, "改名后的任务", updated.Title)
	assert.Equal(t, "新的描述", updated.DescriptionMD)
	assert.WithinDuration(t, now.Add(2*time.Hour), updated.EndTime, time.Second)

	// 尚无领取记录时可以修改题目与奖励。
	open := e.createTask(t, taskInput("未领取任务", []int64{p1.ID}, 10,
		now.Add(-time.Hour), now.Add(time.Hour)))
	changed := taskInput("未领取任务", []int64{p2.ID, p3.ID}, 20,
		now.Add(-time.Hour), now.Add(time.Hour))
	changedTask, err := e.svc.UpdateTask(ctx, adminViewer(e.user.ID), open.ID, changed)
	require.NoError(t, err)
	assert.Equal(t, []int64{p2.ID, p3.ID}, []int64(changedTask.ProblemIDs))
	assert.EqualValues(t, 20, changedTask.RewardCoins)
}

func TestInactiveClaimedTaskStillCompletes(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "inactive-p1", true)
	p2 := e.addProblem(t, "inactive-p2", true)
	now := time.Now()
	task := e.createTask(t, taskInput("下线任务", []int64{p1.ID, p2.ID}, 80,
		now.Add(-time.Hour), now.Add(time.Hour)))
	item := e.claim(t, task.ID)

	inactive := false
	offline := taskInput("下线任务", []int64{p1.ID, p2.ID}, 80,
		now.Add(-time.Hour), now.Add(time.Hour))
	offline.IsActive = &inactive
	_, err := e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID, offline)
	require.NoError(t, err)

	// 已领取用户仍能看到下线任务，游客看不到。
	items, err := e.svc.ListTasks(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	visible := findTask(t, items, task.ID)
	assert.Equal(t, gamification.TaskStatusInactive, visible.Status)
	assert.False(t, visible.IsActive)

	guestItems, err := e.svc.ListTasks(ctx, gamification.Viewer{}, false)
	require.NoError(t, err)
	assert.Empty(t, guestItems)

	// 下线任务对未领取用户不可领取。
	other := &user.User{
		Username:     "tasker-other",
		Nickname:     "其他用户",
		Email:        "tasker-other@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, e.db.Create(other).Error)
	_, err = e.svc.ClaimTask(ctx, other.ID, task.ID)
	assertCode(t, err, 40001)

	// 截止前 AC 全部题目仍会完成并发奖。
	e.addAC(t, p1.ID, item.ClaimedAt.Add(time.Second))
	e.addAC(t, p2.ID, item.ClaimedAt.Add(2*time.Second))
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

	items, err = e.svc.ListTasks(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	completed := findTask(t, items, task.ID)
	assert.Equal(t, gamification.TaskStatusCompleted, completed.Status)
	assert.EqualValues(t, 2, completed.Progress)

	balance, err := e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 80, balance)
}

func TestTaskCompletionConcurrent(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p := e.addProblem(t, "concurrent-task-p", true)
	now := time.Now()
	task := e.createTask(t, taskInput("并发任务", []int64{p.ID}, 80,
		now.Add(-time.Hour), now.Add(time.Hour)))
	item := e.claim(t, task.ID)
	e.addAC(t, p.ID, item.ClaimedAt.Add(time.Second))

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- e.svc.EvaluateUser(ctx, e.user.ID)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	balance, err := e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 80, balance)
	var txCount int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, txCount)
	var record gamification.UserTaskRecord
	require.NoError(t, e.db.Where("user_id = ? AND task_id = ?", e.user.ID, task.ID).First(&record).Error)
	assert.Equal(t, gamification.TaskRecordCompleted, record.Status)
}

func TestTaskClaimConcurrent(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p := e.addProblem(t, "concurrent-claim-p", true)
	now := time.Now()
	task := e.createTask(t, taskInput("并发领取任务", []int64{p.ID}, 10,
		now.Add(-time.Hour), now.Add(time.Hour)))

	const workers = 8
	var wg sync.WaitGroup
	items := make(chan *gamification.TaskItem, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			item, err := e.svc.ClaimTask(ctx, e.user.ID, task.ID)
			items <- item
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(items)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var first *time.Time
	for item := range items {
		require.NotNil(t, item)
		if first == nil {
			first = item.ClaimedAt
			continue
		}
		assert.Equal(t, *first, *item.ClaimedAt, "并发领取应返回同一条记录")
	}
	var count int64
	require.NoError(t, e.db.Model(&gamification.UserTaskRecord{}).
		Where("user_id = ? AND task_id = ?", e.user.ID, task.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestTaskCompletesAfterDeadlineForPreDeadlineACs(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "retro-p1", true)
	p2 := e.addProblem(t, "retro-p2", true)
	now := time.Now()
	task := e.createTask(t, taskInput("补算任务", []int64{p1.ID}, 60,
		now.Add(-3*time.Hour), now.Add(time.Hour)))
	partial := e.createTask(t, taskInput("部分进度任务", []int64{p1.ID, p2.ID}, 50,
		now.Add(-3*time.Hour), now.Add(time.Hour)))
	e.claim(t, task.ID)
	e.claim(t, partial.ID)

	// 模拟时间流逝：两条记录都在截止前领取，任务现已截止。
	claimedAt := now.Add(-2 * time.Hour)
	endedAt := now.Add(-30 * time.Minute)
	require.NoError(t, e.db.Model(&gamification.Task{}).
		Where("id IN ?", []int64{task.ID, partial.ID}).
		Update("end_time", endedAt).Error)
	require.NoError(t, e.db.Model(&gamification.UserTaskRecord{}).
		Where("task_id IN ?", []int64{task.ID, partial.ID}).
		Update("claimed_at", claimedAt).Error)

	// AC 在领取之后、截止之前落库，但直到截止后才补算。
	e.addAC(t, p1.ID, now.Add(-time.Hour))

	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

	items, err := e.svc.ListTasks(ctx, gamification.Viewer{UserID: e.user.ID}, false)
	require.NoError(t, err)
	completed := findTask(t, items, task.ID)
	assert.Equal(t, gamification.TaskStatusCompleted, completed.Status)
	assert.EqualValues(t, 1, completed.Progress)
	assert.NotNil(t, completed.CompletedAt)

	// 截止前只完成部分题目：保持 claimed，展示为 expired，不发奖。
	partialItem := findTask(t, items, partial.ID)
	assert.Equal(t, gamification.TaskStatusExpired, partialItem.Status)
	assert.EqualValues(t, 1, partialItem.Progress)
	assert.Nil(t, partialItem.CompletedAt)

	balance, err := e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 60, balance)
	var txCount int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, txCount)

	// 再次补算仍然只发一次。
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))
	balance, err = e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 60, balance)
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, txCount)
}

func TestTaskShortenedDeadlineExcludesLateACs(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "shorten-p1", true)
	p2 := e.addProblem(t, "shorten-p2", true)
	now := time.Now()
	task := e.createTask(t, taskInput("缩短截止任务", []int64{p1.ID, p2.ID}, 70,
		now.Add(-time.Hour), now.Add(time.Hour)))
	item := e.claim(t, task.ID)

	// 两条 AC 都在原截止前，但都在管理员即将设置的新截止之后。
	e.addAC(t, p1.ID, item.ClaimedAt.Add(time.Second))
	e.addAC(t, p2.ID, item.ClaimedAt.Add(2*time.Second))
	stale, err := e.repo.CountTaskProgress(ctx, e.user.ID, []int64{p1.ID, p2.ID},
		*item.ClaimedAt, task.EndTime)
	require.NoError(t, err)
	require.EqualValues(t, 2, stale, "缩短前按旧截止统计应已完成")

	shortenedEnd := item.ClaimedAt.Add(500 * time.Millisecond)
	_, err = e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID,
		taskInput(task.Title, []int64{p1.ID, p2.ID}, task.RewardCoins,
			now.Add(-time.Hour), shortenedEnd))
	require.NoError(t, err)

	// 结算必须重新读取新截止时间，不能沿用上面的旧进度。
	completed, err := e.repo.TryCompleteTask(ctx, e.user.ID, task.ID)
	require.NoError(t, err)
	assert.False(t, completed)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

	balance, err := e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.Zero(t, balance)
	var record gamification.UserTaskRecord
	require.NoError(t, e.db.Where("user_id = ? AND task_id = ?", e.user.ID, task.ID).
		First(&record).Error)
	assert.Equal(t, gamification.TaskRecordClaimed, record.Status)
	var txCount int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.Zero(t, txCount)
}

func TestTaskShortenedDeadlineStillCompletesEarlyACs(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "shorten-early-p1", true)
	p2 := e.addProblem(t, "shorten-early-p2", true)
	now := time.Now()
	task := e.createTask(t, taskInput("缩短截止但仍完成", []int64{p1.ID, p2.ID}, 70,
		now.Add(-time.Hour), now.Add(2*time.Hour)))
	item := e.claim(t, task.ID)

	e.addAC(t, p1.ID, item.ClaimedAt.Add(time.Second))
	e.addAC(t, p2.ID, item.ClaimedAt.Add(2*time.Second))

	// 新截止仍晚于全部 AC：缩短后应正常结算。
	_, err := e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID,
		taskInput(task.Title, []int64{p1.ID, p2.ID}, task.RewardCoins,
			now.Add(-time.Hour), item.ClaimedAt.Add(10*time.Second)))
	require.NoError(t, err)

	completed, err := e.repo.TryCompleteTask(ctx, e.user.ID, task.ID)
	require.NoError(t, err)
	assert.True(t, completed)

	balance, err := e.repo.Balance(ctx, e.user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 70, balance)
	var txCount int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", e.user.ID).Count(&txCount).Error)
	assert.EqualValues(t, 1, txCount)
}

func TestUpdateTaskAfterProblemUnpublished(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "unpublished-p1", true)
	p2 := e.addProblem(t, "unpublished-p2", true)
	now := time.Now()
	task := e.createTask(t, taskInput("题目下线任务", []int64{p1.ID}, 50,
		now.Add(-time.Hour), now.Add(time.Hour)))
	e.claim(t, task.ID)

	// 题目在领取后被下线：冻结题目不可改，但标题、描述与结束时间仍可编辑。
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p1.ID).
		Update("is_public", false).Error)

	editable := taskInput("题目下线后改名", []int64{p1.ID}, 50,
		now.Add(-time.Hour), now.Add(2*time.Hour))
	editable.DescriptionMD = "下线后的新描述"
	updated, err := e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID, editable)
	require.NoError(t, err)
	assert.Equal(t, "题目下线后改名", updated.Title)
	assert.Equal(t, "下线后的新描述", updated.DescriptionMD)
	assert.WithinDuration(t, now.Add(2*time.Hour), updated.EndTime, time.Second)

	changedProblems := taskInput("题目下线任务", []int64{p2.ID}, 50,
		now.Add(-time.Hour), now.Add(time.Hour))
	_, err = e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID, changedProblems)
	assertCode(t, err, 40001)

	changedReward := taskInput("题目下线任务", []int64{p1.ID}, 60,
		now.Add(-time.Hour), now.Add(time.Hour))
	_, err = e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID, changedReward)
	assertCode(t, err, 40001)
}

func TestTaskClaimUpdateConcurrentFreeze(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	p1 := e.addProblem(t, "freeze-race-p1", true)
	p2 := e.addProblem(t, "freeze-race-p2", true)

	const iterations = 20
	for i := 0; i < iterations; i++ {
		now := time.Now()
		originalProblems := []int64{p1.ID}
		newProblems := []int64{p2.ID}
		task := e.createTask(t, taskInput(fmt.Sprintf("并发冻结任务 %d", i),
			originalProblems, 10, now.Add(-time.Hour), now.Add(time.Hour)))

		var (
			wg        sync.WaitGroup
			updateErr error
			claimErr  error
			claimed   *gamification.TaskItem
		)
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, updateErr = e.svc.UpdateTask(ctx, adminViewer(e.user.ID), task.ID,
				taskInput(task.Title, newProblems, 20, now.Add(-time.Hour), now.Add(time.Hour)))
		}()
		go func() {
			defer wg.Done()
			<-start
			claimed, claimErr = e.svc.ClaimTask(ctx, e.user.ID, task.ID)
		}()
		close(start)
		wg.Wait()

		require.NoError(t, claimErr)
		require.NotNil(t, claimed)
		if updateErr != nil {
			assertCode(t, updateErr, 40001)
		}

		final, err := e.repo.FindTask(ctx, task.ID)
		require.NoError(t, err)

		// 领取结果必须与最终冻结值一致：更新先完成则领取看到新值；
		// 领取先完成则更新失败并保持旧值。
		assert.Equal(t, []int64(final.ProblemIDs), claimed.ProblemIDs)
		assert.EqualValues(t, final.RewardCoins, claimed.RewardCoins)
		if updateErr == nil {
			assert.Equal(t, newProblems, []int64(final.ProblemIDs))
			assert.EqualValues(t, 20, final.RewardCoins)
		} else {
			assert.Equal(t, originalProblems, []int64(final.ProblemIDs))
			assert.EqualValues(t, 10, final.RewardCoins)
		}

		var recordCount int64
		require.NoError(t, e.db.Model(&gamification.UserTaskRecord{}).
			Where("user_id = ? AND task_id = ?", e.user.ID, task.ID).
			Count(&recordCount).Error)
		assert.EqualValues(t, 1, recordCount)

		// 并发结束后补齐题目 AC：按最终冻结奖励只结算一次。
		e.addAC(t, p1.ID, time.Now().Add(time.Second))
		e.addAC(t, p2.ID, time.Now().Add(2*time.Second))
		require.NoError(t, e.svc.EvaluateUser(ctx, e.user.ID))

		var txCount int64
		require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
			Where("user_id = ? AND reason = ? AND ref_type = ? AND ref_id = ?",
				e.user.ID, "task", "task", task.ID).
			Count(&txCount).Error)
		assert.EqualValues(t, 1, txCount)
		var tx gamification.CoinTransaction
		require.NoError(t, e.db.
			Where("user_id = ? AND reason = ? AND ref_type = ? AND ref_id = ?",
				e.user.ID, "task", "task", task.ID).
			First(&tx).Error)
		assert.EqualValues(t, final.RewardCoins, tx.Amount)
	}
}

func TestTaskValidation(t *testing.T) {
	ctx := context.Background()
	e := newTaskEnv(t)
	now := time.Now()
	public := e.addProblem(t, "validation-public", true)
	private := e.addProblem(t, "validation-private", false)
	valid := taskInput("有效任务", []int64{public.ID}, 10,
		now.Add(-time.Hour), now.Add(time.Hour))

	twentyOne := make([]int64, 21)
	for i := range twentyOne {
		twentyOne[i] = int64(i + 1)
	}

	cases := []struct {
		name   string
		mutate func(*gamification.TaskInput)
		want   string
	}{
		{"标题为空", func(in *gamification.TaskInput) { in.Title = "  " }, "标题"},
		{"标题过长", func(in *gamification.TaskInput) { in.Title = strings.Repeat("字", 101) }, "标题"},
		{"描述过长", func(in *gamification.TaskInput) { in.DescriptionMD = strings.Repeat("字", 50001) }, "描述"},
		{"题目为空", func(in *gamification.TaskInput) { in.ProblemIDs = nil }, "题目"},
		{"题目超过 20 个", func(in *gamification.TaskInput) { in.ProblemIDs = twentyOne }, "题目"},
		{"题目重复", func(in *gamification.TaskInput) { in.ProblemIDs = []int64{public.ID, public.ID} }, "重复"},
		{"题目不存在", func(in *gamification.TaskInput) { in.ProblemIDs = []int64{999999} }, "题目"},
		{"题目未公开", func(in *gamification.TaskInput) { in.ProblemIDs = []int64{private.ID} }, "题目"},
		{"奖励为负", func(in *gamification.TaskInput) { in.RewardCoins = -1 }, "奖励"},
		{"奖励过大", func(in *gamification.TaskInput) { in.RewardCoins = 1_000_001 }, "奖励"},
		{"开始时间为零", func(in *gamification.TaskInput) { in.StartTime = time.Time{} }, "时间"},
		{"结束时间为零", func(in *gamification.TaskInput) { in.EndTime = time.Time{} }, "时间"},
		{"开始时间不早于结束", func(in *gamification.TaskInput) { in.EndTime = in.StartTime }, "时间"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			in.Title = "有效任务" + string(rune('a'+i))
			tc.mutate(&in)
			_, err := e.svc.CreateTask(ctx, adminViewer(e.user.ID), in)
			assertCode(t, err, 40001)
			var appErr interface{ Error() string }
			require.ErrorAs(t, err, &appErr)
			assert.Contains(t, appErr.Error(), tc.want)
		})
	}

	created := e.createTask(t, valid)
	assert.True(t, created.IsActive)

	_, err := e.svc.CreateTask(ctx, gamification.Viewer{UserID: e.user.ID}, valid)
	assertCode(t, err, 40300)

	_, err = e.svc.UpdateTask(ctx, gamification.Viewer{UserID: e.user.ID}, created.ID, valid)
	assertCode(t, err, 40300)
	err = e.svc.DeleteTask(ctx, gamification.Viewer{UserID: e.user.ID}, created.ID)
	assertCode(t, err, 40300)

	require.NoError(t, e.svc.DeleteTask(ctx, adminViewer(e.user.ID), created.ID))
	deleted, err := e.repo.FindTask(ctx, created.ID)
	require.NoError(t, err)
	assert.False(t, deleted.IsActive)
}
