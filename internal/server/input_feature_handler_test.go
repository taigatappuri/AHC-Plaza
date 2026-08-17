package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func TestInputFeatureSourceCanBeTestedAndUsedForAnalysis(t *testing.T) {
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("g++がないためスキップします")
	}
	server, runID := newTestServer(t)
	defer server.Close()

	featureDir := filepath.Join(server.Root, "ahc-plaza", "features")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `#include <iostream>
#include <vector>
using namespace std;
int main() {
    int N;
    cin >> N;
    vector<double> A(N);
    double sum = 0;
    for (double &value : A) { cin >> value; sum += value; }
    cout << sum / N << '\n';
}`
	if err := os.WriteFile(filepath.Join(featureDir, "average.cpp"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "ignored.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(server.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded.File.InputFormat.Features = []config.InputFeatureConfig{{
		Name: "average", Source: "features/average.cpp", TimeoutMilliseconds: 2000,
	}}
	if _, err := config.Save(server.ConfigPath, loaded.File); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"0000.txt": "4\n2 4 6 8\n",
		"0001.txt": "2\n10 30\n",
	} {
		if err := os.WriteFile(filepath.Join(server.Root, "ahc-plaza", "inputs", "cases", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Store.SaveCaseResults(context.Background(), []domain.CaseResult{
		{RunID: runID, InputCaseID: "0000", InputPath: "ahc-plaza/inputs/cases/0000.txt", Score: 10, ExecutionTime: time.Millisecond, Status: "succeeded"},
		{RunID: runID, InputCaseID: "0001", InputPath: "ahc-plaza/inputs/cases/0001.txt", Score: 20, ExecutionTime: time.Millisecond, Status: "succeeded"},
	}); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/input-features", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "features/average.cpp") || strings.Contains(response.Body.String(), "ignored.txt") {
		t.Fatalf("feature sources = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/runs/"+runID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("run detail = %d %s", response.Code, response.Body.String())
	}
	var detail struct {
		FeatureData featureDataResponse `json:"feature_data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.FeatureData.Features) != 1 || detail.FeatureData.Features[0].Kind != "cpp" ||
		detail.FeatureData.Cases[0].Values["average"] != 5 || detail.FeatureData.Cases[1].Values["average"] != 20 {
		t.Fatalf("feature data = %#v", detail.FeatureData)
	}

	conditionBody := `{"run_a":"` + runID + `","run_b":"` + runID + `","conditions":[{"feature":"average","operator":"<=","value":10}]}`
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/compare", strings.NewReader(conditionBody)))
	if response.Code != http.StatusOK {
		t.Fatalf("compare = %d %s", response.Code, response.Body.String())
	}
	var comparison comparisonResponse
	if err := json.Unmarshal(response.Body.Bytes(), &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.CaseCount != 1 || comparison.Filter.MatchedCaseCount != 1 {
		t.Fatalf("comparison = %#v", comparison)
	}
}
