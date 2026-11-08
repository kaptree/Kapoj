package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// settingsID is the fixed primary key of the singleton ai_settings row.
const settingsID int16 = 1

// Settings is the singleton configuration row (id=1) for the AI feature switch.
// Missing rows are lazily created on first read with enabled=true.
type Settings struct {
	ID        int16     `gorm:"primaryKey"`
	Enabled   bool      `gorm:"not null;default:true"`
	UpdatedBy int64     `gorm:"not null;default:0"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (Settings) TableName() string { return "ai_settings" }

// SettingsView is the read model returned by the admin AI settings API.
type SettingsView struct {
	Enabled       bool      `json:"enabled"`
	KeyConfigured bool      `json:"keyConfigured"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// SettingsInput is the admin request body for updating the AI switch.
// Enabled is a pointer so an omitted field is distinguishable from an explicit
// false; the handler rejects a nil value rather than silently disabling AI.
type SettingsInput struct {
	Enabled *bool `json:"enabled"`
}

// SettingsService reads and updates the AI feature switch. The current
// KeyManager supplies keyConfigured dynamically; enabling is rejected while no
// key is configured. keyFile is retained for the key-management feature (Task 2).
type SettingsService struct {
	db      *gorm.DB
	keys    *KeyManager
	keyFile string
}

// NewSettingsService constructs an AI settings service.
func NewSettingsService(db *gorm.DB, keys *KeyManager, keyFile string) *SettingsService {
	return &SettingsService{db: db, keys: keys, keyFile: keyFile}
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
	return s.view(settings), nil
}

// Update sets the AI switch and audits the change in the same transaction.
// Enabling requires a configured encryption key; disabling is always allowed.
// The audit callback runs inside the transaction, so its failure rolls back the
// whole update.
func (s *SettingsService) Update(ctx context.Context, actorID int64, enabled bool, audit func(*gorm.DB) error) (*SettingsView, error) {
	if enabled && !s.keys.Configured() {
		return nil, httpx.ErrValidation("未配置 AI 加密密钥，无法启用")
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

		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		if err := tx.Model(&Settings{}).Where("id = ?", settingsID).Updates(map[string]any{
			"enabled":    enabled,
			"updated_by": actorID,
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("ai: update settings: %w", err)
		}

		var updated Settings
		if err := tx.First(&updated, settingsID).Error; err != nil {
			return err
		}
		view = s.view(updated)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// Enabled reports whether the AI feature is on. A missing row is treated as
// enabled=true to preserve the pre-toggle behavior.
func (s *SettingsService) Enabled(ctx context.Context) (bool, error) {
	var settings Settings
	err := s.db.WithContext(ctx).Where("id = ?", settingsID).First(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return settings.Enabled, nil
}

func (s *SettingsService) view(settings Settings) *SettingsView {
	return &SettingsView{
		Enabled:       settings.Enabled,
		KeyConfigured: s.keys.Configured(),
		UpdatedAt:     settings.UpdatedAt.UTC(),
	}
}

// ensureSettings inserts the default settings row when absent.
func ensureSettings(tx *gorm.DB) error {
	settings := Settings{ID: settingsID, Enabled: true, UpdatedAt: time.Now().UTC()}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error; err != nil {
		return fmt.Errorf("ai: ensure settings: %w", err)
	}
	return nil
}
