package gamification

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// TaskStatus 是任务对当前用户的展示状态；expired 只由读取计算，不落库。
type TaskStatus string

const (
	TaskStatusUpcoming  TaskStatus = "upcoming"
	TaskStatusActive    TaskStatus = "active"
	TaskStatusClaimed   TaskStatus = "claimed"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusExpired   TaskStatus = "expired"
	TaskStatusEnded     TaskStatus = "ended"
	TaskStatusInactive  TaskStatus = "inactive"
)

const maxTaskRewardCoins = 1_000_000

// TaskInput 是管理员创建或更新任务的输入。
type TaskInput struct {
	Title         string    `json:"title"`
	DescriptionMD string    `json:"descriptionMd"`
	ProblemIDs    []int64   `json:"problemIds"`
	RewardCoins   int64     `json:"rewardCoins"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	IsActive      *bool     `json:"isActive"`
}

// TaskItem 是任务的 API 表示，包含当前用户视角的领取、完成与进度。
type TaskItem struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	DescriptionMD string     `json:"descriptionMd"`
	ProblemIDs    []int64    `json:"problemIds"`
	RewardCoins   int64      `json:"rewardCoins"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       time.Time  `json:"endTime"`
	IsActive      bool       `json:"isActive"`
	Status        TaskStatus `json:"status"`
	ClaimedAt     *time.Time `json:"claimedAt"`
	CompletedAt   *time.Time `json:"completedAt"`
	Progress      int64      `json:"progress"`
	ProblemCount  int        `json:"problemCount"`
}

// TaskItemFromModel 将存储层任务转换为 API 表示；用于创建/更新响应，
// 此时没有当前用户的领取信息。
func TaskItemFromModel(task Task) TaskItem {
	return baseTaskItem(task, nil, time.Now())
}

