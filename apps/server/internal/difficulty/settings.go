package difficulty

import (
	"context"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// SettingsView is the read model returned by the difficulty settings API.
type SettingsView struct {
	Enabled          bool      `json:"enabled"`
	DefaultScore     int       `json:"defaultScore"`
	MinSample        int       `json:"minSample"`
	PriorStrength    int       `json:"priorStrength"`
	MaxStep          int       `json:"maxStep"`
	WindowDays       int       `json:"windowDays"`
	IncludeContest   bool      `json:"includeContest"`
	CurveGamma       float64   `json:"curveGamma"`
	AIAssessEnabled  bool      `json:"aiAssessEnabled"`
	AITimeoutSeconds int       `json:"aiTimeoutSeconds"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// SettingsInput is the full set of mutable difficulty settings.
type SettingsInput struct {
	Enabled          bool    `json:"enabled"`
	DefaultScore     int     `json:"defaultScore"`
	MinSample        int     `json:"minSample"`
	PriorStrength    int     `json:"priorStrength"`
	MaxStep          int     `json:"maxStep"`
	WindowDays       int     `json:"windowDays"`
	IncludeContest   bool    `json:"includeContest"`
	CurveGamma       float64 `json:"curveGamma"`
	AIAssessEnabled  bool    `json:"aiAssessEnabled"`
	AITimeoutSeconds int     `json:"aiTimeoutSeconds"`
}

// SettingsService reads and updates the singleton difficulty settings row.
type SettingsService struct {
	db *gorm.DB
}

// NewSettingsService constructs a difficulty settings service.
func NewSettingsService(db *gorm.DB) *SettingsService {
	return &SettingsService{db: db}
}

// Get returns the current settings, creating the default row lazily when the
// table is empty.
func (s *SettingsService) Get(ctx context.Context) (*SettingsView, error) {
	var settings Settings
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSettings(tx); err != nil {
			return err
		}
		return tx.First(&settings, settingsID).Error
	})
	if err != nil {
		return nil, err
	}
	return settingsView(settings), nil
}

// Update validates and stores a full settings update. It writes updated_by and
// updated_at and runs the audit callback inside the same transaction, so an
// audit failure rolls the whole update back.
func (s *SettingsService) Update(ctx context.Context, actorID int64, in SettingsInput, audit func(*gorm.DB) error) (*SettingsView, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	var view *SettingsView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSettings(tx); err != nil {
			return err
		}

		var current Settings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&current, settingsID).Error; err != nil {
			return err
		}

		now := time.Now().UTC()
		if err := tx.Model(&Settings{}).Where("id = ?", settingsID).Updates(map[string]any{
			"enabled":            in.Enabled,
			"default_score":      in.DefaultScore,
			"min_sample":         in.MinSample,
			"prior_strength":     in.PriorStrength,
			"max_step":           in.MaxStep,
			"window_days":        in.WindowDays,
			"include_contest":    in.IncludeContest,
			"curve_gamma":        in.CurveGamma,
			"ai_assess_enabled":  in.AIAssessEnabled,
			"ai_timeout_seconds": in.AITimeoutSeconds,
			"updated_by":         actorID,
			"updated_at":         now,
		}).Error; err != nil {
			return fmt.Errorf("difficulty: update settings: %w", err)
		}

		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}

		var updated Settings
		if err := tx.First(&updated, settingsID).Error; err != nil {
			return err
		}
		view = settingsView(updated)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// ensureSettings inserts the default settings row when absent.
func ensureSettings(tx *gorm.DB) error {
	settings := Settings{
		ID:               settingsID,
		Enabled:          defaultEnabled,
		DefaultScore:     defaultScore,
		MinSample:        defaultMinSample,
		PriorStrength:    defaultPriorStrength,
		MaxStep:          defaultMaxStep,
		WindowDays:       defaultWindowDays,
		IncludeContest:   defaultIncludeContest,
		CurveGamma:       defaultCurveGamma,
		AIAssessEnabled:  defaultAIAssessEnabled,
		AITimeoutSeconds: defaultAITimeoutSeconds,
		UpdatedAt:        time.Now().UTC(),
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error; err != nil {
		return fmt.Errorf("difficulty: ensure settings: %w", err)
	}
	return nil
}

func settingsView(settings Settings) *SettingsView {
	return &SettingsView{
		Enabled:          settings.Enabled,
		DefaultScore:     settings.DefaultScore,
		MinSample:        settings.MinSample,
		PriorStrength:    settings.PriorStrength,
		MaxStep:          settings.MaxStep,
		WindowDays:       settings.WindowDays,
		IncludeContest:   settings.IncludeContest,
		CurveGamma:       settings.CurveGamma,
		AIAssessEnabled:  settings.AIAssessEnabled,
		AITimeoutSeconds: settings.AITimeoutSeconds,
		UpdatedAt:        settings.UpdatedAt.UTC(),
	}
}

func (in SettingsInput) validate() error {
	if in.DefaultScore < 1 || in.DefaultScore > 100 {
		return httpx.ErrValidation("默认难度分必须在 1 到 100 之间")
	}
	if in.MinSample < 1 || in.MinSample > 1000 {
		return httpx.ErrValidation("最少样本数必须在 1 到 1000 之间")
	}
	if in.PriorStrength < 1 || in.PriorStrength > 100 {
		return httpx.ErrValidation("先验强度必须在 1 到 100 之间")
	}
	if in.MaxStep < 1 || in.MaxStep > 100 {
		return httpx.ErrValidation("每日步长必须在 1 到 100 之间")
	}
	if in.WindowDays < 0 || in.WindowDays > 3650 {
		return httpx.ErrValidation("统计窗口天数必须在 0 到 3650 之间")
	}
	if math.IsNaN(in.CurveGamma) || in.CurveGamma < 1.0 || in.CurveGamma > 3.0 {
		return httpx.ErrValidation("curve gamma 必须在 1.0 到 3.0 之间")
	}
	if in.AITimeoutSeconds < 5 || in.AITimeoutSeconds > 60 {
		return httpx.ErrValidation("AI 超时秒数必须在 5 到 60 之间")
	}
	return nil
}
