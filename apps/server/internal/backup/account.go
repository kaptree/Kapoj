package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

type accountTable struct {
	name string
	// where is a fmt pattern receiving the user id once or twice.
	where string
}

var accountTables = []accountTable{
	{name: "submissions", where: "user_id = %d"},
	{name: "coin_transactions", where: "user_id = %d"},
	{name: "user_achievements", where: "user_id = %d"},
	{name: "user_task_records", where: "user_id = %d"},
	{name: "user_login_days", where: "user_id = %d"},
	{name: "user_badges", where: "user_id = %d"},
	{name: "user_titles", where: "user_id = %d"},
	{name: "user_roles", where: "user_id = %d"},
	{name: "user_learning_progress", where: "user_id = %d"},
	{name: "learning_path_purchases", where: "user_id = %d"},
	{name: "learning_path_recommendations", where: "user_id = %d"},
	{name: "user_ai_configs", where: "user_id = %d"},
	{name: "activity_logs", where: "user_id = %d"},
	{name: "shared_items", where: "user_id = %d"},
	{name: "rating_history", where: "user_id = %d"},
	{name: "contest_participants", where: "user_id = %d"},
	{name: "certificates", where: "user_id = %d"},
	{name: "notifications", where: "user_id = %d"},
	{name: "problem_solution_posts", where: "user_id = %d"},
	{name: "problem_solution_replies", where: "user_id = %d"},
	{name: "problem_solution_post_likes", where: "user_id = %d"},
	{name: "problem_solution_unlocks", where: "user_id = %d"},
	{name: "testcase_generations", where: "user_id = %d"},
	{name: "testcase_generation_cases", where: "generation_id IN (SELECT id FROM testcase_generations WHERE user_id = %d)"},
	{name: "user_evaluations", where: "author_user_id = %d"},
	{name: "user_follows", where: "follower_id = %d"},
}

func (s *Service) createAccountFile(ctx context.Context, filename string, kind Kind, userID *int64) (*Manifest, error) {
	tmp := filename + ".tmp"
	writer, err := NewWriter(tmp, s.maxEntries)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			writer.Abort()
		}
	}()
	hash, err := s.database.SchemaHash(ctx)
	if err != nil {
		return nil, err
	}
	manifest := &Manifest{Kind: kind, CreatedAt: time.Now().UTC(), SchemaHash: hash}
	for _, table := range accountTables {
		manifest.Tables = append(manifest.Tables, table.name)
	}
	manifest.Tables = append([]string{"users"}, manifest.Tables...)

	var ids []int64
	if kind == KindAccount {
		if userID == nil || *userID <= 0 {
			return nil, httpx.ErrValidation("账号备份需要指定用户")
		}
		ids = []int64{*userID}
	} else {
		conn, err := s.database.connect(ctx)
		if err != nil {
			return nil, err
		}
		rows, err := conn.Query(ctx, `SELECT id FROM users ORDER BY id`)
		if err != nil {
			_ = conn.Close(ctx)
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				_ = conn.Close(ctx)
				return nil, err
			}
			ids = append(ids, id)
		}
		rows.Close()
		_ = conn.Close(ctx)
	}
	manifest.Users = append([]int64(nil), ids...)

	for _, uid := range ids {
		prefix := "account"
		if kind == KindAccounts {
			prefix = fmt.Sprintf("accounts/%d", uid)
		}
		if err := s.exportAccount(ctx, writer, prefix, uid); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(*manifest); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, filename); err != nil {
		return nil, err
	}
	ok = true
	return manifest, nil
}

