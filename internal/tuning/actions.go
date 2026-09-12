package tuning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
	"github.com/taigatappuri/AHC-Plaza/internal/usecase"
)

type Export struct {
	Path       string             `json:"path"`
	Source     string             `json:"source"`
	Parameters map[string]float64 `json:"parameters"`
}

type DeleteResult struct {
	ID         string `json:"id"`
	FreedBytes int64  `json:"freed_bytes"`
}

func (m *Manager) Delete(ctx context.Context, id string) (DeleteResult, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	study, e := m.Store.GetStudy(ctx, id)
	if e != nil {
		return DeleteResult{}, e
	}
	m.mu.Lock()
	_, active := m.active[id]
	m.mu.Unlock()
	if active || study.Status == "preparing" || study.Status == "running" || study.Status == "stopping" {
		return DeleteResult{}, fmt.Errorf("実行中または停止処理中のStudyは削除できません")
	}
	runIDs, e := m.Store.TuningRunIDs(ctx, id)
	if e != nil {
		return DeleteResult{}, e
	}
	studyDir, e := m.dir(id)
	if e != nil {
		return DeleteResult{}, e
	}
	paths := make([]string, 0, len(runIDs)+1)
	for _, runID := range runIDs {
		if runID == "" || !filepath.IsLocal(runID) || strings.ContainsAny(runID, "/\\") || strings.HasPrefix(runID, ".") {
			return DeleteResult{}, fmt.Errorf("保存されたRun IDが不正です")
		}
		paths = append(paths, filepath.Join(m.Root, "ahc-plaza", "runs", runID))
	}
	paths = append(paths, studyDir)
	for _, parent := range []string{filepath.Join(m.Root, "ahc-plaza"), filepath.Join(m.Root, "ahc-plaza", "runs"), filepath.Join(m.Root, "ahc-plaza", "tuning")} {
		info, statErr := os.Lstat(parent)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return DeleteResult{}, fmt.Errorf("削除対象の親が通常のディレクトリではありません: %s", parent)
		}
	}
	freed := m.Usage(id)
	tag, e := usecase.NewRunID()
	if e != nil {
		return DeleteResult{}, e
	}
	type movedPath struct{ from, to string }
	moved := []movedPath{}
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			_ = os.Rename(moved[i].to, moved[i].from)
		}
	}
	for _, path := range paths {
		info, statErr := os.Lstat(path)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			rollback()
			return DeleteResult{}, fmt.Errorf("削除対象が通常のディレクトリではありません: %s", path)
		}
		to := path + ".deleting-" + tag
		if e = os.Rename(path, to); e != nil {
			rollback()
			return DeleteResult{}, e
		}
		moved = append(moved, movedPath{path, to})
	}
	deletedRunIDs, e := m.Store.DeleteTuningStudy(ctx, id)
	if e != nil {
		rollback()
		return DeleteResult{}, e
	}
	for _, path := range moved {
		if e := os.RemoveAll(path.to); e != nil {
			fmt.Fprintln(os.Stderr, "tuning deletion cleanup:", e)
		}
	}
	m.mu.Lock()
	delete(m.usageCache, id)
	for _, runID := range deletedRunIDs {
		delete(m.runUsageCache, runID)
	}
	m.mu.Unlock()
	return DeleteResult{ID: id, FreedBytes: freed}, nil
}

