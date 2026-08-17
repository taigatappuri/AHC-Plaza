package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeCreatesProjectFiles(t *testing.T) {
	root := t.TempDir()
	if err := Initialize(root, "ahc000", "max"); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "ahc-plaza.toml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "[input_format]") || !strings.Contains(string(content), "timeout_seconds = 300") {
		t.Fatalf("初期設定が不正です: %s", content)
	}
	for _, directory := range []string{"solver", "ahc-plaza/runs", "ahc-plaza/inputs", "ahc-plaza/features"} {
		if info, err := os.Stat(filepath.Join(root, directory)); err != nil || !info.IsDir() {
			t.Fatalf("%sが作成されていません: %v", directory, err)
		}
	}
	if err := Initialize(root, "ahc000", "max"); err == nil {
		t.Fatal("既存の設定を上書きできてしまいました")
	}
}
