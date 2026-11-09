package site

import (
	"context"
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

// Repository stores the singleton site settings and the visit counter.
type Repository interface {
	Get(ctx context.Context) (*Settings, error)
	Update(ctx context.Context, settings *Settings) error
	IncrementVisit(ctx context.Context) (int64, error)
	VisitCount(ctx context.Context) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewGormRepository returns a GORM-backed site repository.
func NewGormRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// Get returns the singleton settings row, creating it with defaults on first
// use. A historical row with an empty home_json document is backfilled with the
// canonical default content so the API never exposes `{}`.
func (r *gormRepository) Get(ctx context.Context) (*Settings, error) {
	var settings Settings
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSettings(tx); err != nil {
			return err
		}
		if err := tx.First(&settings, settingsID).Error; err != nil {
			return err
		}
		if reflect.DeepEqual(settings.Home.Data, HomeContent{}) {
			settings.Home = dbtypes.JSON[HomeContent]{Data: DefaultHomeContent()}
			// UpdateColumn skips UpdatedAt auto-updating: backfilling the
			// document is not an administrator edit and must not invalidate
			// image cache versions.
			return tx.Model(&Settings{}).Where("id = ?", settingsID).
				UpdateColumn("home_json", settings.Home).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

// Update locks the singleton row and writes every mutable column.
func (r *gormRepository) Update(ctx context.Context, settings *Settings) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSettings(tx); err != nil {
			return err
		}
		var current Settings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&current, settingsID).Error; err != nil {
			return err
		}
		settings.ID = settingsID
		settings.UpdatedAt = time.Now().UTC()
		return tx.Model(&Settings{}).Where("id = ?", settingsID).Updates(map[string]any{
			"logo_key":             settings.LogoKey,
			"logo_content_type":    settings.LogoContentType,
			"favicon_key":          settings.FaviconKey,
			"favicon_content_type": settings.FaviconContentType,
			"started_at":           settings.StartedAt,
			"footer_icp":           settings.FooterICP,
			"footer_police":        settings.FooterPolice,
			"footer_police_url":    settings.FooterPoliceURL,
			"footer_copyright":     settings.FooterCopyright,
			"footer_text":          settings.FooterText,
			"show_visits":          settings.ShowVisits,
			"home_json":            settings.Home,
			"updated_by":           settings.UpdatedBy,
			"updated_at":           settings.UpdatedAt,
		}).Error
	})
}

// IncrementVisit atomically increments the singleton counter and returns the
// new value.
func (r *gormRepository) IncrementVisit(ctx context.Context) (int64, error) {
	var count int64
	row := r.db.WithContext(ctx).Raw(`
		INSERT INTO site_visit_counters (id, count, updated_at)
		VALUES (1, 1, now())
		ON CONFLICT (id) DO UPDATE
		SET count = site_visit_counters.count + 1, updated_at = now()
		RETURNING count`).Row()
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// VisitCount returns the current counter, creating the singleton row lazily.
func (r *gormRepository) VisitCount(ctx context.Context) (int64, error) {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO site_visit_counters (id, count, updated_at)
		VALUES (1, 0, now())
		ON CONFLICT (id) DO NOTHING`).Error
	if err != nil {
		return 0, err
	}
	var counter VisitCounter
	if err := r.db.WithContext(ctx).First(&counter, settingsID).Error; err != nil {
		return 0, err
	}
	return counter.Count, nil
}

// ensureSettings inserts the default singleton row when it is absent.
func ensureSettings(tx *gorm.DB) error {
	now := time.Now().UTC()
	settings := Settings{
		ID:              settingsID,
		StartedAt:       now,
		FooterCopyright: DefaultFooterInput().Copyright,
		ShowVisits:      true,
		Home:            dbtypes.JSON[HomeContent]{Data: DefaultHomeContent()},
		UpdatedAt:       now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error
}
