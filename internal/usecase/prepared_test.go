package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotTuningProjectExcludesGeneratedAndPreservesExecutable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"include/a.hpp", "scripts/build.sh", "Makefile", ".git/config", "ahc-plaza/data", "build/output", "target/output", "dist/output", "tools/kept", "custom-tools/large"} {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(name), 0644)
	}
	os.Chmod(filepath.Join(root, "scripts/build.sh"), 0755)
	dst := filepath.Join(t.TempDir(), "project")
	if err := SnapshotTuningProject(context.Background(), root, dst, SnapshotLimits{MaxFiles: 10, MaxBytes: 1024, Exclude: []string{"custom-tools"}}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dst, "scripts/build.sh")); err != nil || info.Mode()&0111 == 0 {
		t.Fatalf("mode=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "tools", "kept")); err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{".git", "ahc-plaza", "build", "target", "dist", "custom-tools"} {
		if _, err := os.Stat(filepath.Join(dst, excluded)); !os.IsNotExist(err) {
			t.Fatalf("%s was copied", excluded)
		}
	}
}

func TestSnapshotTuningProjectRejectsSymlinkAndLimits(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("x"), 0644)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := SnapshotTuningProject(context.Background(), root, filepath.Join(t.TempDir(), "dst"), SnapshotLimits{MaxFiles: 10, MaxBytes: 10}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("got %v", err)
	}
	os.Remove(filepath.Join(root, "link"))
	os.WriteFile(filepath.Join(root, "large"), []byte("12345"), 0644)
	if err := SnapshotTuningProject(context.Background(), root, filepath.Join(t.TempDir(), "dst2"), SnapshotLimits{MaxFiles: 10, MaxBytes: 4}); err == nil || !strings.Contains(err.Error(), "容量") {
		t.Fatalf("got %v", err)
	}
}

func TestCopyPreparedTreeRejectsDestinationInsideSource(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0644)
	if err := copyPreparedTree(context.Background(), root, filepath.Join(root, "nested")); err == nil {
		t.Fatal("expected recursive destination rejection")
	}
}
