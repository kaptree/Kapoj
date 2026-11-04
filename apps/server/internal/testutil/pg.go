package testutil

import (
	"os"
	"strings"
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
	var tables []string
	if err := g.Raw("SELECT quote_ident(tablename) FROM pg_tables WHERE schemaname = 'public'").Scan(&tables).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) > 0 {
		if err := g.Exec("TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE").Error; err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}
	sqlDB, _ := g.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return g
}
