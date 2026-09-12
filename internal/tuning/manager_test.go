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

func TestInspectBuildAcceptsArbitraryPipelineAndInfersSource(t *testing.T) {
	bin := t.TempDir()
	for _, program := range []string{"mkdir", "g++", "pahcer", "cargo", "make", "ccache"} {
		if err := os.WriteFile(filepath.Join(bin, program), []byte("#!/bin/sh\necho test-version\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	for _, tc := range []struct {
		name, compile, evaluation, selected, explicit, wantSource, wantError string
	}{
		{"ahc065", "program=\"mkdir\"\nargs=[\"-p\",\"build\"]\n[[test.compile_steps]]\nprogram=\"g++\"\nargs=[\"-Iinclude\",\"main.cpp\",\"-o\",\"build/pahcer_main\"]\n[[test.compile_steps]]\nprogram=\"cargo\"\nargs=[\"build\",\"--release\"]\ncurrent_dir=\"tools\"", `program="./build/pahcer_main"`, "solver/main.cpp", "", "main.cpp", ""},
		{"named source", "program=\"g++\"\nargs=[\"ankake.cpp\",\"-o\",\"main\"]", `program="./main"`, "solver/ankake.cpp", "", "ankake.cpp", ""},
		{"make and script", "program=\"make\"\nargs=[\"solver\"]\n[[test.compile_steps]]\nprogram=\"./scripts/build.sh\"\nargs=[\"src/a.cpp\",\"src/b.cpp\"]", `program="./build/solver"`, "solver/main.cpp", "solver/main.cpp", "solver/main.cpp", ""},
		{"new explicit target", `program="make"`, `program="./build/solver"`, "solver/main.cpp", "generated/main.cpp", "generated/main.cpp", ""},
		{"ambiguous", "program=\"g++\"\nargs=[\"a.cpp\",\"b.cpp\"]", `program="./a.out"`, "solver/chosen.cpp", "", "", "複数"},
		{"empty program", `program=""`, `program="./a.out"`, "solver/main.cpp", "", "", "compile_steps[0].program"},
		{"external workspace", "program=\"g++\"\nargs=[\"main.cpp\"]\ncurrent_dir=\"../old\"", `program="./a.out"`, "solver/main.cpp", "", "", "compile_steps[0].current_dir"},
		{"invalid test cwd", "program=\"g++\"\nargs=[\"main.cpp\"]", "program=\"./a.out\"\ncurrent_dir=\"../../outside\"", "solver/main.cpp", "", "", "test_steps[0].current_dir"},
		{"unresolved bare test", `program="make"`, `program="missing-tester"`, "solver/main.cpp", "", "", "test_steps[0].program"},
		{"missing stdin", `program="make"`, `program="./built"`, "solver/main.cpp", "", "", "stdin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting := filepath.Join(t.TempDir(), "pahcer.toml")
			evaluation := tc.evaluation
			if tc.name != "missing stdin" {
				evaluation += "\nstdin=\"in\""
			}
			content := "[[test.compile_steps]]\n" + tc.compile + "\n[[test.test_steps]]\n" + evaluation + "\n"
			if err := os.WriteFile(setting, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			project := t.TempDir()
			for _, name := range []string{"solver/main.cpp", "solver/ankake.cpp", "solver/chosen.cpp", "main.cpp", "ankake.cpp", "a.cpp", "b.cpp", "scripts/build.sh"} {
				path := filepath.Join(project, name)
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte(name), 0755)
			}
			inspection, err := inspectBuild(context.Background(), setting, project, tc.selected, tc.explicit)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("want %q, got %v", tc.wantError, err)
				}
			} else if err != nil || inspection.SourceTarget != tc.wantSource || len(inspection.Tools) == 0 {
				t.Fatalf("inspection=%+v error=%v", inspection, err)
			}
		})
	}
}

func TestInferSourceTargetAllowsNewUniqueCompilePath(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "solver"), 0755)
	os.WriteFile(filepath.Join(root, "solver", "main.cpp"), []byte("x"), 0644)
	target, err := inferSourceTarget(root, "solver/main.cpp", "", []buildStep{{Program: "g++", Args: []string{"main.cpp"}}})
	if err != nil || target != "main.cpp" {
		t.Fatalf("target=%q err=%v", target, err)
	}
}

