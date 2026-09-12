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

func TestCleanupTuningRunCacheRemovesBuildCopiesAndUpdatesUsage(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ahc-plaza", "runs", "run-1", "workspace", "tools", "target")
	out := filepath.Join(root, "ahc-plaza", "runs", "run-1", "workspace", "tools", "out")
	project := filepath.Join(root, "ahc-plaza", "runs", "run-1", "workspace", "copied-project")
	pahcerResult := filepath.Join(root, "ahc-plaza", "runs", "run-1", "workspace", "pahcer", "json", "result.json")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{project, filepath.Dir(pahcerResult)} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "cache"), []byte("large"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "0.txt"), []byte("result"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "artifact"), make([]byte, 1<<20), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pahcerResult, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	before := directorySize(filepath.Join(root, "ahc-plaza", "runs", "run-1"))
	m := &Manager{Root: root, usageCache: map[string]usageSample{"study-1": {at: time.Now(), bytes: before + 100, runs: map[string]bool{"run-1": true}}}}
	if err := m.cleanupTuningRunCache("study-1", "run-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "0.txt")); err != nil {
		t.Fatalf("output was removed: %v", err)
	}
	if _, err := os.Stat(pahcerResult); err != nil {
		t.Fatalf("pahcer result was removed: %v", err)
	}
	after := directorySize(filepath.Join(root, "ahc-plaza", "runs", "run-1"))
	if before-after < 1<<20 {
		t.Fatalf("cleanup freed only %d bytes", before-after)
	}
	if got := m.usageCache["study-1"].bytes; got != after+100 {
		t.Fatalf("usage cache = %d, want %d", got, after+100)
	}
}

func TestCleanupTuningRunCacheAccountsForWhetherRunWasAlreadyMeasured(t *testing.T) {
	for _, tc := range []struct {
		name       string
		withSample bool
		includes   bool
	}{
		{"cached before Run started", true, false},
		{"measured while Run was running", true, true},
		{"no Study cache", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runDir := filepath.Join(root, "ahc-plaza", "runs", "run-1")
			target := filepath.Join(runDir, "workspace", "tools", "target")
			for _, path := range []string{target, filepath.Join(runDir, "workspace", "tools", "out")} {
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(target, "large"), make([]byte, 4096), 0644); err != nil {
				t.Fatal(err)
			}
			before := directorySize(runDir)
			m := &Manager{Root: root}
			if tc.withSample {
				runs := map[string]bool{}
				bytes := int64(100)
				if tc.includes {
					runs["run-1"] = true
					bytes += before
				}
				m.usageCache = map[string]usageSample{"study": {at: time.Now(), bytes: bytes, runs: runs}}
			}
			if err := m.cleanupTuningRunCache("study", "run-1"); err != nil {
				t.Fatal(err)
			}
			after := directorySize(runDir)
			if tc.withSample {
				if got := m.usageCache["study"].bytes; got != 100+after {
					t.Fatalf("usage = %d, want %d", got, 100+after)
				}
			} else if _, ok := m.usageCache["study"]; ok {
				t.Fatal("cleanup unexpectedly created a Study usage sample")
			}
		})
	}
}

func TestCleanupDeletionTrashRestoresBeforeCommitAndRemovesAfterCommit(t *testing.T) {
	root := t.TempDir()
	database, err := store.OpenSQLite(filepath.Join(root, "ahc-plaza", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveStudy(context.Background(), domain.TuningStudy{ID: "study"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveRun(context.Background(), domain.Run{ID: "run", TuningStudy: "study", Status: domain.RunSucceeded, CreatedAt: time.Now(), StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	suffix := ".deleting-20260913-010203.123456789-abcdef12"
	originals := []string{filepath.Join(root, "ahc-plaza", "tuning", "study"), filepath.Join(root, "ahc-plaza", "runs", "run")}
	for _, original := range originals {
		if err := os.MkdirAll(original+suffix, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupDeletionTrash(root, database); err != nil {
		t.Fatal(err)
	}
	for _, original := range originals {
		if _, err := os.Stat(original); err != nil {
			t.Fatalf("pre-commit trash was not restored: %v", err)
		}
		if err := os.Rename(original, original+suffix); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.DeleteTuningStudy(context.Background(), "study"); err != nil {
		t.Fatal(err)
	}
	if err := cleanupDeletionTrash(root, database); err != nil {
		t.Fatal(err)
	}
	for _, original := range originals {
		if _, err := os.Stat(original + suffix); !os.IsNotExist(err) {
			t.Fatalf("post-commit trash remains: %v", err)
		}
	}
}

func TestCleanupDeletionTrashPreservesCollision(t *testing.T) {
	root := t.TempDir()
	database, err := store.OpenSQLite(filepath.Join(root, "ahc-plaza", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveStudy(context.Background(), domain.TuningStudy{ID: "study"}); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(root, "ahc-plaza", "tuning", "study")
	trash := original + ".deleting-20260913-010203.123456789-abcdef12"
	for _, path := range []string{original, trash} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupDeletionTrash(root, database); err == nil {
		t.Fatal("restore collision was accepted")
	}
	if _, err := os.Stat(trash); err != nil {
		t.Fatalf("colliding trash was removed: %v", err)
	}
}

func TestCleanupTuningRunCacheRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	target := filepath.Join(outside, "tools", "target")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{Root: root}
	if err := m.cleanupTuningRunCache("study", "../outside"); err == nil {
		t.Fatal("unsafe run ID was accepted")
	}
	runDir := filepath.Join(root, "ahc-plaza", "runs", "run-1")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(runDir, "workspace")); err != nil {
		t.Fatal(err)
	}
	if err := m.cleanupTuningRunCache("study", "run-1"); err == nil {
		t.Fatal("symlinked workspace was accepted")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("outside marker was touched: %v", err)
	}
}

func TestEvaluateCleansCacheOnEarlyFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ahc-plaza", "runs", "run-1", "workspace", "tools", "target")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manager{Root: root}
	if _, err := m.evaluate(context.Background(), domain.TuningStudy{ID: "study-1"}, Manifest{}, "run-1", nil); err == nil {
		t.Fatal("missing fixed source was accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target remains after evaluation failure: %v", err)
	}
}

func TestCleanupSavedTuningRunCachesIncludesBaselineAndRecoveredTrials(t *testing.T) {
	root := t.TempDir()
	for _, runID := range []string{"baseline", "completed", "interrupted"} {
		target := filepath.Join(root, "ahc-plaza", "runs", runID, "workspace", "tools", "target")
		if err := os.MkdirAll(target, 0755); err != nil {
			t.Fatal(err)
		}
	}
	m := &Manager{Root: root}
	m.cleanupSavedTuningRunCaches("study", "baseline", []domain.TuningTrial{
		{RunID: "completed", Status: "COMPLETE"},
		{RunID: "interrupted", Status: "RUNNING"},
	})
	for _, runID := range []string{"baseline", "completed", "interrupted"} {
		target := filepath.Join(root, "ahc-plaza", "runs", runID, "workspace", "tools", "target")
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("%s target remains: %v", runID, err)
		}
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
