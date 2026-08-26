package store

import (
	"context"
	"fmt"
)

func (s *SQLiteStore) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  run_number INTEGER,
  problem TEXT NOT NULL,
  objective TEXT NOT NULL,
  solver_path TEXT NOT NULL,
  input_dir TEXT NOT NULL,
  input_cases_hash TEXT NOT NULL,
  source_path TEXT NOT NULL,
  source_hash TEXT NOT NULL,
  config_hash TEXT NOT NULL,
  pahcer_version TEXT NOT NULL,
  compiler_version TEXT NOT NULL,
  threads INTEGER NOT NULL,
  timeout_ms INTEGER NOT NULL,
  status TEXT NOT NULL,
  comment TEXT NOT NULL,
  created_at TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT
)`,
		`CREATE TABLE IF NOT EXISTS input_cases (
  run_id TEXT NOT NULL,
  input_case_id TEXT NOT NULL,
  input_path TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  size INTEGER NOT NULL,
  PRIMARY KEY (run_id, input_case_id),
  FOREIGN KEY (run_id) REFERENCES runs(id)
)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS run_sequences (
  name TEXT PRIMARY KEY,
  next_number INTEGER NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS cases (
  run_id TEXT NOT NULL,
  input_case_id TEXT NOT NULL,
  seed INTEGER NOT NULL DEFAULT 0,
  input_path TEXT NOT NULL,
  score REAL NOT NULL,
  execution_time_ns INTEGER NOT NULL,
  status TEXT NOT NULL,
  stdout_path TEXT NOT NULL,
  stderr_path TEXT NOT NULL,
  output_path TEXT NOT NULL,
  error_message TEXT NOT NULL,
  PRIMARY KEY (run_id, input_case_id),
  FOREIGN KEY (run_id) REFERENCES runs(id)
)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("could not create the SQLite schema: %w", err)
		}
	}
	if err := s.ensureColumn(ctx, "runs", "run_number", "INTEGER"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "runs", "timeout_ms", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	legacyTimeoutExists, err := s.columnExists(ctx, "runs", "timeout_seconds")
	if err != nil {
		return err
	}
	if legacyTimeoutExists {
		if _, err := s.db.ExecContext(ctx, `UPDATE runs SET timeout_ms = timeout_seconds * 1000 WHERE timeout_ms = 0`); err != nil {
			return fmt.Errorf("could not migrate Run timeouts to milliseconds: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `
WITH numbered AS (
  SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, id) +
    (SELECT COALESCE(MAX(run_number), 0) FROM runs) AS number
  FROM runs WHERE run_number IS NULL
)
UPDATE runs SET run_number = (SELECT number FROM numbered WHERE numbered.id = runs.id)
WHERE run_number IS NULL`); err != nil {
		return fmt.Errorf("could not assign existing Run numbers: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO run_sequences (name, next_number)
VALUES ('runs', (SELECT COALESCE(MAX(run_number), 0) + 1 FROM runs))
ON CONFLICT(name) DO UPDATE SET next_number = MAX(next_number, excluded.next_number)`); err != nil {
		return fmt.Errorf("could not initialize the Run sequence: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_run_number ON runs(run_number) WHERE run_number IS NOT NULL`); err != nil {
		return fmt.Errorf("could not create the Run sequence index: %w", err)
	}
	return s.ensureColumn(ctx, "cases", "seed", "INTEGER NOT NULL DEFAULT 0")
}

func (s *SQLiteStore) ensureColumn(ctx context.Context, table, column, definition string) error {
	exists, err := s.columnExists(ctx, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, definition)); err != nil {
		return fmt.Errorf("could not add %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *SQLiteStore) columnExists(ctx context.Context, table, column string) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pragma_table_info(?) WHERE name = ?)`, table, column).Scan(&exists); err != nil {
		return false, fmt.Errorf("could not inspect %s.%s: %w", table, column, err)
	}
	return exists, nil
}