func TestInspectBuildRequiresTestSteps(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"make", "pahcer"} {
		os.WriteFile(filepath.Join(bin, name), []byte("x"), 0700)
	}
	t.Setenv("PATH", bin)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "main.cpp"), []byte("x"), 0644)
	setting := filepath.Join(root, "p.toml")
	os.WriteFile(setting, []byte("[[test.compile_steps]]\nprogram=\"make\"\n"), 0644)
	if _, err := inspectBuild(context.Background(), setting, root, "main.cpp", ""); err == nil || !strings.Contains(err.Error(), "test_steps") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestV1CannotResume(t *testing.T) {
	m := &Manager{}
	if err := m.verify(context.Background(), domain.TuningStudy{}, Manifest{Version: 1}); err == nil || !strings.Contains(err.Error(), "閲覧と書き出し") {
		t.Fatalf("got %v", err)
	}
}

func TestLegacyManifestV2IgnoresOnlyAHCPlazaFingerprint(t *testing.T) {
	saved := Manifest{Version: 2, Prepared: usecase.PreparedRun{SourceTarget: "main.cpp"}, Tools: map[string]string{
		"compile_steps[0]:g++": "g++-same",
		"runner:ahc-plaza":     "old-self-binary",
	}}
	current := BuildInspection{SourceTarget: "main.cpp", Tools: map[string]string{
		"compile_steps[0]:g++": "g++-same",
	}}
	if err := verifyBuildCompatibility(saved, current); err != nil {
		t.Fatalf("self binary difference must be ignored for v2: %v", err)
	}
	current.Tools["compile_steps[0]:g++"] = "changed-g++"
	if err := verifyBuildCompatibility(saved, current); err == nil || !strings.Contains(err.Error(), "compile_steps[0]:g++") {
		t.Fatalf("external tool difference must identify its key: %v", err)
	}
}

func TestManifestV3RejectsCaseExecCompatibilityChange(t *testing.T) {
	v := Manifest{Version: 3, CaseExecVersion: caseExecCompatibilityVersion + 1}
	if err := verifyCaseExecCompatibility(v); err == nil || !strings.Contains(err.Error(), "case-exec互換性") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildCompatibilityReportsSourceTargetChange(t *testing.T) {
	v := Manifest{Version: 3, Prepared: usecase.PreparedRun{SourceTarget: "main.cpp"}}
	if err := verifyBuildCompatibility(v, BuildInspection{SourceTarget: "solver.cpp"}); err == nil || !strings.Contains(err.Error(), "source_target") {
		t.Fatalf("got %v", err)
	}
}

func TestInspectBuildFingerprintChangesWithExecutable(t *testing.T) {
	bin := t.TempDir()
	for _, program := range []string{"make", "pahcer"} {
		if err := os.WriteFile(filepath.Join(bin, program), []byte("first"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	project := t.TempDir()
	os.WriteFile(filepath.Join(project, "main.cpp"), []byte("int main(){}"), 0600)
	setting := filepath.Join(project, "pahcer.toml")
	os.WriteFile(setting, []byte("[[test.compile_steps]]\nprogram=\"make\"\n[[test.test_steps]]\nprogram=\"./main\"\nstdin=\"in\"\n"), 0600)
	first, err := inspectBuild(context.Background(), setting, project, "main.cpp", "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(bin, "make"), []byte("second"), 0700)
	second, err := inspectBuild(context.Background(), setting, project, "main.cpp", "")
	if err != nil {
		t.Fatal(err)
	}
	if sameJSON(first.Tools, second.Tools) {
		t.Fatal("executable change was not detected")
	}
	if _, ok := first.Tools["runner:ahc-plaza"]; ok {
		t.Fatal("new manifests must not fingerprint the AHC Plaza executable")
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

func TestBaselineRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		status        domain.RunStatus
		retry, active bool
	}{
		{domain.RunSucceeded, true, false}, {domain.RunPartial, true, false}, {domain.RunFailed, true, false}, {domain.RunCancelled, true, false},
		{domain.RunQueued, true, true}, {domain.RunRunning, true, true}, {domain.RunStatus("unknown"), false, false},
	} {
		retry, active := baselineRetryClassification(tc.status)
		if retry != tc.retry || active != tc.active {
			t.Fatalf("status=%s retry=%v active=%v", tc.status, retry, active)
		}
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
