package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	userModelNameMaxLen  = 100
	userModelTokenMaxLen = 4096
	userModelTestTimeout = 10 * time.Second
	// userModelTestMaxTokens keeps the connectivity probe cheap; the client
	// only checks that the upstream completes a chat request.
	userModelTestMaxTokens         = 16
	userModelTestSuccessMessage    = "连接成功"
	userModelLastTestErrMaxLen     = 500
	userModelEmptyTokenTestMessage = "未配置 Token"
)

// UserModelView 是用户模型配置的脱敏视图，绝不包含 token 明文或密文。
type UserModelView struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	Protocol          string     `json:"protocol"`
	BaseURL           string     `json:"baseUrl"`
	ModelName         string     `json:"modelName"`
	APIKeyMasked      string     `json:"apiKeyMasked"`
	IsActive          bool       `json:"isActive"`
	LastTestAt        *time.Time `json:"lastTestAt"`
	LastTestOK        bool       `json:"lastTestOk"`
	LastTestError     string     `json:"lastTestError"`
	LastTestLatencyMS int        `json:"lastTestLatencyMs"`
}

// UserModelInput 是新增/编辑用户模型的请求体；更新时 APIKey 为空表示保留
// 原密文。IsActive 只在创建时生效（首个模型自动成为当前模型）。
type UserModelInput struct {
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"baseUrl"`
	ModelName string `json:"modelName"`
	APIKey    string `json:"apiKey"`
	IsActive  bool   `json:"isActive"`
}

// UserModelsView 是用户模型列表：当前模型、全部模型与可用于预填的 provider 目录。
type UserModelsView struct {
	KeyConfigured bool             `json:"keyConfigured"`
	ActiveModelID *int64           `json:"activeModelId"`
	Models        []UserModelView  `json:"models"`
	Providers     []ProviderOption `json:"providers"`
}

// UserModelTestResult 是连通性测试结果；Message 为脱敏分类，不含 token 或
// 上游响应正文。
type UserModelTestResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int    `json:"latencyMs"`
	Message   string `json:"message"`
}

// ListUserModels 返回用户的全部模型（脱敏）、当前模型与 provider 目录。
func (s *Service) ListUserModels(ctx context.Context, userID int64) (*UserModelsView, error) {
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	var models []UserModel
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("id ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("ai: list user models: %w", err)
	}

	view := &UserModelsView{
		KeyConfigured: s.keys.Configured(),
		Models:        make([]UserModelView, 0, len(models)),
		Providers:     providers,
	}
	for i := range models {
		modelView := userModelView(s.keys, models[i])
		view.Models = append(view.Models, modelView)
		if modelView.IsActive {
			id := modelView.ID
			view.ActiveModelID = &id
		}
	}
	return view, nil
}

// CreateUserModel 校验并创建用户模型；token 必填并用平台密钥加密。首个模型
// 或显式 IsActive=true 时设为当前模型，事务内清空其他 active。
func (s *Service) CreateUserModel(ctx context.Context, userID int64, in UserModelInput) (*UserModelView, error) {
	in, err := validateUserModelInput(in, true)
	if err != nil {
		return nil, err
	}
	encrypted, err := s.keys.Encrypt(in.APIKey)
	if err != nil {
		return nil, err
	}

	var view *UserModelView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureUserModelNameFree(tx, userID, in.Name, 0); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&UserModel{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return fmt.Errorf("ai: count user models: %w", err)
		}
		active := count == 0 || in.IsActive
		if active {
			if err := tx.Model(&UserModel{}).
				Where("user_id = ? AND is_active = ?", userID, true).
				UpdateColumn("is_active", false).Error; err != nil {
				return fmt.Errorf("ai: clear active user models: %w", err)
			}
		}

		now := time.Now().UTC()
		model := UserModel{
			UserID:          userID,
			Name:            in.Name,
			Protocol:        in.Protocol,
			BaseURL:         in.BaseURL,
			ModelName:       in.ModelName,
			APIKeyEncrypted: encrypted,
			IsActive:        active,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("ai: create user model: %w", err)
		}
		modelView := userModelView(s.keys, model)
		view = &modelView
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// UpdateUserModel 修改用户模型元数据；APIKey 为空时保留原密文，且不要求
// 平台密钥已配置。active 状态只能通过 SetActiveUserModel 修改。
func (s *Service) UpdateUserModel(ctx context.Context, userID, id int64, in UserModelInput) (*UserModelView, error) {
	in, err := validateUserModelInput(in, false)
	if err != nil {
		return nil, err
	}

	var encrypted *string
	if in.APIKey != "" {
		ciphertext, err := s.keys.Encrypt(in.APIKey)
		if err != nil {
			return nil, err
		}
		encrypted = &ciphertext
	}

	var view *UserModelView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}
		if err := ensureUserModelNameFree(tx, userID, in.Name, id); err != nil {
			return err
		}

		updates := map[string]any{
			"name":       in.Name,
			"protocol":   in.Protocol,
			"base_url":   in.BaseURL,
			"model_name": in.ModelName,
			"updated_at": time.Now().UTC(),
		}
		if encrypted != nil {
			updates["api_key_encrypted"] = *encrypted
			// A previous connectivity result does not describe the new token.
			updates["last_test_at"] = nil
			updates["last_test_ok"] = false
			updates["last_test_error"] = ""
			updates["last_test_latency_ms"] = 0
		}
		if err := tx.Model(&UserModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return fmt.Errorf("ai: update user model: %w", err)
		}
		if err := tx.First(&model, id).Error; err != nil {
			return err
		}
		modelView := userModelView(s.keys, model)
		view = &modelView
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// DeleteUserModel 删除用户模型；当前 active 模型必须先切换，否则返回 40001。
func (s *Service) DeleteUserModel(ctx context.Context, userID, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}
		if model.IsActive {
			return httpx.ErrValidation("该模型正在使用，请先切换")
		}
		if err := tx.Delete(&model).Error; err != nil {
			return fmt.Errorf("ai: delete user model: %w", err)
		}
		return nil
	})
}

// SetActiveUserModel 在当前模型唯一约束下切换 active：事务内先清空该用户
// 其他行的 active，再设置目标行。
func (s *Service) SetActiveUserModel(ctx context.Context, userID, id int64) (*UserModelView, error) {
	var view *UserModelView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}
		if !model.IsActive {
			if err := tx.Model(&UserModel{}).
				Where("user_id = ? AND is_active = ?", userID, true).
				UpdateColumn("is_active", false).Error; err != nil {
				return fmt.Errorf("ai: clear active user models: %w", err)
			}
			if err := tx.Model(&UserModel{}).Where("id = ?", id).
				UpdateColumn("is_active", true).Error; err != nil {
				return fmt.Errorf("ai: set active user model: %w", err)
			}
			if err := tx.First(&model, id).Error; err != nil {
				return err
			}
		}
		modelView := userModelView(s.keys, model)
		view = &modelView
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// TestUserModel 解密 token 并发送一次轻量 chat 请求，更新 last_test_*。
// 上游失败以脱敏分类返回。
func (s *Service) TestUserModel(ctx context.Context, userID, id int64) (*UserModelTestResult, error) {
	if !s.keys.Configured() {
		return nil, errNoKey()
	}

	var model UserModel
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}

	started := time.Now()
	result := &UserModelTestResult{}
	switch {
	case strings.TrimSpace(model.APIKeyEncrypted) == "":
		result.Message = userModelEmptyTokenTestMessage
	case s.client == nil:
		result.Message = "连接失败"
	default:
		plain, decErr := s.keys.Decrypt(model.APIKeyEncrypted)
		if decErr != nil {
			result.Message = "Token 解密失败"
			break
		}
		testCtx, cancel := context.WithTimeout(ctx, userModelTestTimeout)
		_, chatErr := s.client.Chat(testCtx, ChatRequest{
			Protocol: model.Protocol,
			BaseURL:  model.BaseURL,
			APIKey:   plain,
			Model:    model.ModelName,
			Messages: []ChatMessage{
				{Role: "user", Content: "ping"},
			},
			MaxTokens: userModelTestMaxTokens,
		})
		cancel()
		if chatErr == nil {
			result.OK = true
			result.Message = userModelTestSuccessMessage
		} else {
			result.Message = platformTestFailureMessage(chatErr)
		}
	}

	latency := int(time.Since(started).Milliseconds())
	result.LatencyMS = latency
	now := time.Now().UTC()
	lastError := ""
	if !result.OK {
		lastError = truncateRunes(result.Message, userModelLastTestErrMaxLen)
	}
	if err := s.db.WithContext(ctx).Model(&UserModel{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{
			"last_test_at":         now,
			"last_test_ok":         result.OK,
			"last_test_error":      lastError,
			"last_test_latency_ms": latency,
		}).Error; err != nil {
		return nil, fmt.Errorf("ai: update user model test result: %w", err)
	}
	return result, nil
}

// validateUserModelInput trims and validates names/base URL/protocol/token;
// requireToken 用于创建（token 必填），更新时空 token 表示保留原密文。
func validateUserModelInput(in UserModelInput, requireToken bool) (UserModelInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > userModelNameMaxLen {
		return in, httpx.ErrValidation("模型名称长度须为 1-100 个字符")
	}

	protocol, err := normalizeUserModelProtocol(in.Protocol)
	if err != nil {
		return in, err
	}
	in.Protocol = protocol

	baseURL, err := normalizeAIBaseURL(in.BaseURL, protocol)
	if err != nil {
		return in, err
	}
	in.BaseURL = baseURL

	in.ModelName = strings.TrimSpace(in.ModelName)
	if n := utf8.RuneCountInString(in.ModelName); n == 0 || n > userModelNameMaxLen {
		return in, httpx.ErrValidation("模型名长度须为 1-100 个字符")
	}

	in.APIKey = strings.TrimSpace(in.APIKey)
	if n := utf8.RuneCountInString(in.APIKey); n > userModelTokenMaxLen {
		return in, httpx.ErrValidation("API Key 过长")
	}
	if requireToken && in.APIKey == "" {
		return in, httpx.ErrValidation("API Key 不能为空")
	}
	return in, nil
}

// normalizeUserModelProtocol 只接受显式填写的 openai / anthropic；与平台模型
// 不同，用户模型创建不允许省略协议。
func normalizeUserModelProtocol(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", httpx.ErrValidation("不支持的 AI 协议")
	}
	return normalizeAIProtocol(raw)
}

// ensureUserModelNameFree 校验名称在该用户内唯一（excludeID > 0 时忽略自身）。
func ensureUserModelNameFree(tx *gorm.DB, userID int64, name string, excludeID int64) error {
	query := tx.Model(&UserModel{}).Where("user_id = ? AND name = ?", userID, name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return fmt.Errorf("ai: check user model name: %w", err)
	}
	if count > 0 {
		return httpx.ErrValidation("模型名称已存在")
	}
	return nil
}

// userModelView 构造脱敏视图；解密失败时 masked 留空，避免因历史坏密文
// 阻断整个列表。
func userModelView(keys *KeyManager, model UserModel) UserModelView {
	protocol := strings.TrimSpace(model.Protocol)
	if protocol == "" {
		protocol = protocolOpenAI
	}
	view := UserModelView{
		ID:                model.ID,
		Name:              model.Name,
		Protocol:          protocol,
		BaseURL:           model.BaseURL,
		ModelName:         model.ModelName,
		IsActive:          model.IsActive,
		LastTestOK:        model.LastTestOK,
		LastTestError:     model.LastTestError,
		LastTestLatencyMS: model.LastTestLatencyMS,
	}
	if model.LastTestAt != nil {
		at := model.LastTestAt.UTC()
		view.LastTestAt = &at
	}
	if model.APIKeyEncrypted != "" {
		if plain, err := keys.Decrypt(model.APIKeyEncrypted); err == nil {
			view.APIKeyMasked = maskKey(plain)
		}
	}
	return view
}
