package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/inputanalysis"
	"github.com/taigatappuri/AHC-Plaza/internal/inputfeature"
)

const maxFeatureAnalysisIssues = 50

func (s *Server) buildFeatureData(ctx context.Context, cases []domain.CaseResult) (featureDataResponse, error) {
	cfg, err := config.Load(s.ConfigPath)
	if err != nil {
		return featureDataResponse{}, err
	}
	return s.buildFeatureDataForInputFormat(ctx, cases, cfg.File.InputFormat), nil
}

func (s *Server) buildFeatureDataForInputFormat(ctx context.Context, cases []domain.CaseResult, inputFormat config.InputFormatConfig) featureDataResponse {
	variables := inputFormat.Variables
	features := inputFormat.Features
	response := featureDataResponse{
		Features: make([]inputFeatureDefinition, 0, len(variables)+len(features)),
		Cases:    make([]featureCaseValues, 0, len(cases)),
		Issues:   make([]featureExtractionIssue, 0),
	}
	for _, variable := range variables {
		response.Features = append(response.Features, inputFeatureDefinition{
			Name: variable.Name, Kind: "position", Line: variable.Line, Column: variable.Column,
		})
	}
	for _, feature := range features {
		response.Features = append(response.Features, inputFeatureDefinition{
			Name: feature.Name, Kind: "cpp", Source: feature.Source,
		})
	}
	if len(variables) == 0 && len(features) == 0 {
		return response
	}

	type preparedFeature struct {
		config  config.InputFeatureConfig
		program inputfeature.Program
		err     error
	}
	preparedFeatures := make([]preparedFeature, 0, len(features))
	for _, feature := range features {
		program, _, err := s.featureRunner.Prepare(ctx, feature.Source, feature.TimeoutMilliseconds)
		preparedFeatures = append(preparedFeatures, preparedFeature{config: feature, program: program, err: err})
	}

	for _, item := range cases {
		caseValues := featureCaseValues{
			InputCaseID: item.InputCaseID,
			Values:      make(map[string]float64),
		}
		if item.InputPath == "" || filepath.IsAbs(item.InputPath) {
			response.addFeatureIssue(item.InputCaseID, "", "入力パスが不正です")
			response.Cases = append(response.Cases, caseValues)
			continue
		}
		inputPath := filepath.Join(s.Root, filepath.FromSlash(item.InputPath))
		within, pathErr := config.IsWithin(s.Root, inputPath)
		if pathErr != nil || !within {
			response.addFeatureIssue(item.InputCaseID, "", "入力パスがプロジェクト外です")
			response.Cases = append(response.Cases, caseValues)
			continue
		}
		if len(variables) > 0 {
			file, openErr := os.Open(inputPath)
			if openErr != nil {
				response.addFeatureIssue(item.InputCaseID, "", fmt.Sprintf("入力ファイルを開けません: %v", openErr))
				response.Cases = append(response.Cases, caseValues)
				continue
			}
			extracted, extractErr := inputanalysis.Extract(file, variables)
			closeErr := file.Close()
			if extractErr != nil {
				response.addFeatureIssue(item.InputCaseID, "", extractErr.Error())
			} else if closeErr != nil {
				response.addFeatureIssue(item.InputCaseID, "", fmt.Sprintf("入力ファイルを閉じられません: %v", closeErr))
			} else {
				for name, value := range extracted.Values {
					caseValues.Values[name] = value
				}
				for _, variable := range variables {
					if message := extracted.Errors[variable.Name]; message != "" {
						response.addFeatureIssue(item.InputCaseID, variable.Name, message)
					}
				}
			}
		}
		for _, feature := range preparedFeatures {
			if feature.err != nil {
				response.addFeatureIssue(item.InputCaseID, feature.config.Name, feature.err.Error())
				continue
			}
			value, _, evaluateErr := feature.program.EvaluateFile(ctx, inputPath)
			if evaluateErr != nil {
				response.addFeatureIssue(item.InputCaseID, feature.config.Name, evaluateErr.Error())
				continue
			}
			caseValues.Values[feature.config.Name] = value
		}
		response.Cases = append(response.Cases, caseValues)
	}
	return response
}

func (response *featureDataResponse) addFeatureIssue(inputCaseID, feature, message string) {
	response.IssueCount++
	if len(response.Issues) >= maxFeatureAnalysisIssues {
		return
	}
	response.Issues = append(response.Issues, featureExtractionIssue{
		InputCaseID: inputCaseID,
		Feature:     feature,
		Message:     message,
	})
}
