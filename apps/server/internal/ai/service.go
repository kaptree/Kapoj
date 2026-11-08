package ai

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// ProviderOption 是返回给用户的可用 provider 选项。
type ProviderOption struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	DefaultModel string `json:"defaultModel"`
}

// ConfigView 是用户 AI 配置的脱敏视图，绝不包含明文或密文 API key。
// KeyConfigured 报告平台是否已配置 AI 加密密钥（与用户自身是否配置无关）。
type ConfigView struct {
	Configured    bool             `json:"configured"`
	KeyConfigured bool             `json:"keyConfigured"`
	Provider      string           `json:"provider"`
	ModelName     string           `json:"modelName"`
	APIKeyMasked  string           `json:"apiKeyMasked"`
	Providers     []ProviderOption `json:"providers"`
}

// SaveConfigInput 是保存用户 AI 配置的请求体。
type SaveConfigInput struct {
	Provider  string `json:"provider"`
	APIKey    string `json:"apiKey"`
	ModelName string `json:"modelName"`
}

// Service 提供 provider 查询、用户 AI 配置的读取/保存与代码生成能力。
type Service struct {
	db      *gorm.DB
	keys    *KeyManager
	limiter Limiter
	client  ChatClient
}

// NewService 构造 AI 服务；limiter/client 为生成接口所需，可传 nil（仅配置查询时）。
func NewService(db *gorm.DB, keys *KeyManager, limiter Limiter, client ChatClient) *Service {
	return &Service{db: db, keys: keys, limiter: limiter, client: client}
}

// KeyConfigured 报告平台是否已配置 AI 加密密钥。
func (s *Service) KeyConfigured() bool { return s.keys.Configured() }

// ListProviders 返回启用的 provider 选项。
func (s *Service) ListProviders(ctx context.Context) ([]ProviderOption, error) {
	var providers []Provider
	if err := s.db.WithContext(ctx).Where("is_active = ?", true).
		Order("id ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderOption, 0, len(providers))
	for _, p := range providers {
		out = append(out, ProviderOption{Code: p.Code, Name: p.Name, DefaultModel: p.DefaultModel})
	}
	return out, nil
}

// GetConfig 读取用户配置；无配置时返回 configured=false 及启用的 provider 列表。
// 未配置平台密钥时直接返回 keyConfigured=false，不尝试解密用户密文。
func (s *Service) GetConfig(ctx context.Context, userID int64) (*ConfigView, error) {
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	view := &ConfigView{Providers: providers, KeyConfigured: s.keys.Configured()}
	if !s.keys.Configured() {
		return view, nil
	}

	var cfg UserConfig
	err = s.db.WithContext(ctx).Where("user_id = ?", userID).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return view, nil
	}
	if err != nil {
		return nil, err
	}

	plain, err := s.keys.Decrypt(cfg.APIKeyEncrypted)
	if err != nil {
		return nil, err
	}
	view.Configured = true
	view.Provider = cfg.Provider
	view.ModelName = cfg.ModelName
	view.APIKeyMasked = maskKey(plain)
	return view, nil
}

// SaveConfig 保存用户配置：provider 必须启用；model 为空时用默认 model；
// 首次保存必须提供 key；已有配置且 key 为空时保留原密文。
func (s *Service) SaveConfig(ctx context.Context, userID int64, in SaveConfigInput) (*ConfigView, error) {
	if !s.keys.Configured() {
		return nil, errNoKey()
	}

	code := strings.TrimSpace(in.Provider)
	if code == "" {
		return nil, httpx.ErrValidation("服务商不能为空")
	}

	var provider Provider
	err := s.db.WithContext(ctx).Where("code = ?", code).First(&provider).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrValidation("未知的服务商")
	}
	if err != nil {
		return nil, err
	}
	if !provider.IsActive {
		return nil, httpx.ErrValidation("服务商未启用")
	}

	modelName := strings.TrimSpace(in.ModelName)
	if modelName == "" {
		modelName = provider.DefaultModel
	}
	if modelName == "" {
		return nil, httpx.ErrValidation("模型名不能为空")
	}

	var existing UserConfig
	err = s.db.WithContext(ctx).Where("user_id = ?", userID).First(&existing).Error
	hasExisting := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	encrypted := existing.APIKeyEncrypted
	key := strings.TrimSpace(in.APIKey)
	if key != "" {
		encrypted, err = s.keys.Encrypt(key)
		if err != nil {
			return nil, err
		}
	} else if !hasExisting {
		return nil, httpx.ErrValidation("API Key 不能为空")
	}

	cfg := UserConfig{
		UserID:          userID,
		Provider:        code,
		ModelName:       modelName,
		APIKeyEncrypted: encrypted,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := s.upsert(ctx, &cfg); err != nil {
		return nil, err
	}

	plain, err := s.keys.Decrypt(encrypted)
	if err != nil {
		return nil, err
	}
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	return &ConfigView{
		Configured:    true,
		KeyConfigured: true,
		Provider:      code,
		ModelName:     modelName,
		APIKeyMasked:  maskKey(plain),
		Providers:     providers,
	}, nil
}

// upsert 按 user_id 唯一约束插入或更新配置，更新时不改 created_at。
func (s *Service) upsert(ctx context.Context, cfg *UserConfig) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"provider", "model_name", "api_key_encrypted", "updated_at"}),
	}).Create(cfg).Error
}

// maskKey 脱敏 API key：长度 >8 显示前 3 个字符 + … + 后 4 个字符，否则 "****"。
func maskKey(key string) string {
	r := []rune(key)
	if len(r) > 8 {
		return string(r[:3]) + "…" + string(r[len(r)-4:])
	}
	return "****"
}
