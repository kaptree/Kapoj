package problemimport

import "time"

const settingsID int64 = 1

// Settings is the singleton problem_import_settings row.
type Settings struct {
	ID           int64     `gorm:"primaryKey"`
	MaxPackageMB int       `gorm:"column:max_package_mb;not null;default:200"`
	UpdatedBy    int64     `gorm:"column:updated_by;not null;default:0"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:timestamptz;not null"`
}

func (Settings) TableName() string { return "problem_import_settings" }
