package tuning

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
)

type Export struct {
	Path       string             `json:"path"`
	Source     string             `json:"source"`
	Parameters map[string]float64 `json:"parameters"`
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
}

func (m *Manager) Usage(id string) int64 {
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
	var total int64
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, e error) error {
		if e == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
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
		_ = filepath.WalkDir(filepath.Join(m.Root, "ahc-plaza", "runs", id), func(path string, d fs.DirEntry, e error) error {
			if e == nil && !d.IsDir() {
				if info, e := d.Info(); e == nil {
					total += info.Size()
				}
			}
			return nil
		})
	}
	m.mu.Lock()
	if m.usageCache == nil {
		m.usageCache = map[string]usageSample{}
	}
	m.usageCache[id] = usageSample{time.Now(), total}
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
