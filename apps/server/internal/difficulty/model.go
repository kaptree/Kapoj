package difficulty

import "time"

// settingsID is the fixed primary key of the singleton difficulty_settings row.
const settingsID int64 = 1

// Default settings values, chosen to match the design document.
const (
	defaultEnabled          = true
	defaultScore            = 50
	defaultMinSample        = 10
	defaultPriorStrength    = 10
	defaultMaxStep          = 5
	defaultWindowDays       = 365
	defaultIncludeContest   = false
	defaultCurveGamma       = 1.5
	defaultAIAssessEnabled  = true
	defaultAITimeoutSeconds = 15
)

// Settings is the singleton difficulty_settings row. The row always has id=1.
type Settings struct {
	ID               int64     `gorm:"primaryKey"`
	Enabled          bool      `gorm:"not null;default:true"`
	DefaultScore     int       `gorm:"not null;default:50"`
	MinSample        int       `gorm:"not null;default:10"`
	PriorStrength    int       `gorm:"not null;default:10"`
	MaxStep          int       `gorm:"not null;default:5"`
	WindowDays       int       `gorm:"not null;default:365"`
	IncludeContest   bool      `gorm:"not null;default:false"`
	CurveGamma       float64   `gorm:"type:numeric(3,2);not null;default:1.5"`
	AIAssessEnabled  bool      `gorm:"not null;default:true"`
	AITimeoutSeconds int       `gorm:"not null;default:15"`
	UpdatedBy        int64     `gorm:"not null;default:0"`
	UpdatedAt        time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name stable.
func (Settings) TableName() string { return "difficulty_settings" }
