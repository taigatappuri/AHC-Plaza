package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteInitDoctorAndRun(t *testing.T) {
	root := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	if err := execute([]string{"init", "--problem", "ahc000", "--objective", "max"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ahc-plaza", "inputs", "cases"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "solver", "hoge.cpp"), []byte("int main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ahc-plaza", "inputs", "cases", "case.txt"), []byte("case\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pahcer_config.toml"), []byte(`[problem]
score_regex = "Score: ([0-9]+)"

[test]
start_seed = 0
end_seed = 1
threads = 1

[[test_steps]]
program = "solver"
stdin = "tools/in/{SEED}.txt"
measure_time = true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	fakePahcer := filepath.Join(root, "fake-pahcer")
	if err := os.WriteFile(fakePahcer, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{
		"run",
		"--solver", "solver/hoge.cpp",
		"--input-dir", "ahc-plaza/inputs/cases",
		"--pahcer", fakePahcer,
		"--setting-file", "pahcer_config.toml",
		"--json",
	}); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{"doctor"}); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteRunRequiresSolver(t *testing.T) {
	if err := execute([]string{"run"}); err == nil {
		t.Fatal("solverなしの実行を許可しました")
	}
}

func TestRunCaseCommandTimesOut(t *testing.T) {
	var stderr bytes.Buffer
	started := time.Now()
	err := runCaseCommand(context.Background(), []string{"sh", "-c", "sleep 10"}, 30*time.Millisecond, nil, &bytes.Buffer{}, &stderr)
	var commandErr *commandError
	if !errors.As(err, &commandErr) || commandErr.Code != 124 {
		t.Fatalf("error = %#v, want exit code 124", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("ケースのタイムアウト後もプロセスが残っています")
	}
}

func TestRunCaseCommandCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	started := time.Now()
	err := runCaseCommand(ctx, []string{"sh", "-c", "sleep 10"}, time.Second, nil, &bytes.Buffer{}, &bytes.Buffer{})
	var commandErr *commandError
	if !errors.As(err, &commandErr) || commandErr.Code != 130 {
		t.Fatalf("error = %#v, want exit code 130", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("キャンセル後もケースプロセスが残っています")
	}
}

func TestRunCaseCommandForwardsArgumentsAndStandardIO(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runCaseCommand(
		context.Background(),
		[]string{"sh", "-c", `read value; printf 'out:%s:%s' "$value" "$1"; printf 'err:%s' "$1" >&2`, "sh", "argument"},
		time.Second,
		strings.NewReader("input\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "out:input:argument" || stderr.String() != "err:argument" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestExecuteCaseValidatesArgumentsAndRunsCommand(t *testing.T) {
	for _, args := range [][]string{nil, {"--timeout-ms", "1"}} {
		var commandErr *commandError
		if err := executeCase(args); !errors.As(err, &commandErr) || commandErr.Code != 2 {
			t.Fatalf("args = %#v, error = %#v, want exit code 2", args, err)
		}
	}
	if err := executeCase([]string{"--timeout-ms", "1", "--", "true"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunCaseCommandValidatesAndPropagatesExitCode(t *testing.T) {
	for _, test := range []struct {
		name      string
		arguments []string
		timeout   time.Duration
		wantCode  int
	}{
		{name: "commandなし", timeout: time.Second, wantCode: 2},
		{name: "timeoutなし", arguments: []string{"true"}, wantCode: 2},
		{name: "終了コード", arguments: []string{"sh", "-c", "exit 7"}, timeout: time.Second, wantCode: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			var commandErr *commandError
			err := runCaseCommand(context.Background(), test.arguments, test.timeout, nil, &bytes.Buffer{}, &bytes.Buffer{})
			if !errors.As(err, &commandErr) || commandErr.Code != test.wantCode {
				t.Fatalf("error = %#v, want exit code %d", err, test.wantCode)
			}
		})
	}
}

func TestExecuteUninstallRemovesOnlyBinary(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "ahc-plaza")
	if err := os.WriteFile(target, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	projectData := filepath.Join(directory, "ahc-plaza.toml")
	if err := os.WriteFile(projectData, []byte("project"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{"uninstall", "--install-dir", directory}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("ソースファイルが残っています: %v", err)
	}
	if _, err := os.Stat(projectData); err != nil {
		t.Fatalf("プロジェクトファイルまで削除されました: %v", err)
	}
}

func TestServeGUIShutsDownWhenContextIsCancelled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	shutdownStarted := make(chan struct{})
	serveDone := make(chan error, 1)
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() {
		serveDone <- serveGUI(ctx, httpServer, listener, func() { close(shutdownStarted) })
	}()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	_ = response.Body.Close()
	cancel()

	select {
	case <-shutdownStarted:
	case <-time.After(time.Second):
		t.Fatal("終了処理が開始されませんでした")
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTPサーバーが終了しませんでした")
	}
}
