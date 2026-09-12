package tuning

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
	"github.com/taigatappuri/AHC-Plaza/internal/store"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
	bundled "github.com/taigatappuri/AHC-Plaza/internal/tuning/runtime"
	"github.com/taigatappuri/AHC-Plaza/internal/usecase"
)

var ErrSourceChanged = errors.New("ソースが変更されています。再検出してください")

// caseExecCompatibilityVersionはケース実行または結果評価の互換性を壊す変更時に増やします。
const caseExecCompatibilityVersion = 1

type StartRequest struct {
	Solver     string             `json:"solver"`
	InputDir   string             `json:"input_dir"`
	SourceHash string             `json:"source_hash"`
	Parameters []params.Parameter `json:"parameters"`
	Trials     int                `json:"trials"`
	Threads    int                `json:"threads"`
	TimeoutMS  int                `json:"timeout_ms"`
	Seed       int                `json:"seed"`
}
type Manifest struct {
	Python           string              `json:"python"`
	Optuna           string              `json:"optuna"`
	ParserVersion    int                 `json:"parser_version"`
	ObjectiveVersion string              `json:"objective_version"`
	CaseExecVersion  int                 `json:"case_exec_version,omitempty"`
	Version          int                 `json:"version"`
	WorkerHash       string              `json:"worker_hash"`
	RuntimeHash      string              `json:"runtime_hash"`
	PlazaVersion     string              `json:"plaza_version"`
	Prepared         usecase.PreparedRun `json:"prepared"`
	Request          StartRequest        `json:"request"`
	SourceHash       string              `json:"source_hash"`
	Parameters       []params.Parameter  `json:"parameters"`
	Files            map[string]string   `json:"files"`
	Tools            map[string]string   `json:"tools"`
}
type activity struct {
	cancel       context.CancelFunc
	pause        atomic.Bool
	done         chan struct{}
	started      time.Time
	priorElapsed float64
}
type Manager struct {
	operationMu               sync.Mutex
	Root, ConfigPath, Version string
	Store                     *store.SQLiteStore
	mu                        sync.Mutex
	active                    map[string]*activity
	closed                    bool
	wg                        sync.WaitGroup
	usageCache                map[string]usageSample
	runUsageCache             map[string]int64
	usageMu                   sync.Mutex
}

func NewManager(root, configPath, version string, database *store.SQLiteStore) (*Manager, error) {
	m := &Manager{Root: root, ConfigPath: configPath, Version: version, Store: database, active: map[string]*activity{}}
	if e := cleanupDeletionTrash(root); e != nil {
		fmt.Fprintln(os.Stderr, "tuning deletion cleanup:", e)
	}
	studies, e := database.ListStudies(context.Background())
	if e != nil {
		return nil, e
	}
	for _, s := range studies {
		if s.Status == "preparing" || s.Status == "running" || s.Status == "stopping" {
			s.Status = "paused"
			s.Error = "前回の実行が中断されました。保存した条件で再開できます"
			if e = m.save(&s); e != nil {
				return nil, e
			}
		}
	}
	return m, nil
}

