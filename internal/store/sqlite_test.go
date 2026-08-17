package store

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func TestSQLiteStoreSavesAndLoadsRun(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "ahc-plaza", "ahc-plaza.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	run := domain.Run{
		ID:              "run-1",
		Problem:         "ahc000",
		Objective:       "max",
		SolverPath:      "solver/hoge.cpp",
		InputDir:        "ahc-plaza/inputs/cases",
		InputCasesHash:  "cases-hash",
		SourcePath:      "ahc-plaza/runs/run-1/source/hoge.cpp",
		SourceHash:      "source-hash",
		ConfigHash:      "config-hash",
		PahcerVersion:   "0.1.0",
		CompilerVersion: "gcc 14",
		Threads:         4,
		TimeoutSeconds:  10,
		Status:          domain.RunQueued,
		Comment:         "test",
		CreatedAt:       createdAt,
		StartedAt:       createdAt,
	}
	if err := store.SaveRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != run.ID || got.RunNumber != 1 || got.Status != run.Status || got.InputDir != run.InputDir {
		t.Fatalf("loaded run = %#v", got)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %v, want %v", got.CreatedAt, createdAt)
	}
}

func TestSQLiteStoreAssignsMonotonicRunNumbers(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "ahc-plaza.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	base := domain.Run{
		Problem: "ahc000", Objective: "max", SolverPath: "solver/a.cpp",
		InputDir: "ahc-plaza/inputs/cases", InputCasesHash: "hash", SourcePath: "source/a.cpp", SourceHash: "hash",
		ConfigHash: "hash", PahcerVersion: "pahcer", CompilerVersion: "gcc", Status: domain.RunQueued,
		CreatedAt: createdAt, StartedAt: createdAt,
	}
	first := base
	first.ID = "run-first"
	second := base
	second.ID = "run-second"
	second.CreatedAt = createdAt.Add(time.Second)
	second.StartedAt = second.CreatedAt
	for _, run := range []domain.Run{first, second} {
		if err := store.SaveRun(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}

	gotFirst, err := store.GetRun(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotSecond, err := store.GetRun(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotFirst.RunNumber != 1 || gotSecond.RunNumber != 2 {
		t.Fatalf("run numbers = %d, %d; want 1, 2", gotFirst.RunNumber, gotSecond.RunNumber)
	}
}

func TestSQLiteStoreSavesInputCases(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "ahc-plaza.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	run := domain.Run{
		ID: "run-1", Problem: "ahc000", Objective: "max", SolverPath: "solver/a.cpp",
		InputDir: "ahc-plaza/inputs/cases", InputCasesHash: "hash", SourcePath: "source/a.cpp", SourceHash: "hash",
		ConfigHash: "hash", PahcerVersion: "pahcer", CompilerVersion: "gcc", Status: domain.RunQueued,
		CreatedAt: time.Now().UTC(), StartedAt: time.Now().UTC(),
	}
	if err := store.SaveRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	inputCases := []domain.InputCase{{ID: "0000.txt", Path: "ahc-plaza/inputs/cases/0000.txt", SHA256: "abc", Size: 3}}
	if err := store.SaveInputCases(context.Background(), run.ID, inputCases); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStoreListsRunSummaries(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "ahc-plaza.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	run := domain.Run{
		ID: "run-summary", Problem: "ahc000", Objective: "max", SolverPath: "solver/a.cpp",
		InputDir: "ahc-plaza/inputs/cases", InputCasesHash: "hash", SourcePath: "source/a.cpp", SourceHash: "hash",
		ConfigHash: "hash", PahcerVersion: "pahcer", CompilerVersion: "gcc", Status: domain.RunSucceeded,
		CreatedAt: createdAt, StartedAt: createdAt,
	}
	if err := store.SaveRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCaseResults(context.Background(), []domain.CaseResult{
		{RunID: run.ID, InputCaseID: "0000", InputPath: "ahc-plaza/inputs/cases/0000", Score: 10, ExecutionTime: 2 * time.Millisecond, Status: "ok"},
		{RunID: run.ID, InputCaseID: "0001", InputPath: "ahc-plaza/inputs/cases/0001", Score: 20, ExecutionTime: 3 * time.Millisecond, Status: "ok"},
	}); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListRunSummaries(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summary count = %d, want 1", len(summaries))
	}
	got := summaries[0]
	if got.Run.ID != run.ID || got.CaseCount != 2 {
		t.Fatalf("summary identity = %#v", got)
	}
	if got.AverageScore != 15 {
		t.Fatalf("average score = %v, want 15", got.AverageScore)
	}
	if got.MinScore != 10 || got.Q1Score != 12.5 || got.MedianScore != 15 || got.Q3Score != 17.5 || got.MaxScore != 20 {
		t.Fatalf("score statistics = %#v", got.RunStatistics)
	}
	if got.VarianceScore != 50 || got.StdDevScore != math.Sqrt(50) {
		t.Fatalf("score dispersion = %#v", got.RunStatistics)
	}
	if got.TotalExecutionTimeNs != int64(5*time.Millisecond) {
		t.Fatalf("total execution time = %d, want %d", got.TotalExecutionTimeNs, 5*time.Millisecond)
	}
	if got.AverageExecutionTimeNs != 2.5*float64(time.Millisecond) || got.MaxExecutionTimeNs != int64(3*time.Millisecond) {
		t.Fatalf("execution statistics = %#v", got.RunStatistics)
	}
}
