package gamification

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// ListTasks 返回全部任务（含已下线），可见性由服务层按调用者计算。
func (r *Repository) ListTasks(ctx context.Context) ([]Task, error) {
	items := make([]Task, 0)
	if err := r.db.WithContext(ctx).Order("start_time DESC, id DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// FindTask 按 id 读取任务，包括已下线任务。
func (r *Repository) FindTask(ctx context.Context, id int64) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).First(&task, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// CreateTask 写入新任务。
func (r *Repository) CreateTask(ctx context.Context, task *Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// UpdateTaskLocked 锁定任务行后执行 mutate 并保存。hasRecords 与保存读取在
// 同一事务内完成，并与 ClaimTaskLocked 的任务行锁配合，保证领取后冻结字段
// 不会再被修改。
func (r *Repository) UpdateTaskLocked(
	ctx context.Context, id int64, mutate func(task *Task, hasRecords bool) error,
) (*Task, error) {
	var result *Task
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		hasRecords, err := taskHasRecords(tx, task.ID)
		if err != nil {
			return err
		}
		if err := mutate(&task, hasRecords); err != nil {
			return err
		}
		if err := tx.Save(&task).Error; err != nil {
			return err
		}
		result = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeactivateTask 软删除任务。
func (r *Repository) DeactivateTask(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Model(&Task{}).
		Where("id = ?", id).Update("is_active", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func taskHasRecords(db *gorm.DB, taskID int64) (bool, error) {
	var count int64
	if err := db.Model(&UserTaskRecord{}).
		Where("task_id = ?", taskID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CountPublicProblems 统计给定题目中公开存在的数量，用于校验任务题目。
func (r *Repository) CountPublicProblems(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Table("problems").
		Where("is_public = ?", true).
		Where("id IN ?", ids).
		Count(&count).Error
	return count, err
}

// TaskRecordsForUser 返回用户的任务记录，按任务 id 索引。
func (r *Repository) TaskRecordsForUser(ctx context.Context, userID int64) (map[int64]UserTaskRecord, error) {
	var rows []UserTaskRecord
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, err
	}
	records := make(map[int64]UserTaskRecord, len(rows))
	for _, row := range rows {
		records[row.TaskID] = row
	}
	return records, nil
}

func findTaskRecord(db *gorm.DB, userID, taskID int64) (*UserTaskRecord, error) {
	var record UserTaskRecord
	err := db.Where("user_id = ? AND task_id = ?", userID, taskID).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func createTaskRecord(db *gorm.DB, record *UserTaskRecord) (bool, error) {
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(record)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ClaimTaskLocked 锁定任务行后执行 validate 并幂等写入领取记录。任务行锁使
// 领取与 UpdateTaskLocked 串行化：要么领取先落库、更新随后被冻结字段校验
// 拒绝，要么更新先完成、领取看到新的冻结值。
func (r *Repository) ClaimTaskLocked(
	ctx context.Context, userID, taskID int64,
	validate func(task *Task) (time.Time, error),
) (*Task, *UserTaskRecord, error) {
	var taskResult *Task
	var recordResult *UserTaskRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, taskID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		claimedAt, err := validate(&task)
		if err != nil {
			return err
		}
		record, err := findTaskRecord(tx, userID, taskID)
		if err != nil {
			return err
		}
		if record == nil {
			record = &UserTaskRecord{
				UserID:    userID,
				TaskID:    taskID,
				Status:    TaskRecordClaimed,
				ClaimedAt: claimedAt,
			}
			created, err := createTaskRecord(tx, record)
			if err != nil {
				return err
			}
			if !created {
				record, err = findTaskRecord(tx, userID, taskID)
				if err != nil {
					return err
				}
				if record == nil {
					return httpx.ErrInternal
				}
			}
		}
		taskResult = &task
		recordResult = record
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return taskResult, recordResult, nil
}

// ListClaimedTaskRecords 返回用户所有待完成的任务记录。
func (r *Repository) ListClaimedTaskRecords(ctx context.Context, userID int64) ([]UserTaskRecord, error) {
	records := make([]UserTaskRecord, 0)
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, TaskRecordClaimed).
		Order("id ASC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

// CountTaskProgress 统计领取后、截止前的去重 AC 题目数。
func (r *Repository) CountTaskProgress(
	ctx context.Context, userID int64, problemIDs []int64, claimedAt, endTime time.Time,
) (int64, error) {
	return countTaskProgress(r.db.WithContext(ctx), userID, problemIDs, claimedAt, endTime)
}

func countTaskProgress(
	db *gorm.DB, userID int64, problemIDs []int64, claimedAt, endTime time.Time,
) (int64, error) {
	if len(problemIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := db.Table("submissions").
		Where("user_id = ? AND status = ?", userID, "accepted").
		Where("judged_at >= ? AND judged_at < ?", claimedAt, endTime).
		Where("problem_id IN ?", problemIDs).
		Distinct("problem_id").
		Count(&count).Error
	return count, err
}

// TryCompleteTask 在一个事务内锁定任务与领取记录，重新读取当前截止时间并统计
// 领取窗口内的 AC；全部完成时 claimed → completed 并写入金币流水与余额。
// 任务行先于记录行加锁，与 ClaimTaskLocked/UpdateTaskLocked 保持同一顺序，
// 因此缩短截止时间与结算不会互相穿插。重复或并发完成只会成功一次。
func (r *Repository) TryCompleteTask(ctx context.Context, userID, taskID int64) (bool, error) {
	completed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, taskID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		var record UserTaskRecord
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND task_id = ?", userID, taskID).
			Take(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if record.Status != TaskRecordClaimed || len(task.ProblemIDs) == 0 {
			return nil
		}

		progress, err := countTaskProgress(
			tx, userID, task.ProblemIDs, record.ClaimedAt, task.EndTime,
		)
		if err != nil {
			return err
		}
		if progress < int64(len(task.ProblemIDs)) {
			return nil
		}

		now := time.Now()
		res := tx.Model(&UserTaskRecord{}).
			Where("id = ? AND status = ?", record.ID, TaskRecordClaimed).
			Updates(map[string]any{"status": TaskRecordCompleted, "completed_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		if task.RewardCoins > 0 {
			txRecord := &CoinTransaction{
				UserID: userID, Amount: task.RewardCoins, Type: CoinEarn,
				Reason: "task", RefType: "task", RefID: taskID,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(txRecord)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				if err := addUserCoins(tx, userID, task.RewardCoins); err != nil {
					return err
				}
			}
		}
		completed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return completed, nil
}
