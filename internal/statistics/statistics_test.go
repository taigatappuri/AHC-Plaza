package statistics

import (
	"testing"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func TestSummarize(t *testing.T) {
	got, err := Summarize([]float64{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mean != 3 || got.Median != 3 || got.Q1 != 2 || got.Q3 != 4 || got.Variance != 2.5 {
		t.Fatalf("summary = %#v", got)
	}
}

func TestSummarizeRunResults(t *testing.T) {
	got, err := SummarizeRunResults([]domain.CaseResult{
		{Score: 1, ExecutionTime: 2 * time.Millisecond},
		{Score: 2, ExecutionTime: 3 * time.Millisecond},
		{Score: 3, ExecutionTime: 4 * time.Millisecond},
		{Score: 4, ExecutionTime: 5 * time.Millisecond},
		{Score: 5, ExecutionTime: 6 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CaseCount != 5 || got.AverageScore != 3 || got.MinScore != 1 || got.Q1Score != 2 ||
		got.MedianScore != 3 || got.Q3Score != 4 || got.MaxScore != 5 || got.VarianceScore != 2.5 ||
		got.TotalExecutionTimeNs != int64(20*time.Millisecond) || got.AverageExecutionTimeNs != float64(4*time.Millisecond) ||
		got.MaxExecutionTimeNs != int64(6*time.Millisecond) {
		t.Fatalf("run statistics = %#v", got)
	}
}

func TestComparePairsByInputCaseID(t *testing.T) {
	a := []domain.CaseResult{{InputCaseID: "b", Score: 30}, {InputCaseID: "a", Score: 20}}
	b := []domain.CaseResult{{InputCaseID: "a", Score: 10}, {InputCaseID: "b", Score: 20}}
	got, err := Compare(a, b, "max", 0.95, 1000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.CaseCount != 2 || got.MeanImprovement != 10 || got.MedianImprovement != 10 {
		t.Fatalf("comparison = %#v", got)
	}
}

func TestCompareMinReversesImprovement(t *testing.T) {
	a := []domain.CaseResult{{InputCaseID: "a", Score: 10}}
	b := []domain.CaseResult{{InputCaseID: "a", Score: 20}}
	got, err := Compare(a, b, "min", 0.95, 1000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.MeanImprovement != 10 {
		t.Fatalf("mean improvement = %v", got.MeanImprovement)
	}
}

func TestCompareRequiresCommonCases(t *testing.T) {
	_, err := Compare(
		[]domain.CaseResult{{InputCaseID: "a", Score: 1}},
		[]domain.CaseResult{{InputCaseID: "b", Score: 1}},
		"max", 0.95, 100, 1,
	)
	if err == nil {
		t.Fatal("共通ケースなしの比較を許可しました")
	}
}
