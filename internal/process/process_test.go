package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutput(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux専用のプロセス制御テストです")
	}
	runDir := t.TempDir()
	result, err := Run(context.Background(), Request{
		Command:    []string{"sh", "-c", "printf stdout; printf stderr >&2"},
		Dir:        runDir,
		StdoutPath: filepath.Join(runDir, "stdout.log"),
		StderrPath: filepath.Join(runDir, "stderr.log"),
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusSucceeded || result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	stdout, err := os.ReadFile(filepath.Join(runDir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(filepath.Join(runDir, "stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "stdout" || string(stderr) != "stderr" {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestRunTimesOutAndKillsProcessGroup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux専用のプロセス制御テストです")
	}
	runDir := t.TempDir()
	started := time.Now()
	result, err := Run(context.Background(), Request{
		Command:    []string{"sh", "-c", "sleep 10"},
		Dir:        runDir,
		StdoutPath: filepath.Join(runDir, "stdout.log"),
		StderrPath: filepath.Join(runDir, "stderr.log"),
		Timeout:    30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusTimeout {
		t.Fatalf("status = %q, want timeout", result.Status)
	}
	if time.Since(started) > time.Second {
		t.Fatal("timeout後もプロセスが長時間残っています")
	}
}

func TestRunCanBeCancelled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux専用のプロセス制御テストです")
	}
	runDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan Result, 1)
	go func() {
		result, err := Run(ctx, Request{
			Command:    []string{"sh", "-c", "sleep 10"},
			Dir:        runDir,
			StdoutPath: filepath.Join(runDir, "stdout.log"),
			StderrPath: filepath.Join(runDir, "stderr.log"),
			Timeout:    time.Second,
		})
		if err != nil {
			t.Errorf("Run error: %v", err)
		}
		resultCh <- result
	}()
	cancel()
	select {
	case result := <-resultCh:
		if result.Status != StatusCancelled {
			t.Fatalf("status = %q, want cancelled", result.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("キャンセル後に実行が終了しません")
	}
}

func TestRunReturnsFailedForNonZeroExit(t *testing.T) {
	runDir := t.TempDir()
	result, err := Run(context.Background(), Request{
		Command:    []string{"sh", "-c", "printf failure >&2; exit 7"},
		Dir:        runDir,
		StdoutPath: filepath.Join(runDir, "stdout.log"),
		StderrPath: filepath.Join(runDir, "stderr.log"),
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || result.ExitCode != 7 {
		t.Fatalf("result = %#v", result)
	}
	stderr, err := os.ReadFile(filepath.Join(runDir, "stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stderr), "failure") {
		t.Fatalf("stderr = %q", stderr)
	}
}
