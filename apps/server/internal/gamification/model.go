package gamification

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

// CoinType 是金币流水的类型。
type CoinType string

const (
	CoinEarn        CoinType = "earn"
	CoinSpend       CoinType = "spend"
	CoinAdminAdjust CoinType = "admin_adjust"
)

// CoinTransaction 是用户金币余额的不可变流水。
type CoinTransaction struct {
	ID        int64     `gorm:"primaryKey"`
	UserID    int64     `gorm:"not null;index:idx_coin_transactions_user_created,priority:1;uniqueIndex:uniq_coin_transactions_idem,priority:1"`
	Amount    int64     `gorm:"not null"`
	Type      CoinType  `gorm:"size:20;not null"`
	Reason    string    `gorm:"size:40;not null;uniqueIndex:uniq_coin_transactions_idem,priority:2"`
	RefType   string    `gorm:"size:20;not null;default:'';uniqueIndex:uniq_coin_transactions_idem,priority:3,where:ref_type <> ''"`
	RefID     int64     `gorm:"not null;default:0;uniqueIndex:uniq_coin_transactions_idem,priority:4"`
	Note      string    `gorm:"size:200;not null;default:''"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null;index:idx_coin_transactions_user_created,priority:2"`
}

// Achievement 是管理员配置的成就。
type Achievement struct {
	ID              int64                        `gorm:"primaryKey"`
	Code            string                       `gorm:"size:64;not null;uniqueIndex:uniq_achievements_code"`
	Name            string                       `gorm:"size:100;not null"`
	Description     string                       `gorm:"size:500;not null;default:''"`
	IconURL         string                       `gorm:"size:512;not null;default:''"`
	ConditionType   string                       `gorm:"size:20;not null"`
	ConditionConfig dbtypes.JSON[map[string]any] `gorm:"column:condition_config_json;type:jsonb;not null;default:'{}'"`
	RewardCoins     int64                        `gorm:"not null;default:0"`
	RewardBadgeID   *int64                       `gorm:"column:reward_badge_id"`
	SortOrder       int                          `gorm:"not null;default:0"`
	IsActive        bool                         `gorm:"not null"`
	CreatedAt       time.Time                    `gorm:"type:timestamptz;not null"`
	UpdatedAt       time.Time                    `gorm:"type:timestamptz;not null"`
}

// UserAchievement 记录用户已解锁的成就。
type UserAchievement struct {
	ID            int64     `gorm:"primaryKey"`
	UserID        int64     `gorm:"not null;uniqueIndex:uniq_user_achievements,priority:1"`
	AchievementID int64     `gorm:"not null;uniqueIndex:uniq_user_achievements,priority:2"`
	UnlockedAt    time.Time `gorm:"type:timestamptz;not null"`
}

// Task 是管理员发布的金币任务。
type Task struct {
	ID            int64               `gorm:"primaryKey"`
	Title         string              `gorm:"size:100;not null"`
	DescriptionMD string              `gorm:"column:description_md;type:text;not null;default:''"`
	ProblemIDs    dbtypes.List[int64] `gorm:"column:problem_ids_json;type:jsonb;not null;default:'[]'"`
	RewardCoins   int64               `gorm:"not null;default:0"`
	StartTime     time.Time           `gorm:"type:timestamptz;not null"`
	EndTime       time.Time           `gorm:"type:timestamptz;not null"`
	IsActive      bool                `gorm:"not null"`
	CreatedBy     int64               `gorm:"not null;default:0"`
	CreatedAt     time.Time           `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time           `gorm:"type:timestamptz;not null"`
}

// TaskRecordStatus 是用户任务记录的持久化状态；expired 等展示状态不落库。
type TaskRecordStatus string

const (
	TaskRecordClaimed   TaskRecordStatus = "claimed"
	TaskRecordCompleted TaskRecordStatus = "completed"
)

// UserTaskRecord 记录用户领取与完成任务的进度。
type UserTaskRecord struct {
	ID          int64            `gorm:"primaryKey"`
	UserID      int64            `gorm:"not null;uniqueIndex:uniq_user_task_records,priority:1"`
	TaskID      int64            `gorm:"not null;uniqueIndex:uniq_user_task_records,priority:2"`
	Status      TaskRecordStatus `gorm:"size:20;not null"`
	ClaimedAt   time.Time        `gorm:"type:timestamptz;not null"`
	CompletedAt *time.Time       `gorm:"type:timestamptz"`
}

// UserLoginDay 记录用户每个登录日期，用于连续登录成就。
type UserLoginDay struct {
	ID        int64     `gorm:"primaryKey"`
	UserID    int64     `gorm:"not null;uniqueIndex:uniq_user_login_days,priority:1"`
	LoginDate time.Time `gorm:"type:date;not null;uniqueIndex:uniq_user_login_days,priority:2"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}
