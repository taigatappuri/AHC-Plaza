package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
)

func TestConfigAPIUpdatesAHCPlazaTOML(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Close()

	body := `{
		"project":{"problem":"demo-next","objective":"min"},
		"paths":{"solver_dir":"src","tools_dir":"tester"},
		"execution":{"default_input_dir":"tester/in","threads":4,"timeout_ms":20000},
		"pahcer":{"setting_file":"tester/pahcer.toml"},
		"score":{"invalid_score":-1,"include_invalid_cases":false},
		"statistics":{"confidence_level":0.99,"bootstrap_iterations":20000},
		"input_format":{"variables":[{"name":"N","line":1,"column":1},{"name":"M","line":1,"column":2}],"features":[{"name":"average","source":"features/average.cpp","timeout_ms":2000}]}
	}`
	request := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var received config.FileConfig
	if err := json.NewDecoder(response.Body).Decode(&received); err != nil {
		t.Fatal(err)
	}
	if received.Project.Problem != "demo-next" || received.Paths.SolverDir != "src" || received.Score.IncludeInvalidCases {
		t.Fatalf("response = %#v", received)
	}
	if len(received.InputFormat.Variables) != 2 || received.InputFormat.Variables[1].Name != "M" {
		t.Fatalf("input format = %#v", received.InputFormat)
	}
	if len(received.InputFormat.Features) != 1 || received.InputFormat.Features[0].Name != "average" {
		t.Fatalf("input features = %#v", received.InputFormat)
	}
	loaded, err := config.Load(server.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.File, received) {
		t.Fatalf("file = %#v, response = %#v", loaded.File, received)
	}
	if files, err := os.ReadDir(server.Root); err != nil {
		t.Fatal(err)
	} else {
		for _, file := range files {
			if strings.HasPrefix(file.Name(), "ahc-plaza.toml-") {
				t.Fatalf("一時ファイルが残っています: %s", file.Name())
			}
		}
	}
}

func TestConfigAPIRejectsInvalidUpdateWithoutReplacingFile(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Close()
	original, err := os.ReadFile(server.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	body := `{
		"project":{"problem":"demo","objective":"max"},
		"paths":{"solver_dir":"../outside","tools_dir":"tools"},
		"execution":{"default_input_dir":"ahc-plaza/inputs","threads":0,"timeout_ms":10000},
		"pahcer":{"setting_file":"pahcer_config.toml"},
		"score":{"invalid_score":0,"include_invalid_cases":true},
		"statistics":{"confidence_level":0.95,"bootstrap_iterations":10000}
	}`
	request := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	current, err := os.ReadFile(server.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatal("検証失敗時にahc-plaza.tomlが変更されました")
	}
}
