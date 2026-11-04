package gamification

import "gorm.io/gorm"

// Repository 提供游戏化模块的数据库访问。
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}
