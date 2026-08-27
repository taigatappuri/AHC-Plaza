package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/project"
	"github.com/taigatappuri/AHC-Plaza/internal/server"
	"github.com/taigatappuri/AHC-Plaza/internal/usecase"
)

var version = "0.1.0-dev"

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		var commandErr *commandError
		if errors.As(err, &commandErr) {
			os.Exit(commandErr.Code)
		}
		os.Exit(1)
	}
}

type commandError struct {
	Code int
	Err  error
}

func (e *commandError) Error() string { return e.Err.Error() }

func (e *commandError) Unwrap() error { return e.Err }

func execute(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return nil
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println(version)
		return nil
	}

	switch args[0] {
	case "case-exec":
		return executeCase(args[1:])
	case "init":
		return executeInit(args[1:])
	case "doctor":
		return executeDoctor(args[1:])
	case "run":
		return executeRun(args[1:])
	case "gui":
		return executeGUI(args[1:])
	case "update":
		return executeUpdate(args[1:])
	case "uninstall":
		return executeUninstall(args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func executeUninstall(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	defaultDir := os.Getenv("AHC_PLAZA_INSTALL_DIR")
	if defaultDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine the home directory: %w", err)
		}
		defaultDir = filepath.Join(home, ".local", "bin")
	}
	installDir := flags.String("install-dir", defaultDir, "AHC Plaza installation directory")
	if err := flags.Parse(args); err != nil {
		return &commandError{Code: 2, Err: err}
	}
	if *installDir == "" {
		return &commandError{Code: 2, Err: fmt.Errorf("the installation directory must not be empty")}
	}
	target := filepath.Join(*installDir, "ahc-plaza")
	if err := os.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("AHC Plaza is not installed: %s\n", target)
			return nil
		}
		return fmt.Errorf("could not uninstall AHC Plaza: %w", err)
	}
	fmt.Printf("Uninstalled AHC Plaza: %s\n", target)
	fmt.Println("Project files, including ahc-plaza.toml, the ahc-plaza directory, and run results, were not removed.")
	return nil
}

func executeGUI(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return executeGUIContext(ctx, args)
}

func executeGUIContext(ctx context.Context, args []string) (resultErr error) {
	flags := flag.NewFlagSet("gui", flag.ContinueOnError)
	configPath := flags.String("config", "ahc-plaza.toml", "configuration file")
	port := flags.Int("port", 8080, "listening port")
	if err := flags.Parse(args); err != nil {
		return &commandError{Code: 2, Err: err}
	}
	if *port < 1 || *port > 65535 {
		return &commandError{Code: 2, Err: fmt.Errorf("invalid port number: %d", *port)}
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine the current directory: %w", err)
	}
	address := "127.0.0.1:" + strconv.Itoa(*port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("could not start the GUI server: %w", err)
	}
	webServer, err := server.New(root, *configPath)
	if err != nil {
		_ = listener.Close()
		return err
	}
	defer func() {
		if err := webServer.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("could not close the GUI server: %w", err))
		}
	}()
	fmt.Printf("AHC Plaza GUI: http://%s\nPress Ctrl-C to stop.\n", address)
	httpServer := &http.Server{Handler: webServer.Handler()}
	return serveGUI(ctx, httpServer, listener, webServer.BeginShutdown)
}

func serveGUI(ctx context.Context, httpServer *http.Server, listener net.Listener, beginShutdown func()) error {
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- httpServer.Serve(listener)
	}()

	select {
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("could not start the GUI server: %w", err)
		}
		return nil
	case <-ctx.Done():
		if beginShutdown != nil {
			beginShutdown()
		}
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			_ = httpServer.Close()
			<-serveErrors
			return fmt.Errorf("could not stop the GUI server gracefully: %w", err)
		}
		// Serveが起動準備中でも確実に終了できるよう、リスナーも明示的に閉じます。
		_ = listener.Close()
		if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("GUI server stopped unexpectedly: %w", err)
		}
		return nil
	}
}

func executeInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	problem := flags.String("problem", "", "problem name")
	objective := flags.String("objective", "", "max or min")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine the current directory: %w", err)
	}
	if err := project.Initialize(root, *problem, *objective); err != nil {
		return &commandError{Code: 2, Err: err}
	}
	fmt.Println("Initialized the AHC Plaza project.")
	return nil
}

func executeDoctor(args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	configPath := flags.String("config", "ahc-plaza.toml", "configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*configPath); err != nil {
		return &commandError{Code: 3, Err: fmt.Errorf("configuration file not found: %w", err)}
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return &commandError{Code: 3, Err: err}
	}
	fmt.Printf("OK: configuration %s\n", filepath.Base(cfg.FilePath))
	for _, result := range project.Check(cfg) {
		fmt.Println(result)
	}
	return nil
}

func executeRun(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	solver := flags.String("solver", "", "C++ solver file to run")
	inputDir := flags.String("input-dir", "", "input case directory")
	threads := flags.Int("threads", -1, "number of parallel workers")
	timeout := flags.Int("timeout-ms", 0, "per-case timeout in milliseconds")
	comment := flags.String("comment", "", "comment for this run")
	configPath := flags.String("config", "ahc-plaza.toml", "configuration file")
	pahcerBinary := flags.String("pahcer", "", "pahcer executable")
	settingFile := flags.String("setting-file", "", "pahcer configuration file")
	jsonOutput := flags.Bool("json", false, "output in JSON format")
	if err := flags.Parse(args); err != nil {
		return &commandError{Code: 2, Err: err}
	}
	if *solver == "" {
		return &commandError{Code: 2, Err: fmt.Errorf("--solver is required")}
	}

	summary, err := usecase.ExecuteRun(context.Background(), usecase.RunRequest{
		ConfigPath:          *configPath,
		Solver:              *solver,
		InputDir:            *inputDir,
		Threads:             *threads,
		TimeoutMilliseconds: *timeout,
		Comment:             *comment,
		PahcerBinary:        *pahcerBinary,
		SettingFile:         *settingFile,
	})
	if *jsonOutput {
		if encodeErr := json.NewEncoder(os.Stdout).Encode(summary); encodeErr != nil {
			return &commandError{Code: 4, Err: encodeErr}
		}
	} else if summary.RunID != "" {
		fmt.Printf("Run: #%d (%s)\nStatus: %s\nInput cases: %d\n", summary.RunNumber, summary.RunID, summary.Status, summary.InputCaseCount)
		fmt.Printf("Created: %s\nStarted: %s\n", summary.CreatedAt.Format(time.RFC3339Nano), summary.StartedAt.Format(time.RFC3339Nano))
		if summary.FinishedAt != nil {
			fmt.Printf("Finished: %s\n", summary.FinishedAt.Format(time.RFC3339Nano))
		}
		fmt.Printf("Source: %s\n", summary.SourceSnapshot)
	}
	if err != nil {
		return &commandError{Code: 4, Err: err}
	}
	return nil
}

func printUsage() {
	fmt.Println(`Usage:
  ahc-plaza init --problem <PROBLEM_NAME> --objective <max|min>
  ahc-plaza doctor
  ahc-plaza run [OPTIONS]
  ahc-plaza gui [OPTIONS]
  ahc-plaza update
  ahc-plaza uninstall [--install-dir <DIR>]
  ahc-plaza --version`)
}
