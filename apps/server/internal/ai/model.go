package ai

import "time"

// Provider 是平台支持的 AI 提供商；Code 稳定不可改，平台级密钥通过 APIKeyRef
// 引用（环境变量名或密钥服务中的 ID），不存明文。
type Provider struct {
	ID           int64     `gorm:"primaryKey"`
	Code         string    `gorm:"size:64;not null;uniqueIndex:uniq_ai_providers_code"`
	Name         string    `gorm:"size:100;not null"`
	BaseURL      string    `gorm:"size:512;not null"`
	DefaultModel string    `gorm:"column:model_name;size:100;not null"`
	IsActive     bool      `gorm:"not null;default:true"`
	APIKeyRef    string    `gorm:"size:255;not null;default:''"`
	CreatedAt    time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt    time.Time `gorm:"type:timestamptz;not null"`
}

func (Provider) TableName() string { return "ai_providers" }

// UserConfig 是用户自行填写的 AI 配置，每个用户唯一一条；Provider 为 provider code。
type UserConfig struct {
	ID              int64     `gorm:"primaryKey"`
	UserID          int64     `gorm:"not null;uniqueIndex:uniq_user_ai_configs_user"`
	Provider        string    `gorm:"size:64;not null"`
	ModelName       string    `gorm:"size:100;not null"`
	APIKeyEncrypted string    `gorm:"size:1024;not null"`
	CreatedAt       time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt       time.Time `gorm:"type:timestamptz;not null"`
}

func (UserConfig) TableName() string { return "user_ai_configs" }

// Hint 是题目 AI 提示缓存，每题只缓存一份（首个用户点击后生成）。
type Hint struct {
	ID        int64     `gorm:"primaryKey"`
	ProblemID int64     `gorm:"not null;uniqueIndex:uniq_ai_hints_problem"`
	Content   string    `gorm:"type:text;not null"`
	CreatedBy int64     `gorm:"not null"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}

func (Hint) TableName() string { return "ai_hints" }
