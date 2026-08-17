package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisualizerStatusAndAssetServing(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Close()

	directory := server.visualizerDirectory()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "visualizer.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/visualizer", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"entry":"visualizer.html"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/visualizer/visualizer.html", nil))
	if response.Code != http.StatusOK || response.Body.String() != "<html>ok</html>" {
		t.Fatalf("asset response = %d, body = %q", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/visualizer/../ahc-plaza.toml", nil))
	if response.Code == http.StatusOK {
		t.Fatalf("path traversal unexpectedly succeeded: %d", response.Code)
	}
}

func TestValidateVisualizerURL(t *testing.T) {
	if _, err := validateVisualizerURL("https://img.atcoder.jp/ahc000/visualizer.html", true); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"http://img.atcoder.jp/ahc000/visualizer.html",
		"https://example.com/ahc000/visualizer.html",
		"https://img.atcoder.jp/ahc000/visualizer.js",
	} {
		if _, err := validateVisualizerURL(value, true); err == nil {
			t.Fatalf("URL was accepted unexpectedly: %s", value)
		}
	}
}
