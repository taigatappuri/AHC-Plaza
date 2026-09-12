package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
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

func TestTuningValidationRouteIsNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	defer s.Close()

	r := httptest.NewRequest("POST", "/api/tuning/studies/removed-validation/validate", bytes.NewBufferString(`{"input_dir":"ahc-plaza/inputs/cases","threads":1}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestDeleteTuningStudyRemovesOnlyRelatedRuns(t *testing.T) {
	s, ordinaryRunID := newTestServer(t)
	defer s.Close()
	ctx := context.Background()
	studyID, tuningRunID := "study-delete", "tuning-run"
	if err := s.Store.SaveStudy(ctx, domain.TuningStudy{ID: studyID, Status: "completed", BaselineRun: tuningRunID}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run := domain.Run{ID: tuningRunID, TuningStudy: studyID, Problem: "demo", Objective: "max", SourcePath: "source.cpp", Status: domain.RunSucceeded, CreatedAt: now, StartedAt: now}
	if err := s.Store.SaveRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SaveCaseResults(ctx, []domain.CaseResult{{RunID: tuningRunID, InputCaseID: "0", Status: "succeeded"}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(s.Root, "ahc-plaza", "tuning", studyID), filepath.Join(s.Root, "ahc-plaza", "runs", tuningRunID)} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "data"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/tuning/studies/"+studyID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if _, err := s.Store.GetStudy(ctx, studyID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("study remains: %v", err)
	}
	if _, err := s.Store.GetRun(ctx, tuningRunID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("tuning Run remains: %v", err)
	}
	if results, err := s.Store.GetCaseResults(ctx, tuningRunID); err != nil || len(results) != 0 {
		t.Fatalf("tuning cases remain: %v %v", results, err)
	}
	if _, err := s.Store.GetRun(ctx, ordinaryRunID); err != nil {
		t.Fatalf("ordinary Run was deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "ahc-plaza", "runs", tuningRunID)); !os.IsNotExist(err) {
		t.Fatalf("tuning Run directory remains: %v", err)
	}
}

func TestDeleteRunningTuningStudyIsRejected(t *testing.T) {
	s, _ := newTestServer(t)
	defer s.Close()
	if err := s.Store.SaveStudy(context.Background(), domain.TuningStudy{ID: "running-study", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/tuning/studies/running-study", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
}