func cleanupDeletionTrash(root string) error {
	dataRoot := filepath.Join(root, "ahc-plaza")
	if info, e := os.Lstat(dataRoot); e == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return fmt.Errorf("削除待ち領域の親が通常のディレクトリではありません: %s", dataRoot)
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	for _, parent := range []string{filepath.Join(root, "ahc-plaza", "tuning"), filepath.Join(root, "ahc-plaza", "runs")} {
		info, e := os.Lstat(parent)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("削除待ち領域の親が通常のディレクトリではありません: %s", parent)
		}
		entries, e := os.ReadDir(parent)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), ".deleting-") {
				if e := os.RemoveAll(filepath.Join(parent, entry.Name())); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func (m *Manager) save(s *domain.TuningStudy) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.UpdatedAt = time.Now().UTC()
	err := m.Store.SaveStudy(ctx, *s)
	return err
}
func (m *Manager) dir(id string) (string, error) {
	if id == "" || !filepath.IsLocal(id) || strings.ContainsAny(id, "/\\") || strings.HasPrefix(id, ".") {
		return "", fmt.Errorf("Study IDが不正です")
	}
	return filepath.Join(m.Root, "ahc-plaza", "tuning", id), nil
}
func (m *Manager) Scan(solver string) (params.Scan, error) {
	cfg, e := config.Load(m.ConfigPath)
	if e != nil {
		return params.Scan{}, e
	}
	path, e := cfg.ResolveProjectPath("solver", solver)
	if e != nil {
		return params.Scan{}, e
	}
	info, e := os.Stat(path)
	if e != nil {
		return params.Scan{}, e
	}
	if !info.Mode().IsRegular() || info.Size() > params.MaxSource {
		return params.Scan{}, fmt.Errorf("ソースファイルは8 MiB以下の通常ファイルにしてください")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return params.Scan{}, e
	}
	return params.Parse(b)
}
func (m *Manager) Start(ctx context.Context, r StartRequest) (domain.TuningStudy, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	s := domain.TuningStudy{}
	if r.Trials < 1 || r.Trials > 10000 || r.Threads < 0 || r.Threads > 256 || r.TimeoutMS < 0 || r.Seed < 0 || uint64(r.Seed) > 4294967295 {
		return s, fmt.Errorf("試行数(1〜10000)・並列数・タイムアウト・seedを確認してください")
	}
	m.mu.Lock()
	unavailable := m.closed || len(m.active) > 0
	m.mu.Unlock()
	if unavailable {
		return s, fmt.Errorf("別のStudyが実行中または終了中です")
	}
	info := bundled.Status(m.Root)
	if !info.Available {
		return s, fmt.Errorf("Python環境を同梱した配布ビルドが必要です")
	}
	scan, e := m.Scan(r.Solver)
	if e != nil {
		return s, e
	}
	if scan.Hash != r.SourceHash {
		return s, ErrSourceChanged
	}
	ps, e := params.Resolve(scan, r.Parameters)
	if e != nil {
		return s, e
	}
	if r.Threads == 0 {
		r.Threads = min(4, max(1, (runtime.NumCPU()+1)/2))
	}
	id, e := usecase.NewRunID()
	if e != nil {
		return s, e
	}
	dir, _ := m.dir(id)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return s, e
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(dir)
		}
	}()
	p, e := usecase.PrepareTuningInputs(ctx, usecase.RunRequest{ConfigPath: m.ConfigPath, InputDir: r.InputDir}, filepath.Join(dir, "fixed"))
	if e != nil {
		return s, e
	}
	if r.TimeoutMS == 0 {
		r.TimeoutMS = p.Config.File.Execution.TimeoutMilliseconds
	}
	if r.TimeoutMS < 1 || r.TimeoutMS > 86400000 {
		return s, fmt.Errorf("タイムアウトは1〜86400000 msです")
	}
	inspection, e := inspectBuild(ctx, p.SettingFile, p.ProjectDir, r.Solver, p.Config.File.Tuning.SourceTarget)
	if e != nil {
		return s, e
	}
	p.SourceTarget = inspection.SourceTarget
	if e = os.WriteFile(filepath.Join(dir, "fixed", "original.cpp"), []byte(scan.Source), 0600); e != nil {
		return s, e
	}
	files, e := hashTree(filepath.Join(dir, "fixed"))
	if e != nil {
		return s, e
	}
	p.CompilerVersion = "see tuning manifest v3"
	p.PahcerVersion = "see tuning manifest v3"
	manifest := Manifest{Python: info.Python, Optuna: info.Optuna, ParserVersion: params.Version, ObjectiveVersion: "raw-mean-v1", CaseExecVersion: caseExecCompatibilityVersion, Version: 3, WorkerHash: workerHash(), RuntimeHash: info.SHA256, PlazaVersion: m.Version, Prepared: p, Request: r, SourceHash: scan.Hash, Parameters: ps, Files: files, Tools: inspection.Tools}
	b, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		return s, e
	}
	if e = atomicFile(filepath.Join(dir, "manifest.json"), b); e != nil {
		return s, e
	}
	baseline, e := usecase.NewRunID()
	if e != nil {
		return s, e
	}
	s = domain.TuningStudy{ID: id, Status: "preparing", Trials: r.Trials, ManifestHash: params.Hash(b), BaselineRun: baseline, CreatedAt: time.Now().UTC()}
	if e = m.save(&s); e != nil {
		return s, e
	}
	success = true
	e = m.launch(s, manifest)
	return s, e
}
func (m *Manager) Load(ctx context.Context, id string) (domain.TuningStudy, Manifest, error) {
	s, e := m.Store.GetStudy(ctx, id)
	if e != nil {
		return s, Manifest{}, e
	}
	dir, e := m.dir(id)
	if e != nil {
		return s, Manifest{}, e
	}
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return s, Manifest{}, e
	}
	if params.Hash(b) != s.ManifestHash {
		return s, Manifest{}, fmt.Errorf("manifestが変更されています")
	}
	var v Manifest
	e = json.Unmarshal(b, &v)
	return s, v, e
}
func (m *Manager) Resume(ctx context.Context, id string, additional int) (domain.TuningStudy, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	m.mu.Lock()
	_, active := m.active[id]
	closed := m.closed
	m.mu.Unlock()
	if active || closed {
		return domain.TuningStudy{}, fmt.Errorf("実行中または終了中です")
	}
	s, v, e := m.Load(ctx, id)
	if e != nil {
		return s, e
	}
	if additional < 0 || s.Trials+additional > 10000 {
		return s, fmt.Errorf("追加試行数が不正です")
	}
	if e = m.verify(ctx, s, v); e != nil {
		return s, e
	}
	s.Trials += additional
	s.Status = "preparing"
	s.Error = ""
	if e = m.save(&s); e != nil {
		return s, e
	}
	return s, m.launch(s, v)
}
func (m *Manager) verify(ctx context.Context, s domain.TuningStudy, v Manifest) error {
	if v.Version == 1 {
		return fmt.Errorf("manifest v1のStudyは閲覧と書き出しのみ対応しています。再開するには新しいStudyを作成してください")
	}
	if v.Version != 2 && v.Version != 3 {
		return fmt.Errorf("未対応のmanifest version %dです。新しいStudyを作成してください", v.Version)
	}
	if e := verifyCaseExecCompatibility(v); e != nil {
		return e
	}
	if v.ParserVersion != params.Version || v.ObjectiveVersion != "raw-mean-v1" || v.WorkerHash != workerHash() || v.RuntimeHash != bundled.Status(m.Root).SHA256 || v.PlazaVersion != m.Version {
		return fmt.Errorf("Plazaまたは同梱環境の版が変わっています。環境を復元するか新しいStudyを作成してください")
	}
	dir, _ := m.dir(s.ID)
	files, e := hashTree(filepath.Join(dir, "fixed"))
	if e != nil {
		return e
	}
	if !sameJSON(files, v.Files) {
		return fmt.Errorf("固定した入力・tools・ソースが変更されています")
	}
	inspection, e := inspectBuild(ctx, v.Prepared.SettingFile, v.Prepared.ProjectDir, v.Request.Solver, v.Prepared.SourceTarget)
	if e != nil {
		return e
	}
	return verifyBuildCompatibility(v, inspection)
}

