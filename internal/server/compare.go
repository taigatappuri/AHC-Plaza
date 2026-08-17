package server

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/statistics"
)

const maxComparisonConditions = 32

var comparisonOperators = map[string]func(float64, float64) bool{
	"<":  func(a, b float64) bool { return a < b },
	"<=": func(a, b float64) bool { return a <= b },
	"=":  func(a, b float64) bool { return a == b },
	"!=": func(a, b float64) bool { return a != b },
	">=": func(a, b float64) bool { return a >= b },
	">":  func(a, b float64) bool { return a > b },
}

type comparisonFilterSummary struct {
	Active            bool               `json:"active"`
	Conditions        []featureCondition `json:"conditions"`
	OriginalCaseCount int                `json:"original_case_count"`
	MatchedCaseCount  int                `json:"matched_case_count"`
	ExcludedCaseCount int                `json:"excluded_case_count"`
	UnreadableCount   int                `json:"unreadable_count"`
}

type comparisonResponse struct {
	statistics.Comparison
	Filter comparisonFilterSummary `json:"filter"`
}

func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	var input compareRequest
	if writeErrorIf(w, http.StatusBadRequest, decodeJSON(r, &input)) {
		return
	}
	runA, err := s.Store.GetRun(r.Context(), input.RunA)
	if writeErrorIf(w, http.StatusNotFound, err) {
		return
	}
	runB, err := s.Store.GetRun(r.Context(), input.RunB)
	if writeErrorIf(w, http.StatusNotFound, err) {
		return
	}
	if runA.Problem != runB.Problem || runA.Objective != runB.Objective {
		writeError(w, http.StatusBadRequest, errors.New("異なる問題または目的は比較できません"))
		return
	}
	casesA, err := s.Store.GetCaseResults(r.Context(), input.RunA)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	casesB, err := s.Store.GetCaseResults(r.Context(), input.RunB)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	cfg, err := config.Load(s.ConfigPath)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	if writeErrorIf(w, http.StatusBadRequest, normalizeAndValidateConditions(input.Conditions, cfg.File.InputFormat)) {
		return
	}
	casesA = statistics.FilterInvalid(casesA, cfg.File.Score.IncludeInvalidCases)
	casesB = statistics.FilterInvalid(casesB, cfg.File.Score.IncludeInvalidCases)
	filterSummary := comparisonFilterSummary{
		Active:     len(input.Conditions) > 0,
		Conditions: input.Conditions,
	}
	if len(input.Conditions) > 0 {
		conditionFormat := inputFormatForConditions(input.Conditions, cfg.File.InputFormat)
		dataA := s.buildFeatureDataForInputFormat(r.Context(), casesA, conditionFormat)
		dataB := s.buildFeatureDataForInputFormat(r.Context(), casesB, conditionFormat)
		casesA, casesB, filterSummary = filterComparisonCases(casesA, casesB, dataA, dataB, input.Conditions)
	} else {
		filterSummary.OriginalCaseCount = commonCaseCount(casesA, casesB)
		filterSummary.MatchedCaseCount = filterSummary.OriginalCaseCount
	}
	comparison, err := statistics.Compare(casesA, casesB, runA.Objective, cfg.File.Statistics.ConfidenceLevel, cfg.File.Statistics.BootstrapIterations, 1)
	if writeErrorIf(w, http.StatusBadRequest, err) {
		return
	}
	filterSummary.MatchedCaseCount = comparison.CaseCount
	writeJSON(w, http.StatusOK, comparisonResponse{Comparison: comparison, Filter: filterSummary})
}

