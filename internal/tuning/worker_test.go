package tuning

import (
	"context"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
	bundled "github.com/taigatappuri/AHC-Plaza/internal/tuning/runtime"
	"os"
	"path/filepath"
	"testing"
)

func TestBundledWorkerWithoutSystemPython(t *testing.T) {
	root := t.TempDir()
	if !bundled.Status(root).Available {
		t.Skip("run with -tags tuning_bundle to test embedded CPython")
	}
	t.Setenv("PATH", t.TempDir())
	dir := filepath.Join(root, "study")
	os.MkdirAll(dir, 0700)
	w, e := startWorker(context.Background(), root, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer w.close()
	scan, e := params.Parse([]byte("constexpr int X=5; // @tune 1 10\n"))
	if e != nil {
		t.Fatal(e)
	}
	req := map[string]any{"op": "init", "storage": filepath.Join(dir, "optuna.sqlite3"), "study_id": "test", "seed": 42, "direction": "maximize", "manifest_hash": "test-hash", "parameters": scan.Parameters}
	if e = w.call(context.Background(), req, nil); e != nil {
		b, _ := os.ReadFile(filepath.Join(dir, "worker.log"))
		t.Fatalf("%v: %s", e, b)
	}
	var trial struct {
		Number int                `json:"number"`
		Params map[string]float64 `json:"params"`
	}
	if e = w.call(context.Background(), map[string]any{"op": "ask", "request_id": "request-1"}, &trial); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = w.call(context.Background(), map[string]any{"op": "tell", "number": trial.Number, "value": 10, "status": "COMPLETE", "result_hash": "ten"}, nil); e != nil {
			t.Fatal(e)
		}
	}
	if e = w.call(context.Background(), map[string]any{"op": "tell", "number": trial.Number, "value": 11, "status": "COMPLETE", "result_hash": "eleven"}, nil); e == nil {
		t.Fatal("conflicting tell accepted")
	}
}
