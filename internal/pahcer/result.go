package pahcer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ResultCase struct {
	Seed          uint64  `json:"seed"`
	Score         float64 `json:"score"`
	ExecutionTime float64 `json:"execution_time"`
	ErrorMessage  string  `json:"error_message"`
}

type ResultFile struct {
	WrongAnswerSeeds []uint64     `json:"wa_seeds"`
	Cases            []ResultCase `json:"cases"`
}

// TimedOut はケース実行ラッパーが返すタイムアウト用の終了コードを判定します。
func (r ResultCase) TimedOut() bool {
	return strings.Contains(r.ErrorMessage, "(exit status: 124)")
}

func LoadLatestResult(workspaceDir string) (ResultFile, error) {
	paths, err := filepath.Glob(filepath.Join(workspaceDir, "pahcer", "json", "result_*.json"))
	if err != nil {
		return ResultFile{}, fmt.Errorf("could not find pahcer result: %w", err)
	}
	if len(paths) == 0 {
		return ResultFile{}, os.ErrNotExist
	}
	sort.Strings(paths)
	content, err := os.ReadFile(paths[len(paths)-1])
	if err != nil {
		return ResultFile{}, fmt.Errorf("could not read pahcer result: %w", err)
	}
	var result ResultFile
	if err := json.Unmarshal(content, &result); err != nil {
		return ResultFile{}, fmt.Errorf("could not parse pahcer result JSON: %w", err)
	}
	return result, nil
}

func ExecutionDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
