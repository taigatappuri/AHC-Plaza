package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type Status string

const (
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusTimeout   Status = "timeout"
	StatusCancelled Status = "cancelled"
)

type Request struct {
	Command    []string
	Dir        string
	Env        []string
	StdoutPath string
	StderrPath string
	Timeout    time.Duration
}

type Result struct {
	Status     Status
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
	Error      error
}

func Run(ctx context.Context, request Request) (Result, error) {
	if len(request.Command) == 0 {
		return Result{}, errors.New("execution command is empty")
	}
	if request.Timeout < 0 {
		return Result{}, errors.New("timeout must not be negative")
	}
	if request.StdoutPath == "" || request.StderrPath == "" {
		return Result{}, errors.New("stdout and stderr destinations are required")
	}
	if err := os.MkdirAll(filepath.Dir(request.StdoutPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("could not create stdout directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(request.StderrPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("could not create stderr directory: %w", err)
	}

	stdout, err := os.Create(request.StdoutPath)
	if err != nil {
		return Result{}, fmt.Errorf("could not create stdout file: %w", err)
	}
	defer stdout.Close()
	stderr, err := os.Create(request.StderrPath)
	if err != nil {
		return Result{}, fmt.Errorf("could not create stderr file: %w", err)
	}
	defer stderr.Close()

	runContext := ctx
	cancel := func() {}
	if request.Timeout > 0 {
		runContext, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	defer cancel()
	command := exec.CommandContext(runContext, request.Command[0], request.Command[1:]...)
	command.Dir = request.Dir
	command.Env = append(os.Environ(), request.Env...)
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		killProcessGroup(command.Process)
		return nil
	}

	startedAt := time.Now().UTC()
	err = command.Run()
	result := Result{Status: StatusSucceeded, StartedAt: startedAt, FinishedAt: time.Now().UTC(), Error: err}
	switch {
	case ctx.Err() != nil:
		result.Status = StatusCancelled
		result.Error = ctx.Err()
	case request.Timeout > 0 && errors.Is(runContext.Err(), context.DeadlineExceeded):
		result.Status = StatusTimeout
		result.Error = context.DeadlineExceeded
	case err != nil:
		result.Status = StatusFailed
	}
	result.ExitCode = exitCode(result.Error)
	return result, nil
}

func killProcessGroup(process *os.Process) {
	if process == nil {
		return
	}
	if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err != nil {
		_ = process.Kill()
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}
