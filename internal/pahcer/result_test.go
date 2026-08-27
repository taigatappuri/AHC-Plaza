package pahcer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLatestResult(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "pahcer", "json")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"case_count":2,"wa_seeds":[1],"cases":[{"seed":0,"score":42,"execution_time":0.125},{"seed":1,"score":999,"execution_time":1.5,"error_message":"WA"}]}`
	if err := os.WriteFile(filepath.Join(directory, "result_0001.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := LoadLatestResult(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.WrongAnswerSeeds) != 1 || result.Cases[1].Score != 999 {
		t.Fatalf("result = %#v", result)
	}
	if got := ExecutionDuration(result.Cases[0].ExecutionTime).Milliseconds(); got != 125 {
		t.Fatalf("duration = %dms", got)
	}
}

func TestResultCaseTimedOut(t *testing.T) {
	timeout := ResultCase{ErrorMessage: `Failed to run (exit status: 124). command: "ahc-plaza" "case-exec"`}
	if !timeout.TimedOut() {
		t.Fatal("タイムアウト用終了コードをTLEとして検出できませんでした")
	}
	failed := ResultCase{ErrorMessage: `Failed to run (exit status: 1). command: "ahc-plaza" "case-exec"`}
	if failed.TimedOut() {
		t.Fatal("通常の実行エラーをTLEとして検出しました")
	}
}
