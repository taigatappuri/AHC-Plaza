package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func (s *SQLiteStore) migrateTuning(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS tuning_studies(id TEXT PRIMARY KEY,status TEXT NOT NULL,data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS tuning_trials(study_id TEXT NOT NULL,number INTEGER NOT NULL,run_id TEXT NOT NULL,data TEXT NOT NULL,PRIMARY KEY(study_id,number))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS tuning_run_unique ON tuning_trials(run_id) WHERE run_id != ''`,
		`CREATE TABLE IF NOT EXISTS tuning_outbox(study_id TEXT NOT NULL,number INTEGER NOT NULL,data TEXT NOT NULL,delivered INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(study_id,number))`,
		`CREATE TABLE IF NOT EXISTS tuning_profiles(source_hash TEXT PRIMARY KEY,data TEXT NOT NULL)`,
	} {
		if _, e := s.db.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	return nil
}
func (s *SQLiteStore) SaveStudy(ctx context.Context, v domain.TuningStudy) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO tuning_studies VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,data=excluded.data`, v.ID, v.Status, b)
	return e
}
func (s *SQLiteStore) GetStudy(ctx context.Context, id string) (domain.TuningStudy, error) {
	var v domain.TuningStudy
	var b []byte
	e := s.db.QueryRowContext(ctx, `SELECT data FROM tuning_studies WHERE id=?`, id).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *SQLiteStore) ListStudies(ctx context.Context) ([]domain.TuningStudy, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT data FROM tuning_studies ORDER BY id DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.TuningStudy{}
	for rows.Next() {
		var b []byte
		var v domain.TuningStudy
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) SaveTrial(ctx context.Context, v domain.TuningTrial) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, `INSERT INTO tuning_trials VALUES(?,?,?,?) ON CONFLICT(study_id,number) DO UPDATE SET run_id=excluded.run_id,data=excluded.data`, v.StudyID, v.Number, v.RunID, b); e != nil {
			return e
		}
		if v.Status != "RUNNING" {
			_, e := tx.ExecContext(ctx, `INSERT INTO tuning_outbox VALUES(?,?,?,?) ON CONFLICT(study_id,number) DO UPDATE SET data=excluded.data,delivered=excluded.delivered`, v.StudyID, v.Number, b, v.Delivered)
			return e
		}
		return nil
	})
}
func (s *SQLiteStore) Trials(ctx context.Context, id string, offset, limit int) ([]domain.TuningTrial, error) {
	if offset < 0 || limit < 1 || limit > 10000 {
		return nil, fmt.Errorf("ページ範囲が不正です")
	}
	rows, e := s.db.QueryContext(ctx, `SELECT data FROM tuning_trials WHERE study_id=? ORDER BY number LIMIT ? OFFSET ?`, id, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.TuningTrial{}
	for rows.Next() {
		var b []byte
		var v domain.TuningTrial
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) SaveProfile(ctx context.Context, hash string, data []byte) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO tuning_profiles VALUES(?,?) ON CONFLICT(source_hash) DO UPDATE SET data=excluded.data`, hash, data)
	return e
}
func (s *SQLiteStore) Profile(ctx context.Context, hash string) (json.RawMessage, error) {
	var b []byte
	e := s.db.QueryRowContext(ctx, `SELECT data FROM tuning_profiles WHERE source_hash=?`, hash).Scan(&b)
	if e == sql.ErrNoRows {
		return nil, nil
	}
	return b, e
}
