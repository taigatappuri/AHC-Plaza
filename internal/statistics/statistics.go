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
	CaseCount          int      `json:"case_count"`
	MeanA              float64  `json:"mean_a"`
	MeanB              float64  `json:"mean_b"`
	MeanImprovement    float64  `json:"mean_improvement"`
	MedianImprovement  float64  `json:"median_improvement"`
	ImprovementRate    float64  `json:"improvement_rate"`
	StatisticalMethod  string   `json:"statistical_method"`
	InferenceAvailable bool     `json:"inference_available"`
	InferenceNote      string   `json:"inference_note,omitempty"`
	PValue             *float64 `json:"p_value"`
	ConfidenceLevel    float64  `json:"confidence_level"`
	ConfidenceLow      *float64 `json:"confidence_low"`
	ConfidenceHigh     *float64 `json:"confidence_high"`
	EffectSize         float64  `json:"effect_size"`
	Significant        bool     `json:"significant"`
	Pairs              []Pair   `json:"pairs,omitempty"`
}

const (
	bootstrapTMethod                    = "paired_bootstrap_t"
	minimumValidBootstrapFraction       = 0.9
	minimumValidBootstrapReplicateCount = 2
)

type bootstrapTResult struct {
	PValue float64
	Low    float64
	High   float64
}

// FilterInvalid は、設定に応じてWAとTLEのケースを統計対象から除外します。
func FilterInvalid(results []domain.CaseResult, includeInvalid bool) []domain.CaseResult {
	if includeInvalid {
		return results
	}
	filtered := make([]domain.CaseResult, 0, len(results))
	for _, result := range results {
		if result.Status != "wa" && result.Status != "tle" {
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
	summaryA, err := Summarize(valuesA)
	if err != nil {
		return Comparison{}, fmt.Errorf("Run Aの統計を計算できません: %w", err)
	}
	summaryB, err := Summarize(valuesB)
	if err != nil {
		return Comparison{}, fmt.Errorf("Run Bの統計を計算できません: %w", err)
	}
	summaryDifference, err := Summarize(differences)
	if err != nil {
		return Comparison{}, fmt.Errorf("スコア差の統計を計算できません: %w", err)
	}
	bootstrapResult, inferenceNote := bootstrapTMean(differences, confidenceLevel, bootstrapIterations, randomSeed)
	var pValue, low, high *float64
	significant := false
	if inferenceNote == "" {
		pValue = &bootstrapResult.PValue
		low = &bootstrapResult.Low
		high = &bootstrapResult.High
		significant = bootstrapResult.Low > 0 || bootstrapResult.High < 0
	}
	improvementRate := 0.0
	if summaryB.Mean != 0 {
		improvementRate = summaryDifference.Mean / math.Abs(summaryB.Mean)
	}
	effectSize := 0.0
	if summaryDifference.StdDev != 0 {
		effectSize = summaryDifference.Mean / summaryDifference.StdDev
	}
	return Comparison{
		CaseCount:          len(pairs),
		MeanA:              summaryA.Mean,
		MeanB:              summaryB.Mean,
		MeanImprovement:    summaryDifference.Mean,
		MedianImprovement:  summaryDifference.Median,
		ImprovementRate:    improvementRate,
		StatisticalMethod:  bootstrapTMethod,
		InferenceAvailable: inferenceNote == "",
		InferenceNote:      inferenceNote,
		PValue:             pValue,
		ConfidenceLevel:    confidenceLevel,
		ConfidenceLow:      low,
		ConfidenceHigh:     high,
		EffectSize:         effectSize,
		Significant:        significant,
		Pairs:              pairs,
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

// bootstrapTMean は、対応のある差分の平均についてbootstrap-t信頼区間と両側p値を計算します。
func bootstrapTMean(values []float64, confidenceLevel float64, iterations int, seed int64) (bootstrapTResult, string) {
	if len(values) < 2 {
		return bootstrapTResult{}, "信頼区間の計算には2ケース以上必要です"
	}
	observedMean, observedStdDev := meanAndSampleStdDev(values)
	observedStandardError := observedStdDev / math.Sqrt(float64(len(values)))
	if observedStandardError == 0 || math.IsNaN(observedStandardError) || math.IsInf(observedStandardError, 0) {
		return bootstrapTResult{}, "ケース間のスコア差に分散がないため信頼区間を計算できません"
	}

	random := rand.New(rand.NewSource(seed))
	tStatistics := make([]float64, 0, iterations)
	for iteration := 0; iteration < iterations; iteration++ {
		resampledMean := 0.0
		resampledSquaredDifferenceSum := 0.0
		for index := 0; index < len(values); index++ {
			value := values[random.Intn(len(values))]
			count := float64(index + 1)
			difference := value - resampledMean
			resampledMean += difference / count
			resampledSquaredDifferenceSum += difference * (value - resampledMean)
		}
		resampledVariance := resampledSquaredDifferenceSum / float64(len(values)-1)
		if resampledVariance < 0 {
			resampledVariance = 0
		}
		resampledStdDev := math.Sqrt(resampledVariance)
		resampledStandardError := resampledStdDev / math.Sqrt(float64(len(values)))
		if resampledStandardError == 0 || math.IsNaN(resampledStandardError) || math.IsInf(resampledStandardError, 0) {
			continue
		}
		tStatistic := (resampledMean - observedMean) / resampledStandardError
		if !math.IsNaN(tStatistic) && !math.IsInf(tStatistic, 0) {
			tStatistics = append(tStatistics, tStatistic)
		}
	}
	minimumValidCount := int(math.Ceil(float64(iterations) * minimumValidBootstrapFraction))
	if minimumValidCount < minimumValidBootstrapReplicateCount {
		minimumValidCount = minimumValidBootstrapReplicateCount
	}
	if len(tStatistics) < minimumValidCount {
		return bootstrapTResult{}, "分散を計算できない再標本が多いため信頼区間を計算できません"
	}

	sort.Float64s(tStatistics)
	alpha := (1 - confidenceLevel) / 2
	lowerT := quantile(tStatistics, alpha)
	upperT := quantile(tStatistics, 1-alpha)
	low := observedMean - upperT*observedStandardError
	high := observedMean - lowerT*observedStandardError
	if math.IsNaN(low) || math.IsInf(low, 0) || math.IsNaN(high) || math.IsInf(high, 0) {
		return bootstrapTResult{}, "信頼区間が有限値にならないため計算できません"
	}

	observedT := observedMean / observedStandardError
	lowerTailCount := sort.Search(len(tStatistics), func(index int) bool {
		return tStatistics[index] > observedT
	})
	upperTailCount := len(tStatistics) - sort.SearchFloat64s(tStatistics, observedT)
	lowerTailProbability := float64(lowerTailCount+1) / float64(len(tStatistics)+1)
	upperTailProbability := float64(upperTailCount+1) / float64(len(tStatistics)+1)
	pValue := 2 * math.Min(lowerTailProbability, upperTailProbability)
	if pValue > 1 {
		pValue = 1
	}
	return bootstrapTResult{PValue: pValue, Low: low, High: high}, ""
}

func meanAndSampleStdDev(values []float64) (float64, float64) {
	currentMean := 0.0
	squaredDifferenceSum := 0.0
	for index, value := range values {
		count := float64(index + 1)
		difference := value - currentMean
		currentMean += difference / count
		squaredDifferenceSum += difference * (value - currentMean)
	}
	if len(values) < 2 {
		return currentMean, 0
	}
	variance := squaredDifferenceSum / float64(len(values)-1)
	if variance < 0 {
		variance = 0
	}
	return currentMean, math.Sqrt(variance)
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