func normalizeAndValidateConditions(conditions []featureCondition, inputFormat config.InputFormatConfig) error {
	if len(conditions) > maxComparisonConditions {
		return fmt.Errorf("入力条件は%d件以下にしてください", maxComparisonConditions)
	}
	featureNames := make(map[string]struct{}, len(inputFormat.Variables)+len(inputFormat.Features))
	for _, variable := range inputFormat.Variables {
		featureNames[variable.Name] = struct{}{}
	}
	for _, feature := range inputFormat.Features {
		featureNames[feature.Name] = struct{}{}
	}
	for index := range conditions {
		conditions[index].Feature = strings.TrimSpace(conditions[index].Feature)
		if _, exists := featureNames[conditions[index].Feature]; !exists {
			return fmt.Errorf("入力条件%dの特徴量が設定されていません: %q", index+1, conditions[index].Feature)
		}
		if _, exists := comparisonOperators[conditions[index].Operator]; !exists {
			return fmt.Errorf("入力条件%dの演算子が不正です: %q", index+1, conditions[index].Operator)
		}
		if conditions[index].Value == nil || math.IsNaN(*conditions[index].Value) || math.IsInf(*conditions[index].Value, 0) {
			return fmt.Errorf("入力条件%dの値が不正です", index+1)
		}
	}
	return nil
}

func inputFormatForConditions(conditions []featureCondition, inputFormat config.InputFormatConfig) config.InputFormatConfig {
	wanted := make(map[string]struct{}, len(conditions))
	for _, condition := range conditions {
		wanted[condition.Feature] = struct{}{}
	}
	result := config.InputFormatConfig{
		Variables: make([]config.InputVariableConfig, 0, len(wanted)),
		Features:  make([]config.InputFeatureConfig, 0, len(wanted)),
	}
	for _, variable := range inputFormat.Variables {
		if _, exists := wanted[variable.Name]; exists {
			result.Variables = append(result.Variables, variable)
		}
	}
	for _, feature := range inputFormat.Features {
		if _, exists := wanted[feature.Name]; exists {
			result.Features = append(result.Features, feature)
		}
	}
	return result
}

func filterComparisonCases(casesA, casesB []domain.CaseResult, dataA, dataB featureDataResponse, conditions []featureCondition) ([]domain.CaseResult, []domain.CaseResult, comparisonFilterSummary) {
	resultsB := make(map[string]domain.CaseResult, len(casesB))
	for _, item := range casesB {
		resultsB[item.InputCaseID] = item
	}
	valuesA := featureValuesByCase(dataA)
	valuesB := featureValuesByCase(dataB)
	filteredA := make([]domain.CaseResult, 0)
	filteredB := make([]domain.CaseResult, 0)
	summary := comparisonFilterSummary{Active: true, Conditions: conditions}
	for _, itemA := range casesA {
		itemB, exists := resultsB[itemA.InputCaseID]
		if !exists {
			continue
		}
		summary.OriginalCaseCount++
		caseValuesA := valuesA[itemA.InputCaseID]
		caseValuesB := valuesB[itemA.InputCaseID]
		readable := true
		matches := true
		for _, condition := range conditions {
			valueA, okA := caseValuesA[condition.Feature]
			valueB, okB := caseValuesB[condition.Feature]
			if !okA || !okB {
				readable = false
				continue
			}
			if !matchesFeatureCondition(valueA, condition) || !matchesFeatureCondition(valueB, condition) {
				matches = false
			}
		}
		if !readable {
			summary.UnreadableCount++
			continue
		}
		if !matches {
			summary.ExcludedCaseCount++
			continue
		}
		filteredA = append(filteredA, itemA)
		filteredB = append(filteredB, itemB)
	}
	summary.MatchedCaseCount = len(filteredA)
	return filteredA, filteredB, summary
}

func featureValuesByCase(data featureDataResponse) map[string]map[string]float64 {
	result := make(map[string]map[string]float64, len(data.Cases))
	for _, item := range data.Cases {
		result[item.InputCaseID] = item.Values
	}
	return result
}

func matchesFeatureCondition(value float64, condition featureCondition) bool {
	compare, ok := comparisonOperators[condition.Operator]
	return ok && compare(value, *condition.Value)
}

func commonCaseCount(casesA, casesB []domain.CaseResult) int {
	idsB := make(map[string]struct{}, len(casesB))
	for _, item := range casesB {
		idsB[item.InputCaseID] = struct{}{}
	}
	count := 0
	for _, item := range casesA {
		if _, exists := idsB[item.InputCaseID]; exists {
			count++
		}
	}
	return count
}
