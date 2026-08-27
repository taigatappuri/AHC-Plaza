package usecase

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/pahcer"
)

func TestBuildCaseResultsDistinguishesTLEFromWA(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	resultDir := filepath.Join(workspaceDir, "pahcer", "json")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
  "wa_seeds": [1, 2],
  "cases": [
    {"seed": 0, "score": 100, "execution_time": 0.1, "error_message": ""},
    {"seed": 1, "score": 200, "execution_time": 2.0, "error_message": "Failed to run (exit status: 124). command: ahc-plaza case-exec"},
    {"seed": 2, "score": 300, "execution_time": 0.2, "error_message": "WA"}
  ]
}`
	if err := os.WriteFile(filepath.Join(resultDir, "result_0001.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	inputCases := []domain.InputCase{
		{ID: "0000", Path: filepath.Join(root, "in", "0000.txt")},
		{ID: "0001", Path: filepath.Join(root, "in", "0001.txt")},
		{ID: "0002", Path: filepath.Join(root, "in", "0002.txt")},
	}
	results, err := buildCaseResults(root, "run-1", pahcer.Workspace{Dir: workspaceDir}, inputCases, -1)
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses := []string{"succeeded", "tle", "wa"}
	for index, wantStatus := range wantStatuses {
		if results[index].Status != wantStatus {
			t.Fatalf("case %d status = %q, want %q", index, results[index].Status, wantStatus)
		}
	}
	if results[0].Score != 100 || results[1].Score != -1 || results[2].Score != -1 {
		t.Fatalf("scores = [%v, %v, %v]", results[0].Score, results[1].Score, results[2].Score)
	}
}
