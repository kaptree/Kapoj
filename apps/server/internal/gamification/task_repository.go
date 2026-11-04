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

// UpdateTask 覆盖保存任务；冻结字段由服务层保证。
func (r *Repository) UpdateTask(ctx context.Context, task *Task) error {
	return r.db.WithContext(ctx).Save(task).Error
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

// TaskHasRecords 判断任务是否已有用户领取记录。
func (r *Repository) TaskHasRecords(ctx context.Context, taskID int64) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&UserTaskRecord{}).
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

// FindTaskRecord 按用户与任务读取记录；不存在时返回 nil, nil。
func (r *Repository) FindTaskRecord(ctx context.Context, userID, taskID int64) (*UserTaskRecord, error) {
	var record UserTaskRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND task_id = ?", userID, taskID).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// CreateTaskRecord 幂等写入领取记录，第二个返回值表示本次是否真正插入。
func (r *Repository) CreateTaskRecord(ctx context.Context, record *UserTaskRecord) (bool, error) {
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(record)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
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
	if len(problemIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Table("submissions").
		Where("user_id = ? AND status = ?", userID, "accepted").
		Where("judged_at >= ? AND judged_at < ?", claimedAt, endTime).
		Where("problem_id IN ?", problemIDs).
		Distinct("problem_id").
		Count(&count).Error
	return count, err
}

// CompleteTask 在一个事务内锁定记录、claimed → completed，并写入金币流水与余额。
// 重复或并发完成只会成功一次；第二个返回值表示本次是否完成。
func (r *Repository) CompleteTask(ctx context.Context, userID, taskID, rewardCoins int64) (bool, error) {
	completed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record UserTaskRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND task_id = ?", userID, taskID).
			Take(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if record.Status != TaskRecordClaimed {
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

		if rewardCoins > 0 {
			txRecord := &CoinTransaction{
				UserID: userID, Amount: rewardCoins, Type: CoinEarn,
				Reason: "task", RefType: "task", RefID: taskID,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(txRecord)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				if err := addUserCoins(tx, userID, rewardCoins); err != nil {
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
