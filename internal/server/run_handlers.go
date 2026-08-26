package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/statistics"
	"github.com/taigatappuri/AHC-Plaza/internal/usecase"
)

func (s *Server) handleSolvers(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.ConfigPath)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	directory, err := cfg.ResolveProjectPath("paths.solver_dir", cfg.File.Paths.SolverDir)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	solvers, err := listRegularFiles(cfg.ProjectRoot, directory, ".cpp")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("solverを一覧できません: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"solvers": solvers})
}

func listRegularFiles(root, directory, extension string) ([]string, error) {
	info, err := os.Stat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("ディレクトリではありません: %s", directory)
	}
	files := make([]string, 0)
	err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != directory && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") || !strings.EqualFold(filepath.Ext(entry.Name()), extension) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err == nil {
			files = append(files, filepath.ToSlash(relative))
		}
		return err
	})
	sort.Strings(files)
	return files, err
}

func (s *Server) handleInputDirectories(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.ConfigPath)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}

	inputRoot, err := cfg.InputDir("")
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	entries, readErr := os.ReadDir(inputRoot)
	if errors.Is(readErr, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, map[string]any{"input_directories": []string{}})
		return
	}
	if readErr != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("入力セットを一覧できません: %w", readErr))
		return
	}
	inputDirectories := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		hasCases, hasCasesErr := hasInputCases(filepath.Join(inputRoot, entry.Name()))
		if hasCasesErr != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("入力セットを確認できません: %w", hasCasesErr))
			return
		}
		if !hasCases {
			continue
		}
		relative, relErr := filepath.Rel(cfg.ProjectRoot, filepath.Join(inputRoot, entry.Name()))
		if relErr != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("入力セットのパスを解決できません: %w", relErr))
			return
		}
		inputDirectories = append(inputDirectories, filepath.ToSlash(relative))
	}
	sort.Strings(inputDirectories)
	writeJSON(w, http.StatusOK, map[string]any{"input_directories": inputDirectories})
}

func hasInputCases(directory string) (bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return false, err
		}
		if info.Mode().IsRegular() {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		runs, err := s.Store.ListRunSummaries(r.Context(), limit)
		if writeErrorIf(w, http.StatusInternalServerError, err) {
			return
		}
		writeJSON(w, http.StatusOK, runs)
	case http.MethodPost:
		s.createRun(w, r)
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	var input usecase.RunRequest
	if writeErrorIf(w, http.StatusBadRequest, decodeJSON(r, &input)) {
		return
	}
	if input.Solver == "" {
		writeError(w, http.StatusBadRequest, errors.New("solverは必須です"))
		return
	}
	if writeErrorIf(w, http.StatusBadRequest, s.validateRunRequest(input)) {
		return
	}
	runID, err := usecase.NewRunID()
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	input.RunID, input.ConfigPath = runID, s.ConfigPath
	if !s.registerRun(runID, cancel) {
		cancel()
		writeError(w, http.StatusServiceUnavailable, errors.New("サーバーを終了中です"))
		return
	}
	go func() {
		defer s.finishRun(runID)
		_, runErr := usecase.ExecuteRunWithStore(ctx, input, s.Store)
		if runErr != nil {
			s.recordRunFailure(runID, runErr)
			finishedAt := time.Now().UTC()
			_ = s.Store.UpdateRunStatus(context.Background(), runID, domain.RunFailed, &finishedAt)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID, "status": string(domain.RunQueued)})
}

func (s *Server) validateRunRequest(input usecase.RunRequest) error {
	cfg, err := config.Load(s.ConfigPath)
	if err != nil {
		return err
	}
	inputDir, err := cfg.InputSetDir(input.InputDir)
	if err != nil {
		return err
	}
	inputInfo, err := os.Stat(inputDir)
	if err != nil {
		return fmt.Errorf("入力ケースのディレクトリを開けません: %w", err)
	}
	if !inputInfo.IsDir() {
		return errors.New("入力ケースのパスがディレクトリではありません")
	}
	solver, err := cfg.ResolveProjectPath("solver", input.Solver)
	if err != nil {
		return err
	}
	if err := validateFile(solver, "solver"); err != nil {
		return err
	}
	settingFile := input.SettingFile
	if settingFile == "" {
		settingFile = cfg.File.Pahcer.SettingFile
	}
	settingPath, err := cfg.ResolveProjectPath("setting_file", settingFile)
	if err != nil {
		return err
	}
	if err := validateFile(settingPath, "setting_file"); err != nil {
		return err
	}
	if input.Threads < -1 {
		return errors.New("threadsは-1以上にしてください")
	}
	if input.TimeoutMilliseconds < 0 {
		return errors.New("timeout_msは0以上にしてください")
	}
	return nil
}

func validateFile(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%sを開けません: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%sがファイルではありません", label)
	}
	return nil
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, errors.New("Runが指定されていません"))
		return
	}
	runID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		s.getRun(w, r, runID)
		return
	}
	if len(parts) == 2 {
		routes := map[string]struct {
			method string
			handle func()
		}{
			"source":      {http.MethodGet, func() { s.getSource(w, r, runID) }},
			"logs":        {http.MethodGet, func() { s.getLogs(w, r, runID) }},
			"events":      {http.MethodGet, func() { s.streamEvents(w, r, runID) }},
			"case-output": {http.MethodGet, func() { s.getCaseFile(w, r, runID, false) }},
			"case-input":  {http.MethodGet, func() { s.getCaseFile(w, r, runID, true) }},
			"cancel":      {http.MethodPost, func() { s.cancelRun(w, r, runID) }},
			"comment":     {http.MethodPut, func() { s.updateRunComment(w, r, runID) }},
		}
		if route, ok := routes[parts[1]]; ok {
			if r.Method != route.method {
				writeMethodNotAllowed(w, route.method)
				return
			}
			route.handle()
			return
		}
	}
	writeError(w, http.StatusNotFound, errors.New("APIエンドポイントがありません"))
}

