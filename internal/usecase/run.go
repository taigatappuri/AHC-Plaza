package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/cases"
	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/pahcer"
	"github.com/taigatappuri/AHC-Plaza/internal/process"
	"github.com/taigatappuri/AHC-Plaza/internal/snapshot"
	"github.com/taigatappuri/AHC-Plaza/internal/store"
)

func ExecuteRun(ctx context.Context, request RunRequest) (RunSummary, error) {
	return executeRun(ctx, request, nil)
}

// ExecuteRunWithStore はGUIサーバーが保持するDBを使ってRunを実行します。
func ExecuteRunWithStore(ctx context.Context, request RunRequest, database *store.SQLiteStore) (RunSummary, error) {
	return executeRun(ctx, request, database)
}

func executeRun(ctx context.Context, request RunRequest, database *store.SQLiteStore) (RunSummary, error) {
	if request.ConfigPath == "" {
		request.ConfigPath = "ahc-plaza.toml"
	}
	cfg, err := config.Load(request.ConfigPath)
	if err != nil {
		return RunSummary{}, err
	}
	if request.Solver == "" {
		return RunSummary{}, errors.New("solver is required")
	}
	if request.Threads < 0 {
		request.Threads = cfg.File.Execution.Threads
	}
	if request.TimeoutMilliseconds <= 0 {
		request.TimeoutMilliseconds = cfg.File.Execution.TimeoutMilliseconds
	}
	if request.PahcerBinary == "" {
		request.PahcerBinary = "pahcer"
	}
	if request.SettingFile == "" {
		request.SettingFile = cfg.File.Pahcer.SettingFile
	}

	inputDir, err := cfg.InputSetDir(request.InputDir)
	if err != nil {
		return RunSummary{}, err
	}
	inputCases, err := cases.Discover(inputDir)
	if err != nil {
		return RunSummary{}, err
	}
	if len(inputCases) == 0 {
		return RunSummary{}, errors.New("no input cases found")
	}

	solverPath, err := cfg.ResolveProjectPath("solver", request.Solver)
	if err != nil {
		return RunSummary{}, err
	}
	toolsDir, err := cfg.ResolveProjectPath("paths.tools_dir", cfg.File.Paths.ToolsDir)
	if err != nil {
		return RunSummary{}, err
	}
	settingFile, err := cfg.ResolveProjectPath("setting_file", request.SettingFile)
	if err != nil {
		return RunSummary{}, err
	}
	caseRunner, err := os.Executable()
	if err != nil {
		return RunSummary{}, fmt.Errorf("could not locate the case runner: %w", err)
	}
	runID := request.RunID
	if runID == "" {
		runID, err = NewRunID()
		if err != nil {
			return RunSummary{}, err
		}
	}
	runDir := filepath.Join(cfg.PathRoot, "runs", runID)
	if err := os.MkdirAll(filepath.Join(runDir, "logs"), 0o755); err != nil {
		return RunSummary{}, fmt.Errorf("could not create the Run directory: %w", err)
	}

	source, err := snapshot.Create(cfg.ProjectRoot, solverPath, runDir)
	if err != nil {
		return RunSummary{}, err
	}
	workspace, err := pahcer.PrepareWorkspace(runDir, source.SnapshotPath, toolsDir, settingFile, inputCases, pahcer.WorkspaceOptions{
		Threads: request.Threads, CaseTimeoutMilliseconds: request.TimeoutMilliseconds, CaseRunner: caseRunner,
	})
	if err != nil {
		return RunSummary{}, err
	}

	createdAt := time.Now().UTC()
	run := domain.Run{
		ID:                  runID,
		Problem:             cfg.File.Project.Problem,
		Objective:           cfg.File.Project.Objective,
		SolverPath:          relativePath(cfg.ProjectRoot, solverPath),
		InputDir:            relativePath(cfg.ProjectRoot, inputDir),
		InputCasesHash:      cases.HashList(inputCases),
		SourcePath:          relativePath(cfg.ProjectRoot, source.SnapshotPath),
		SourceHash:          source.SHA256,
		ConfigHash:          fileHash(cfg.FilePath),
		PahcerVersion:       "unknown",
		CompilerVersion:     "unknown",
		Threads:             request.Threads,
		TimeoutMilliseconds: request.TimeoutMilliseconds,
		Status:              domain.RunQueued,
		Comment:             request.Comment,
		CreatedAt:           createdAt,
		StartedAt:           createdAt,
	}

	if database == nil {
		database, err = store.OpenSQLite(filepath.Join(cfg.PathRoot, "ahc-plaza.db"))
		if err != nil {
			return RunSummary{}, err
		}
		defer database.Close()
	}
	if err := database.SaveRun(ctx, run); err != nil {
		return RunSummary{}, err
	}
	persistedRun, err := database.GetRun(ctx, runID)
	if err != nil {
		return RunSummary{}, err
	}
	run.RunNumber = persistedRun.RunNumber
	if err := database.SaveInputCases(ctx, runID, inputCases); err != nil {
		return RunSummary{}, err
	}
	startedAt := time.Now().UTC()
	if err := database.UpdateRunStartedAt(ctx, runID, startedAt); err != nil {
		return RunSummary{}, err
	}
	run.StartedAt = startedAt
	if err := database.UpdateRunStatus(ctx, runID, domain.RunRunning, nil); err != nil {
		return RunSummary{}, err
	}
	runner := pahcer.NewRunner(request.PahcerBinary)
	stdoutPath := filepath.Join(runDir, "logs", "stdout.log")
	stderrPath := filepath.Join(runDir, "logs", "stderr.log")
	processResult, runErr := runner.Run(ctx, pahcer.Request{
		WorkingDir:  workspace.Dir,
		SettingFile: workspace.SettingFile,
		StdoutPath:  stdoutPath,
		StderrPath:  stderrPath,
		Timeout:     0,
	})
	finishedAt := time.Now().UTC()
	run.FinishedAt = &finishedAt
	status := processToRunStatus(processResult.Status)
	caseResults, parseErr := buildCaseResults(cfg.ProjectRoot, runID, workspace, inputCases, cfg.File.Score.InvalidScore)
	if parseErr != nil && processResult.Status == process.StatusSucceeded {
		status = domain.RunFailed
	}
	if err := database.UpdateRunStatus(ctx, runID, status, &finishedAt); err != nil {
		return RunSummary{}, err
	}
	if parseErr == nil && len(caseResults) > 0 {
		if err := database.SaveCaseResults(ctx, caseResults); err != nil {
			return RunSummary{}, err
		}
	}

	summary := RunSummary{
		RunID:          runID,
		RunNumber:      run.RunNumber,
		Status:         status,
		CreatedAt:      run.CreatedAt,
		StartedAt:      run.StartedAt,
		FinishedAt:     run.FinishedAt,
		InputCaseCount: len(inputCases),
		SourceSnapshot: run.SourcePath,
		ExitCode:       processResult.ExitCode,
	}
	if processResult.Error != nil {
		summary.Error = processResult.Error.Error()
	}
	if parseErr != nil {
		summary.Error = parseErr.Error()
	}
	if err := writeJSON(filepath.Join(runDir, "result.json"), summary); err != nil {
		return RunSummary{}, err
	}
	if runErr != nil {
		return summary, runErr
	}
	if processResult.Status != process.StatusSucceeded {
		return summary, fmt.Errorf("pahcer execution failed: %s", processResult.Status)
	}
	if parseErr != nil {
		return summary, parseErr
	}
	return summary, nil
}

func NewRunID() (string, error) {
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("could not generate a Run ID: %w", err)
	}
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405.000000000"), hex.EncodeToString(randomBytes)), nil
}

func fileHash(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}

func writeJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("could not generate JSON: %w", err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o644); err != nil {
		return fmt.Errorf("could not save JSON: %w", err)
	}
	return nil
}
