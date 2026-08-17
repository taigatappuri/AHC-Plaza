package usecase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/pahcer"
	"github.com/taigatappuri/AHC-Plaza/internal/process"
)

// buildCaseResults はpahcerのseedを保存済み入力ケースへ対応付けます。
// 入力ケースの並び順はDiscoverで固定されているため、同じRun内で再現できます。
func buildCaseResults(root, runID string, workspace pahcer.Workspace, inputCases []domain.InputCase, invalidScore float64) ([]domain.CaseResult, error) {
	resultFile, err := pahcer.LoadLatestResult(workspace.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(resultFile.Cases) == 0 {
		return nil, nil
	}
	waSeeds := make(map[uint64]bool, len(resultFile.WrongAnswerSeeds))
	for _, seed := range resultFile.WrongAnswerSeeds {
		waSeeds[seed] = true
	}
	results := make([]domain.CaseResult, 0, len(resultFile.Cases))
	for _, item := range resultFile.Cases {
		if item.Seed >= uint64(len(inputCases)) {
			return nil, fmt.Errorf("pahcer result seed exceeds the number of input cases: %d", item.Seed)
		}
		inputCase := inputCases[item.Seed]
		caseNumber := fmt.Sprintf("%04d", item.Seed)
		errorMessage := item.ErrorMessage
		status := "succeeded"
		if waSeeds[item.Seed] || errorMessage != "" {
			status = "wa"
			item.Score = invalidScore
		}
		outputPath := filepath.Join(workspace.Dir, "tools", "out", caseNumber+".txt")
		stderrPath := filepath.Join(workspace.Dir, "tools", "err", caseNumber+".txt")
		results = append(results, domain.CaseResult{
			RunID:         runID,
			InputCaseID:   inputCase.ID,
			Seed:          item.Seed,
			InputPath:     relativePath(root, inputCase.Path),
			Score:         item.Score,
			ExecutionTime: pahcer.ExecutionDuration(item.ExecutionTime),
			Status:        status,
			StdoutPath:    relativePath(root, outputPath),
			StderrPath:    relativePath(root, stderrPath),
			OutputPath:    relativePath(root, outputPath),
			ErrorMessage:  errorMessage,
		})
	}
	return results, nil
}

func processToRunStatus(status process.Status) domain.RunStatus {
	switch status {
	case process.StatusSucceeded:
		return domain.RunSucceeded
	case process.StatusCancelled:
		return domain.RunCancelled
	default:
		return domain.RunFailed
	}
}
