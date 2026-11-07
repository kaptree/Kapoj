package problem

import (
	"time"

	"gorm.io/gorm"
)

// SolutionPost 是解题思路论坛的帖子；通过 DeletedAt 实现软删除。
type SolutionPost struct {
	ID        int64          `gorm:"primaryKey"`
	ProblemID int64          `gorm:"not null;index"`
	UserID    int64          `gorm:"not null;index"`
	Title     string         `gorm:"size:100;not null"`
	ContentMD string         `gorm:"column:content_md;type:text;not null"`
	CreatedAt time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt time.Time      `gorm:"type:timestamptz;not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (SolutionPost) TableName() string { return "problem_solution_posts" }

// SolutionReply 是帖子回复；帖子删除后其回复整体不可见。
type SolutionReply struct {
	ID        int64          `gorm:"primaryKey"`
	PostID    int64          `gorm:"not null;index"`
	UserID    int64          `gorm:"not null;index"`
	ContentMD string         `gorm:"column:content_md;type:text;not null"`
	CreatedAt time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt time.Time      `gorm:"type:timestamptz;not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (SolutionReply) TableName() string { return "problem_solution_replies" }
