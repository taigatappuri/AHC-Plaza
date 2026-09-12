package usecase

import (
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"math"
	"testing"
)

func TestTuningObjectiveRejectsInvalidOrMissing(t *testing.T) {
	valid := []domain.CaseResult{{Seed: 0, Status: "succeeded", Score: -10}, {Seed: 1, Status: "succeeded", Score: 0}}
	if value, e := EvaluateTuningResults(valid, 2); e != nil || value != -5 {
		t.Fatal(value, e)
	}
	for _, status := range []string{"wa", "tle", "failed", "succeeded"} {
		t.Run(status, func(t *testing.T) {
			results := append([]domain.CaseResult(nil), valid...)
			results[1].Status = status
			if status == "succeeded" {
				results[1].Score = math.NaN()
			}
			if _, e := EvaluateTuningResults(results, 2); e == nil {
				t.Fatal("invalid case accepted")
			}
		})
	}
	for _, results := range [][]domain.CaseResult{nil, valid[:1], {valid[0], valid[0]}, {valid[0], {Seed: 9, Status: "succeeded"}}} {
		if _, e := EvaluateTuningResults(results, 2); e == nil {
			t.Fatal("incomplete or duplicate accepted")
		}
	}
}
