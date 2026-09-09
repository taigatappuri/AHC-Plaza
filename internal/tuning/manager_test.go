package tuning

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/store"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
	bundled "github.com/taigatappuri/AHC-Plaza/internal/tuning/runtime"
	"github.com/taigatappuri/AHC-Plaza/internal/usecase"
)

func TestRecoverRunAndOutboxAfterInterruption(t *testing.T) {
	ctx := context.Background()
	db, e := store.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	m := &Manager{Store: db}
	run := domain.Run{ID: "r", Status: domain.RunRunning, CreatedAt: time.Now(), StartedAt: time.Now()}
	if e = db.SaveRun(ctx, run); e != nil {
		t.Fatal(e)
	}
	if e = db.FinalizeRun(ctx, "r", domain.RunSucceeded, time.Now(), []domain.CaseResult{{RunID: "r", InputCaseID: "0", Seed: 0, Status: "succeeded", Score: 7}}); e != nil {
		t.Fatal(e)
	}
	trial := domain.TuningTrial{StudyID: "s", Number: 0, RunID: "r", Status: "RUNNING"}
	m.recoverTrial(ctx, &trial, 1)
	if trial.Status != "COMPLETE" || trial.Value == nil || *trial.Value != 7 {
		t.Fatal(trial)
	}
	if e = db.SaveTrial(ctx, trial); e != nil {
		t.Fatal(e)
	}
	rows, e := db.Trials(ctx, "s", 0, 10)
	if e != nil || len(rows) != 1 || rows[0].Delivered {
		t.Fatal(rows, e)
	}
	if e = db.FailActiveRun(ctx, "r"); e != nil {
		t.Fatal(e)
	}
	saved, e := db.GetRun(ctx, "r")
	if e != nil || saved.Status != domain.RunSucceeded {
		t.Fatal(saved, e)
	}
}
func TestBestIncludesBaselineForBothDirections(t *testing.T) {
	baseline := 10.0
	worse := 8.0
	better := 12.0
	s := domain.TuningStudy{BaselineRun: "baseline", BaselineValue: &baseline}
	trials := []domain.TuningTrial{{RunID: "low", Status: "COMPLETE", Value: &worse}, {RunID: "high", Status: "COMPLETE", Value: &better}, {Status: "FAIL"}}
	updateBest(&s, trials, "maximize")
	if s.BestRun != "high" || s.Completed != 2 || s.Failed != 1 {
		t.Fatal(s)
	}
	updateBest(&s, trials, "minimize")
	if s.BestRun != "low" {
		t.Fatal(s)
	}
	updateBest(&s, trials[:1], "maximize")
	if s.BestRun != "baseline" {
		t.Fatal(s)
	}
}
func TestPendingAskReconcilesExistingTrial(t *testing.T) {
	// Simulate crash after Trial/Run save but before clearing the persisted ask request.
	root := t.TempDir()
	if !bundled.Status(root).Available {
		t.Skip("requires embedded runtime")
	}
	ctx := context.Background()
	dir := filepath.Join(root, "ahc-plaza", "tuning", "study")
	os.MkdirAll(dir, 0700)
	db, e := store.OpenSQLite(filepath.Join(root, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	m := &Manager{Root: root, Store: db}
	scan, e := params.Parse([]byte("int X=1; // @tune 1 2\n"))
	if e != nil {
		t.Fatal(e)
	}
	w, e := startWorker(ctx, root, dir)
	if e != nil {
		t.Fatal(e)
	}
	init := map[string]any{"op": "init", "storage": filepath.Join(dir, "optuna.sqlite3"), "study_id": "study", "seed": 42, "direction": "maximize", "manifest_hash": "hash", "parameters": scan.Parameters}
	if e = w.call(ctx, init, nil); e != nil {
		t.Fatal(e)
	}
	var trial domain.TuningTrial
	if e = w.call(ctx, map[string]any{"op": "ask", "request_id": "pending"}, &trial); e != nil {
		t.Fatal(e)
	}
	w.close()
	value := 10.0
	trial.StudyID = "study"
	trial.RequestID = "pending"
	trial.RunID = "already-evaluated"
	trial.Status = "COMPLETE"
	trial.Value = &value
	trial.ResultHash = resultHash(trial)
	if e = db.SaveTrial(ctx, trial); e != nil {
		t.Fatal(e)
	}
	s := domain.TuningStudy{ID: "study", Trials: 1, Status: "paused", PendingRequest: "pending", ManifestHash: "hash", BaselineRun: "baseline", BaselineValue: &value}
	v := Manifest{Parameters: scan.Parameters, Request: StartRequest{Seed: 42}, Prepared: usecase.PreparedRun{}}
	m.run(ctx, &activity{}, &s, v)
	if s.Status != "completed" || s.PendingRequest != "" || s.Completed != 1 {
		t.Fatal(s)
	}
	saved, e := db.Trials(ctx, "study", 0, 10)
	if e != nil || len(saved) != 1 || saved[0].RunID != "already-evaluated" || !saved[0].Delivered {
		b, _ := json.Marshal(saved)
		t.Fatal(string(b), e)
	}
}
