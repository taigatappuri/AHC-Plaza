package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func executeCase(args []string) error {
	flags := flag.NewFlagSet("case-exec", flag.ContinueOnError)
	timeoutMilliseconds := flags.Int("timeout-ms", 0, "per-case timeout in milliseconds")
	if err := flags.Parse(args); err != nil {
		return &commandError{Code: 2, Err: err}
	}
	if *timeoutMilliseconds < 1 {
		return &commandError{Code: 2, Err: errors.New("--timeout-ms must be at least 1 millisecond")}
	}
	command := flags.Args()
	if len(command) == 0 {
		return &commandError{Code: 2, Err: errors.New("case command is required")}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runCaseCommand(ctx, command, time.Duration(*timeoutMilliseconds)*time.Millisecond, os.Stdin, os.Stdout, os.Stderr)
}

func runCaseCommand(ctx context.Context, arguments []string, timeout time.Duration, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(arguments) == 0 {
		return &commandError{Code: 2, Err: errors.New("case command is required")}
	}
	if timeout <= 0 {
		return &commandError{Code: 2, Err: errors.New("case timeout must be positive")}
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runContext, arguments[0], arguments[1:]...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			_ = command.Process.Kill()
		}
		return nil
	}
	err := command.Run()
	if errors.Is(runContext.Err(), context.DeadlineExceeded) {
		return &commandError{Code: 124, Err: fmt.Errorf("case timed out after %s", timeout)}
	}
	if ctx.Err() != nil {
		return &commandError{Code: 130, Err: ctx.Err()}
	}
	if err == nil {
		return nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return &commandError{Code: exitError.ExitCode(), Err: err}
	}
	return &commandError{Code: 1, Err: err}
}
