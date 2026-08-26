package domain

import "time"

type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunPartial   RunStatus = "partial"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

type Run struct {
	ID                  string     `json:"id"`
	RunNumber           int        `json:"run_number"`
	Problem             string     `json:"problem"`
	Objective           string     `json:"objective"`
	SolverPath          string     `json:"solver_path"`
	InputDir            string     `json:"input_dir"`
	InputCasesHash      string     `json:"input_cases_hash"`
	SourcePath          string     `json:"source_path"`
	SourceHash          string     `json:"source_hash"`
	ConfigHash          string     `json:"config_hash"`
	PahcerVersion       string     `json:"pahcer_version"`
	CompilerVersion     string     `json:"compiler_version"`
	Threads             int        `json:"threads"`
	TimeoutMilliseconds int        `json:"timeout_ms"`
	Status              RunStatus  `json:"status"`
	Comment             string     `json:"comment"`
	StartedAt           time.Time  `json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type InputCase struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type CaseResult struct {
	RunID         string        `json:"run_id"`
	InputCaseID   string        `json:"input_case_id"`
	Seed          uint64        `json:"seed"`
	InputPath     string        `json:"input_path"`
	Score         float64       `json:"score"`
	ExecutionTime time.Duration `json:"execution_time_ns"`
	Status        string        `json:"status"`
	StdoutPath    string        `json:"stdout_path"`
	StderrPath    string        `json:"stderr_path"`
	OutputPath    string        `json:"output_path"`
	ErrorMessage  string        `json:"error_message,omitempty"`
}

// RunWithStats はで使う集約済みの統計値です。
// ケース結果をRunごとに再取得せず、一覧表示に必要な値をまとめて扱います。
type RunWithStats struct {
	Run
	RunStatistics
}

// RunStatistics はRunに含まれるケース結果の集計値です。
// 一覧と詳細で同じ統計定義を使うため、APIの集計値もこの構造に揃えます。
type RunStatistics struct {
	CaseCount              int     `json:"case_count"`
	AverageScore           float64 `json:"average_score"`
	MinScore               float64 `json:"min_score"`
	Q1Score                float64 `json:"q1_score"`
	MedianScore            float64 `json:"median_score"`
	Q3Score                float64 `json:"q3_score"`
	MaxScore               float64 `json:"max_score"`
	VarianceScore          float64 `json:"variance_score"`
	StdDevScore            float64 `json:"stddev_score"`
	TotalExecutionTimeNs   int64   `json:"total_execution_time_ns"`
	AverageExecutionTimeNs float64 `json:"average_execution_time_ns"`
	MaxExecutionTimeNs     int64   `json:"max_execution_time_ns"`
}

type SourceSnapshot struct {
	SnapshotPath string
	SHA256       string
}
