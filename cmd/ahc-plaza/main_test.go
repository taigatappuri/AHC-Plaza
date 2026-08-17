package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
