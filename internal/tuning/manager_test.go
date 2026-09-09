package tuning

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/store"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
	bundled "github.com/taigatappuri/AHC-Plaza/internal/tuning/runtime"
	"github.com/taigatappuri/AHC-Plaza/internal/usecase"
)

func TestInspectToolsValidatesCandidateBuild(t *testing.T) {
	bin := t.TempDir()
	for _, program := range []string{"g++", "pahcer", "cargo"} {
		if err := os.WriteFile(filepath.Join(bin, program), []byte("#!/bin/sh\necho test-version\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	for _, tc := range []struct {
		name, compile, evaluation, wantError string
	}{
		{"direct", `args=["main.cpp", "-o", "main.exe"]`, `program="./main.exe"`, ""},
		{"tester", `args=["main.cpp", "-o", "main.exe"]`, "program=\"./tools/target/release/tester\"\nargs=[\"./main.exe\"]", ""},
		{"cargo tester", "args=[\"main.cpp\", \"-o\", \"main.exe\"]\n[[test.compile_steps]]\nprogram=\"cargo\"\nargs=[\"build\", \"--release\"]\ncurrent_dir=\"tools\"", `program="./main.exe"`, ""},
		{"different binary", `args=["main.cpp", "-o", "main.exe"]`, `program="./old.exe"`, "test_steps"},
		{"other source", `args=["main.cpp", "helper.cpp"]`, `program="./a.out"`, "別ソース"},
		{"external include", `args=["main.cpp", "-I../include"]`, `program="./a.out"`, "外部ソース"},
		{"overwrites source", `args=["main.cpp", "-o", "main.cpp"]`, `program="./main.cpp"`, "出力先"},
		{"external workspace", `args=["main.cpp"]`, "program=\"./a.out\"\ncurrent_dir=\"../old\"", "作業場所"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting := filepath.Join(t.TempDir(), "pahcer.toml")
			content := "[[test.compile_steps]]\nprogram=\"g++\"\n" + tc.compile + "\n[[test.test_steps]]\n" + tc.evaluation + "\n"
			if err := os.WriteFile(setting, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			versions, err := inspectTools(context.Background(), setting)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("want %q, got %v", tc.wantError, err)
				}
			} else if err != nil || len(versions) != 3 {
				t.Fatalf("versions=%v error=%v", versions, err)
			}
		})
	}
}

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