func verifyCaseExecCompatibility(v Manifest) error {
	if v.Version == 3 && v.CaseExecVersion != caseExecCompatibilityVersion {
		return fmt.Errorf("case-exec互換性が変わっています。新しいStudyを作成してください")
	}
	return nil
}

func verifyBuildCompatibility(v Manifest, current BuildInspection) error {
	if current.SourceTarget != v.Prepared.SourceTarget {
		return fmt.Errorf("source_targetが変更されています。保存時の設定を復元してください")
	}
	savedTools := v.Tools
	if v.Version == 2 {
		savedTools = make(map[string]string, len(v.Tools))
		for key, value := range v.Tools {
			if key != "runner:ahc-plaza" {
				savedTools[key] = value
			}
		}
	}
	keys := make(map[string]struct{}, len(savedTools)+len(current.Tools))
	for key := range savedTools {
		keys[key] = struct{}{}
	}
	for key := range current.Tools {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if savedTools[key] != current.Tools[key] {
			return fmt.Errorf("外部ツール %q が変更されています。保存時の環境を復元してください", key)
		}
	}
	return nil
}
func (m *Manager) launch(s domain.TuningStudy, v Manifest) error {
	ctx, cancel := context.WithCancel(context.Background())
	a := &activity{cancel: cancel, done: make(chan struct{}), started: time.Now(), priorElapsed: s.Elapsed}
	m.mu.Lock()
	if m.closed || len(m.active) > 0 {
		m.mu.Unlock()
		cancel()
		s.Status = "paused"
		s.Error = "別のStudyが実行中です"
		m.save(&s)
		return fmt.Errorf("同時に実行できるStudyは1つです")
	}
	m.active[s.ID] = a
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		defer cancel()
		defer func() { m.mu.Lock(); delete(m.active, s.ID); m.mu.Unlock(); close(a.done) }()
		m.run(ctx, a, &s, v)
	}()
	return nil
}
func (m *Manager) Pause(id string, immediate bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.active[id]
	if a == nil {
		return fmt.Errorf("Studyは実行中ではありません")
	}
	a.pause.Store(true)
	if immediate {
		a.cancel()
	}
	return nil
}
func (m *Manager) BeginShutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, a := range m.active {
		a.pause.Store(true)
		a.cancel()
	}
}
func (m *Manager) Close() {
	m.BeginShutdown()
	m.operationMu.Lock()
	m.operationMu.Unlock()
	m.wg.Wait()
}
func (m *Manager) Wait(ctx context.Context, id string) error {
	m.mu.Lock()
	a := m.active[id]
	m.mu.Unlock()
	if a == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.done:
		return nil
	}
}
func (m *Manager) Busy() bool { m.mu.Lock(); defer m.mu.Unlock(); return len(m.active) > 0 }
func (m *Manager) run(ctx context.Context, a *activity, s *domain.TuningStudy, v Manifest) {
	start := time.Now()
	before := s.Elapsed
	defer func() {
		s.Elapsed = before + time.Since(start).Seconds()
		if ctx.Err() != nil || a.pause.Load() {
			s.Status = "paused"
		}
		if e := m.save(s); e != nil {
			fmt.Fprintln(os.Stderr, "tuning state persistence:", e)
		}
	}()
	fail := func(e error) { s.Status = "failed"; s.Error = e.Error() }
	dir, _ := m.dir(s.ID)
	w, e := startWorker(ctx, m.Root, dir)
	if e != nil {
		fail(e)
		return
	}
	defer w.close()
	direction := "maximize"
	if v.Prepared.Config.File.Project.Objective == "min" {
		direction = "minimize"
	}
	e = w.call(ctx, map[string]any{"op": "init", "storage": filepath.Join(dir, "optuna.sqlite3"), "study_id": s.ID, "seed": v.Request.Seed, "direction": direction, "manifest_hash": s.ManifestHash, "parameters": v.Parameters}, nil)
	if e != nil {
		fail(e)
		return
	}
	// 新しい候補の取得前に保存済み結果とOptunaを照合します。
	trials, e := m.Store.Trials(ctx, s.ID, 0, 10000)
	if e != nil {
		fail(e)
		return
	}
	for i := range trials {
		t := &trials[i]
		if t.RequestID == s.PendingRequest {
			s.PendingRequest = ""
		}
		if t.Status == "RUNNING" {
			m.recoverTrial(ctx, t, len(v.Prepared.Inputs))
		}
		m.cleanupSavedTuningRunCaches(s.ID, "", []domain.TuningTrial{*t})
		if !t.Delivered {
			if e = m.deliver(ctx, w, t); e != nil {
				fail(e)
				return
			}
		}
	}
	m.cleanupSavedTuningRunCaches(s.ID, s.BaselineRun, nil)
	if s.BaselineValue == nil {
		run, e := m.Store.GetRun(ctx, s.BaselineRun)
		if e == nil {
			results, readErr := m.Store.GetCaseResults(ctx, s.BaselineRun)
			value, evalErr := usecase.EvaluateTuningResults(results, len(v.Prepared.Inputs))
			if readErr != nil || evalErr != nil || run.Status != domain.RunSucceeded {
				retry, active := baselineRetryClassification(run.Status)
				if !retry {
					fail(fmt.Errorf("デフォルト値の保存結果が不完全です"))
					return
				}
				if active {
					if e = m.Store.FailActiveRun(ctx, s.BaselineRun); e != nil {
						fail(e)
						return
					}
				}
				s.BaselineRun, e = usecase.NewRunID()
				if e != nil {
					fail(e)
					return
				}
				if e = m.save(s); e != nil {
					fail(e)
					return
				}
				value, e = m.evaluate(ctx, *s, v, s.BaselineRun, nil)
				if e != nil {
					fail(fmt.Errorf("デフォルト値の評価に失敗: %w", e))
					return
				}
			}
			s.BaselineValue = &value
		} else if errors.Is(e, sql.ErrNoRows) {
			value, e := m.evaluate(ctx, *s, v, s.BaselineRun, nil)
			if e != nil {
				fail(fmt.Errorf("デフォルト値の評価に失敗: %w", e))
				return
			}
			s.BaselineValue = &value
		} else {
			fail(e)
			return
		}
		s.Elapsed = before + time.Since(start).Seconds()
		s.BestValue = s.BaselineValue
		s.BestRun = s.BaselineRun
		s.BestParams = map[string]float64{}
		if e = m.save(s); e != nil {
			fail(e)
			return
		}
	}
	updateBest(s, trials, direction)
	s.Status = "running"
	if e = m.save(s); e != nil {
		fail(e)
		return
	}
	consecutive := 0
	for i := len(trials) - 1; i >= 0 && trials[i].Status != "COMPLETE"; i-- {
		consecutive++
	}
	for len(trials) < s.Trials {
		if ctx.Err() != nil || a.pause.Load() {
			s.Status = "paused"
			return
		}
		s.Elapsed = before + time.Since(start).Seconds()
		if consecutive >= 5 {
			fail(fmt.Errorf("5回連続で候補が失敗しました。ログと探索範囲を確認してください"))
			return
		}
		if s.PendingRequest == "" {
			s.PendingRequest, e = usecase.NewRunID()
			if e != nil {
				fail(e)
				return
			}
			if e = m.save(s); e != nil {
				fail(e)
				return
			}
		}
		var t domain.TuningTrial
		if e = w.call(ctx, map[string]any{"op": "ask", "request_id": s.PendingRequest}, &t); e != nil {
			fail(e)
			return
		}
		t.StudyID = s.ID
		t.RequestID = s.PendingRequest
		t.RunID, e = usecase.NewRunID()
		if e != nil {
			fail(e)
			return
		}
		t.Status = "RUNNING"
		if e = params.ValidateValues(v.Parameters, t.Params); e != nil {
			fail(e)
			return
		}
		if e = m.Store.SaveTrial(ctx, t); e != nil {
			fail(e)
			return
		}
		s.PendingRequest = ""
		if e = m.save(s); e != nil {
			fail(e)
			return
		}
		source, _ := os.ReadFile(filepath.Join(dir, "fixed", "original.cpp"))
		_, t.Actual, e = params.Generate(source, v.SourceHash, v.Parameters, t.Params)
		if e != nil {
			fail(e)
			return
		}
		if e = m.Store.SaveTrial(ctx, t); e != nil {
			fail(e)
			return
		}
		value, evalErr := m.evaluate(ctx, *s, v, t.RunID, t.Params)
		t.Status = "COMPLETE"
		if evalErr == nil {
			t.Value = &value
			consecutive = 0
		} else {
			t.Status = "FAIL"
			t.Reason = evalErr.Error()
			consecutive++
			if ctx.Err() != nil {
				t.Status = "cancelled"
			}
		}
		t.ResultHash = resultHash(t)
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		e = m.Store.SaveTrial(persistCtx, t)
		cancel()
		if e != nil {
			fail(e)
			return
		}
		trials = append(trials, t)
		updateBest(s, trials, direction)
		s.Elapsed = before + time.Since(start).Seconds()
		if e = m.save(s); e != nil {
			fail(e)
			return
		}
		if ctx.Err() != nil {
			return
		}
		if e = m.deliver(ctx, w, &t); e != nil {
			fail(e)
			return
		}
		if evalErr != nil {
			var candidateErr *CandidateError
			if !errors.As(evalErr, &candidateErr) {
				fail(evalErr)
				return
			}
		}
	}
	s.Status = "completed"
}

