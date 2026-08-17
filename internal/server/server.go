package server

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/inputfeature"
	"github.com/taigatappuri/AHC-Plaza/internal/store"
)

//go:embed static
var staticFiles embed.FS

// Server はHTTPアダプターとRunのキャンセル管理を保持します。
// 実際の処理はハンドラとusecaseへ委譲し、ここではライフサイクルだけを管理します。
type Server struct {
	Root          string
	ConfigPath    string
	Store         *store.SQLiteStore
	featureRunner *inputfeature.Runner

	mu           sync.Mutex
	visualizerMu sync.Mutex
	runWG        sync.WaitGroup
	closeOnce    sync.Once
	closeErr     error
	closing      bool
	cancels      map[string]context.CancelFunc
	failures     map[string]string
}

func New(root, configPath string) (*Server, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("could not resolve the project root: %w", err)
	}
	root = absoluteRoot
	if configPath == "" {
		configPath = filepath.Join(root, "ahc-plaza.toml")
	} else if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(root, configPath)
	}
	database, err := store.OpenSQLite(filepath.Join(root, "ahc-plaza", "ahc-plaza.db"))
	if err != nil {
		return nil, err
	}
	if _, err := database.MarkUnfinishedRunsFailed(context.Background(), time.Now().UTC()); err != nil {
		database.Close()
		return nil, err
	}
	featureRunner, err := inputfeature.NewRunner(root)
	if err != nil {
		database.Close()
		return nil, err
	}
	return &Server{Root: root, ConfigPath: configPath, Store: database, featureRunner: featureRunner, cancels: make(map[string]context.CancelFunc), failures: make(map[string]string)}, nil
}

func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.BeginShutdown()
		s.runWG.Wait()
		s.mu.Lock()
		s.cancels = make(map[string]context.CancelFunc)
		s.failures = make(map[string]string)
		s.mu.Unlock()
		s.closeErr = s.Store.Close()
	})
	return s.closeErr
}

// BeginShutdown は新しいRunの受付を止め、実行中のRunへ停止を要求します。
// DBはCloseでRunの後処理完了を待ってから閉じます。
func (s *Server) BeginShutdown() {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	s.closing = true
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for _, cancel := range s.cancels {
		cancels = append(cancels, cancel)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Server) registerRun(runID string, cancel context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return false
	}
	s.cancels[runID] = cancel
	s.runWG.Add(1)
	return true
}

func (s *Server) finishRun(runID string) {
	s.mu.Lock()
	delete(s.cancels, runID)
	s.mu.Unlock()
	s.runWG.Done()
}

func (s *Server) recordRunFailure(runID string, err error) {
	s.mu.Lock()
	s.failures[runID] = err.Error()
	s.mu.Unlock()
}

func (s *Server) runFailure(runID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures[runID]
}

// Handler はAPIと埋め込み済みフロントエンドを同じHTTPサーバーへ登録します。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, method := range []string{"GET", "POST", "PUT"} {
		mux.HandleFunc(method+" /api/runs", s.handleRuns)
		mux.HandleFunc(method+" /api/runs/", s.handleRun)
	}
	mux.HandleFunc("GET /api/solvers", s.handleSolvers)
	mux.HandleFunc("GET /api/input-directories", s.handleInputDirectories)
	mux.HandleFunc("GET /api/input-generators", s.handleInputGenerators)
	mux.HandleFunc("POST /api/input-generate-tool", s.handleToolInputGenerate)
	mux.HandleFunc("GET /api/input-features", s.handleInputFeatures)
	mux.HandleFunc("POST /api/compare", s.handleCompare)
	for _, method := range []string{"GET", "PUT"} {
		mux.HandleFunc(method+" /api/config", s.handleConfig)
	}
	for _, method := range []string{"GET", "DELETE"} {
		mux.HandleFunc(method+" /api/visualizer", s.handleVisualizer)
	}
	mux.HandleFunc("POST /api/visualizer/download", s.handleVisualizerDownload)
	mux.HandleFunc("GET /visualizer/", s.handleVisualizerAsset)
	mux.HandleFunc("GET /", s.handleStatic)
	return securityHeaders(mux)
}
