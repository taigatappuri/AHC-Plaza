package statistics

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

type Summary struct {
	Count    int     `json:"count"`
	Mean     float64 `json:"mean"`
	Median   float64 `json:"median"`
	Min      float64 `json:"min"`
	Q1       float64 `json:"q1"`
	Q3       float64 `json:"q3"`
	Max      float64 `json:"max"`
	Variance float64 `json:"variance"`
	StdDev   float64 `json:"stddev"`
}

// SummarizeRunResults はRunのスコアと実行時間を同じ統計定義で集計します。
func SummarizeRunResults(results []domain.CaseResult) (domain.RunStatistics, error) {
	if len(results) == 0 {
		return domain.RunStatistics{}, errors.New("統計対象のケース結果がありません")
	}
	scores := make([]float64, 0, len(results))
	var totalExecutionTimeNs int64
	maxExecutionTimeNs := int64(0)
	for _, result := range results {
		scores = append(scores, result.Score)
		executionTimeNs := result.ExecutionTime.Nanoseconds()
		totalExecutionTimeNs += executionTimeNs
		if executionTimeNs > maxExecutionTimeNs {
			maxExecutionTimeNs = executionTimeNs
		}
	}
	scoreSummary, err := Summarize(scores)
	if err != nil {
		return domain.RunStatistics{}, err
	}
	return domain.RunStatistics{
		CaseCount:              scoreSummary.Count,
		AverageScore:           scoreSummary.Mean,
		MinScore:               scoreSummary.Min,
		Q1Score:                scoreSummary.Q1,
		MedianScore:            scoreSummary.Median,
		Q3Score:                scoreSummary.Q3,
		MaxScore:               scoreSummary.Max,
		VarianceScore:          scoreSummary.Variance,
		StdDevScore:            scoreSummary.StdDev,
		TotalExecutionTimeNs:   totalExecutionTimeNs,
		AverageExecutionTimeNs: float64(totalExecutionTimeNs) / float64(len(results)),
		MaxExecutionTimeNs:     maxExecutionTimeNs,
	}, nil
}

type Pair struct {
	InputCaseID string  `json:"input_case_id"`
	A           float64 `json:"a"`
	B           float64 `json:"b"`
	Difference  float64 `json:"difference"`
}

type Comparison struct {
	CaseCount         int     `json:"case_count"`
	MeanA             float64 `json:"mean_a"`
	MeanB             float64 `json:"mean_b"`
	MeanImprovement   float64 `json:"mean_improvement"`
	MedianImprovement float64 `json:"median_improvement"`
	ImprovementRate   float64 `json:"improvement_rate"`
	PValue            float64 `json:"p_value"`
	ConfidenceLevel   float64 `json:"confidence_level"`
	ConfidenceLow     float64 `json:"confidence_low"`
	ConfidenceHigh    float64 `json:"confidence_high"`
	EffectSize        float64 `json:"effect_size"`
	Significant       bool    `json:"significant"`
	Pairs             []Pair  `json:"pairs,omitempty"`
}

