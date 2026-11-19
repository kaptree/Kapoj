// Package migrations embeds the versioned SQL migrations in the server binary.
package migrations

import "embed"

// FS contains the .up.sql/.down.sql files applied by golang-migrate.
//
//go:embed *.sql
var FS embed.FS
