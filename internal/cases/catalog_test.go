package cases

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSortsAndIgnoresUnsupportedEntries(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"0002.txt": "two",
		"0000.txt": "zero",
		".hidden":  "hidden",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("case count = %d, want 2", len(got))
	}
	if got[0].ID != "0000.txt" || got[1].ID != "0002.txt" {
		t.Fatalf("case order = %#v", []string{got[0].ID, got[1].ID})
	}
	if got[0].SHA256 == "" || got[1].SHA256 == "" {
		t.Fatal("ケースのハッシュが空です")
	}
}

func TestDiscoverRejectsSymlinkAsCase(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinkを作成できない環境です: %v", err)
	}

	got, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("symlinkを入力ケースとして扱いました: %#v", got)
	}
}

func TestHashListIsOrderSensitiveByCaseIDOrder(t *testing.T) {
	first, err := Discover(writeCases(t, map[string]string{"a": "1", "b": "2"}))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Discover(writeCases(t, map[string]string{"a": "1", "b": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	if HashList(first) == HashList(second) {
		t.Fatal("内容が異なるケース一覧で同じハッシュになりました")
	}
}

func writeCases(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
