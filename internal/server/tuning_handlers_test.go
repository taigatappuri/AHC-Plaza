package server

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTuningRejectsCrossOriginMutation(t *testing.T) {
	s := &Server{}
	for _, origin := range []string{"https://evil.example", "null"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/tuning/environment/setup", bytes.NewBufferString("{}"))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.handleTuning(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
}

func TestTuningInputsIgnoreOrdinaryRunInputRoot(t *testing.T) {
	s, _ := newTestServer(t)
	defer s.Close()
	content, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(s.ConfigPath, []byte(strings.ReplaceAll(string(content), "ahc-plaza/inputs", "other-inputs")), 0600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(s.Root, "other-inputs", "outside")
	if err = os.MkdirAll(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(other, "0.txt"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/api/input-directories", 200, "other-inputs/outside"},
		{"/api/tuning/input-directories", 200, "ahc-plaza/inputs/cases"},
		{"/api/tuning/input-count?input_dir=ahc-plaza/inputs/cases", 200, `"count":1`},
		{"/api/tuning/input-count?input_dir=other-inputs/outside", 400, "direct child"},
	} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