// FilterInvalid は、設定に応じてWrong Answerのケースを統計対象から除外します。
func FilterInvalid(results []domain.CaseResult, includeInvalid bool) []domain.CaseResult {
	if includeInvalid {
		return results
	}
	filtered := make([]domain.CaseResult, 0, len(results))
	for _, result := range results {
		if result.Status != "wa" {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func Summarize(values []float64) (Summary, error) {
	if len(values) == 0 {
		return Summary{}, errors.New("統計対象のデータがありません")
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	for _, value := range ordered {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Summary{}, errors.New("NaNとInfは統計に含められません")
		}
	}
	mean := mean(ordered)
	variance := 0.0
	if len(ordered) > 1 {
		for _, value := range ordered {
			difference := value - mean
			variance += difference * difference
		}
		variance /= float64(len(ordered) - 1)
	}
	return Summary{
		Count:    len(ordered),
		Mean:     mean,
		Median:   quantile(ordered, 0.5),
		Min:      ordered[0],
		Q1:       quantile(ordered, 0.25),
		Q3:       quantile(ordered, 0.75),
		Max:      ordered[len(ordered)-1],
		Variance: variance,
		StdDev:   math.Sqrt(variance),
	}, nil
}

func Compare(a, b []domain.CaseResult, objective string, confidenceLevel float64, bootstrapIterations int, randomSeed int64) (Comparison, error) {
	if objective != "max" && objective != "min" {
		return Comparison{}, fmt.Errorf("目的が不正です: %s", objective)
	}
	if confidenceLevel <= 0 || confidenceLevel >= 1 {
		return Comparison{}, errors.New("信頼水準は0より大きく1未満です")
	}
	if bootstrapIterations <= 0 {
		return Comparison{}, errors.New("Bootstrap回数は1以上です")
	}
	pairs := pairResults(a, b, objective)
	if len(pairs) == 0 {
		return Comparison{}, errors.New("共通ケースがありません")
	}
	differences := make([]float64, len(pairs))
	valuesA := make([]float64, len(pairs))
	valuesB := make([]float64, len(pairs))
	for index, pair := range pairs {
		valuesA[index] = pair.A
		valuesB[index] = pair.B
		differences[index] = pair.Difference
	}
	summaryA, _ := Summarize(valuesA)
	summaryB, _ := Summarize(valuesB)
	summaryDifference, _ := Summarize(differences)
	pValue := pairedPermutationPValue(differences, bootstrapIterations, randomSeed)
	low, high := bootstrapMeanInterval(differences, confidenceLevel, bootstrapIterations, randomSeed+1)
	improvementRate := 0.0
	if summaryB.Mean != 0 {
		improvementRate = summaryDifference.Mean / math.Abs(summaryB.Mean)
	}
	effectSize := 0.0
	if summaryDifference.StdDev != 0 {
		effectSize = summaryDifference.Mean / summaryDifference.StdDev
	}
	return Comparison{
		CaseCount:         len(pairs),
		MeanA:             summaryA.Mean,
		MeanB:             summaryB.Mean,
		MeanImprovement:   summaryDifference.Mean,
		MedianImprovement: summaryDifference.Median,
		ImprovementRate:   improvementRate,
		PValue:            pValue,
		ConfidenceLevel:   confidenceLevel,
		ConfidenceLow:     low,
		ConfidenceHigh:    high,
		EffectSize:        effectSize,
		Significant:       pValue < 1-confidenceLevel,
		Pairs:             pairs,
	}, nil
}

func pairResults(a, b []domain.CaseResult, objective string) []Pair {
	byID := make(map[string]domain.CaseResult, len(b))
	for _, item := range b {
		byID[item.InputCaseID] = item
	}
	result := make([]Pair, 0)
	for _, itemA := range a {
		itemB, ok := byID[itemA.InputCaseID]
		if !ok {
			continue
		}
		difference := itemA.Score - itemB.Score
		if objective == "min" {
			difference = -difference
		}
		result = append(result, Pair{InputCaseID: itemA.InputCaseID, A: itemA.Score, B: itemB.Score, Difference: difference})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].InputCaseID < result[j].InputCaseID })
	return result
}

func pairedPermutationPValue(differences []float64, iterations int, seed int64) float64 {
	observed := math.Abs(mean(differences))
	if observed == 0 {
		return 1
	}
	if len(differences) <= 16 {
		permutations := 1 << len(differences)
		greaterOrEqual := 0
		for mask := 0; mask < permutations; mask++ {
			var sum float64
			for index, difference := range differences {
				if mask&(1<<index) != 0 {
					sum += difference
				} else {
					sum -= difference
				}
			}
			if math.Abs(sum/float64(len(differences))) >= observed {
				greaterOrEqual++
			}
		}
		return float64(greaterOrEqual) / float64(permutations)
	}
	random := rand.New(rand.NewSource(seed))
	greaterOrEqual := 0
	for iteration := 0; iteration < iterations; iteration++ {
		var sum float64
		for _, difference := range differences {
			if random.Intn(2) == 0 {
				sum += difference
			} else {
				sum -= difference
			}
		}
		if math.Abs(sum/float64(len(differences))) >= observed {
			greaterOrEqual++
		}
	}
	return float64(greaterOrEqual+1) / float64(iterations+1)
}

func bootstrapMeanInterval(values []float64, confidenceLevel float64, iterations int, seed int64) (float64, float64) {
	random := rand.New(rand.NewSource(seed))
	means := make([]float64, iterations)
	for iteration := range means {
		var sum float64
		for index := 0; index < len(values); index++ {
			sum += values[random.Intn(len(values))]
		}
		means[iteration] = sum / float64(len(values))
	}
	sort.Float64s(means)
	alpha := (1 - confidenceLevel) / 2
	return quantile(means, alpha), quantile(means, 1-alpha)
}

func quantile(ordered []float64, probability float64) float64 {
	if len(ordered) == 1 {
		return ordered[0]
	}
	position := probability * float64(len(ordered)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return ordered[lower]
	}
	weight := position - float64(lower)
	return ordered[lower] + (ordered[upper]-ordered[lower])*weight
}

func mean(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
