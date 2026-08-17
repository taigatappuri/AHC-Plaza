package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func TestWriteMethodNotAllowed(t *testing.T) {
	response := httptest.NewRecorder()
	writeMethodNotAllowed(response, http.MethodGet, http.MethodPost)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != "GET, POST" {
		t.Fatalf("Allow = %q, want %q", allow, "GET, POST")
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("レスポンスを読めません: %v", err)
	}
	if body["error"] != "許可されていないメソッドです" {
		t.Fatalf("error = %q", body["error"])
	}
}

func TestHandlerListsRunAndShowsSnapshot(t *testing.T) {
	server, runID := newTestServer(t)
	defer server.Close()

	request := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var runs []struct {
		domain.Run
		domain.RunStatistics
	}
	if err := json.NewDecoder(response.Body).Decode(&runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != runID || runs[0].RunNumber != 1 || runs[0].CaseCount != 2 || runs[0].AverageScore != 15 {
		t.Fatalf("runs = %#v", runs)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/runs/"+runID, nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "source.cpp") {
		t.Fatalf("run response = %d %s", response.Code, response.Body.String())
	}
	var detail struct {
		Statistics domain.RunStatistics `json:"statistics"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Statistics.AverageScore != 15 || detail.Statistics.VarianceScore != 50 ||
		detail.Statistics.AverageExecutionTimeNs != 2.5*float64(time.Millisecond) ||
		detail.Statistics.MaxExecutionTimeNs != int64(3*time.Millisecond) {
		t.Fatalf("detail statistics = %#v", detail.Statistics)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/runs/"+runID+"/source", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "snapshot\n" {
		t.Fatalf("source response = %d %q", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/runs/"+runID+"/case-output?case_id=0000", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "answer\n" {
		t.Fatalf("case output response = %d %q", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/runs/"+runID+"/case-input?case_id=0000", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "input\n" {
		t.Fatalf("case input response = %d %q", response.Code, response.Body.String())
	}
}

func TestHandlerUpdatesRunComment(t *testing.T) {
	server, runID := newTestServer(t)
	defer server.Close()

	request := httptest.NewRequest(http.MethodPut, "/api/runs/"+runID+"/comment", strings.NewReader(`{"comment":"改善版"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	run, err := server.Store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Comment != "改善版" {
		t.Fatalf("comment = %q, want %q", run.Comment, "改善版")
	}
}

func TestHandlerListsSolverSources(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Close()

	if err := os.MkdirAll(filepath.Join(server.Root, "solver", "experimental"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"solver/main.cpp":                "int main() {}\n",
		"solver/experimental/fast.cpp":   "int main() {}\n",
		"solver/README.txt":              "not a solver\n",
		"solver/experimental/ignored.cc": "int main() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(server.Root, filepath.FromSlash(path)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/solvers", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		Solvers []string `json:"solvers"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	want := []string{"solver/experimental/fast.cpp", "solver/main.cpp"}
	if strings.Join(result.Solvers, "\n") != strings.Join(want, "\n") {
		t.Fatalf("solvers = %#v, want %#v", result.Solvers, want)
	}
}

func TestRunDetailIncludesConfiguredInputVariableValues(t *testing.T) {
	server, runID := newTestServer(t)
	defer server.Close()

	loaded, err := config.Load(server.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded.File.InputFormat.Variables = []config.InputVariableConfig{
		{Name: "N", Line: 1, Column: 1},
		{Name: "M", Line: 1, Column: 2},
	}
	if _, err := config.Save(server.ConfigPath, loaded.File); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"0000.txt": "10 20\n",
		"0001.txt": "30 nope\n",
	} {
		if err := os.WriteFile(filepath.Join(server.Root, "ahc-plaza", "inputs", "cases", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Store.SaveCaseResults(context.Background(), []domain.CaseResult{
		{RunID: runID, InputCaseID: "0000", InputPath: "ahc-plaza/inputs/cases/0000.txt", Score: 10, ExecutionTime: 2 * time.Millisecond, Status: "succeeded"},
		{RunID: runID, InputCaseID: "0001", InputPath: "ahc-plaza/inputs/cases/0001.txt", Score: 20, ExecutionTime: 3 * time.Millisecond, Status: "succeeded"},
	}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/runs/"+runID, nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var detail struct {
		FeatureData featureDataResponse `json:"feature_data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	data := detail.FeatureData
	if len(data.Features) != 2 || len(data.Cases) != 2 {
		t.Fatalf("feature data = %#v", data)
	}
	if data.Cases[0].Values["N"] != 10 || data.Cases[0].Values["M"] != 20 || data.Cases[1].Values["N"] != 30 {
		t.Fatalf("case values = %#v", data.Cases)
	}
	if _, exists := data.Cases[1].Values["M"]; exists || data.IssueCount != 1 || len(data.Issues) != 1 {
		t.Fatalf("issues = %d %#v", data.IssueCount, data.Issues)
	}

}

func TestHandlerListsInputDirectories(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Close()

	for _, directory := range []string{
		"ahc-plaza/inputs/baseline",
		"ahc-plaza/inputs/large",
		"ahc-plaza/inputs/empty",
		"ahc-plaza/inputs/.hidden",
		"tools/other",
	} {
		if err := os.MkdirAll(filepath.Join(server.Root, filepath.FromSlash(directory)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		"ahc-plaza/inputs/baseline/0000.txt": "case\n",
		"ahc-plaza/inputs/large/0001.txt":    "case\n",
		"ahc-plaza/inputs/.hidden/0002.txt":  "case\n",
		"tools/other/0003.txt":               "case\n",
	} {
		if err := os.WriteFile(filepath.Join(server.Root, filepath.FromSlash(path)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/input-directories", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		InputDirectories []string `json:"input_directories"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	want := []string{"ahc-plaza/inputs/baseline", "ahc-plaza/inputs/cases", "ahc-plaza/inputs/large"}
	if strings.Join(result.InputDirectories, "\n") != strings.Join(want, "\n") {
		t.Fatalf("input directories = %#v, want %#v", result.InputDirectories, want)
	}
}

func TestCompareRuns(t *testing.T) {
	server, runID := newTestServer(t)
	defer server.Close()

	secondID := "run-second"
	now := time.Now().UTC()
	if err := server.Store.SaveRun(context.Background(), domain.Run{
		ID: secondID, Problem: "demo", Objective: "max", SolverPath: "solver/main.cpp",
		InputDir: "ahc-plaza/inputs/cases", SourcePath: "ahc-plaza/runs/second/source.cpp", SourceHash: "b",
		ConfigHash: "c", PahcerVersion: "test", CompilerVersion: "test", Status: domain.RunSucceeded,
		CreatedAt: now.Add(time.Second), StartedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Store.SaveCaseResults(context.Background(), []domain.CaseResult{
		{RunID: secondID, InputCaseID: "0000", Score: 12, Status: "succeeded"},
		{RunID: secondID, InputCaseID: "0001", Score: 22, Status: "succeeded"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"run_a":"` + runID + `","run_b":"` + secondID + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/compare", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"significant"`) {
		t.Fatalf("comparison = %s", response.Body.String())
	}
}

func TestCompareRunsAppliesInputConditionsBeforeStatisticalTest(t *testing.T) {
	server, runID := newTestServer(t)
	defer server.Close()

	loaded, err := config.Load(server.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded.File.InputFormat.Variables = []config.InputVariableConfig{{Name: "N", Line: 1, Column: 1}}
	if _, err := config.Save(server.ConfigPath, loaded.File); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"0000.txt": "10\n", "0001.txt": "40\n"} {
		if err := os.WriteFile(filepath.Join(server.Root, "ahc-plaza", "inputs", "cases", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Store.SaveCaseResults(context.Background(), []domain.CaseResult{
		{RunID: runID, InputCaseID: "0000", InputPath: "ahc-plaza/inputs/cases/0000.txt", Score: 10, Status: "succeeded"},
		{RunID: runID, InputCaseID: "0001", InputPath: "ahc-plaza/inputs/cases/0001.txt", Score: 20, Status: "succeeded"},
	}); err != nil {
		t.Fatal(err)
	}

	secondID := "run-filtered-second"
	now := time.Now().UTC()
	if err := server.Store.SaveRun(context.Background(), domain.Run{
		ID: secondID, Problem: "demo", Objective: "max", SolverPath: "solver/main.cpp",
		InputDir: "ahc-plaza/inputs/cases", SourcePath: "ahc-plaza/runs/filtered-second/source.cpp", SourceHash: "b",
		ConfigHash: "c", PahcerVersion: "test", CompilerVersion: "test", Status: domain.RunSucceeded,
		CreatedAt: now.Add(time.Second), StartedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Store.SaveCaseResults(context.Background(), []domain.CaseResult{
		{RunID: secondID, InputCaseID: "0000", InputPath: "ahc-plaza/inputs/cases/0000.txt", Score: 12, Status: "succeeded"},
		{RunID: secondID, InputCaseID: "0001", InputPath: "ahc-plaza/inputs/cases/0001.txt", Score: 42, Status: "succeeded"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"run_a":"` + runID + `","run_b":"` + secondID + `","conditions":[{"feature":"N","operator":">=","value":5},{"feature":"N","operator":"<=","value":30}]}`
	request := httptest.NewRequest(http.MethodPost, "/api/compare", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var comparison comparisonResponse
	if err := json.NewDecoder(response.Body).Decode(&comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.CaseCount != 1 || comparison.MeanA != 10 || comparison.MeanB != 12 {
		t.Fatalf("comparison = %#v", comparison.Comparison)
	}
	if !comparison.Filter.Active || len(comparison.Filter.Conditions) != 2 || comparison.Filter.OriginalCaseCount != 2 || comparison.Filter.MatchedCaseCount != 1 || comparison.Filter.ExcludedCaseCount != 1 || comparison.Filter.UnreadableCount != 0 {
		t.Fatalf("filter = %#v", comparison.Filter)
	}

	invalidBody := `{"run_a":"` + runID + `","run_b":"` + secondID + `","conditions":[{"feature":"UNKNOWN","operator":"<=","value":30}]}`
	request = httptest.NewRequest(http.MethodPost, "/api/compare", strings.NewReader(invalidBody))
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid condition status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestEventsReportsRunFailureBeforePersistence(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Close()

	runID := "run-before-persistence"
	server.recordRunFailure(runID, errors.New("workspaceを準備できません"))
	request := httptest.NewRequest(http.MethodGet, "/api/runs/"+runID+"/events", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"status":"failed"`) || !strings.Contains(body, "workspaceを準備できません") {
		t.Fatalf("event = %s", body)
	}
}

func TestNewMarksUnfinishedRunsFailed(t *testing.T) {
	root := t.TempDir()
	first, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index, status := range []domain.RunStatus{domain.RunQueued, domain.RunRunning, domain.RunSucceeded} {
		run := domain.Run{
			ID:              fmt.Sprintf("run-recovery-%d", index),
			Problem:         "demo",
			Objective:       "max",
			SolverPath:      "solver/main.cpp",
			InputDir:        "ahc-plaza/inputs/cases",
			InputCasesHash:  "input-hash",
			SourcePath:      "ahc-plaza/runs/source.cpp",
			SourceHash:      "source-hash",
			ConfigHash:      "config-hash",
			PahcerVersion:   "test",
			CompilerVersion: "test",
			Status:          status,
			CreatedAt:       now,
			StartedAt:       now,
		}
		if err := first.Store.SaveRun(context.Background(), run); err != nil {
			first.Close()
			t.Fatal(err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for index, wantStatus := range []domain.RunStatus{domain.RunFailed, domain.RunFailed, domain.RunSucceeded} {
		run, err := reopened.Store.GetRun(context.Background(), fmt.Sprintf("run-recovery-%d", index))
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != wantStatus {
			t.Fatalf("Run %d status = %s, want %s", index, run.Status, wantStatus)
		}
		if wantStatus == domain.RunFailed && run.FinishedAt == nil {
			t.Fatalf("Run %dの終了日時が保存されていません", index)
		}
		if wantStatus == domain.RunSucceeded && run.FinishedAt != nil {
			t.Fatalf("完了済みRun %dの終了日時を変更しました", index)
		}
	}
}

func TestCloseCancelsAndWaitsForRegisteredRuns(t *testing.T) {
	server, _ := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	if !server.registerRun("run-closing", cancel) {
		t.Fatal("Runを登録できませんでした")
	}
	cleanupDone := make(chan struct{})
	go func() {
		defer server.finishRun("run-closing")
		<-ctx.Done()
		close(cleanupDone)
	}()

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cleanupDone:
	default:
		t.Fatal("Runの終了処理を待たずにサーバーを閉じました")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("2回目のCloseが失敗しました: %v", err)
	}
}

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ahc-plaza.toml"), []byte(`[project]
problem = "demo"
objective = "max"

[execution]
default_input_dir = "ahc-plaza/inputs"

[pahcer]
setting_file = "pahcer_config.toml"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(root, "ahc-plaza", "runs", "run-first", "source.cpp")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("snapshot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server, err := New(root, filepath.Join(root, "ahc-plaza.toml"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run := domain.Run{
		ID: "run-first", Problem: "demo", Objective: "max", SolverPath: "solver/main.cpp",
		InputDir: "ahc-plaza/inputs/cases", SourcePath: "ahc-plaza/runs/run-first/source.cpp", SourceHash: "a",
		ConfigHash: "c", PahcerVersion: "test", CompilerVersion: "test", Status: domain.RunSucceeded,
		CreatedAt: now, StartedAt: now,
	}
	if err := server.Store.SaveRun(context.Background(), run); err != nil {
		server.Close()
		t.Fatal(err)
	}
	if err := server.Store.SaveCaseResults(context.Background(), []domain.CaseResult{
		{RunID: run.ID, InputCaseID: "0000", Seed: 0, InputPath: "ahc-plaza/inputs/cases/0000.txt", Score: 10, ExecutionTime: 2 * time.Millisecond, Status: "succeeded", OutputPath: "ahc-plaza/runs/run-first/workspace/tools/out/0000.txt"},
		{RunID: run.ID, InputCaseID: "0001", Score: 20, ExecutionTime: 3 * time.Millisecond, Status: "succeeded"},
	}); err != nil {
		server.Close()
		t.Fatal(err)
	}
	outputPath := filepath.Join(root, "ahc-plaza", "runs", run.ID, "workspace", "tools", "out", "0000.txt")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		server.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("answer\n"), 0o644); err != nil {
		server.Close()
		t.Fatal(err)
	}
	inputPath := filepath.Join(root, "ahc-plaza", "inputs", "cases", "0000.txt")
	if err := os.MkdirAll(filepath.Dir(inputPath), 0o755); err != nil {
		server.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, []byte("input\n"), 0o644); err != nil {
		server.Close()
		t.Fatal(err)
	}
	return server, run.ID
}