func (s *Service) exportAccount(ctx context.Context, archive *Writer, prefix string, userID int64) error {
	conn, err := s.database.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// user.json is for humans/tools; user.csv is the restore source.
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT row_to_json(u)::text FROM users u WHERE id = $1`, userID).
		Scan(&raw); err != nil {
		return err
	}
	w, err := archive.Create(prefix + "/user.json")
	if err != nil {
		return err
	}
	var pretty any
	if json.Unmarshal(raw, &pretty) == nil {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(pretty); err != nil {
			return err
		}
	} else if _, err := w.Write(raw); err != nil {
		return err
	}

	if err := copyTable(ctx, tx, archive, prefix+"/user.csv",
		fmt.Sprintf("SELECT * FROM users WHERE id = %d", userID)); err != nil {
		return err
	}
	for _, table := range accountTables {
		where := fmt.Sprintf(table.where, userID)
		query := fmt.Sprintf("SELECT * FROM %s WHERE %s",
			pgx.Identifier{table.name}.Sanitize(), where)
		if err := copyTable(ctx, tx, archive, prefix+"/"+table.name+".csv", query); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.addAccountFiles(ctx, archive, prefix+"/files/", userID)
}

func copyTable(ctx context.Context, tx pgx.Tx, archive *Writer, entry, query string) error {
	w, err := archive.Create(entry)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf(`COPY (%s) TO STDOUT WITH (FORMAT csv, HEADER true)`, query)
	_, err = tx.Conn().PgConn().CopyTo(ctx, w, sql)
	return err
}

func (s *Service) addAccountFiles(ctx context.Context, archive *Writer, prefix string, userID int64) error {
	conn, err := s.database.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	keys := make(map[string]struct{})
	var avatar string
	if err := conn.QueryRow(ctx, `SELECT avatar_key FROM users WHERE id = $1`, userID).Scan(&avatar); err == nil && avatar != "" {
		keys[avatar] = struct{}{}
	}
	rows, err := conn.Query(ctx, `
		SELECT image_key FROM certificates WHERE user_id = $1 AND image_key <> ''
		UNION
		SELECT input_file FROM testcase_generation_cases AS c
			JOIN testcase_generations AS g ON g.id = c.generation_id
			WHERE g.user_id = $1 AND input_file <> ''
		UNION
		SELECT output_file FROM testcase_generation_cases AS c
			JOIN testcase_generations AS g ON g.id = c.generation_id
			WHERE g.user_id = $1 AND output_file <> ''`, userID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		keys[key] = struct{}{}
	}
	rows.Close()
	for key := range keys {
		if err := addStoreFile(ctx, archive, s.files, prefix+key, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) restoreAccount(ctx context.Context, zipPath string, targetUserID int64) (err error) {
	reader, err := OpenReader(zipPath, s.maxEntries, s.maxUncompressed)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	if reader.Manifest.Kind != KindAccount && reader.Manifest.Kind != KindAccounts {
		return httpx.ErrValidation("不是账号备份包")
	}
	prefix := "account"
	if reader.Manifest.Kind == KindAccounts {
		prefix = fmt.Sprintf("accounts/%d", targetUserID)
	}
	found := false
	for _, id := range reader.Manifest.Users {
		if id == targetUserID {
			found = true
			break
		}
	}
	if !found || !reader.Has(prefix+"/user.csv") {
		return httpx.ErrValidation("备份包中不存在该用户")
	}
	currentHash, err := s.database.SchemaHash(ctx)
	if err != nil {
		return err
	}
	if currentHash != reader.Manifest.SchemaHash {
		return ErrSchemaMismatch
	}

	tempFiles, err := os.MkdirTemp("", "kapoj-restore-account-*")
	if err != nil {
		return err
	}
	keepSnapshot := false
	defer func() {
		if !keepSnapshot {
			_ = os.RemoveAll(tempFiles)
		}
	}()
	filesDir := filepath.Join(tempFiles, "files")
	if err := extractStorage(reader, prefix+"/files/", filesDir); err != nil {
		return err
	}
	keys, err := listDirKeys(filesDir)
	if err != nil {
		return err
	}
	previousDir := filepath.Join(tempFiles, "previous")
	existed, err := snapshotStoreKeys(ctx, s.files, keys, previousDir)
	if err != nil {
		return err
	}
	if copyErr := copyDirToStore(ctx, s.files, filesDir); copyErr != nil {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		rollbackErr := restoreStoreKeys(rollbackCtx, s.files, keys, previousDir, existed)
		cancel()
		if rollbackErr != nil {
			keepSnapshot = true
			return errors.Join(
				fmt.Errorf("恢复文件失败: %w", copyErr),
				fmt.Errorf("文件回滚失败，快照保留在 %s: %w", previousDir, rollbackErr),
			)
		}
		return fmt.Errorf("恢复文件失败: %w", copyErr)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		defer cancel()
		if rollbackErr := restoreStoreKeys(rollbackCtx, s.files, keys, previousDir, existed); rollbackErr != nil {
			keepSnapshot = true
			slog.Warn("backup: account file rollback failed",
				"err", rollbackErr, "snapshot", previousDir,
			)
			err = errors.Join(err,
				fmt.Errorf("账号文件回滚失败，快照保留在 %s: %w", previousDir, rollbackErr),
			)
		}
	}()

	conn, err := s.database.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		return fmt.Errorf("database role cannot disable foreign key checks: %w", err)
	}
	uid := strconv.FormatInt(targetUserID, 10)
	if err := restoreUserRow(ctx, reader, tx, prefix+"/user.csv", targetUserID); err != nil {
		return err
	}

	// Remove the user's personal rows. Foreign key triggers are disabled, so
	// restoring the same primary keys will not cascade into other users.
	deleteStatements := []string{
		fmt.Sprintf(`DELETE FROM testcase_generation_cases WHERE generation_id IN (SELECT id FROM testcase_generations WHERE user_id = %s)`, uid),
		fmt.Sprintf(`DELETE FROM testcase_generations WHERE user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM problem_solution_replies WHERE user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM problem_solution_post_likes WHERE user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM problem_solution_unlocks WHERE user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM problem_solution_posts WHERE user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM notifications WHERE user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM certificates WHERE user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM user_evaluations WHERE author_user_id = %s`, uid),
		fmt.Sprintf(`DELETE FROM user_follows WHERE follower_id = %s`, uid),
	}
	for _, table := range []string{
		"submissions", "coin_transactions", "user_achievements", "user_task_records",
		"user_login_days", "user_badges", "user_titles", "user_roles",
		"user_learning_progress", "learning_path_purchases", "learning_path_recommendations",
		"user_ai_configs", "activity_logs", "shared_items", "rating_history",
		"contest_participants",
	} {
		deleteStatements = append(deleteStatements,
			fmt.Sprintf(`DELETE FROM %s WHERE user_id = %s`, pgx.Identifier{table}.Sanitize(), uid))
	}
	for _, stmt := range deleteStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	for _, table := range accountTables {
		entry := prefix + "/" + table.name + ".csv"
		rc, err := reader.Open(entry)
		if err != nil {
			return err
		}
		sql := fmt.Sprintf(`COPY %s FROM STDIN WITH (FORMAT csv, HEADER true)`,
			pgx.Identifier{table.name}.Sanitize())
		_, copyErr := tx.Conn().PgConn().CopyFrom(ctx, rc, sql)
		_ = rc.Close()
		if copyErr != nil {
			return fmt.Errorf("restore account table %s: %w", table.name, copyErr)
		}
	}
	if err := resetSequences(ctx, tx); err != nil {
		return err
	}
	if err := recalculateAfterAccountRestore(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

func restoreUserRow(ctx context.Context, reader *Reader, tx pgx.Tx, entry string, userID int64) error {
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE _restore_users (LIKE users INCLUDING DEFAULTS) ON COMMIT DROP`); err != nil {
		return err
	}
	rc, err := reader.Open(entry)
	if err != nil {
		return err
	}
	_, copyErr := tx.Conn().PgConn().CopyFrom(ctx, rc,
		`COPY _restore_users FROM STDIN WITH (FORMAT csv, HEADER true)`)
	_ = rc.Close()
	if copyErr != nil {
		return copyErr
	}
	var conflict int64
	err = tx.QueryRow(ctx, `
		SELECT u.id FROM users AS u
		JOIN _restore_users AS r ON (u.username = r.username OR u.email = r.email)
		WHERE u.id <> r.id LIMIT 1`).Scan(&conflict)
	if err == nil {
		return httpx.ErrValidation(fmt.Sprintf("用户名或邮箱已被用户 %d 占用", conflict))
	}
	if err != pgx.ErrNoRows {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'users'
		ORDER BY ordinal_position`)
	if err != nil {
		return err
	}
	var columns []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			rows.Close()
			return err
		}
		columns = append(columns, col)
	}
	rows.Close()
	if len(columns) == 0 {
		return errors.New("users table has no columns")
	}
	updates := make([]string, 0, len(columns)-1)
	for _, col := range columns {
		if col == "id" {
			continue
		}
		updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s",
			pgx.Identifier{col}.Sanitize(), pgx.Identifier{col}.Sanitize()))
	}
	if len(updates) == 0 {
		return errors.New("users table has no updatable columns")
	}
	sql := fmt.Sprintf(`INSERT INTO users SELECT * FROM _restore_users
		ON CONFLICT (id) DO UPDATE SET %s`, strings.Join(updates, ", "))
	_, err = tx.Exec(ctx, sql)
	return err
}

func recalculateAfterAccountRestore(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `
		UPDATE problems AS p
		SET submit_count = (SELECT COUNT(*) FROM submissions AS s WHERE s.problem_id = p.id),
		    accept_count = (SELECT COUNT(*) FROM submissions AS s WHERE s.problem_id = p.id AND s.status = 'accepted')`); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE learning_paths AS p
		SET participant_count = (
			SELECT COUNT(*) FROM (
				SELECT user_id FROM learning_path_purchases WHERE path_id = p.id
				UNION
				SELECT user_id FROM user_learning_progress WHERE path_id = p.id
			) AS participants
		)`)
	return err
}
