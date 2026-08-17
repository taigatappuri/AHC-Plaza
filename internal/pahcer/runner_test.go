package pahcer

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/process"
)

func TestRunnerDelegatesToProcessExecutor(t *testing.T) {
	executor := &fakeExecutor{}
	runner := Runner{Binary: "pahcer", RunProcess: executor.Run}
	result, err := runner.Run(context.Background(), Request{
		WorkingDir:  "/tmp/run",
		SettingFile: "pahcer_config.toml",
		StdoutPath:  "/tmp/run/stdout.log",
		StderrPath:  "/tmp/run/stderr.log",
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != process.StatusSucceeded {
		t.Fatalf("status = %q", result.Status)
	}
	want := []string{"pahcer", "run", "--json", "--setting-file", "pahcer_config.toml"}
	if !reflect.DeepEqual(executor.request.Command, want) {
		t.Fatalf("command = %#v, want %#v", executor.request.Command, want)
	}
}

type fakeExecutor struct {
	request process.Request
}

func (f *fakeExecutor) Run(_ context.Context, request process.Request) (process.Result, error) {
	f.request = request
	return process.Result{Status: process.StatusSucceeded}, nil
}
