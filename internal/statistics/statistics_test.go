package statistics

import (
	"reflect"
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

func TestCompareUsesBootstrapTForPairedMean(t *testing.T) {
	a, b := comparisonResultsFromDifferences([]float64{2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
	got, err := Compare(a, b, "max", 0.95, 10_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.StatisticalMethod != bootstrapTMethod || !got.InferenceAvailable || got.InferenceNote != "" {
		t.Fatalf("推測方法 = %#v", got)
	}
	if got.PValue == nil || got.ConfidenceLow == nil || got.ConfidenceHigh == nil {
		t.Fatalf("bootstrap-tの結果がありません: %#v", got)
	}
	if *got.ConfidenceLow <= 0 || *got.ConfidenceHigh <= *got.ConfidenceLow || !got.Significant {
		t.Fatalf("信頼区間 = [%v, %v], significant = %v", *got.ConfidenceLow, *got.ConfidenceHigh, got.Significant)
	}
	if *got.PValue <= 0 || *got.PValue >= 0.05 {
		t.Fatalf("p値 = %v", *got.PValue)
	}
}

func TestCompareBootstrapTDoesNotDetectZeroMeanDifference(t *testing.T) {
	a, b := comparisonResultsFromDifferences([]float64{-4, -3, -2, -1, 0, 1, 2, 3, 4})
	got, err := Compare(a, b, "max", 0.95, 10_000, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !got.InferenceAvailable || got.PValue == nil || got.ConfidenceLow == nil || got.ConfidenceHigh == nil {
		t.Fatalf("bootstrap-tの結果がありません: %#v", got)
	}
	if *got.ConfidenceLow > 0 || *got.ConfidenceHigh < 0 || got.Significant {
		t.Fatalf("信頼区間 = [%v, %v], significant = %v", *got.ConfidenceLow, *got.ConfidenceHigh, got.Significant)
	}
	if *got.PValue < 0.05 || *got.PValue > 1 {
		t.Fatalf("p値 = %v", *got.PValue)
	}
}

func TestCompareBootstrapTIsReproducibleWithFixedSeed(t *testing.T) {
	a, b := comparisonResultsFromDifferences([]float64{-2, 1, 3, 4, 7, 8, 9, 12})
	first, err := Compare(a, b, "max", 0.95, 2_000, 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Compare(a, b, "max", 0.95, 2_000, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("同じ乱数seedで結果が変化しました:\nfirst = %#v\nsecond = %#v", first, second)
	}
}

func TestCompareBootstrapTReportsUnavailableInference(t *testing.T) {
	tests := []struct {
		name        string
		differences []float64
	}{
		{name: "1ケース", differences: []float64{1}},
		{name: "差分の分散なし", differences: []float64{2, 2, 2, 2}},
		{name: "ゼロ分散の再標本が多い", differences: []float64{0, 0, 0, 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := comparisonResultsFromDifferences(test.differences)
			got, err := Compare(a, b, "max", 0.95, 10_000, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got.InferenceAvailable || got.InferenceNote == "" || got.PValue != nil || got.ConfidenceLow != nil || got.ConfidenceHigh != nil || got.Significant {
				t.Fatalf("comparison = %#v", got)
			}
		})
	}
}

func comparisonResultsFromDifferences(differences []float64) ([]domain.CaseResult, []domain.CaseResult) {
	a := make([]domain.CaseResult, len(differences))
	b := make([]domain.CaseResult, len(differences))
	for index, difference := range differences {
		caseID := string(rune('a' + index))
		a[index] = domain.CaseResult{InputCaseID: caseID, Score: difference}
		b[index] = domain.CaseResult{InputCaseID: caseID, Score: 0}
	}
	return a, b
}

func TestFilterInvalidExcludesWAAndTLE(t *testing.T) {
	results := []domain.CaseResult{
		{InputCaseID: "ok", Status: "succeeded"},
		{InputCaseID: "wa", Status: "wa"},
		{InputCaseID: "tle", Status: "tle"},
	}
	filtered := FilterInvalid(results, false)
	if len(filtered) != 1 || filtered[0].InputCaseID != "ok" {
		t.Fatalf("filtered = %#v, want succeeded only", filtered)
	}
	if included := FilterInvalid(results, true); len(included) != len(results) {
		t.Fatalf("includeInvalid result count = %d, want %d", len(included), len(results))
	}
}
