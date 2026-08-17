package inputfeature

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerCompilesAndEvaluatesFeature(t *testing.T) {
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("g++がないためスキップします")
	}
	root := t.TempDir()
	featureDir := filepath.Join(root, "ahc-plaza", "features")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `#include <iostream>
#include <vector>
using namespace std;
int main() {
    int N;
    cin >> N;
    vector<int> A(N);
    double sum = 0;
    for (int &value : A) { cin >> value; sum += value; }
    cout << sum / N << '\n';
}`
	if err := os.WriteFile(filepath.Join(featureDir, "average.cpp"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(root, "input.txt")
	if err := os.WriteFile(inputPath, []byte("4\n2 4 6 8\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner, err := NewRunner(root)
	if err != nil {
		t.Fatal(err)
	}
	program, cached, err := runner.Prepare(context.Background(), "features/average.cpp", 2000)
	if err != nil {
		t.Fatal(err)
	}
	if cached {
		t.Fatal("初回コンパイルがキャッシュ扱いになりました")
	}
	value, _, err := program.EvaluateFile(context.Background(), inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if value != 5 {
		t.Fatalf("value = %v, want 5", value)
	}
	_, cached, err = runner.Prepare(context.Background(), "features/average.cpp", 2000)
	if err != nil || !cached {
		t.Fatalf("cached = %v, err = %v", cached, err)
	}
}

func TestRunnerRejectsInvalidOutputAndCompileError(t *testing.T) {
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("g++がないためスキップします")
	}
	root := t.TempDir()
	featureDir := filepath.Join(root, "ahc-plaza", "features")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "invalid-output.cpp"), []byte(`#include <iostream>
int main() { std::cout << "1 2\n"; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "compile-error.cpp"), []byte(`int main( {`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner, err := NewRunner(root)
	if err != nil {
		t.Fatal(err)
	}
	program, _, err := runner.Prepare(context.Background(), "features/invalid-output.cpp", 2000)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input.txt")
	if err := os.WriteFile(input, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := program.EvaluateFile(context.Background(), input); err == nil || !strings.Contains(err.Error(), "数値を1個") {
		t.Fatalf("invalid output error = %v", err)
	}
	if _, _, err := runner.Prepare(context.Background(), "features/compile-error.cpp", 2000); err == nil || !strings.Contains(err.Error(), "コンパイル") {
		t.Fatalf("compile error = %v", err)
	}
}
