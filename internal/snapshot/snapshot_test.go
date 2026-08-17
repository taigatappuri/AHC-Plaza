package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatePreservesSourceAtRunStart(t *testing.T) {
	root := t.TempDir()
	solverDir := filepath.Join(root, "solver")
	if err := os.MkdirAll(solverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(solverDir, "hoge.cpp")
	if err := os.WriteFile(source, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, "ahc-plaza", "runs", "run-1")

	snapshot, err := Create(root, source, runDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(snapshot.SnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "before\n" {
		t.Fatalf("snapshot = %q, want original source", content)
	}
	if snapshot.SHA256 == "" {
		t.Fatal("snapshot hash is empty")
	}
}

func TestCreateRejectsSourceOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.cpp")
	if err := os.WriteFile(outside, []byte("int main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Create(root, outside, filepath.Join(root, "ahc-plaza", "runs", "run-1"))
	if err == nil {
		t.Fatal("プロジェクト外のsolverを受け入れました")
	}
}

func TestCreateRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	solverDir := filepath.Join(root, "solver")
	if err := os.MkdirAll(solverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.cpp")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(solverDir, "link.cpp")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Skipf("symlinkを作成できない環境です: %v", err)
	}
	if _, err := Create(root, symlink, filepath.Join(root, "ahc-plaza", "runs", "run-1")); err == nil {
		t.Fatal("symlinkをsolverとして受け入れました")
	}
}
