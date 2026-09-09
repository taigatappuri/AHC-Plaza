package store

import (
	"context"
	"database/sql"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"time"
)

func (s *SQLiteStore) AcquireExecution(ctx context.Context) (func(), error) {
	select {
	case s.execution <- struct{}{}:
		return func() { <-s.execution }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (s *SQLiteStore) FinalizeRun(ctx context.Context, id string, status domain.RunStatus, finished time.Time, results []domain.CaseResult) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		var previous string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM runs WHERE id=?`, id).Scan(&previous); err != nil {
			return err
		}
		if previous != "running" && previous != "queued" {
			return nil
		}
		for _, r := range results {
			if _, e := tx.ExecContext(ctx, `INSERT OR REPLACE INTO cases (run_id,input_case_id,seed,input_path,score,execution_time_ns,status,stdout_path,stderr_path,output_path,error_message) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, r.RunID, r.InputCaseID, r.Seed, r.InputPath, r.Score, r.ExecutionTime.Nanoseconds(), r.Status, r.StdoutPath, r.StderrPath, r.OutputPath, r.ErrorMessage); e != nil {
				return e
			}
		}
		_, e := tx.ExecContext(ctx, `UPDATE runs SET status=?,finished_at=? WHERE id=? AND status IN ('queued','running')`, status, formatTime(finished), id)
		return e
	})
}

// FailActiveRun は後処理が既に確定したcancelled/succeededを上書きしません。
func (s *SQLiteStore) FailActiveRun(ctx context.Context, id string) error {
	_, e := s.db.ExecContext(ctx, `UPDATE runs SET status='failed',finished_at=? WHERE id=? AND status IN ('queued','running')`, formatTime(time.Now()), id)
	return e
}
