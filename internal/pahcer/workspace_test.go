package pahcer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func TestPrepareWorkspaceStagesSourceAndInputCases(t *testing.T) {
	root := t.TempDir()
	setting := `[problem]
score_regex = "Score: ([0-9]+)"

[test]
start_seed = 0
end_seed = 10
threads = 0

[[test_steps]]
program = "solver"
args = ["--mode", "fast"]
stdin = "tools/in/{SEED}.txt"
measure_time = true
`
	if err := os.WriteFile(filepath.Join(root, "pahcer_config.toml"), []byte(setting), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "solver.cpp")
	if err := os.WriteFile(source, []byte("int main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(root, "input")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(inputDir, "case-a.txt")
	if err := os.WriteFile(inputPath, []byte("input\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	workspace, err := PrepareWorkspace(filepath.Join(root, "run"), source, filepath.Join(root, "tools"), filepath.Join(root, "pahcer_config.toml"), []domain.InputCase{{
		ID: "case-a.txt", Path: inputPath, SHA256: "hash", Size: 6,
	}}, WorkspaceOptions{Threads: 4, CaseTimeoutMilliseconds: 75, CaseRunner: "/opt/ahc-plaza"})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.SettingFile != "pahcer_config.toml" {
		t.Fatalf("setting file = %q", workspace.SettingFile)
	}
	if _, err := os.Stat(filepath.Join(workspace.Dir, "main.cpp")); err != nil {
		t.Fatal(err)
	}
	stagedInput, err := os.ReadFile(filepath.Join(workspace.Dir, "tools", "in", "0000.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stagedInput) != "input\n" {
		t.Fatalf("staged input = %q", stagedInput)
	}
	configured, err := os.ReadFile(filepath.Join(workspace.Dir, workspace.SettingFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configured), "tools/in/{SEED04}.txt") {
		t.Fatalf("stdin path was not rewritten: %s", configured)
	}
	if !strings.Contains(string(configured), "end_seed = 1") {
		t.Fatalf("end_seed was not rewritten: %s", configured)
	}
	if !strings.Contains(string(configured), `program = "/opt/ahc-plaza"`) ||
		!strings.Contains(string(configured), `args = ["case-exec", "--timeout-ms", "75", "--", "solver", "--mode", "fast"]`) {
		t.Fatalf("case timeout was not configured: %s", configured)
	}
}

func TestPrepareWorkspaceAcceptsExamplePahcerConfig(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..", "example")
	if _, err := os.Stat(filepath.Join(projectRoot, "pahcer_config.toml")); err != nil {
		t.Skipf("exampleプロジェクトがありません: %v", err)
	}
	sourcePath := filepath.Join(projectRoot, "solver", "main.cpp")
	if _, err := os.Stat(sourcePath); err != nil {
		sourcePath = filepath.Join(projectRoot, "main.cpp")
	}
	inputCases := []domain.InputCase{{
		ID: "0000.txt", Path: filepath.Join(projectRoot, "tools", "in", "0000.txt"), SHA256: "hash", Size: 1,
	}}
	workspace, err := PrepareWorkspace(
		t.TempDir(),
		sourcePath,
		filepath.Join(projectRoot, "tools"),
		filepath.Join(projectRoot, "pahcer_config.toml"),
		inputCases,
		WorkspaceOptions{Threads: 1, CaseTimeoutMilliseconds: 2000, CaseRunner: "/opt/ahc-plaza"},
	)
	if err != nil {
		t.Fatal(err)
	}
	configured, err := os.ReadFile(filepath.Join(workspace.Dir, workspace.SettingFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configured), "tools/in/{SEED04}.txt") {
		t.Fatalf("example config was not rewritten: %s", configured)
	}
}

func TestPrepareWorkspaceOverlaysCandidateOnFixedProject(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "fixed-project")
	if err := os.MkdirAll(filepath.Join(project, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(project, "include", "lib.hpp"), []byte("header"), 0644)
	os.WriteFile(filepath.Join(project, "Makefile"), []byte("all:"), 0644)
	os.MkdirAll(filepath.Join(project, "solver"), 0755)
	os.WriteFile(filepath.Join(project, "solver", "main.cpp"), []byte("old"), 0644)
	candidate := filepath.Join(root, "candidate.cpp")
	os.WriteFile(candidate, []byte("new"), 0644)
	tools := filepath.Join(root, "tools")
	os.MkdirAll(tools, 0755)
	setting := filepath.Join(root, "pahcer.toml")
	os.WriteFile(setting, []byte("[test]\n[[test.test_steps]]\nprogram=\"./solver/main\"\nstdin=\"x\"\n"), 0644)
	input := filepath.Join(root, "input.txt")
	os.WriteFile(input, []byte("x"), 0644)
	w, err := PrepareWorkspace(filepath.Join(root, "run"), candidate, tools, setting, []domain.InputCase{{ID: "0", Path: input}}, WorkspaceOptions{Threads: 1, CaseTimeoutMilliseconds: 1, CaseRunner: "runner", ProjectDir: project, SourceTarget: "solver/main.cpp"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(w.Dir, "solver", "main.cpp"))
	if string(got) != "new" {
		t.Fatalf("candidate=%q", got)
	}
	if _, err := os.Stat(filepath.Join(w.Dir, "include", "lib.hpp")); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareWorkspaceCandidateWinsInsideTools(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	tools := filepath.Join(root, "tools")
	os.MkdirAll(project, 0755)
	os.MkdirAll(tools, 0755)
	os.WriteFile(filepath.Join(tools, "solver.cpp"), []byte("tools"), 0644)
	candidate := filepath.Join(root, "candidate.cpp")
	os.WriteFile(candidate, []byte("candidate"), 0644)
	setting := filepath.Join(root, "p.toml")
	os.WriteFile(setting, []byte("[test]\n[[test.test_steps]]\nprogram=\"./solver\"\nstdin=\"x\"\n"), 0644)
	input := filepath.Join(root, "in")
	os.WriteFile(input, []byte("x"), 0644)
	w, err := PrepareWorkspace(filepath.Join(root, "run"), candidate, tools, setting, []domain.InputCase{{ID: "0", Path: input}}, WorkspaceOptions{Threads: 1, CaseTimeoutMilliseconds: 1, CaseRunner: "runner", ProjectDir: project, SourceTarget: "tools/solver.cpp"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(w.Dir, "tools", "solver.cpp"))
	if string(got) != "candidate" {
		t.Fatalf("got %q", got)
	}
}
