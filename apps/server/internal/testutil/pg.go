package testutil

import (
	"os"
	"testing"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/db"
)

// PG connects to KAPOJ_TEST_DATABASE_URL, migrates it and empties all tables.
// The test is skipped when the variable is not set.
func PG(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KAPOJ_TEST_DATABASE_URL not set; skipping database test")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.Migrate(g); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	if err := g.Exec("TRUNCATE users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	sqlDB, _ := g.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return g
}
