package site

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

// settingsID is the fixed primary key of the singleton site_settings row.
const settingsID int16 = 1

// Settings is the singleton site configuration row. Branding and footer fields
// are plain columns; the editable home page is stored as a JSONB document so
// its structure can evolve without adding dozens of columns.
type Settings struct {
	ID                 int16                     `gorm:"primaryKey"`
	LogoKey            string                    `gorm:"column:logo_key;size:255;not null;default:''"`
	LogoContentType    string                    `gorm:"column:logo_content_type;size:100;not null;default:''"`
	FaviconKey         string                    `gorm:"column:favicon_key;size:255;not null;default:''"`
	FaviconContentType string                    `gorm:"column:favicon_content_type;size:100;not null;default:''"`
	StartedAt          time.Time                 `gorm:"column:started_at;type:timestamptz;not null"`
	FooterICP          string                    `gorm:"column:footer_icp;size:100;not null;default:''"`
	FooterPolice       string                    `gorm:"column:footer_police;size:100;not null;default:''"`
	FooterPoliceURL    string                    `gorm:"column:footer_police_url;size:500;not null;default:''"`
	FooterCopyright    string                    `gorm:"column:footer_copyright;size:200;not null;default:''"`
	FooterText         string                    `gorm:"column:footer_text;type:text;not null;default:''"`
	ShowVisits         bool                      `gorm:"column:show_visits;not null;default:true"`
	Home               dbtypes.JSON[HomeContent] `gorm:"column:home_json;type:jsonb;not null;default:'{}'"`
	UpdatedBy          int64                     `gorm:"column:updated_by;not null;default:0"`
	UpdatedAt          time.Time                 `gorm:"type:timestamptz;not null"`
}

// TableName keeps the singleton table name from the design document.
func (Settings) TableName() string { return "site_settings" }

// VisitCounter is the singleton total-visit counter row.
type VisitCounter struct {
	ID        int16     `gorm:"primaryKey"`
	Count     int64     `gorm:"not null;default:0"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the singleton table name from the design document.
func (VisitCounter) TableName() string { return "site_visit_counters" }
