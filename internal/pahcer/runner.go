package pahcer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/process"
)

type Runner struct {
	Binary     string
	RunProcess func(context.Context, process.Request) (process.Result, error)
}

type Request struct {
	WorkingDir  string
	SettingFile string
	StdoutPath  string
	StderrPath  string
	Timeout     time.Duration
}

func NewRunner(binary string) Runner {
	return Runner{Binary: binary, RunProcess: process.Run}
}

func (r Runner) Run(ctx context.Context, request Request) (process.Result, error) {
	if r.Binary == "" {
		return process.Result{}, errors.New("pahcer executable is required")
	}
	if request.WorkingDir == "" {
		return process.Result{}, errors.New("pahcer working directory is required")
	}
	if request.StdoutPath == "" {
		request.StdoutPath = filepath.Join(request.WorkingDir, "stdout.log")
	}
	if request.StderrPath == "" {
		request.StderrPath = filepath.Join(request.WorkingDir, "stderr.log")
	}
	command := []string{r.Binary, "run", "--json", "--setting-file", request.SettingFile}
	result, err := r.RunProcess(ctx, process.Request{
		Command:    command,
		Dir:        request.WorkingDir,
		StdoutPath: request.StdoutPath,
		StderrPath: request.StderrPath,
		Timeout:    request.Timeout,
	})
	if err != nil {
		return process.Result{}, fmt.Errorf("could not execute pahcer: %w", err)
	}
	return result, nil
}
