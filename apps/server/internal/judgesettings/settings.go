package judgesettings

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// SettingsView is the admin API read model.
type SettingsView struct {
	OutputFloorBytes     int64     `json:"outputFloorBytes"`
	OutputSlackBytes     int64     `json:"outputSlackBytes"`
	MaxOutputBytes       int64     `json:"maxOutputBytes"`
	MaxInputFileBytes    int64     `json:"maxInputFileBytes"`
	MaxOutputFileBytes   int64     `json:"maxOutputFileBytes"`
	MaxBatchTotalBytes   int64     `json:"maxBatchTotalBytes"`
	MaxProblemTotalBytes int64     `json:"maxProblemTotalBytes"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

// SettingsInput is the mutable subset of the singleton.
type SettingsInput struct {
	OutputFloorBytes     int64 `json:"outputFloorBytes"`
	OutputSlackBytes     int64 `json:"outputSlackBytes"`
	MaxOutputBytes       int64 `json:"maxOutputBytes"`
	MaxInputFileBytes    int64 `json:"maxInputFileBytes"`
	MaxOutputFileBytes   int64 `json:"maxOutputFileBytes"`
	MaxBatchTotalBytes   int64 `json:"maxBatchTotalBytes"`
	MaxProblemTotalBytes int64 `json:"maxProblemTotalBytes"`
}

// TestcaseLimits is the upload-limit subset consumed by the problem service.
type TestcaseLimits struct {
	MaxInputBytes        int64
	MaxOutputBytes       int64
	MaxBatchTotalBytes   int64
	MaxProblemTotalBytes int64
}

// SettingsService reads and updates the singleton judge_settings row.
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
		return nil, fmt.Errorf("judgesettings: load settings: %w", err)
	}
	return settingsView(settings), nil
}

// Update validates and stores all settings. The audit callback runs in the same
// transaction, so an audit failure rolls the whole update back.
func (s *SettingsService) Update(
	ctx context.Context,
	actorID int64,
	in SettingsInput,
	audit func(*gorm.DB) error,
) (*SettingsView, error) {
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
			"output_floor_bytes":      in.OutputFloorBytes,
			"output_slack_bytes":      in.OutputSlackBytes,
			"max_output_bytes":        in.MaxOutputBytes,
			"max_input_file_bytes":    in.MaxInputFileBytes,
			"max_output_file_bytes":   in.MaxOutputFileBytes,
			"max_batch_total_bytes":   in.MaxBatchTotalBytes,
			"max_problem_total_bytes": in.MaxProblemTotalBytes,
			"updated_by":              actorID,
			"updated_at":              now,
		}).Error; err != nil {
			return fmt.Errorf("judgesettings: update settings: %w", err)
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

// TestcaseLimits returns the four upload limits used by the problem service.
func (s *SettingsService) TestcaseLimits(ctx context.Context) (TestcaseLimits, error) {
	view, err := s.Get(ctx)
	if err != nil {
		return TestcaseLimits{}, err
	}
	return TestcaseLimits{
		MaxInputBytes:        view.MaxInputFileBytes,
		MaxOutputBytes:       view.MaxOutputFileBytes,
		MaxBatchTotalBytes:   view.MaxBatchTotalBytes,
		MaxProblemTotalBytes: view.MaxProblemTotalBytes,
	}, nil
}

func ensureSettings(tx *gorm.DB) error {
	now := time.Now().UTC()
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Settings{
		ID:                   settingsID,
		OutputFloorBytes:     DefaultOutputFloorBytes,
		OutputSlackBytes:     DefaultOutputSlackBytes,
		MaxOutputBytes:       DefaultMaxOutputBytes,
		MaxInputFileBytes:    DefaultMaxInputFileBytes,
		MaxOutputFileBytes:   DefaultMaxOutputFileBytes,
		MaxBatchTotalBytes:   DefaultMaxBatchTotalBytes,
		MaxProblemTotalBytes: DefaultMaxProblemTotalBytes,
		UpdatedBy:            0,
		UpdatedAt:            now,
	}).Error
}

func settingsView(settings Settings) *SettingsView {
	return &SettingsView{
		OutputFloorBytes:     settings.OutputFloorBytes,
		OutputSlackBytes:     settings.OutputSlackBytes,
		MaxOutputBytes:       settings.MaxOutputBytes,
		MaxInputFileBytes:    settings.MaxInputFileBytes,
		MaxOutputFileBytes:   settings.MaxOutputFileBytes,
		MaxBatchTotalBytes:   settings.MaxBatchTotalBytes,
		MaxProblemTotalBytes: settings.MaxProblemTotalBytes,
		UpdatedAt:            settings.UpdatedAt.UTC(),
	}
}

func (in SettingsInput) validate() error {
	if in.OutputFloorBytes < MinOutputFloorBytes || in.OutputFloorBytes > MaxOutputFloorBytes {
		return httpx.ErrValidation("输出下限需为 1 KiB–64 MiB")
	}
	if in.OutputSlackBytes < MinOutputSlackBytes || in.OutputSlackBytes > MaxOutputSlackBytes {
		return httpx.ErrValidation("输出余量需为 0–64 MiB")
	}
	if in.MaxOutputBytes < MinMaxOutputBytes || in.MaxOutputBytes > MaxMaxOutputBytes {
		return httpx.ErrValidation("最大输出需为 1–64 MiB")
	}
	if in.MaxOutputBytes < in.OutputFloorBytes {
		return httpx.ErrValidation("最大输出不能小于输出下限")
	}
	if in.MaxOutputBytes < in.OutputSlackBytes {
		return httpx.ErrValidation("最大输出不能小于输出余量")
	}
	if in.MaxInputFileBytes < MinMaxInputFileBytes || in.MaxInputFileBytes > MaxMaxInputFileBytes {
		return httpx.ErrValidation("单个输入文件上限需为 1–256 MiB")
	}
	if in.MaxOutputFileBytes < MinMaxOutputFileBytes || in.MaxOutputFileBytes > MaxMaxOutputFileBytes {
		return httpx.ErrValidation("单个输出文件上限需为 1–64 MiB")
	}
	if in.MaxOutputFileBytes > in.MaxOutputBytes {
		return httpx.ErrValidation("单个输出文件上限不能超过最大输出")
	}
	if in.MaxBatchTotalBytes < MinMaxBatchTotalBytes || in.MaxBatchTotalBytes > MaxMaxBatchTotalBytes {
		return httpx.ErrValidation("批量上传总量需为 1–2048 MiB")
	}
	if in.MaxProblemTotalBytes < MinMaxProblemTotalBytes || in.MaxProblemTotalBytes > MaxMaxProblemTotalBytes {
		return httpx.ErrValidation("单题导入总量需为 1–2048 MiB")
	}
	return nil
}