func (s *Server) updateRunComment(w http.ResponseWriter, r *http.Request, runID string) {
	var input struct {
		Comment string `json:"comment"`
	}
	if writeErrorIf(w, http.StatusBadRequest, decodeJSON(r, &input)) {
		return
	}
	run, ok := s.loadRun(w, r, runID)
	if !ok {
		return
	}
	if writeErrorIf(w, http.StatusInternalServerError, s.Store.UpdateRunComment(r.Context(), runID, input.Comment)) {
		return
	}
	run.Comment = input.Comment
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) getLogs(w http.ResponseWriter, r *http.Request, runID string) {
	if !validRunID(runID) {
		writeError(w, http.StatusBadRequest, errors.New("Run IDが不正です"))
		return
	}
	if _, ok := s.loadRun(w, r, runID); !ok {
		return
	}
	readLog := func(name string) string {
		content, _ := os.ReadFile(filepath.Join(s.Root, "ahc-plaza", "runs", runID, "logs", name))
		return string(content)
	}
	writeJSON(w, http.StatusOK, map[string]string{"stdout": readLog("stdout.log"), "stderr": readLog("stderr.log")})
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request, runID string) {
	run, ok := s.loadRun(w, r, runID)
	if !ok {
		return
	}
	cases, ok := s.loadCases(w, r, runID)
	if !ok {
		return
	}
	var err error
	runStatistics := domain.RunStatistics{}
	if len(cases) > 0 {
		runStatistics, err = statistics.SummarizeRunResults(cases)
		if writeErrorIf(w, http.StatusInternalServerError, err) {
			return
		}
	}
	featureData, err := s.buildFeatureData(r.Context(), cases)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	var executionResult *usecase.RunSummary
	resultPath := filepath.Join(s.Root, "ahc-plaza", "runs", runID, "result.json")
	if content, readErr := os.ReadFile(resultPath); readErr == nil {
		var result usecase.RunSummary
		if json.Unmarshal(content, &result) == nil {
			executionResult = &result
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"run": run, "cases": cases, "statistics": runStatistics,
		"feature_data": featureData, "result": executionResult,
	})
}

func (s *Server) getSource(w http.ResponseWriter, r *http.Request, runID string) {
	run, ok := s.loadRun(w, r, runID)
	if !ok {
		return
	}
	path := filepath.Join(s.Root, filepath.FromSlash(run.SourcePath))
	within, err := config.IsWithin(s.Root, path)
	if err != nil || !within {
		writeError(w, http.StatusForbidden, errors.New("ソースパスが不正です"))
		return
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, errors.New("ソーススナップショットがありません"))
		return
	}
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(content)
}

func (s *Server) loadRun(w http.ResponseWriter, r *http.Request, runID string) (domain.Run, bool) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("Runがありません"))
		return run, false
	}
	return run, !writeErrorIf(w, http.StatusInternalServerError, err)
}

func (s *Server) loadCases(w http.ResponseWriter, r *http.Request, runID string) ([]domain.CaseResult, bool) {
	cases, err := s.Store.GetCaseResults(r.Context(), runID)
	return cases, !writeErrorIf(w, http.StatusInternalServerError, err)
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request, runID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("SSEを利用できません"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// 状態は長時間変化しないため、クライアントの再描画頻度を抑えます。
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	for {
		run, err := s.Store.GetRun(r.Context(), runID)
		if errors.Is(err, sql.ErrNoRows) {
			if failure := s.runFailure(runID); failure != "" {
				writeSSE(w, map[string]string{"run_id": runID, "status": string(domain.RunFailed), "error": failure})
				flusher.Flush()
				return
			}
			writeSSE(w, map[string]string{"run_id": runID, "status": string(domain.RunQueued)})
			flusher.Flush()
		} else if err != nil {
			writeSSE(w, map[string]string{"run_id": runID, "status": "error", "error": err.Error()})
			flusher.Flush()
			return
		} else {
			event := map[string]interface{}{
				"run_id":      runID,
				"run_number":  run.RunNumber,
				"status":      run.Status,
				"created_at":  run.CreatedAt,
				"started_at":  run.StartedAt,
				"finished_at": run.FinishedAt,
			}
			if failure := s.runFailure(runID); failure != "" {
				event["error"] = failure
			}
			writeSSE(w, event)
			flusher.Flush()
			if isTerminal(run.Status) {
				return
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-deadline.C:
			return
		}
	}
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request, runID string) {
	s.mu.Lock()
	cancel, ok := s.cancels[runID]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("実行中のRunがありません"))
		return
	}
	cancel()
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID, "status": string(domain.RunCancelled)})
}

func isTerminal(status domain.RunStatus) bool {
	return status == domain.RunSucceeded || status == domain.RunPartial || status == domain.RunFailed || status == domain.RunCancelled
}
