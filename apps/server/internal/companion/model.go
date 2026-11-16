package companion

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

// State is the per-user companion state. The user's row is created lazily and
// updated in place; bond and level are denormalized for cheap reads.
type State struct {
	UserID        int64                `gorm:"primaryKey"`
	Bond          int                  `gorm:"not null;default:0"`
	Level         int                  `gorm:"not null;default:0"`
	DailyGain     int                  `gorm:"not null;default:0"`
	DailyDate     string               `gorm:"size:10;not null;default:''"`
	ActivityLevel string               `gorm:"size:10;not null;default:'normal'"`
	DockEnabled   bool                 `gorm:"not null;default:true"`
	PublicProfile bool                 `gorm:"not null;default:false"`
	EquippedSkin  string               `gorm:"size:40;not null;default:''"`
	UnlockedSkins dbtypes.List[string] `gorm:"column:unlocked_skins_json;type:jsonb;not null;default:'[]'"`
	LastPetDate   string               `gorm:"size:10;not null;default:''"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TableName keeps the table name from the design document.
func (State) TableName() string { return "companion_states" }

// Event is one companion event. The unique (user_id, key) index makes every
// award idempotent; the other indexes serve the unread badge and date queries.
type Event struct {
	ID        int64                        `gorm:"primaryKey"`
	UserID    int64                        `gorm:"not null;uniqueIndex:uniq_companion_events_user_key,priority:1;index:idx_companion_events_user_unread,priority:1;index:idx_companion_events_user_date,priority:1"`
	Key       string                       `gorm:"size:64;not null;uniqueIndex:uniq_companion_events_user_key,priority:2"`
	Type      string                       `gorm:"size:20;not null;index:idx_companion_events_user_unread,priority:2;index:idx_companion_events_user_date,priority:2"`
	Bond      int                          `gorm:"not null;default:0"`
	EventDate string                       `gorm:"size:10;not null;default:'';index:idx_companion_events_user_date,priority:3"`
	Payload   dbtypes.JSON[map[string]any] `gorm:"type:jsonb;not null;default:'{}'"`
	ReadAt    *time.Time                   `gorm:"index:idx_companion_events_user_unread,priority:3"`
	CreatedAt time.Time                    `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (Event) TableName() string { return "companion_events" }
