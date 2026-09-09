package domain

import "time"

type TuningStudy struct {
	ID             string             `json:"id"`
	Status         string             `json:"status"`
	Error          string             `json:"error"`
	Trials         int                `json:"trials"`
	Completed      int                `json:"completed"`
	Failed         int                `json:"failed"`
	Seconds        int                `json:"seconds"`
	Elapsed        float64            `json:"elapsed"`
	BaselineRun    string             `json:"baseline_run"`
	BaselineValue  *float64           `json:"baseline_value"`
	BestRun        string             `json:"best_run"`
	BestValue      *float64           `json:"best_value"`
	BestParams     map[string]float64 `json:"best_params"`
	PendingRequest string             `json:"pending_request"`
	ManifestHash   string             `json:"manifest_hash"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Validations    []TuningValidation `json:"validations"`
}
type TuningTrial struct {
	StudyID    string             `json:"study_id"`
	Number     int                `json:"number"`
	RunID      string             `json:"run_id"`
	RequestID  string             `json:"request_id"`
	Params     map[string]float64 `json:"params"`
	Actual     map[string]float64 `json:"actual"`
	Status     string             `json:"status"`
	Value      *float64           `json:"value"`
	Reason     string             `json:"reason"`
	ResultHash string             `json:"result_hash"`
	Delivered  bool               `json:"delivered"`
}
type TuningValidation struct {
	BaselineRun  string `json:"baseline_run"`
	CandidateRun string `json:"candidate_run"`
	InputDir     string `json:"input_dir"`
	Error        string `json:"error"`
}
