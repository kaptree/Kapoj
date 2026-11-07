package rating

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// settingsID is the fixed primary key of the singleton rating_settings row.
const settingsID int64 = 1

// Default settings values, chosen to match the design document.
const (
	defaultDecayThreshold     = 2400
	defaultDecayInactiveDays  = 60
	defaultDecayPercent       = 3
	defaultDecayMax           = 100
	defaultSeasonMonths       = 6
	defaultSeasonResetPercent = 30
)

// SettingsInput is the full set of mutable rating settings. SeasonAnchor is an
// admin-set YYYY-MM-DD date; the current season is read-only.
type SettingsInput struct {
	DecayEnabled       bool   `json:"decayEnabled"`
	DecayThreshold     int    `json:"decayThreshold"`
	DecayInactiveDays  int    `json:"decayInactiveDays"`
	DecayPercent       int    `json:"decayPercent"`
	DecayMax           int    `json:"decayMax"`
	SeasonEnabled      bool   `json:"seasonEnabled"`
	SeasonMonths       int    `json:"seasonMonths"`
	SeasonResetPercent int    `json:"seasonResetPercent"`
	SeasonAnchor       string `json:"seasonAnchor"`
}

// SeasonView describes a season's public identity and schedule.
type SeasonView struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	StartsAt    time.Time `json:"startsAt"`
	EndsAt      time.Time `json:"endsAt"`
	NextResetAt time.Time `json:"nextResetAt"`
}

