package backup

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("backup not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// EnsureSchedules creates the two preset schedule rows once.
func (r *Repository) EnsureSchedules(ctx context.Context) error {
	now := time.Now().UTC()
	for _, kind := range []Kind{KindPlatform, KindAccounts} {
		row := BackupSchedule{
			Kind: kind, Enabled: false, Frequency: FrequencyDaily,
			RunHour: 3, RunMinute: 0, Weekday: 1, DayOfMonth: 1, UpdatedAt: now,
		}
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Overview(ctx context.Context) (*Overview, error) {
	out := &Overview{
		AccountPackages: []BackupPackage{},
		Schedules:       []BackupSchedule{},
		Restores:        []BackupRestoreJob{},
	}
	var platform, accounts BackupPackage
	if err := r.db.WithContext(ctx).
		Where("kind = ? AND status = ? AND filename NOT LIKE ?", KindPlatform, PackageReady, "safety/%").
		Order("id DESC").First(&platform).Error; err == nil {
		out.PlatformPackage = &platform
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Where("kind = ? AND status = ?", KindAccounts, PackageReady).
		Order("id DESC").First(&accounts).Error; err == nil {
		out.AccountsPackage = &accounts
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Where("kind = ?", KindAccount).
		Order("id DESC").Limit(50).Find(&out.AccountPackages).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Order("kind").Find(&out.Schedules).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Order("id DESC").Limit(20).Find(&out.Restores).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) FindSchedule(ctx context.Context, kind Kind) (*BackupSchedule, error) {
	var row BackupSchedule
	if err := r.db.WithContext(ctx).Where("kind = ?", kind).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) SaveSchedule(ctx context.Context, kind Kind, in ScheduleInput, next *time.Time, actorID int64) (*BackupSchedule, error) {
	row, err := r.FindSchedule(ctx, kind)
	if err != nil {
		return nil, err
	}
	row.Enabled = in.Enabled
	row.Frequency = in.Frequency
	row.RunHour = in.RunHour
	row.RunMinute = in.RunMinute
	row.Weekday = in.Weekday
	row.DayOfMonth = in.DayOfMonth
	row.NextRunAt = next
	row.UpdatedBy = actorID
	row.UpdatedAt = time.Now().UTC()
	if !row.Enabled {
		row.NextRunAt = nil
	}
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// ClaimDueSchedules atomically returns due schedules and advances next_run_at.
func (r *Repository) ClaimDueSchedules(ctx context.Context, now time.Time) ([]BackupSchedule, error) {
	out := make([]BackupSchedule, 0)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("enabled = true AND next_run_at IS NOT NULL AND next_run_at <= ?", now).
			Order("next_run_at").Find(&out).Error; err != nil {
			return err
		}
		for i := range out {
			next := NextRun(out[i], now)
			out[i].NextRunAt = &next
			out[i].LastRunAt = &now
			out[i].LastStatus = "queued"
			out[i].LastError = ""
			out[i].UpdatedAt = now
			if err := tx.Model(&BackupSchedule{}).Where("id = ?", out[i].ID).Updates(map[string]any{
				"next_run_at": next, "last_run_at": now, "last_status": "queued",
				"last_error": "", "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (r *Repository) SetScheduleResult(ctx context.Context, kind Kind, status string, errMsg string) error {
	return r.db.WithContext(ctx).Model(&BackupSchedule{}).Where("kind = ?", kind).
		Updates(map[string]any{"last_status": status, "last_error": errMsg}).Error
}

func (r *Repository) CreatePackage(ctx context.Context, row *BackupPackage) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *Repository) FinishPackage(ctx context.Context, id int64, status PackageStatus, filename string, size int64, errMsg, schemaHash string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&BackupPackage{}).Where("id = ?", id).
		Updates(map[string]any{
			"status": status, "filename": filename, "size_bytes": size,
			"error_message": errMsg, "schema_hash": schemaHash, "completed_at": now,
		}).Error
}

func (r *Repository) FindPackage(ctx context.Context, id int64) (*BackupPackage, error) {
	var row BackupPackage
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) LatestPackage(ctx context.Context, kind Kind, userID *int64) (*BackupPackage, error) {
	q := r.db.WithContext(ctx).Where("kind = ? AND status = ?", kind, PackageReady)
	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	}
	var row BackupPackage
	if err := q.Order("id DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// RemoveOlderPackages deletes prior package rows and returns their filenames.
func (r *Repository) RemoveOlderPackages(ctx context.Context, kind Kind, userID *int64, keepID int64) ([]string, error) {
	q := r.db.WithContext(ctx).Where("kind = ? AND id <> ?", kind, keepID)
	if kind == KindPlatform {
		// Safety snapshots are managed separately and must survive normal
		// platform backup retention.
		q = q.Where("filename NOT LIKE ?", "safety/%")
	}
	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	} else {
		q = q.Where("user_id IS NULL")
	}
	var rows []BackupPackage
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(rows))
	files := make([]string, 0, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
		if rows[i].Filename != "" {
			files = append(files, rows[i].Filename)
		}
	}
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&BackupPackage{}).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// DeleteSafetyPackagesExcept removes stale safety-package rows after their
// objects have been pruned by the restore workflow.
func (r *Repository) DeleteSafetyPackagesExcept(ctx context.Context, keepID int64) error {
	return r.db.WithContext(ctx).
		Where("filename LIKE ? AND id <> ?", "safety/%", keepID).
		Delete(&BackupPackage{}).Error
}

func (r *Repository) CreateRestoreJob(ctx context.Context, row *BackupRestoreJob) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *Repository) FinishRestoreJob(ctx context.Context, id int64, status RestoreStatus, errMsg string, restart bool) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&BackupRestoreJob{}).Where("id = ?", id).
		Updates(map[string]any{
			"status": status, "error_message": errMsg, "completed_at": now,
			"restart_required": restart,
		}).Error
}

func (r *Repository) FindRestoreJob(ctx context.Context, id int64) (*BackupRestoreJob, error) {
	var row BackupRestoreJob
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) AnyRunningRestore(ctx context.Context) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&BackupRestoreJob{}).
		Where("status = ?", RestoreRunning).Count(&count).Error
	return count > 0, err
}

func (r *Repository) AnyRunningPackage(ctx context.Context) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&BackupPackage{}).
		Where("status = ?", PackageRunning).Count(&count).Error
	return count > 0, err
}
