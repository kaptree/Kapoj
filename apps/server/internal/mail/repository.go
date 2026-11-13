package mail

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository stores the singleton mail settings row.
type Repository interface {
	Get(ctx context.Context) (*Settings, error)
	Update(ctx context.Context, settings *Settings) error
}

type gormRepository struct{ db *gorm.DB }

// NewGormRepository returns a GORM-backed mail settings repository.
func NewGormRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Get(ctx context.Context) (*Settings, error) {
	var settings Settings
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSettings(tx); err != nil {
			return err
		}
		return tx.First(&settings, settingsID).Error
	})
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

func (r *gormRepository) Update(ctx context.Context, settings *Settings) error {
	settings.ID = settingsID
	if settings.UpdatedAt.IsZero() {
		settings.UpdatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Save(settings).Error
}

func ensureSettings(tx *gorm.DB) error {
	now := time.Now().UTC()
	settings := Settings{
		ID:                       settingsID,
		Provider:                 ProviderQQ,
		SMTPHost:                 "smtp.qq.com",
		SMTPPort:                 465,
		SMTPTLS:                  TLSSSL,
		FromName:                 "kapOJ",
		SendingEnabled:           false,
		RequireEmailVerification: true,
		UpdatedAt:                now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error
}

// memoryRepository is the in-memory test double for Repository.
type memoryRepository struct {
	mu       sync.Mutex
	settings Settings
}

// NewMemoryRepository returns an in-memory mail settings repository.
func NewMemoryRepository() Repository {
	now := time.Now().UTC()
	return &memoryRepository{settings: Settings{
		ID:                       settingsID,
		Provider:                 ProviderQQ,
		SMTPHost:                 "smtp.qq.com",
		SMTPPort:                 465,
		SMTPTLS:                  TLSSSL,
		FromName:                 "kapOJ",
		RequireEmailVerification: true,
		UpdatedAt:                now,
	}}
}

func (r *memoryRepository) Get(_ context.Context) (*Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := r.settings
	return &copy, nil
}

func (r *memoryRepository) Update(_ context.Context, settings *Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *settings
	copy.ID = settingsID
	if copy.UpdatedAt.IsZero() {
		copy.UpdatedAt = time.Now().UTC()
	}
	r.settings = copy
	return nil
}
