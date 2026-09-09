package tuning

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/pahcer"
	"github.com/taigatappuri/AHC-Plaza/internal/store"
)

// 合成solverによる工程別の測定です。実問題の探索時間を予測するものではありません。
func BenchmarkTuningPhases(b *testing.B) {
	compiler, err := exec.LookPath("g++")
	if err != nil {
		b.Skip("g++ unavailable")
	}
	for _, fixture := range []struct {
		name      string
		functions int
		cases     int
		toolBytes int
	}{
		{"small", 1, 2, 1024},
		{"large", 1500, 100, 16 << 20},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			root := b.TempDir()
			write := func(path string, content []byte) {
				b.Helper()
				if err := os.WriteFile(path, content, 0600); err != nil {
					b.Fatal(err)
				}
			}
			var source strings.Builder
			source.WriteString("#include <cstdio>\nconstexpr int WIDTH=10; // @tune 1 100\n")
			for i := 0; i < fixture.functions; i++ {
				fmt.Fprintf(&source, "__attribute__((noinline)) unsigned f%d(unsigned x){for(int j=0;j<WIDTH;j++)x=x*1664525u+%du;return x;}\n", i, i+1)
			}
			source.WriteString("int main(){unsigned x; if(scanf(\"%u\",&x)!=1)return 1;")
			for i := 0; i < fixture.functions; i++ {
				fmt.Fprintf(&source, "x=f%d(x);", i)
			}
			source.WriteString("printf(\"%u\\n\",x);}\n")
			solver := filepath.Join(root, "main.cpp")
			write(solver, []byte(source.String()))
			tools := filepath.Join(root, "tools")
			if err := os.Mkdir(tools, 0700); err != nil {
				b.Fatal(err)
			}
			write(filepath.Join(tools, "fixture.dat"), make([]byte, fixture.toolBytes))
			input := filepath.Join(root, "input.txt")
			write(input, []byte("42\n"))
			inputs := make([]domain.InputCase, fixture.cases)
			for i := range inputs {
				inputs[i] = domain.InputCase{ID: fmt.Sprint(i), Path: input, Size: 3}
			}
			setting := filepath.Join(root, "pahcer.toml")
			write(setting, []byte("[test]\n[[test.test_steps]]\nprogram=\"./main.exe\"\nargs=[]\nstdin=\"input.txt\"\n"))
			b.Run("workspace", func(b *testing.B) {
				runs := b.TempDir()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					run := filepath.Join(runs, fmt.Sprintf("run-%d", n))
					if _, err := pahcer.PrepareWorkspace(run, solver, tools, setting, inputs, pahcer.WorkspaceOptions{Threads: 1, CaseTimeoutMilliseconds: 1000, CaseRunner: "/usr/bin/true"}); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(source.Len()+fixture.toolBytes+3*fixture.cases), "staged-B/run")
			})
			binary := filepath.Join(root, "main.exe")
			b.Run("compile", func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					if output, err := exec.Command(compiler, "-std=c++17", "-O2", solver, "-o", binary).CombinedOutput(); err != nil {
						b.Fatalf("%v: %s", err, output)
					}
				}
			})
			b.Run("cases", func(b *testing.B) {
				if _, err := os.Stat(binary); err != nil {
					if output, err := exec.Command(compiler, "-std=c++17", "-O2", solver, "-o", binary).CombinedOutput(); err != nil {
						b.Fatalf("%v: %s", err, output)
					}
				}
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					for range inputs {
						cmd := exec.Command(binary)
						cmd.Stdin = strings.NewReader("42\n")
						if err := cmd.Run(); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
			b.Run("database", func(b *testing.B) {
				db, err := store.OpenSQLite(filepath.Join(b.TempDir(), "bench.db"))
				if err != nil {
					b.Fatal(err)
				}
				defer db.Close()
				b.ResetTimer()
				ctx := context.Background()
				for n := 0; n < b.N; n++ {
					id := fmt.Sprint(n)
					if err := db.SaveRun(ctx, domain.Run{ID: id, Status: domain.RunRunning, StartedAt: time.Now()}); err != nil {
						b.Fatal(err)
					}
					results := make([]domain.CaseResult, len(inputs))
					for i := range results {
						results[i] = domain.CaseResult{RunID: id, InputCaseID: fmt.Sprint(i), Seed: uint64(i), Score: 42, Status: "succeeded"}
					}
					if err := db.FinalizeRun(ctx, id, domain.RunSucceeded, time.Now(), results); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
