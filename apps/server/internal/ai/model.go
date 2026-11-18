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

// Settings 是 ai_settings 单例行；KeyFingerprint 记录当前密钥文件的 SHA-256
// 指纹（仅诊断/一致性，不存密钥本身）。PlatformModelID 指向当前选中的平台模型。
type Settings struct {
	ID              int16     `gorm:"primaryKey"`
	Enabled         bool      `gorm:"not null;default:true"`
	KeyFingerprint  string    `gorm:"column:key_fingerprint;size:64;not null;default:''"`
	PlatformModelID *int64    `gorm:"column:platform_model_id"`
	UpdatedBy       int64     `gorm:"not null;default:0"`
	UpdatedAt       time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (Settings) TableName() string { return "ai_settings" }

// PlatformModel 是平台级 AI 模型（系统任务使用），支持 OpenAI 兼容与
// Anthropic 协议，token 加密存储。与用户级 ai_providers / user_ai_configs
// 语义独立。
type PlatformModel struct {
	ID                int64      `gorm:"primaryKey"`
	Name              string     `gorm:"size:100;not null;uniqueIndex:uniq_ai_platform_models_name"`
	Protocol          string     `gorm:"size:20;not null;default:openai"`
	BaseURL           string     `gorm:"column:base_url;size:512;not null"`
	ModelName         string     `gorm:"column:model_name;size:100;not null"`
	APIKeyEncrypted   string     `gorm:"column:api_key_encrypted;type:text;not null;default:''"`
	Enabled           bool       `gorm:"not null;default:true"`
	LastTestAt        *time.Time `gorm:"column:last_test_at;type:timestamptz"`
	LastTestOK        bool       `gorm:"column:last_test_ok;not null;default:false"`
	LastTestError     string     `gorm:"column:last_test_error;size:500;not null;default:''"`
	LastTestLatencyMS int        `gorm:"column:last_test_latency_ms;not null;default:0"`
	CreatedBy         int64      `gorm:"not null;default:0"`
	CreatedAt         time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt         time.Time  `gorm:"type:timestamptz;not null"`
}

func (PlatformModel) TableName() string { return "ai_platform_models" }

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

// UserModel 是用户保存的一条自定义模型配置；同一用户可保存多条，名称在用户
// 内唯一，其中最多一条 is_active（由部分唯一索引 uniq_user_ai_models_active
// 保证）。Protocol 取 openai / anthropic。
type UserModel struct {
	ID                int64      `gorm:"primaryKey"`
	UserID            int64      `gorm:"not null;uniqueIndex:uniq_user_ai_models_user_name,priority:1;index:idx_user_ai_models_user_active,priority:1"`
	Name              string     `gorm:"size:100;not null;uniqueIndex:uniq_user_ai_models_user_name,priority:2"`
	Protocol          string     `gorm:"size:20;not null;default:openai"`
	BaseURL           string     `gorm:"column:base_url;size:512;not null"`
	ModelName         string     `gorm:"column:model_name;size:100;not null"`
	APIKeyEncrypted   string     `gorm:"column:api_key_encrypted;type:text;not null;default:''"`
	IsActive          bool       `gorm:"column:is_active;not null;default:false;index:idx_user_ai_models_user_active,priority:2"`
	LastTestAt        *time.Time `gorm:"column:last_test_at;type:timestamptz"`
	LastTestOK        bool       `gorm:"column:last_test_ok;not null;default:false"`
	LastTestError     string     `gorm:"column:last_test_error;size:500;not null;default:''"`
	LastTestLatencyMS int        `gorm:"column:last_test_latency_ms;not null;default:0"`
	CreatedAt         time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt         time.Time  `gorm:"type:timestamptz;not null"`
}

func (UserModel) TableName() string { return "user_ai_models" }

// Hint 是题目 AI 提示缓存，每题只缓存一份（首个用户点击后生成）。
type Hint struct {
	ID        int64     `gorm:"primaryKey"`
	ProblemID int64     `gorm:"not null;uniqueIndex:uniq_ai_hints_problem"`
	Content   string    `gorm:"type:text;not null"`
	CreatedBy int64     `gorm:"not null"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}

func (Hint) TableName() string { return "ai_hints" }
