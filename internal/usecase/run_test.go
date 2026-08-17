package usecase

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func TestExecuteRunCreatesSnapshotAndPersistsSuccess(t *testing.T) {
	root := newTestProject(t, "exit 0")
	solver := filepath.Join(root, "solver", "hoge.cpp")
	if err := os.WriteFile(solver, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakePahcer := filepath.Join(root, "fake-pahcer")

	summary, err := ExecuteRun(context.Background(), RunRequest{
		ConfigPath:     filepath.Join(root, "ahc-plaza.toml"),
		Solver:         "solver/hoge.cpp",
		InputDir:       "ahc-plaza/inputs/cases",
		Threads:        1,
		TimeoutSeconds: 2,
		PahcerBinary:   fakePahcer,
		SettingFile:    "pahcer_config.toml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Status != domain.RunSucceeded || summary.InputCaseCount != 2 || summary.RunNumber != 1 || summary.CreatedAt.IsZero() || summary.StartedAt.IsZero() || summary.FinishedAt == nil {
		t.Fatalf("summary = %#v", summary)
	}
	snapshotPath := filepath.Join(root, summary.SourceSnapshot)
	if err := os.WriteFile(solver, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(snapshot) != "before\n" {
		t.Fatalf("snapshot = %q", snapshot)
	}
	if _, err := os.Stat(filepath.Join(root, "ahc-plaza", "ahc-plaza.db")); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteRunPersistsFailure(t *testing.T) {
	root := newTestProject(t, "exit 7")
	summary, err := ExecuteRun(context.Background(), RunRequest{
		ConfigPath:     filepath.Join(root, "ahc-plaza.toml"),
		Solver:         "solver/hoge.cpp",
		InputDir:       "ahc-plaza/inputs/cases",
		Threads:        1,
		TimeoutSeconds: 2,
		PahcerBinary:   filepath.Join(root, "fake-pahcer"),
		SettingFile:    "pahcer_config.toml",
	})
	if err == nil {
		t.Fatal("失敗したpahcerを成功扱いにしました")
	}
	if summary.Status != domain.RunFailed || summary.ExitCode != 7 {
		t.Fatalf("summary = %#v", summary)
	}
}

func newTestProject(t *testing.T, command string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"solver", "ahc-plaza/inputs/cases"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := `[project]
problem = "ahc000"
objective = "max"

[paths]
solver_dir = "solver"
tools_dir = "tools"

[execution]
default_input_dir = "ahc-plaza/inputs"
threads = 1
timeout_seconds = 2

[pahcer]
setting_file = "pahcer_config.toml"

[score]
invalid_score = 0.0
include_invalid_cases = true

[statistics]
confidence_level = 0.95
bootstrap_iterations = 10
`
	if err := os.WriteFile(filepath.Join(root, "ahc-plaza.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	setting := `[problem]
score_regex = "Score: ([0-9]+)"

[test]
start_seed = 0
end_seed = 2
threads = 1

[[test_steps]]
program = "solver"
stdin = "tools/in/{SEED}.txt"
measure_time = true
`
	if err := os.WriteFile(filepath.Join(root, "pahcer_config.toml"), []byte(setting), 0o644); err != nil {
		t.Fatal(err)
	}
	for index, content := range []string{"case0\n", "case1\n"} {
		if err := os.WriteFile(filepath.Join(root, "ahc-plaza/inputs/cases", string(rune('0'+index))+".txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "solver", "hoge.cpp"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" + command + "\n"
	if err := os.WriteFile(filepath.Join(root, "fake-pahcer"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}