// SettingsView is the read model returned by the admin settings API.
type SettingsView struct {
	DecayEnabled       bool       `json:"decayEnabled"`
	DecayThreshold     int        `json:"decayThreshold"`
	DecayInactiveDays  int        `json:"decayInactiveDays"`
	DecayPercent       int        `json:"decayPercent"`
	DecayMax           int        `json:"decayMax"`
	SeasonEnabled      bool       `json:"seasonEnabled"`
	SeasonMonths       int        `json:"seasonMonths"`
	SeasonResetPercent int        `json:"seasonResetPercent"`
	SeasonAnchor       string     `json:"seasonAnchor"`
	CurrentSeason      SeasonView `json:"currentSeason"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// Settings returns the current settings, creating the default row lazily when
// the table is empty.
func (s *Service) Settings(ctx context.Context) (*SettingsView, error) {
	now := s.now()
	var settings RatingSettings
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ensureSettingsTx(tx, now); err != nil {
			return err
		}
		return tx.First(&settings, settingsID).Error
	})
	if err != nil {
		return nil, err
	}
	return settingsView(settings, now), nil
}

// UpdateSettings validates and stores a full settings update, and audits the
// change. When the anchor or season length changes it pre-marks the new
// previous season's run key as skipped so the change never triggers an
// immediate full reset.
func (s *Service) UpdateSettings(ctx context.Context, actorID int64, in SettingsInput) (*SettingsView, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	anchor, err := parseAnchor(in.SeasonAnchor)
	if err != nil {
		return nil, err
	}
	now := s.now()

	var view *SettingsView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ensureSettingsTx(tx, now); err != nil {
			return err
		}

		var current RatingSettings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&current, settingsID).Error; err != nil {
			return err
		}

		anchorChanged := !current.SeasonAnchor.Equal(anchor)
		monthsChanged := current.SeasonMonths != in.SeasonMonths

		if err := tx.Model(&RatingSettings{}).Where("id = ?", settingsID).Updates(map[string]any{
			"decay_enabled":        in.DecayEnabled,
			"decay_threshold":      in.DecayThreshold,
			"decay_inactive_days":  in.DecayInactiveDays,
			"decay_percent":        in.DecayPercent,
			"decay_max":            in.DecayMax,
			"season_enabled":       in.SeasonEnabled,
			"season_months":        in.SeasonMonths,
			"season_reset_percent": in.SeasonResetPercent,
			"season_anchor":        anchor,
			"updated_by":           actorID,
			"updated_at":           now,
		}).Error; err != nil {
			return fmt.Errorf("rating: update settings: %w", err)
		}

		if anchorChanged || monthsChanged {
			if err := markPreviousSeasonSkipped(tx, anchor, in.SeasonMonths, now); err != nil {
				return err
			}
		}

		var updated RatingSettings
		if err := tx.First(&updated, settingsID).Error; err != nil {
			return err
		}
		view = settingsView(updated, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// CurrentSeason returns the season containing now, creating default settings
// if needed.
func (s *Service) CurrentSeason(ctx context.Context) (SeasonView, error) {
	view, err := s.Settings(ctx)
	if err != nil {
		return SeasonView{}, err
	}
	return view.CurrentSeason, nil
}

// ensureSettingsTx inserts the default settings row when absent, and on first
// creation pre-marks the previous season's run key as skipped so the very first
// deployment never triggers a historical season reset.
func (s *Service) ensureSettingsTx(tx *gorm.DB, now time.Time) error {
	anchor := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	settings := RatingSettings{
		ID:                 settingsID,
		DecayEnabled:       true,
		DecayThreshold:     defaultDecayThreshold,
		DecayInactiveDays:  defaultDecayInactiveDays,
		DecayPercent:       defaultDecayPercent,
		DecayMax:           defaultDecayMax,
		SeasonEnabled:      true,
		SeasonMonths:       defaultSeasonMonths,
		SeasonResetPercent: defaultSeasonResetPercent,
		SeasonAnchor:       anchor,
		UpdatedAt:          now,
	}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings)
	if res.Error != nil {
		return fmt.Errorf("rating: ensure settings: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil
	}
	return markPreviousSeasonSkipped(tx, anchor, defaultSeasonMonths, now)
}

// markPreviousSeasonSkipped writes a processed run marker for the previous
// season under the given anchor/months, unless one already exists.
func markPreviousSeasonSkipped(tx *gorm.DB, anchor time.Time, months int, now time.Time) error {
	prev := seasonView(anchor, months, currentSeasonIndex(anchor, months, now)-1)
	run := RatingRun{
		Key:         "season:" + prev.Key,
		Kind:        HistorySeasonReset,
		ProcessedAt: &now,
		CreatedAt:   now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&run).Error
}

func settingsView(settings RatingSettings, now time.Time) *SettingsView {
	return &SettingsView{
		DecayEnabled:       settings.DecayEnabled,
		DecayThreshold:     settings.DecayThreshold,
		DecayInactiveDays:  settings.DecayInactiveDays,
		DecayPercent:       settings.DecayPercent,
		DecayMax:           settings.DecayMax,
		SeasonEnabled:      settings.SeasonEnabled,
		SeasonMonths:       settings.SeasonMonths,
		SeasonResetPercent: settings.SeasonResetPercent,
		SeasonAnchor:       settings.SeasonAnchor.Format("2006-01-02"),
		CurrentSeason:      seasonView(settings.SeasonAnchor, settings.SeasonMonths, currentSeasonIndex(settings.SeasonAnchor, settings.SeasonMonths, now)),
		UpdatedAt:          settings.UpdatedAt.UTC(),
	}
}

// seasonView builds the season [anchor + index*months, anchor + (index+1)*months).
func seasonView(anchor time.Time, months, index int) SeasonView {
	start := seasonStart(anchor, months, index)
	end := seasonStart(anchor, months, index+1)
	return SeasonView{
		Key:         start.Format("2006-01-02"),
		Name:        start.Format("2006-01-02") + " ~ " + end.AddDate(0, 0, -1).Format("2006-01-02"),
		StartsAt:    start,
		EndsAt:      end,
		NextResetAt: end,
	}
}

func seasonStart(anchor time.Time, months, index int) time.Time {
	return anchor.AddDate(0, months*index, 0)
}

// currentSeasonIndex returns the season index containing now. Anchors are
// always on the first day of a month, so the number of month boundaries crossed
// from the anchor, divided by the season length, is exact.
func currentSeasonIndex(anchor time.Time, months int, now time.Time) int {
	if now.Before(anchor) {
		return 0
	}
	elapsed := (now.Year()-anchor.Year())*12 + int(now.Month()) - int(anchor.Month())
	return elapsed / months
}

func parseAnchor(value string) (time.Time, error) {
	anchor, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, httpx.ErrValidation("赛季锚点必须是 YYYY-MM-DD 格式")
	}
	return anchor, nil
}

func (in SettingsInput) validate() error {
	if in.DecayThreshold < 1500 || in.DecayThreshold > 4000 {
		return httpx.ErrValidation("衰减阈值必须在 1500 到 4000 之间")
	}
	if in.DecayInactiveDays < 1 || in.DecayInactiveDays > 365 {
		return httpx.ErrValidation("未活跃天数必须在 1 到 365 之间")
	}
	if in.DecayPercent < 1 || in.DecayPercent > 100 {
		return httpx.ErrValidation("衰减比例必须在 1 到 100 之间")
	}
	if in.DecayMax < 1 || in.DecayMax > 1000 {
		return httpx.ErrValidation("单次衰减上限必须在 1 到 1000 之间")
	}
	if in.SeasonMonths < 1 || in.SeasonMonths > 24 {
		return httpx.ErrValidation("赛季月数必须在 1 到 24 之间")
	}
	if in.SeasonResetPercent < 0 || in.SeasonResetPercent > 100 {
		return httpx.ErrValidation("重置比例必须在 0 到 100 之间")
	}
	if _, err := parseAnchor(in.SeasonAnchor); err != nil {
		return err
	}
	return nil
}
