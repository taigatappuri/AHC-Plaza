package usecase

import (
	"fmt"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"math"
)

// EvaluateTuningResults は通常表示用のinvalid_scoreを目的値に使いません。
func EvaluateTuningResults(results []domain.CaseResult, count int) (float64, error) {
	if count < 1 || len(results) != count {
		return 0, fmt.Errorf("ケース結果が不足しています: %d/%d", len(results), count)
	}
	seen := map[uint64]bool{}
	mean := 0.0
	for _, r := range results {
		if r.Seed >= uint64(count) || seen[r.Seed] {
			return 0, fmt.Errorf("seedが重複または範囲外です")
		}
		seen[r.Seed] = true
		if r.Status != "succeeded" || math.IsNaN(r.Score) || math.IsInf(r.Score, 0) {
			return 0, fmt.Errorf("ケース%sが成功していません: %s", r.InputCaseID, r.Status)
		}
		mean += r.Score / float64(count)
	}
	if math.IsNaN(mean) || math.IsInf(mean, 0) {
		return 0, fmt.Errorf("目的値が有限ではありません")
	}
	return mean, nil
}
