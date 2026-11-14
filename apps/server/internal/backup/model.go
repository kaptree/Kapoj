package backup

import "time"

type Kind string

const (
	KindPlatform Kind = "platform"
	KindAccounts Kind = "accounts"
	KindAccount  Kind = "account"
)

type PackageStatus string

const (
	PackageRunning PackageStatus = "running"
	PackageReady   PackageStatus = "ready"
	PackageFailed  PackageStatus = "failed"
)

type RestoreStatus string

const (
	RestoreRunning RestoreStatus = "running"
	RestoreSuccess RestoreStatus = "success"
	RestoreFailed  RestoreStatus = "failed"
)

type Frequency string

const (
	FrequencyDaily   Frequency = "daily"
	FrequencyWeekly  Frequency = "weekly"
	FrequencyMonthly Frequency = "monthly"
)

// BackupPackage is one generated ZIP and its current state.
type BackupPackage struct {
	ID          int64         `gorm:"primaryKey" json:"id"`
	Kind        Kind          `gorm:"size:20;not null;index:idx_backup_packages_kind_created,priority:1" json:"kind"`
	UserID      *int64        `gorm:"column:user_id;index:idx_backup_packages_kind_created,priority:2" json:"userId,omitempty"`
	Filename    string        `gorm:"size:255;not null;default:''" json:"filename"`
	SizeBytes   int64         `gorm:"not null;default:0" json:"sizeBytes"`
	Status      PackageStatus `gorm:"size:20;not null;index:idx_backup_packages_status" json:"status"`
	Error       string        `gorm:"column:error_message;type:text;not null;default:''" json:"error,omitempty"`
	SchemaHash  string        `gorm:"size:80;not null;default:''" json:"schemaHash"`
	CreatedBy   int64         `gorm:"not null;default:0" json:"createdBy"`
	CreatedAt   time.Time     `gorm:"type:timestamptz;not null" json:"createdAt"`
	CompletedAt *time.Time    `gorm:"type:timestamptz" json:"completedAt,omitempty"`
}

func (BackupPackage) TableName() string { return "backup_packages" }

// BackupSchedule stores the preset schedule for platform or all-account backups.
type BackupSchedule struct {
	ID         int64      `gorm:"primaryKey" json:"id"`
	Kind       Kind       `gorm:"size:20;not null;uniqueIndex:uniq_backup_schedules_kind" json:"kind"`
	Enabled    bool       `gorm:"not null;default:false" json:"enabled"`
	Frequency  Frequency  `gorm:"size:20;not null;default:daily" json:"frequency"`
	RunHour    int        `gorm:"not null;default:3" json:"runHour"`
	RunMinute  int        `gorm:"not null;default:0" json:"runMinute"`
	Weekday    int        `gorm:"not null;default:1" json:"weekday"`
	DayOfMonth int        `gorm:"not null;default:1" json:"dayOfMonth"`
	NextRunAt  *time.Time `gorm:"type:timestamptz" json:"nextRunAt,omitempty"`
	LastRunAt  *time.Time `gorm:"type:timestamptz" json:"lastRunAt,omitempty"`
	LastStatus string     `gorm:"size:20;not null;default:''" json:"lastStatus,omitempty"`
	LastError  string     `gorm:"column:last_error;type:text;not null;default:''" json:"lastError,omitempty"`
	UpdatedBy  int64      `gorm:"not null;default:0" json:"updatedBy"`
	UpdatedAt  time.Time  `gorm:"type:timestamptz;not null" json:"updatedAt"`
}

func (BackupSchedule) TableName() string { return "backup_schedules" }

// BackupRestoreJob tracks an asynchronous restore.
type BackupRestoreJob struct {
	ID              int64         `gorm:"primaryKey" json:"id"`
	Kind            Kind          `gorm:"size:20;not null" json:"kind"`
	PackageID       *int64        `json:"packageId,omitempty"`
	UploadedFile    string        `gorm:"size:255;not null;default:''" json:"uploadedFile,omitempty"`
	TargetUserID    *int64        `gorm:"column:target_user_id" json:"targetUserId,omitempty"`
	Status          RestoreStatus `gorm:"size:20;not null" json:"status"`
	Error           string        `gorm:"column:error_message;type:text;not null;default:''" json:"error,omitempty"`
	CreatedBy       int64         `gorm:"not null;default:0" json:"createdBy"`
	CreatedAt       time.Time     `gorm:"type:timestamptz;not null" json:"createdAt"`
	CompletedAt     *time.Time    `gorm:"type:timestamptz" json:"completedAt,omitempty"`
	RestartRequired bool          `gorm:"not null;default:false" json:"restartRequired"`
}

func (BackupRestoreJob) TableName() string { return "backup_restore_jobs" }

// ScheduleInput is the admin request for a preset schedule.
type ScheduleInput struct {
	Enabled    bool      `json:"enabled"`
	Frequency  Frequency `json:"frequency"`
	RunHour    int       `json:"runHour"`
	RunMinute  int       `json:"runMinute"`
	Weekday    int       `json:"weekday"`
	DayOfMonth int       `json:"dayOfMonth"`
}

// Overview is the admin read model.
type Overview struct {
	PlatformPackage *BackupPackage     `json:"platformPackage"`
	AccountsPackage *BackupPackage     `json:"accountsPackage"`
	AccountPackages []BackupPackage    `json:"accountPackages"`
	Schedules       []BackupSchedule   `json:"schedules"`
	Restores        []BackupRestoreJob `json:"restores"`
}
