package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInputGeneratorsAPIListsRustBins(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Close()
	binDirectory := filepath.Join(server.Root, "tools", "src", "bin")
	if err := os.MkdirAll(binDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gen.rs", "vis.rs", "README.md"} {
		if err := os.WriteFile(filepath.Join(binDirectory, name), []byte("fn main() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/input-generators", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		Generators []inputGeneratorOption `json:"generators"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Generators) != 1 || result.Generators[0].Name != "gen" {
		t.Fatalf("generators = %#v", result.Generators)
	}
}

func TestWriteSeedsCreatesSequentialSeedList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seeds.txt")
	if err := writeSeeds(path, 10, 3); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != "10\n11\n12" {
		t.Fatalf("seeds = %q", content)
	}
}

func TestWriteInputParametersPreservesArbitraryExpressions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "constraints.txt")
	parameters := []inputParameter{
		{Name: "N", Constraint: "N >= 50"},
		{Name: "M", Constraint: "M >= N * 2"},
	}
	if err := validateInputParameters(parameters); err != nil {
		t.Fatal(err)
	}
	if err := writeInputParameters(path, parameters); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# AHC_PLAZA_CONSTRAINTS_VERSION=1\nN\tN >= 50\nM\tM >= N * 2\n" {
		t.Fatalf("constraints = %q", content)
	}
}

func TestInputDirectoryNameAcceptsSafeNames(t *testing.T) {
	for _, name := range []string{"baseline", "n50-large", "case.v2", "日本語ケース", "ケース 50"} {
		if !validInputDirectoryName(name) {
			t.Fatalf("入力ディレクトリ名 %q を受け入れられません", name)
		}
	}
	for _, name := range []string{"", ".hidden", "..", "../outside", `a\b`, "a/b", "名前\n"} {
		if validInputDirectoryName(name) {
			t.Fatalf("不正な入力ディレクトリ名 %q を受け入れました", name)
		}
	}
}
