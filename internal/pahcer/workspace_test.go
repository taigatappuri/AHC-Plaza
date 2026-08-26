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