func baselineRetryClassification(status domain.RunStatus) (retry, active bool) {
	switch status {
	case domain.RunQueued, domain.RunRunning:
		return true, true
	case domain.RunSucceeded, domain.RunPartial, domain.RunFailed, domain.RunCancelled:
		return true, false
	default:
		return false, false
	}
}

type CandidateError struct{ Err error }

func (e *CandidateError) Error() string { return e.Err.Error() }
func (e *CandidateError) Unwrap() error { return e.Err }
func (m *Manager) evaluate(ctx context.Context, s domain.TuningStudy, v Manifest, runID string, values map[string]float64) (float64, error) {
	defer func() {
		if e := m.cleanupTuningRunCache(s.ID, runID); e != nil {
			fmt.Fprintln(os.Stderr, "tuning cache cleanup:", e)
		}
	}()
	dir, _ := m.dir(s.ID)
	original, e := os.ReadFile(filepath.Join(dir, "fixed", "original.cpp"))
	if e != nil {
		return 0, e
	}
	source, _, e := params.Generate(original, v.SourceHash, v.Parameters, values)
	if e != nil {
		return 0, e
	}
	candidate := filepath.Join(dir, "candidates", runID+".cpp")
	if e = os.MkdirAll(filepath.Dir(candidate), 0700); e != nil {
		return 0, e
	}
	if e = atomicFile(candidate, source); e != nil {
		return 0, e
	}
	relative, e := filepath.Rel(m.Root, candidate)
	if e != nil {
		return 0, e
	}
	summary, e := usecase.ExecuteRunWithStore(ctx, usecase.RunRequest{RunID: runID, ConfigPath: m.ConfigPath, Solver: relative, Prepared: &v.Prepared, Threads: v.Request.Threads, TimeoutMilliseconds: v.Request.TimeoutMS, TuningStudy: s.ID, Comment: "Tuning " + s.ID}, m.Store)
	if e != nil {
		if summary.RunID != "" && summary.ExitCode >= 0 && (summary.Status == domain.RunFailed || summary.Status == domain.RunCancelled) {
			return 0, &CandidateError{e}
		}
		return 0, e
	}
	if summary.Status != domain.RunSucceeded {
		return 0, fmt.Errorf("Runが成功していません: %s", summary.Status)
	}
	results, e := m.Store.GetCaseResults(ctx, runID)
	if e != nil {
		return 0, e
	}
	value, e := usecase.EvaluateTuningResults(results, len(v.Prepared.Inputs))
	if e != nil {
		return 0, &CandidateError{e}
	}
	return value, nil
}

