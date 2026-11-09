// Package testutil provides integration-test fixtures for the judge.
package testutil

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// PG connects to KAPOJ_TEST_DATABASE_URL and empties the judge-related tables.
// The test is skipped when the variable is not set.
func PG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KAPOJ_TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	t.Cleanup(pool.Close)

	mustExec(t, pool, `
CREATE TABLE IF NOT EXISTS users (
  id bigint PRIMARY KEY,
  username varchar(32) NOT NULL DEFAULT '',
  nickname varchar(64) NOT NULL DEFAULT '',
  email varchar(254) NOT NULL DEFAULT '',
  password_hash varchar(100) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
)`)
	mustExec(t, pool, `
CREATE TABLE IF NOT EXISTS problems (
  id bigserial PRIMARY KEY,
  title varchar(200) NOT NULL DEFAULT '',
  slug varchar(63) NOT NULL DEFAULT '',
  statement_md text NOT NULL DEFAULT '',
  type varchar(20) NOT NULL DEFAULT 'traditional',
  difficulty int NOT NULL DEFAULT 1,
  time_limit_ms int NOT NULL DEFAULT 1000,
  memory_limit_mb int NOT NULL DEFAULT 256,
  checker_path varchar(255) NOT NULL DEFAULT '',
  answers_json jsonb NOT NULL DEFAULT '[]',
  is_public boolean NOT NULL DEFAULT false,
  submit_count int NOT NULL DEFAULT 0,
  accept_count int NOT NULL DEFAULT 0,
  created_by bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
)`)
	mustExec(t, pool, `
CREATE TABLE IF NOT EXISTS problem_testcases (
  id bigserial PRIMARY KEY,
  problem_id bigint NOT NULL,
  input_file varchar(255) NOT NULL DEFAULT '',
  output_file varchar(255) NOT NULL DEFAULT '',
  input_size bigint NOT NULL DEFAULT 0,
  output_size bigint NOT NULL DEFAULT 0,
  score int NOT NULL DEFAULT 10,
  is_sample boolean NOT NULL DEFAULT false,
  sort_order int NOT NULL DEFAULT 1
)`)
	mustExec(t, pool, `
CREATE TABLE IF NOT EXISTS submissions (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL,
  problem_id bigint NOT NULL,
  contest_id bigint,
  language varchar(20) NOT NULL,
  code text NOT NULL,
  answer text NOT NULL DEFAULT '',
  status varchar(30) NOT NULL DEFAULT 'pending',
  time_used_ms int NOT NULL DEFAULT 0,
  memory_used_kb int NOT NULL DEFAULT 0,
  score int NOT NULL DEFAULT 0,
  error_message text NOT NULL DEFAULT '',
  judge_result_json jsonb NOT NULL DEFAULT '[]',
  created_at timestamptz NOT NULL DEFAULT now(),
  judged_at timestamptz
)`)
	mustExec(t, pool, `
CREATE TABLE IF NOT EXISTS testcase_generations (
  id bigserial PRIMARY KEY,
  problem_id bigint NOT NULL,
  user_id bigint NOT NULL,
  status varchar(20) NOT NULL,
  language varchar(20) NOT NULL,
  standard_code text NOT NULL,
  generator_code text NOT NULL,
  testcase_count int NOT NULL,
  time_limit_ms int NOT NULL,
  memory_limit_mb int NOT NULL,
  error_message text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
)`)
	mustExec(t, pool, `
CREATE TABLE IF NOT EXISTS testcase_generation_cases (
  id bigserial PRIMARY KEY,
  generation_id bigint NOT NULL,
  sort_order int NOT NULL,
  input_file varchar(255) NOT NULL,
  output_file varchar(255) NOT NULL,
  input_size bigint NOT NULL DEFAULT 0,
  output_size bigint NOT NULL DEFAULT 0,
  CONSTRAINT uniq_generation_cases_order UNIQUE (generation_id, sort_order)
)`)
	// Older test databases may predate the objective columns; keep the
	// CREATE TABLE above as the source of truth for fresh databases.
	mustExec(t, pool, `ALTER TABLE problems ADD COLUMN IF NOT EXISTS answers_json jsonb NOT NULL DEFAULT '[]'`)
	mustExec(t, pool, `ALTER TABLE submissions ADD COLUMN IF NOT EXISTS answer text NOT NULL DEFAULT ''`)
	mustExec(t, pool, `TRUNCATE testcase_generation_cases, testcase_generations, submissions, problem_testcases, problems, users RESTART IDENTITY CASCADE`)
	return pool
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql)
	require.NoError(t, err)
}

func InsertUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, nickname, email, password_hash, created_at, updated_at)
		 VALUES ('judge', 'judge', 'judge@example.com', 'x', now(), now()) RETURNING id`,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func InsertProblem(t *testing.T, pool *pgxpool.Pool, typ string, timeLimitMS, memoryMB int, checker string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO problems (title, slug, statement_md, type, difficulty, time_limit_ms, memory_limit_mb, checker_path, is_public, created_by, created_at, updated_at)
		 VALUES ('P', 'p', 's', $1, 1, $2, $3, $4, true, 1, now(), now()) RETURNING id`,
		typ, timeLimitMS, memoryMB, checker,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// InsertObjectiveProblem creates an objective problem with the accepted answers.
func InsertObjectiveProblem(t *testing.T, pool *pgxpool.Pool, answers []string) int64 {
	t.Helper()
	if answers == nil {
		answers = []string{}
	}
	raw, err := json.Marshal(answers)
	require.NoError(t, err)
	var id int64
	err = pool.QueryRow(context.Background(),
		`INSERT INTO problems (title, slug, statement_md, type, difficulty, time_limit_ms, memory_limit_mb, checker_path, answers_json, is_public, created_by, created_at, updated_at)
		 VALUES ('Objective', 'objective', 's', 'objective', 1, 1000, 256, '', $1::jsonb, true, 1, now(), now()) RETURNING id`,
		string(raw),
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func InsertTestcase(t *testing.T, pool *pgxpool.Pool, problemID int64, input, output string, score, order int) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO problem_testcases (problem_id, input_file, output_file, score, sort_order)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		problemID, input, output, score, order,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func InsertSubmission(t *testing.T, pool *pgxpool.Pool, userID, problemID int64, language, code, status string) int64 {
	t.Helper()
	return InsertSubmissionAt(t, pool, userID, problemID, language, code, status, time.Now())
}

func InsertSubmissionAt(t *testing.T, pool *pgxpool.Pool, userID, problemID int64, language, code, status string, created time.Time) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, problemID, language, code, status, created,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// InsertObjectiveSubmission creates an objective submission carrying an answer.
func InsertObjectiveSubmission(t *testing.T, pool *pgxpool.Pool, userID, problemID int64, answer, status string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO submissions (user_id, problem_id, language, code, answer, status, created_at)
		 VALUES ($1, $2, 'answer', '', $3, $4, now()) RETURNING id`,
		userID, problemID, answer, status,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func InsertGeneration(t *testing.T, pool *pgxpool.Pool, userID, problemID int64, language, standard, generator string, count, timeLimitMS, memoryMB int, status string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO testcase_generations (problem_id, user_id, status, language, standard_code, generator_code, testcase_count, time_limit_ms, memory_limit_mb, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now()) RETURNING id`,
		problemID, userID, status, language, standard, generator, count, timeLimitMS, memoryMB,
	).Scan(&id)
	require.NoError(t, err)
	return id
}
