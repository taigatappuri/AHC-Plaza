package usecase

import (
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

// RunRequest はCLI/APIから受け取ったRun実行条件です。
type RunRequest struct {
	RunID               string `json:"-"`
	ConfigPath          string `json:"-"`
	Solver              string `json:"solver"`
	InputDir            string `json:"input_dir"`
	Threads             int    `json:"threads"`
	TimeoutMilliseconds int    `json:"timeout_ms"`
	Comment             string `json:"comment"`
	PahcerBinary        string `json:"pahcer"`
	SettingFile         string `json:"setting_file"`
}

// RunSummary はRun完了後にCLI/APIへ返す要約です。
type RunSummary struct {
	RunID          string           `json:"run_id"`
	RunNumber      int              `json:"run_number"`
	Status         domain.RunStatus `json:"status"`
	CreatedAt      time.Time        `json:"created_at"`
	StartedAt      time.Time        `json:"started_at"`
	FinishedAt     *time.Time       `json:"finished_at,omitempty"`
	InputCaseCount int              `json:"input_case_count"`
	SourceSnapshot string           `json:"source_snapshot"`
	ExitCode       int              `json:"exit_code"`
	Error          string           `json:"error,omitempty"`
}