func (m *Manager) cleanupTuningRunCache(studyID, runID string) error {
	m.usageMu.Lock()
	defer m.usageMu.Unlock()
	if runID == "" || !filepath.IsLocal(runID) || strings.ContainsAny(runID, "/\\") || strings.HasPrefix(runID, ".") {
		return fmt.Errorf("Run IDが不正です")
	}
	runDir := filepath.Join(m.Root, "ahc-plaza", "runs", runID)
	before := directorySize(runDir)
	workspace := filepath.Join(runDir, "workspace")
	tools := filepath.Join(workspace, "tools")
	for _, path := range []string{filepath.Join(m.Root, "ahc-plaza"), filepath.Join(m.Root, "ahc-plaza", "runs"), runDir, workspace, tools} {
		info, e := os.Lstat(path)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("チューニングRunのキャッシュ用パスが通常のディレクトリではありません: %s", path)
		}
	}
	if e := removeChildrenExcept(workspace, map[string]bool{"tools": true, "pahcer": true}); e != nil {
		return e
	}
	if e := removeChildrenExcept(tools, map[string]bool{"in": true, "out": true, "err": true}); e != nil {
		return e
	}
	bytes := directorySize(runDir)
	m.mu.Lock()
	if m.runUsageCache == nil {
		m.runUsageCache = map[string]int64{}
	}
	m.runUsageCache[runID] = bytes
	if sample, ok := m.usageCache[studyID]; ok {
		sample.bytes += bytes - before
		m.usageCache[studyID] = sample
	}
	m.mu.Unlock()
	return nil
}

