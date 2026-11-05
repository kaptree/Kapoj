package learning

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

// 学习路线节点的进度状态。
const (
	ProgressLocked    = "locked"
	ProgressActive    = "active"
	ProgressCompleted = "completed"
)

// LearningPath 是一条官方或用户自定义学习路线。
type LearningPath struct {
	ID            int64     `gorm:"primaryKey"`
	Title         string    `gorm:"size:100;not null"`
	DescriptionMD string    `gorm:"column:description_md;type:text;not null;default:''"`
	CoverURL      string    `gorm:"size:512;not null;default:''"`
	IsOfficial    bool      `gorm:"not null"`
	IsPublic      bool      `gorm:"not null"`
	IsActive      bool      `gorm:"not null;index:idx_learning_paths_active"`
	CreatedBy     int64     `gorm:"not null;index:idx_learning_paths_created_by"`
	SortOrder     int       `gorm:"not null;default:0"`
	CreatedAt     time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time `gorm:"type:timestamptz;not null"`
}

func (LearningPath) TableName() string { return "learning_paths" }

// LearningPathNode 是学习路线中的一个树节点。ProblemIDs 保留题目顺序。
type LearningPathNode struct {
	ID            int64               `gorm:"primaryKey"`
	PathID        int64               `gorm:"not null;index:idx_learning_path_nodes_path"`
	ParentID      *int64              `gorm:"index:idx_learning_path_nodes_parent"`
	Title         string              `gorm:"size:200;not null"`
	DescriptionMD string              `gorm:"column:description_md;type:text;not null;default:''"`
	ProblemIDs    dbtypes.List[int64] `gorm:"column:problem_ids_json;type:jsonb;not null;default:'[]'"`
	SortOrder     int                 `gorm:"not null;default:0"`
	RequiredCount int                 `gorm:"not null;default:1"`
	CreatedAt     time.Time           `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time           `gorm:"type:timestamptz;not null"`
}

func (LearningPathNode) TableName() string { return "learning_path_nodes" }

// UserLearningProgress 记录用户在路线节点上的服务端进度。本阶段只建表，
// 进度计算与事件在 M4-2B 实现。
type UserLearningProgress struct {
	ID          int64      `gorm:"primaryKey"`
	UserID      int64      `gorm:"not null;uniqueIndex:uniq_user_learning_progress,priority:1;index:idx_user_learning_progress_user"`
	PathID      int64      `gorm:"not null;index:idx_user_learning_progress_path"`
	NodeID      int64      `gorm:"not null;uniqueIndex:uniq_user_learning_progress,priority:2"`
	Status      string     `gorm:"size:20;not null"`
	SolvedCount int        `gorm:"not null;default:0"`
	CompletedAt *time.Time `gorm:"type:timestamptz"`
	UpdatedAt   time.Time  `gorm:"type:timestamptz;not null"`
}

func (UserLearningProgress) TableName() string { return "user_learning_progress" }