// ListTasks 返回任务大厅。游客只看到上架且未结束的任务；登录用户额外看到
// 自己领取过的任务（包括已下线、已结束与已完成）。读取前会补算任务完成。
func (s *Service) ListTasks(ctx context.Context, v Viewer, includeInactive bool) ([]TaskItem, error) {
	if includeInactive && !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	if v.UserID > 0 {
		if err := s.EvaluateTasks(ctx, v.UserID); err != nil {
			return nil, err
		}
	}
	tasks, err := s.repo.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	records := map[int64]UserTaskRecord{}
	if v.UserID > 0 {
		records, err = s.repo.TaskRecordsForUser(ctx, v.UserID)
		if err != nil {
			return nil, err
		}
	}
	now := time.Now()
	items := make([]TaskItem, 0, len(tasks))
	for _, task := range tasks {
		record, claimed := records[task.ID]
		if !includeInactive && !claimed && !(task.IsActive && now.Before(task.EndTime)) {
			continue
		}
		var item TaskItem
		if claimed {
			item, err = s.taskItem(ctx, task, &record, now)
		} else {
			item = baseTaskItem(task, nil, now)
		}
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// CreateTask 创建任务；isActive 缺省为 true。
func (s *Service) CreateTask(ctx context.Context, v Viewer, in TaskInput) (*Task, error) {
	if !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	in, err := s.normalizeTaskInput(ctx, in)
	if err != nil {
		return nil, err
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	task := &Task{
		Title:         in.Title,
		DescriptionMD: in.DescriptionMD,
		ProblemIDs:    dbtypes.List[int64](in.ProblemIDs),
		RewardCoins:   in.RewardCoins,
		StartTime:     in.StartTime,
		EndTime:       in.EndTime,
		IsActive:      active,
		CreatedBy:     v.UserID,
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// UpdateTask 更新任务；已有领取记录后冻结题目与奖励，其余字段仍可修改。
func (s *Service) UpdateTask(ctx context.Context, v Viewer, id int64, in TaskInput) (*Task, error) {
	if !v.IsAdmin {
		return nil, httpx.ErrForbidden
	}
	existing, err := s.repo.FindTask(ctx, id)
	if err != nil {
		return nil, err
	}
	in, err = s.normalizeTaskInput(ctx, in)
	if err != nil {
		return nil, err
	}
	hasRecords, err := s.repo.TaskHasRecords(ctx, id)
	if err != nil {
		return nil, err
	}
	if hasRecords && (!sameInt64s(existing.ProblemIDs, in.ProblemIDs) || existing.RewardCoins != in.RewardCoins) {
		return nil, httpx.ErrValidation("任务已有领取记录，不能修改题目或奖励")
	}

	existing.Title = in.Title
	existing.DescriptionMD = in.DescriptionMD
	existing.ProblemIDs = dbtypes.List[int64](in.ProblemIDs)
	existing.RewardCoins = in.RewardCoins
	existing.StartTime = in.StartTime
	existing.EndTime = in.EndTime
	if in.IsActive != nil {
		existing.IsActive = *in.IsActive
	}
	if err := s.repo.UpdateTask(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteTask 软删除任务，已领取用户仍可在截止前完成并照常发奖。
func (s *Service) DeleteTask(ctx context.Context, v Viewer, id int64) error {
	if !v.IsAdmin {
		return httpx.ErrForbidden
	}
	if _, err := s.repo.FindTask(ctx, id); err != nil {
		return err
	}
	return s.repo.DeactivateTask(ctx, id)
}

// ClaimTask 领取任务。任务必须上架且在领取窗口内；已领取返回原记录，
// 已完成返回 40001。
func (s *Service) ClaimTask(ctx context.Context, userID, id int64) (*TaskItem, error) {
	if userID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	task, err := s.repo.FindTask(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if !task.IsActive {
		return nil, httpx.ErrValidation("任务已下线，不能领取")
	}
	if now.Before(task.StartTime) {
		return nil, httpx.ErrValidation("任务尚未开始")
	}
	if !now.Before(task.EndTime) {
		return nil, httpx.ErrValidation("任务已截止，不能领取")
	}

	record, err := s.repo.FindTaskRecord(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if record == nil {
		created, err := s.repo.CreateTaskRecord(ctx, &UserTaskRecord{
			UserID:    userID,
			TaskID:    id,
			Status:    TaskRecordClaimed,
			ClaimedAt: now,
		})
		if err != nil {
			return nil, err
		}
		if !created {
			// 并发领取时重新读取先写入的记录。
			record, err = s.repo.FindTaskRecord(ctx, userID, id)
			if err != nil {
				return nil, err
			}
		} else {
			record = &UserTaskRecord{
				UserID:    userID,
				TaskID:    id,
				Status:    TaskRecordClaimed,
				ClaimedAt: now,
			}
		}
	}
	if record != nil && record.Status == TaskRecordCompleted {
		return nil, httpx.ErrValidation("任务已完成，不能再次领取")
	}
	if record == nil {
		return nil, httpx.ErrInternal
	}
	item, err := s.taskItem(ctx, *task, record, now)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// EvaluateTasks 遍历用户已领取的任务，在截止前集齐全部题目 AC 时原子完成并发奖。
// 任务即使被下架也参与判定；截止后未完成的记录保持 claimed。
func (s *Service) EvaluateTasks(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	records, err := s.repo.ListClaimedTaskRecords(ctx, userID)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	now := time.Now()
	for _, record := range records {
		task, err := s.repo.FindTask(ctx, record.TaskID)
		if err != nil {
			if errors.Is(err, httpx.ErrNotFound) {
				continue
			}
			return err
		}
		if len(task.ProblemIDs) == 0 || !now.Before(task.EndTime) {
			continue
		}
		progress, err := s.repo.CountTaskProgress(
			ctx, userID, task.ProblemIDs, record.ClaimedAt, task.EndTime,
		)
		if err != nil {
			return err
		}
		if progress < int64(len(task.ProblemIDs)) {
			continue
		}
		if _, err := s.repo.CompleteTask(ctx, userID, task.ID, task.RewardCoins); err != nil {
			return err
		}
	}
	return nil
}

// taskItem 在基础表示上补算当前用户的任务进度。
func (s *Service) taskItem(ctx context.Context, task Task, record *UserTaskRecord, now time.Time) (TaskItem, error) {
	item := baseTaskItem(task, record, now)
	if record == nil {
		return item, nil
	}
	if record.Status == TaskRecordCompleted {
		item.Progress = int64(len(task.ProblemIDs))
		return item, nil
	}
	progress, err := s.repo.CountTaskProgress(
		ctx, record.UserID, task.ProblemIDs, record.ClaimedAt, task.EndTime,
	)
	if err != nil {
		return TaskItem{}, err
	}
	item.Progress = progress
	return item, nil
}

func baseTaskItem(task Task, record *UserTaskRecord, now time.Time) TaskItem {
	problemIDs := make([]int64, len(task.ProblemIDs))
	copy(problemIDs, task.ProblemIDs)
	item := TaskItem{
		ID:            task.ID,
		Title:         task.Title,
		DescriptionMD: task.DescriptionMD,
		ProblemIDs:    problemIDs,
		RewardCoins:   task.RewardCoins,
		StartTime:     task.StartTime.UTC(),
		EndTime:       task.EndTime.UTC(),
		IsActive:      task.IsActive,
		Status:        taskStatus(task, record, now),
		ProblemCount:  len(task.ProblemIDs),
	}
	if record != nil {
		claimedAt := record.ClaimedAt.UTC()
		item.ClaimedAt = &claimedAt
		if record.CompletedAt != nil {
			completedAt := record.CompletedAt.UTC()
			item.CompletedAt = &completedAt
		}
	}
	return item
}

func taskStatus(task Task, record *UserTaskRecord, now time.Time) TaskStatus {
	if record != nil && record.Status == TaskRecordCompleted {
		return TaskStatusCompleted
	}
	if !task.IsActive {
		return TaskStatusInactive
	}
	if record != nil {
		if !now.Before(task.EndTime) {
			return TaskStatusExpired
		}
		return TaskStatusClaimed
	}
	if now.Before(task.StartTime) {
		return TaskStatusUpcoming
	}
	if !now.Before(task.EndTime) {
		return TaskStatusEnded
	}
	return TaskStatusActive
}

// normalizeTaskInput 校验并规范化管理员输入，题目必须全部存在且公开。
func (s *Service) normalizeTaskInput(ctx context.Context, in TaskInput) (TaskInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.DescriptionMD = strings.TrimSpace(in.DescriptionMD)
	if err := validateText("任务标题", in.Title, 1, 100); err != nil {
		return in, err
	}
	if err := validateText("任务描述", in.DescriptionMD, 0, 50000); err != nil {
		return in, err
	}
	if len(in.ProblemIDs) < 1 || len(in.ProblemIDs) > 20 {
		return in, httpx.ErrValidation("任务题目必须为 1-20 个")
	}
	seen := make(map[int64]struct{}, len(in.ProblemIDs))
	for _, id := range in.ProblemIDs {
		if id <= 0 {
			return in, httpx.ErrValidation("任务题目 ID 必须为正数")
		}
		if _, ok := seen[id]; ok {
			return in, httpx.ErrValidation("任务题目不能重复")
		}
		seen[id] = struct{}{}
	}
	count, err := s.repo.CountPublicProblems(ctx, in.ProblemIDs)
	if err != nil {
		return in, err
	}
	if count != int64(len(seen)) {
		return in, httpx.ErrValidation("任务题目不存在或未公开")
	}
	if in.RewardCoins < 0 {
		return in, httpx.ErrValidation("任务奖励金币不能为负数")
	}
	if in.RewardCoins > maxTaskRewardCoins {
		return in, httpx.ErrValidation("任务奖励金币不能超过 " + strconv.Itoa(maxTaskRewardCoins))
	}
	if !in.StartTime.Before(in.EndTime) {
		return in, httpx.ErrValidation("任务开始时间必须早于结束时间")
	}
	return in, nil
}

func sameInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
