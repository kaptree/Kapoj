package ai

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SeedProviders 幂等地插入内置 provider。按 code 冲突时不做任何更新，
// 因此不会覆盖管理员对已有 provider 的修改。
func SeedProviders(ctx context.Context, db *gorm.DB) error {
	defs := ProviderDefs()
	providers := make([]Provider, 0, len(defs))
	for _, d := range defs {
		providers = append(providers, Provider{
			Code:         d.Code,
			Name:         d.Name,
			BaseURL:      d.BaseURL,
			DefaultModel: d.DefaultModel,
			IsActive:     true,
		})
	}
	if len(providers) == 0 {
		return nil
	}
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&providers).Error; err != nil {
		return fmt.Errorf("seed ai providers: %w", err)
	}
	return nil
}
