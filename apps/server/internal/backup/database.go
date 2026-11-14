package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrSchemaMismatch = errors.New("backup schema does not match the current database")

// Backup metadata tables are preserved across a platform restore so the
// current package/restore history remains downloadable.
var backupMetadataTables = map[string]bool{
	"backup_packages":     true,
	"backup_restore_jobs": true,
}

type Database struct{ dsn string }

func NewDatabase(dsn string) *Database { return &Database{dsn: dsn} }

func (d *Database) connect(ctx context.Context) (*pgx.Conn, error) {
	return pgx.Connect(ctx, d.dsn)
}

type schemaInfo struct {
	version string
	hash    string
	tables  []string
	columns []string
}

func (d *Database) schema(ctx context.Context, conn *pgx.Conn) (*schemaInfo, error) {
	var version string
	if err := conn.QueryRow(ctx, `SHOW server_version`).Scan(&version); err != nil {
		return nil, err
	}
	tables := make([]string, 0)
	tableRows, err := conn.Query(ctx, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	for tableRows.Next() {
		var table string
		if err := tableRows.Scan(&table); err != nil {
			tableRows.Close()
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := tableRows.Err(); err != nil {
		tableRows.Close()
		return nil, err
	}
	tableRows.Close()

	definitions := make([]string, 0)
	colRows, err := conn.Query(ctx, `
		SELECT table_name || ':' || column_name || ':' || data_type || ':' || is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		return nil, err
	}
	for colRows.Next() {
		var definition string
		if err := colRows.Scan(&definition); err != nil {
			colRows.Close()
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	if err := colRows.Err(); err != nil {
		colRows.Close()
		return nil, err
	}
	colRows.Close()
	sum := sha256.Sum256([]byte(strings.Join(definitions, ";")))
	return &schemaInfo{
		version: version,
		hash:    hex.EncodeToString(sum[:]),
		tables:  tables,
		columns: definitions,
	}, nil
}

// SchemaHash returns the current public schema fingerprint.
func (d *Database) SchemaHash(ctx context.Context) (string, error) {
	conn, err := d.connect(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close(ctx) }()
	info, err := d.schema(ctx, conn)
	if err != nil {
		return "", err
	}
	return info.hash, nil
}

// Dump streams every public table except backup metadata into database/*.csv.
func (d *Database) Dump(ctx context.Context, archive *Writer, kind Kind) (*Manifest, error) {
	conn, err := d.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close(ctx) }()
	info, err := d.schema(ctx, conn)
	if err != nil {
		return nil, err
	}
	manifest := &Manifest{
		Kind: Kind(kind), CreatedAt: time.Now().UTC(),
		SchemaHash: info.hash, DatabaseVersion: info.version,
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, table := range info.tables {
		if backupMetadataTables[table] {
			continue
		}
		writer, err := archive.Create("database/" + table + ".csv")
		if err != nil {
			return nil, err
		}
		sql := fmt.Sprintf(`COPY (SELECT * FROM %s) TO STDOUT WITH (FORMAT csv, HEADER true)`,
			pgx.Identifier{table}.Sanitize())
		if _, err := tx.Conn().PgConn().CopyTo(ctx, writer, sql); err != nil {
			return nil, fmt.Errorf("dump table %s: %w", table, err)
		}
		manifest.Tables = append(manifest.Tables, table)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return manifest, nil
}

// Restore replaces all current business tables from a validated backup.
func (d *Database) Restore(ctx context.Context, reader *Reader, manifest Manifest) error {
	conn, err := d.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	info, err := d.schema(ctx, conn)
	if err != nil {
		return err
	}
	if info.hash != manifest.SchemaHash {
		return ErrSchemaMismatch
	}
	expected := make([]string, 0, len(info.tables))
	for _, table := range info.tables {
		if !backupMetadataTables[table] {
			expected = append(expected, table)
		}
	}
	sort.Strings(expected)
	got := append([]string(nil), manifest.Tables...)
	sort.Strings(got)
	if strings.Join(expected, ",") != strings.Join(got, ",") {
		return ErrSchemaMismatch
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		return fmt.Errorf("database role cannot disable foreign key checks: %w", err)
	}
	truncate := make([]string, 0, len(info.tables))
	for _, table := range info.tables {
		if !backupMetadataTables[table] {
			truncate = append(truncate, pgx.Identifier{table}.Sanitize())
		}
	}
	if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(truncate, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		return err
	}
	for _, table := range manifest.Tables {
		rc, err := reader.Open("database/" + table + ".csv")
		if err != nil {
			return err
		}
		sql := fmt.Sprintf(`COPY %s FROM STDIN WITH (FORMAT csv, HEADER true)`,
			pgx.Identifier{table}.Sanitize())
		_, copyErr := tx.Conn().PgConn().CopyFrom(ctx, rc, sql)
		_ = rc.Close()
		if copyErr != nil {
			return fmt.Errorf("restore table %s: %w", table, copyErr)
		}
	}
	if err := resetSequences(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func resetSequences(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND (is_identity = 'YES' OR column_default LIKE 'nextval(%')
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type seqCol struct{ table, column string }
	var cols []seqCol
	for rows.Next() {
		var c seqCol
		if err := rows.Scan(&c.table, &c.column); err != nil {
			return err
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range cols {
		var sequence *string
		if err := tx.QueryRow(ctx,
			`SELECT pg_get_serial_sequence($1, $2)`, c.table, c.column).Scan(&sequence); err != nil {
			return err
		}
		if sequence == nil || *sequence == "" {
			continue
		}
		sql := fmt.Sprintf(
			`SELECT setval($1, COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false)`,
			pgx.Identifier{c.column}.Sanitize(), pgx.Identifier{c.table}.Sanitize())
		if _, err := tx.Exec(ctx, sql, *sequence); err != nil {
			return err
		}
	}
	return nil
}

// CopyQuery writes a filtered table export used by account backups.
func (d *Database) CopyQuery(ctx context.Context, writer io.Writer, query string) error {
	conn, err := d.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.PgConn().CopyTo(ctx, writer, query)
	return err
}
