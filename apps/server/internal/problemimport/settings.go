package problemimport

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	DefaultMaxPackageMB = 200
	MinMaxPackageMB     = 1
	MaxMaxPackageMB     = 2048

	MiB                  int64 = 1 << 20
	MinUncompressedBytes int64 = 1 << 30
)

// SettingsView is the read model returned by the admin and import-page APIs.
type SettingsView struct {
	MaxPackageMB         int       `json:"maxPackageMb"`
	MaxZipBytes          int64     `json:"maxZipBytes"`
	MaxUncompressedBytes int64     `json:"maxUncompressedBytes"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

// SettingsInput is the mutable subset of the singleton.
type SettingsInput struct {
	MaxPackageMB int `json:"maxPackageMb"`
}

// SettingsService reads and updates the singleton problem_import_settings row.
type SettingsService struct {
	db *gorm.DB
}

func NewSettingsService(db *gorm.DB) *SettingsService {
	return &SettingsService{db: db}
}

// Get returns the current settings, lazily creating the default row.
func (s *SettingsService) Get(ctx context.Context) (*SettingsView, error) {
	var settings Settings
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSettings(tx); err != nil {
			return err
		}
		return tx.First(&settings, settingsID).Error
	})
	if err != nil {
		return nil, fmt.Errorf("problemimport: load settings: %w", err)
	}
	return settingsView(settings), nil
}

// Update validates and stores the package limit. The audit callback runs in
// the same transaction, so an audit failure rolls back the update.
func (s *SettingsService) Update(
	ctx context.Context,
	actorID int64,
	in SettingsInput,
	audit func(*gorm.DB) error,
) (*SettingsView, error) {
	if in.MaxPackageMB < MinMaxPackageMB || in.MaxPackageMB > MaxMaxPackageMB {
		return nil, httpx.ErrValidation("题包大小上限需为 1–2048 MB")
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
			"max_package_mb": in.MaxPackageMB,
			"updated_by":     actorID,
			"updated_at":     now,
		}).Error; err != nil {
			return fmt.Errorf("problemimport: update settings: %w", err)
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

func ensureSettings(tx *gorm.DB) error {
	now := time.Now().UTC()
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Settings{
		ID:           settingsID,
		MaxPackageMB: DefaultMaxPackageMB,
		UpdatedBy:    0,
		UpdatedAt:    now,
	}).Error
}

func settingsView(settings Settings) *SettingsView {
	maxZip := int64(settings.MaxPackageMB) * MiB
	maxUncompressed := maxZip * 2
	if maxUncompressed < MinUncompressedBytes {
		maxUncompressed = MinUncompressedBytes
	}
	return &SettingsView{
		MaxPackageMB:         settings.MaxPackageMB,
		MaxZipBytes:          maxZip,
		MaxUncompressedBytes: maxUncompressed,
		UpdatedAt:            settings.UpdatedAt,
	}
}
