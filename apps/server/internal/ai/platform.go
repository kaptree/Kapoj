package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	platformNameMaxLen         = 100
	platformBaseURLMaxLen      = 512
	platformLastTestErrMaxLen  = 500
	platformTestTimeout        = 10 * time.Second
	platformTestSuccessMessage = "连接成功"
)

// PlatformModelView 是平台模型的脱敏视图，绝不包含 token 明文或密文。
type PlatformModelView struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	BaseURL           string     `json:"baseUrl"`
	ModelName         string     `json:"modelName"`
	Enabled           bool       `json:"enabled"`
	APIKeyMasked      string     `json:"apiKeyMasked"`
	LastTestAt        *time.Time `json:"lastTestAt"`
	LastTestOK        bool       `json:"lastTestOk"`
	LastTestError     string     `json:"lastTestError"`
	LastTestLatencyMS int        `json:"lastTestLatencyMs"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// PlatformModelInput 是新增/编辑平台模型的请求体；更新时 APIKey 为空表示
// 保留原密文。
type PlatformModelInput struct {
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	ModelName string `json:"modelName"`
	APIKey    string `json:"apiKey"`
	Enabled   bool   `json:"enabled"`
}

// PlatformTestResult 是连通性测试结果；Message 为脱敏分类，不含 token 或
// 上游响应正文。
type PlatformTestResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int    `json:"latencyMs"`
	Message   string `json:"message"`
}

// PlatformService 维护平台级模型（系统任务使用），与用户级 AI 配置无关。
type PlatformService struct {
	db       *gorm.DB
	keys     *KeyManager
	client   ChatClient
	settings *SettingsService
}

// NewPlatformService 构造平台模型服务。client 用于连通性测试，settings 用于
// 选择当前模型并返回统一设置视图。
func NewPlatformService(db *gorm.DB, keys *KeyManager, client ChatClient, settings *SettingsService) *PlatformService {
	return &PlatformService{db: db, keys: keys, client: client, settings: settings}
}

// List 返回全部平台模型（脱敏），按创建顺序排列。
func (s *PlatformService) List(ctx context.Context) ([]PlatformModelView, error) {
	var models []PlatformModel
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("ai: list platform models: %w", err)
	}
	out := make([]PlatformModelView, 0, len(models))
	for i := range models {
		out = append(out, platformModelView(s.keys, models[i]))
	}
	return out, nil
}

// Create 校验并创建平台模型；token 必填并用当前密钥加密。审计回调在事务内
// 执行，失败回滚。
func (s *PlatformService) Create(ctx context.Context, actorID int64, in PlatformModelInput, audit func(*gorm.DB) error) (*PlatformModelView, error) {
	in, err := validatePlatformModelInput(in, true)
	if err != nil {
		return nil, err
	}
	encrypted, err := s.keys.Encrypt(in.APIKey)
	if err != nil {
		return nil, err
	}

	var view *PlatformModelView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensurePlatformModelNameFree(tx, in.Name, 0); err != nil {
			return err
		}
		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		model := PlatformModel{
			Name:            in.Name,
			BaseURL:         in.BaseURL,
			ModelName:       in.ModelName,
			APIKeyEncrypted: encrypted,
			Enabled:         in.Enabled,
			CreatedBy:       actorID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("ai: create platform model: %w", err)
		}
		// GORM 的 default:true 标签会把零值 false 覆盖为 true，创建后显式写入
		// 请求的启用状态。
		if err := tx.Model(&model).Update("enabled", in.Enabled).Error; err != nil {
			return fmt.Errorf("ai: set platform model enabled: %w", err)
		}
		if err := tx.First(&model, model.ID).Error; err != nil {
			return err
		}
		modelView := platformModelView(s.keys, model)
		view = &modelView
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// Update 修改平台模型元数据；APIKey 为空时保留原密文。审计回调在事务内执行。
func (s *PlatformService) Update(ctx context.Context, actorID, id int64, in PlatformModelInput, audit func(*gorm.DB) error) (*PlatformModelView, error) {
	_ = actorID // 审计调用方通过 audit 回调记录操作者
	in, err := validatePlatformModelInput(in, false)
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

	var view *PlatformModelView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model PlatformModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}
		if err := ensurePlatformModelNameFree(tx, in.Name, id); err != nil {
			return err
		}
		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}

		updates := map[string]any{
			"name":       in.Name,
			"base_url":   in.BaseURL,
			"model_name": in.ModelName,
			"enabled":    in.Enabled,
			"updated_at": time.Now().UTC(),
		}
		if encrypted != nil {
			updates["api_key_encrypted"] = *encrypted
		}
		if err := tx.Model(&PlatformModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return fmt.Errorf("ai: update platform model: %w", err)
		}
		if err := tx.First(&model, id).Error; err != nil {
			return err
		}
		modelView := platformModelView(s.keys, model)
		view = &modelView
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// Delete 删除平台模型；当前选中的模型必须先切换，否则返回 40001。
func (s *PlatformService) Delete(ctx context.Context, actorID, id int64, audit func(*gorm.DB) error) error {
	_ = actorID // 审计调用方通过 audit 回调记录操作者
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model PlatformModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}

		var settings Settings
		err := tx.First(&settings, settingsID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if settings.PlatformModelID != nil && *settings.PlatformModelID == id {
			return httpx.ErrValidation("该模型正在使用，请先切换")
		}

		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}
		if err := tx.Delete(&model).Error; err != nil {
			return fmt.Errorf("ai: delete platform model: %w", err)
		}
		return nil
	})
}

// Test 解密 token 并对 {baseURL}/chat/completions 发送一次轻量请求，更新
// last_test_*。上游失败以脱敏分类返回，不断言 AI 总开关状态。
func (s *PlatformService) Test(ctx context.Context, id int64) (*PlatformTestResult, error) {
	if !s.keys.Configured() {
		return nil, errNoKey()
	}

	var model PlatformModel
	if err := s.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}

	started := time.Now()
	result := &PlatformTestResult{}
	switch {
	case strings.TrimSpace(model.APIKeyEncrypted) == "":
		result.Message = "未配置 Token"
	case s.client == nil:
		result.Message = "连接失败"
	default:
		plain, decErr := s.keys.Decrypt(model.APIKeyEncrypted)
		if decErr != nil {
			result.Message = "Token 解密失败"
			break
		}
		testCtx, cancel := context.WithTimeout(ctx, platformTestTimeout)
		_, chatErr := s.client.Chat(testCtx, ChatRequest{
			BaseURL: model.BaseURL,
			APIKey:  plain,
			Model:   model.ModelName,
			Messages: []ChatMessage{
				{Role: "user", Content: "ping"},
			},
		})
		cancel()
		if chatErr == nil {
			result.OK = true
			result.Message = platformTestSuccessMessage
		} else {
			result.Message = platformTestFailureMessage(chatErr)
		}
	}

	latency := int(time.Since(started).Milliseconds())
	result.LatencyMS = latency
	now := time.Now().UTC()
	lastError := ""
	if !result.OK {
		lastError = truncateRunes(result.Message, platformLastTestErrMaxLen)
	}
	if err := s.db.WithContext(ctx).Model(&PlatformModel{}).Where("id = ?", id).Updates(map[string]any{
		"last_test_at":         now,
		"last_test_ok":         result.OK,
		"last_test_error":      lastError,
		"last_test_latency_ms": latency,
	}).Error; err != nil {
		return nil, fmt.Errorf("ai: update platform model test result: %w", err)
	}
	return result, nil
}

// Select 选择当前平台模型（nil 表示取消选择），写入 ai_settings 并返回设置视图。
// 审计回调在事务内执行。
func (s *PlatformService) Select(ctx context.Context, actorID int64, modelID *int64, audit func(*gorm.DB) error) (*SettingsView, error) {
	var view *SettingsView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if modelID != nil {
			var model PlatformModel
			if err := tx.First(&model, *modelID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return httpx.ErrNotFound
				}
				return err
			}
			if !model.Enabled {
				return httpx.ErrValidation("该模型未启用")
			}
		}

		if err := ensureSettings(tx); err != nil {
			return err
		}
		var settings Settings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&settings, settingsID).Error; err != nil {
			return err
		}

		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}

		var value any
		if modelID != nil {
			value = *modelID
		}
		if err := tx.Model(&Settings{}).Where("id = ?", settingsID).Updates(map[string]any{
			"platform_model_id": value,
			"updated_by":        actorID,
			"updated_at":        time.Now().UTC(),
		}).Error; err != nil {
			return fmt.Errorf("ai: select platform model: %w", err)
		}

		if err := tx.First(&settings, settingsID).Error; err != nil {
			return err
		}
		var buildErr error
		view, buildErr = s.settings.view(tx, settings)
		return buildErr
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// validatePlatformModelInput trims and validates names/base URL; requireToken
// 用于创建（token 必填），更新时空 token 表示保留原密文。
func validatePlatformModelInput(in PlatformModelInput, requireToken bool) (PlatformModelInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > platformNameMaxLen {
		return in, httpx.ErrValidation("模型名称长度须为 1-100 个字符")
	}
	in.ModelName = strings.TrimSpace(in.ModelName)
	if n := utf8.RuneCountInString(in.ModelName); n == 0 || n > platformNameMaxLen {
		return in, httpx.ErrValidation("模型名长度须为 1-100 个字符")
	}
	baseURL, err := normalizePlatformBaseURL(in.BaseURL)
	if err != nil {
		return in, err
	}
	in.BaseURL = baseURL
	in.APIKey = strings.TrimSpace(in.APIKey)
	if requireToken && in.APIKey == "" {
		return in, httpx.ErrValidation("API Token 不能为空")
	}
	return in, nil
}

// normalizePlatformBaseURL 只接受 http/https、必须有主机、不允许 userinfo，
// 去掉末尾 "/" 且不允许以 /chat/completions 结尾（客户端会拼接该路径）。
// 内网/本地地址允许，便于接入自建模型。
func normalizePlatformBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", httpx.ErrValidation("接口地址不能为空")
	}
	if len(trimmed) > platformBaseURLMaxLen {
		return "", httpx.ErrValidation("接口地址过长")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", httpx.ErrValidation("接口地址格式无效")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", httpx.ErrValidation("接口地址必须以 http:// 或 https:// 开头")
	}
	if parsed.Host == "" {
		return "", httpx.ErrValidation("接口地址缺少主机名")
	}
	if parsed.User != nil {
		return "", httpx.ErrValidation("接口地址不能包含用户名或密码")
	}
	normalized := strings.TrimRight(trimmed, "/")
	if strings.HasSuffix(strings.ToLower(normalized), "/chat/completions") {
		return "", httpx.ErrValidation("接口地址不能以 /chat/completions 结尾")
	}
	return normalized, nil
}

// ensurePlatformModelNameFree 校验名称唯一（excludeID > 0 时忽略自身）。
func ensurePlatformModelNameFree(tx *gorm.DB, name string, excludeID int64) error {
	query := tx.Model(&PlatformModel{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return fmt.Errorf("ai: check platform model name: %w", err)
	}
	if count > 0 {
		return httpx.ErrValidation("模型名称已存在")
	}
	return nil
}

// platformModelView 构造脱敏视图；解密失败时 masked 留空，避免因历史坏密文
// 阻断整个列表。
func platformModelView(keys *KeyManager, model PlatformModel) PlatformModelView {
	view := PlatformModelView{
		ID:                model.ID,
		Name:              model.Name,
		BaseURL:           model.BaseURL,
		ModelName:         model.ModelName,
		Enabled:           model.Enabled,
		LastTestOK:        model.LastTestOK,
		LastTestError:     model.LastTestError,
		LastTestLatencyMS: model.LastTestLatencyMS,
		CreatedAt:         model.CreatedAt.UTC(),
		UpdatedAt:         model.UpdatedAt.UTC(),
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

// platformTestFailureMessage 把上游错误映射为脱敏分类，绝不携带 token 或
// 上游响应正文。
func platformTestFailureMessage(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "连接超时"
	case errors.Is(err, context.Canceled):
		return "连接已取消"
	case errors.Is(err, errUpstreamStatus):
		if code := upstreamStatusCode(err); code > 0 {
			return fmt.Sprintf("HTTP %d", code)
		}
		return "上游返回错误状态"
	case errors.Is(err, errMalformedResponse), errors.Is(err, errEmptyChoices):
		return "响应格式错误"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "连接超时"
	}
	return "连接失败"
}

// upstreamStatusCode 从 ErrUpstreamStatus 的错误文本中解析 HTTP 状态码。
func upstreamStatusCode(err error) int {
	suffix := strings.TrimPrefix(err.Error(), errUpstreamStatus.Error()+": ")
	code, convErr := strconv.Atoi(suffix)
	if convErr != nil {
		return 0
	}
	return code
}

// truncateRunes 按字符数截断，避免破坏 UTF-8。
func truncateRunes(text string, max int) string {
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	return string([]rune(text)[:max])
}