func (m *Manager) candidate(ctx context.Context, s domain.TuningStudy, number *int) (map[string]float64, error) {
	if number == nil {
		if s.BestValue == nil {
			return nil, fmt.Errorf("成功した評価がありません")
		}
		return s.BestParams, nil
	}
	trials, e := m.Store.Trials(ctx, s.ID, 0, 10000)
	if e != nil {
		return nil, e
	}
	for _, t := range trials {
		if t.Number == *number && t.Status == "COMPLETE" {
			return t.Params, nil
		}
	}
	return nil, fmt.Errorf("成功したTrialを指定してください")
}
func (m *Manager) Export(ctx context.Context, id string, number *int) (Export, error) {
	s, v, e := m.Load(ctx, id)
	if e != nil {
		return Export{}, e
	}
	values, e := m.candidate(ctx, s, number)
	if e != nil {
		return Export{}, e
	}
	dir, _ := m.dir(id)
	original, e := os.ReadFile(filepath.Join(dir, "fixed", "original.cpp"))
	if e != nil {
		return Export{}, e
	}
	source, actual, e := params.Generate(original, v.SourceHash, v.Parameters, values)
	if e != nil {
		return Export{}, e
	}
	exports := filepath.Join(dir, "exports")
	if e = os.MkdirAll(exports, 0700); e != nil {
		return Export{}, e
	}
	name := "main.best"
	if number != nil {
		name = fmt.Sprintf("main.trial-%d", *number)
	}
	path := filepath.Join(exports, name+".cpp")
	// 同じビルド条件で成功したRunのスナップショットと一致することを確認します。
	selectedRun := s.BestRun
	if number != nil {
		ts, e := m.Store.Trials(ctx, id, 0, 10000)
		if e != nil {
			return Export{}, e
		}
		for _, t := range ts {
			if t.Number == *number {
				selectedRun = t.RunID
			}
		}
	}
	run, e := m.Store.GetRun(ctx, selectedRun)
	if e != nil {
		return Export{}, e
	}
	if params.Hash(source) != run.SourceHash {
		return Export{}, fmt.Errorf("保存した成功Runと書き出しソースが一致しません")
	}
	if e = atomicFile(path, source); e != nil {
		return Export{}, e
	}
	b, _ := json.MarshalIndent(map[string]any{"study_id": id, "run_id": selectedRun, "parameters": values, "actual": actual, "source_hash": params.Hash(source)}, "", "  ")
	if e = atomicFile(filepath.Join(exports, name+".json"), b); e != nil {
		return Export{}, e
	}
	rel, _ := filepath.Rel(m.Root, path)
	return Export{Path: rel, Source: string(source), Parameters: values}, nil
}

type usageSample struct {
	at    time.Time
	bytes int64
	runs  map[string]bool
}

func (m *Manager) Usage(id string) int64 {
	m.usageMu.Lock()
	defer m.usageMu.Unlock()
	m.mu.Lock()
	sample, ok := m.usageCache[id]
	m.mu.Unlock()
	if ok && time.Since(sample.at) < 30*time.Second {
		return sample.bytes
	}
	dir, e := m.dir(id)
	if e != nil {
		return 0
	}
	total := directorySize(dir)
	ts, _ := m.Store.Trials(context.Background(), id, 0, 10000)
	s, e := m.Store.GetStudy(context.Background(), id)
	if e != nil {
		return total
	}
	ids := []string{s.BaselineRun}
	for _, t := range ts {
		ids = append(ids, t.RunID)
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		m.mu.Lock()
		bytes, ok := m.runUsageCache[id]
		m.mu.Unlock()
		if !ok {
			bytes = directorySize(filepath.Join(m.Root, "ahc-plaza", "runs", id))
			m.mu.Lock()
			if m.runUsageCache == nil {
				m.runUsageCache = map[string]int64{}
			}
			m.runUsageCache[id] = bytes
			m.mu.Unlock()
		}
		total += bytes
	}
	m.mu.Lock()
	if m.usageCache == nil {
		m.usageCache = map[string]usageSample{}
	}
	includedRuns := make(map[string]bool, len(ids))
	for _, runID := range ids {
		if runID != "" {
			includedRuns[runID] = true
		}
	}
	m.usageCache[id] = usageSample{at: time.Now(), bytes: total, runs: includedRuns}
	m.mu.Unlock()
	return total
}

// ToolHealth はユーザー環境を変更せず同梱ライブラリの起動を確認します。
func ToolHealth(ctx context.Context, python string) error {
	timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	b, e := exec.CommandContext(timeout, python, "-I", "-B", "-c", "import optuna, numpy, sqlalchemy; print(optuna.__version__)").CombinedOutput()
	if e != nil {
		return fmt.Errorf("同梱環境を実行できません: %v: %s", e, b)
	}
	return nil
}

func (m *Manager) LiveStudy(s domain.TuningStudy) (domain.TuningStudy, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.active[s.ID]
	if a == nil {
		return s, false
	}
	if !a.started.IsZero() {
		s.Elapsed = a.priorElapsed + time.Since(a.started).Seconds()
	}
	if a.pause.Load() {
		s.Status = "stopping"
	}
	return s, true
}
