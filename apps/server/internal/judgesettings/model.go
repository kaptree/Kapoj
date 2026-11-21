package judgesettings

import "time"

const settingsID int64 = 1

// 默认值与 judge 现有固定限制保持一致；修改默认值会改变未配置时的行为。
const (
	DefaultOutputFloorBytes     int64 = 1 << 20
	DefaultOutputSlackBytes     int64 = 1 << 20
	DefaultMaxOutputBytes       int64 = 16 << 20
	DefaultMaxInputFileBytes    int64 = 16 << 20
	DefaultMaxOutputFileBytes   int64 = 16 << 20
	DefaultMaxBatchTotalBytes   int64 = 200 << 20
	DefaultMaxProblemTotalBytes int64 = 200 << 20

	MiB int64 = 1 << 20
)

// 管理 API 接受的字节范围；关系约束在 SettingsInput.validate 中校验。
const (
	MinOutputFloorBytes int64 = 1 << 10
	MaxOutputFloorBytes int64 = 64 * MiB

	MinOutputSlackBytes int64 = 0
	MaxOutputSlackBytes int64 = 64 * MiB

	MinMaxOutputBytes int64 = 1 * MiB
	MaxMaxOutputBytes int64 = 64 * MiB

	MinMaxInputFileBytes int64 = 1 * MiB
	MaxMaxInputFileBytes int64 = 256 * MiB

	MinMaxOutputFileBytes int64 = 1 * MiB
	MaxMaxOutputFileBytes int64 = 64 * MiB

	MinMaxBatchTotalBytes int64 = 1 * MiB
	MaxMaxBatchTotalBytes int64 = 2048 * MiB

	MinMaxProblemTotalBytes int64 = 1 * MiB
	MaxMaxProblemTotalBytes int64 = 2048 * MiB
)

// Settings is the singleton judge_settings row read by both server and judge.
type Settings struct {
	ID                   int64     `gorm:"primaryKey"`
	OutputFloorBytes     int64     `gorm:"column:output_floor_bytes;not null;default:1048576"`
	OutputSlackBytes     int64     `gorm:"column:output_slack_bytes;not null;default:1048576"`
	MaxOutputBytes       int64     `gorm:"column:max_output_bytes;not null;default:16777216"`
	MaxInputFileBytes    int64     `gorm:"column:max_input_file_bytes;not null;default:16777216"`
	MaxOutputFileBytes   int64     `gorm:"column:max_output_file_bytes;not null;default:16777216"`
	MaxBatchTotalBytes   int64     `gorm:"column:max_batch_total_bytes;not null;default:209715200"`
	MaxProblemTotalBytes int64     `gorm:"column:max_problem_total_bytes;not null;default:209715200"`
	UpdatedBy            int64     `gorm:"column:updated_by;not null;default:0"`
	UpdatedAt            time.Time `gorm:"column:updated_at;type:timestamptz;not null"`
}

// TableName keeps the shared server/judge table name stable.
func (Settings) TableName() string { return "judge_settings" }
