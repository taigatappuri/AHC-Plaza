package store

import (
	"database/sql"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

type rowScanner interface {
	Scan(...interface{}) error
}

// scanRunColumns はrunsの列を読み取り、必要に応じて後続列も同時に読み取ります。
func scanRunColumns(row rowScanner, run *domain.Run, extra ...interface{}) error {
	var status, createdAt, startedAt string
	var finishedAt sql.NullString
	args := []interface{}{
		&run.ID, &run.RunNumber, &run.Problem, &run.Objective, &run.SolverPath, &run.InputDir, &run.InputCasesHash,
		&run.SourcePath, &run.SourceHash, &run.ConfigHash, &run.PahcerVersion, &run.CompilerVersion,
		&run.Threads, &run.TimeoutSeconds, &status, &run.Comment,
		&createdAt, &startedAt, &finishedAt,
	}
	args = append(args, extra...)
	if err := row.Scan(args...); err != nil {
		return err
	}
	run.Status = domain.RunStatus(status)
	var err error
	run.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return err
	}
	run.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return err
	}
	if finishedAt.Valid && finishedAt.String != "" {
		value, err := parseTime(finishedAt.String)
		if err != nil {
			return err
		}
		run.FinishedAt = &value
	}
	return nil
}