func removeChildrenExcept(dir string, keep map[string]bool) error {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if keep[entry.Name()] {
			info, e := entry.Info()
			if e != nil {
				return e
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("保持対象が通常のディレクトリではありません: %s", filepath.Join(dir, entry.Name()))
			}
			continue
		}
		if e := os.RemoveAll(filepath.Join(dir, entry.Name())); e != nil {
			return e
		}
	}
	return nil
}

func directorySize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, e error) error {
		if e == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func (m *Manager) cleanupSavedTuningRunCaches(studyID, baselineRun string, trials []domain.TuningTrial) {
	runIDs := make([]string, 0, len(trials)+1)
	if baselineRun != "" {
		runIDs = append(runIDs, baselineRun)
	}
	for _, trial := range trials {
		if trial.RunID != "" {
			runIDs = append(runIDs, trial.RunID)
		}
	}
	for _, runID := range runIDs {
		if e := m.cleanupTuningRunCache(studyID, runID); e != nil {
			fmt.Fprintln(os.Stderr, "saved tuning cache cleanup:", e)
		}
	}
}

func (m *Manager) recoverTrial(ctx context.Context, t *domain.TuningTrial, count int) {
	run, e := m.Store.GetRun(ctx, t.RunID)
	results, re := m.Store.GetCaseResults(ctx, t.RunID)
	value, ve := usecase.EvaluateTuningResults(results, count)
	if e == nil && re == nil && ve == nil && run.Status == domain.RunSucceeded {
		t.Status = "COMPLETE"
		t.Value = &value
	} else {
		t.Status = "FAIL"
		t.Value = nil
		t.Reason = "前回の評価が中断されました"
	}
	t.ResultHash = resultHash(*t)
}
func (m *Manager) deliver(ctx context.Context, w *worker, t *domain.TuningTrial) error {
	if t.ResultHash == "" {
		t.ResultHash = resultHash(*t)
	}
	if e := m.Store.SaveTrial(ctx, *t); e != nil {
		return e
	}
	if e := w.call(ctx, map[string]any{"op": "tell", "number": t.Number, "value": t.Value, "status": t.Status, "result_hash": t.ResultHash, "reason": t.Reason}, nil); e != nil {
		return e
	}
	t.Delivered = true
	return m.Store.SaveTrial(ctx, *t)
}
func resultHash(t domain.TuningTrial) string {
	b, _ := json.Marshal([]any{t.Number, t.Status, t.Value, t.Params, t.Actual})
	return params.Hash(b)
}
func updateBest(s *domain.TuningStudy, trials []domain.TuningTrial, direction string) {
	s.BestRun = s.BaselineRun
	s.BestValue = s.BaselineValue
	s.BestParams = map[string]float64{}
	s.Completed = 0
	s.Failed = 0
	for _, t := range trials {
		if t.Status != "COMPLETE" || t.Value == nil {
			s.Failed++
			continue
		}
		s.Completed++
		if s.BestValue == nil || (direction == "maximize" && *t.Value > *s.BestValue) || (direction == "minimize" && *t.Value < *s.BestValue) {
			v := *t.Value
			s.BestValue = &v
			s.BestRun = t.RunID
			s.BestParams = t.Params
		}
	}
}
func hashTree(root string) (map[string]string, error) {
	out := map[string]string{}
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("固定条件には通常ファイルのみ許可します: %s", path)
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, path)
		out[rel] = fmt.Sprintf("%04o:%s", info.Mode().Perm(), params.Hash(b))
		return nil
	})
	return out, e
}
func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func atomicFile(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".write-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
