package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/statistics"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("could not create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("could not open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) SaveRun(ctx context.Context, run domain.Run) error {
	if run.ID == "" {
		return errors.New("Run ID is required")
	}
	return s.transaction(ctx, func(tx *sql.Tx) error {
		var runNumber int
		if err := tx.QueryRowContext(ctx, `UPDATE run_sequences SET next_number = next_number + 1 WHERE name = 'runs' RETURNING next_number - 1`).Scan(&runNumber); err != nil {
			return fmt.Errorf("could not get Run sequence number: %w", err)
		}
		_, err := tx.ExecContext(ctx, `
	INSERT INTO runs (
  id, run_number, problem, objective, solver_path, input_dir, input_cases_hash,
  source_path, source_hash, config_hash, pahcer_version, compiler_version,
  threads, timeout_ms, status, comment, created_at, started_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, run.ID, runNumber, run.Problem, run.Objective, run.SolverPath, run.InputDir, run.InputCasesHash,
			run.SourcePath, run.SourceHash, run.ConfigHash, run.PahcerVersion, run.CompilerVersion,
			run.Threads, run.TimeoutMilliseconds, string(run.Status), run.Comment,
			formatTime(run.CreatedAt), formatTime(run.StartedAt), formatOptionalTime(run.FinishedAt))
		return err
	})
}
func (s *SQLiteStore) GetRun(ctx context.Context, id string) (domain.Run, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, run_number, problem, objective, solver_path, input_dir, input_cases_hash,
       source_path, source_hash, config_hash, pahcer_version, compiler_version,
       threads, timeout_ms, status, comment, created_at, started_at, finished_at
FROM runs WHERE id = ?
`, id)
	var run domain.Run
	err := scanRunColumns(row, &run)
	return run, err
}

func (s *SQLiteStore) UpdateRunStartedAt(ctx context.Context, id string, startedAt time.Time) error {
	return s.updateRun(ctx, id, `UPDATE runs SET started_at = ? WHERE id = ?`, formatTime(startedAt))
}
func (s *SQLiteStore) UpdateRunStatus(ctx context.Context, id string, status domain.RunStatus, finishedAt *time.Time) error {
	return s.updateRun(ctx, id, `UPDATE runs SET status = ?, finished_at = ? WHERE id = ?`, string(status), formatOptionalTime(finishedAt))
}

// MarkUnfinishedRunsFailed は前回のプロセス終了時に残った未完了Runを失敗として確定します。
func (s *SQLiteStore) MarkUnfinishedRunsFailed(ctx context.Context, finishedAt time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE runs SET status = ?, finished_at = ?
WHERE status IN (?, ?)
`, string(domain.RunFailed), formatTime(finishedAt), string(domain.RunQueued), string(domain.RunRunning))
	if err != nil {
		return 0, fmt.Errorf("could not mark unfinished Runs as failed: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("could not get unfinished Run update count: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) UpdateRunComment(ctx context.Context, id, comment string) error {
	return s.updateRun(ctx, id, `UPDATE runs SET comment = ? WHERE id = ?`, comment)
}

func (s *SQLiteStore) updateRun(ctx context.Context, id, query string, args ...any) error {
	result, err := s.db.ExecContext(ctx, query, append(args, id)...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("could not get Run update count: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("Run not found: %s", id)
	}
	return nil
}

func (s *SQLiteStore) SaveInputCases(ctx context.Context, runID string, inputCases []domain.InputCase) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		for _, inputCase := range inputCases {
			if _, err := tx.ExecContext(ctx, `
	INSERT INTO input_cases (run_id, input_case_id, input_path, sha256, size)
	VALUES (?, ?, ?, ?, ?)
	`, runID, inputCase.ID, inputCase.Path, inputCase.SHA256, inputCase.Size); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLiteStore) SaveCaseResults(ctx context.Context, results []domain.CaseResult) error {
	if len(results) == 0 {
		return nil
	}
	return s.transaction(ctx, func(tx *sql.Tx) error {
		for _, result := range results {
			_, err := tx.ExecContext(ctx, `
	INSERT OR REPLACE INTO cases (
  run_id, input_case_id, seed, input_path, score, execution_time_ns, status,
  stdout_path, stderr_path, output_path, error_message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, result.RunID, result.InputCaseID, result.Seed, result.InputPath, result.Score,
				result.ExecutionTime.Nanoseconds(), result.Status, result.StdoutPath,
				result.StderrPath, result.OutputPath, result.ErrorMessage)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLiteStore) transaction(ctx context.Context, action func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := action(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ListRunSummaries はRun一覧に必要な集約値を1回のSQLで取得します。
// API層でRunごとにCase結果を問い合わせるN+1を避けるための読み取り専用クエリです。
func (s *SQLiteStore) ListRunSummaries(ctx context.Context, limit int) ([]domain.RunWithStats, error) {
	if limit <= 0 {
		limit = -1
	}
	// JOIN後にLIMITするとケース数でRunが欠けるため、先にRunだけを絞ります。
	rows, err := s.db.QueryContext(ctx, `
	SELECT r.id, r.run_number, r.problem, r.objective, r.solver_path, r.input_dir, r.input_cases_hash,
	       r.source_path, r.source_hash, r.config_hash, r.pahcer_version, r.compiler_version,
       r.threads, r.timeout_ms, r.status, r.comment, r.created_at, r.started_at, r.finished_at,
       c.input_case_id, c.score, c.execution_time_ns
FROM (
  SELECT * FROM runs ORDER BY run_number DESC, id LIMIT ?
	) r
	LEFT JOIN cases c ON c.run_id = r.id
	ORDER BY r.run_number DESC, r.id, c.input_case_id`, limit)
	if err != nil {
		return nil, fmt.Errorf("could not get aggregated Run list: %w", err)
	}
	defer rows.Close()
	result := make([]domain.RunWithStats, 0)
	caseResults := make(map[string][]domain.CaseResult)
	for rows.Next() {
		var run domain.Run
		var inputCaseID sql.NullString
		var score sql.NullFloat64
		var executionTimeNs sql.NullInt64
		if err := scanRunColumns(rows, &run, &inputCaseID, &score, &executionTimeNs); err != nil {
			return nil, err
		}
		if len(result) == 0 || result[len(result)-1].ID != run.ID {
			result = append(result, domain.RunWithStats{Run: run})
		}
		if !inputCaseID.Valid {
			continue
		}
		if !score.Valid || !executionTimeNs.Valid {
			return nil, fmt.Errorf("invalid aggregated Case result: %s", inputCaseID.String)
		}
		caseResults[run.ID] = append(caseResults[run.ID], domain.CaseResult{
			RunID:         run.ID,
			InputCaseID:   inputCaseID.String,
			Score:         score.Float64,
			ExecutionTime: time.Duration(executionTimeNs.Int64),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read aggregated Run list: %w", err)
	}
	for index := range result {
		runID := result[index].Run.ID
		if len(caseResults[runID]) == 0 {
			continue
		}
		runStatistics, err := statistics.SummarizeRunResults(caseResults[runID])
		if err != nil {
			return nil, fmt.Errorf("could not aggregate Run statistics: %w", err)
		}
		result[index].RunStatistics = runStatistics
	}
	return result, nil
}

func (s *SQLiteStore) GetCaseResults(ctx context.Context, runID string) ([]domain.CaseResult, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT run_id, input_case_id, input_path, score, execution_time_ns, status,
       seed, stdout_path, stderr_path, output_path, error_message
FROM cases WHERE run_id = ? ORDER BY input_case_id
`, runID)
	if err != nil {
		return nil, fmt.Errorf("could not get Case results: %w", err)
	}
	defer rows.Close()
	result := make([]domain.CaseResult, 0)
	for rows.Next() {
		var item domain.CaseResult
		var executionTime int64
		if err := rows.Scan(&item.RunID, &item.InputCaseID, &item.InputPath, &item.Score,
			&executionTime, &item.Status, &item.Seed, &item.StdoutPath, &item.StderrPath,
			&item.OutputPath, &item.ErrorMessage); err != nil {
			return nil, fmt.Errorf("could not read Case result: %w", err)
		}
		item.ExecutionTime = time.Duration(executionTime)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read Case result list: %w", err)
	}
	return result, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
