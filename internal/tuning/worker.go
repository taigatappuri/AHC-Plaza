package tuning

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/process"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
	bundled "github.com/taigatappuri/AHC-Plaza/internal/tuning/runtime"
)

//go:embed worker.py
var workerSource []byte

const WorkerVersion = 1

type worker struct {
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Scanner
	log  *os.File
	id   int
	once sync.Once
}

func startWorker(ctx context.Context, root, dir string) (*worker, error) {
	info, e := bundled.Setup(ctx, root)
	if e != nil {
		return nil, e
	}
	path := filepath.Join(dir, "worker.py")
	if e = os.WriteFile(path, workerSource, 0600); e != nil {
		return nil, e
	}
	log, e := os.OpenFile(filepath.Join(dir, "worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	cmd := exec.CommandContext(ctx, filepath.Join(info.Path, "bin", "python3.12"), "-I", "-B", "-u", path)
	cmd.Dir = dir
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			process.KillTree(cmd.Process.Pid)
		}
		return nil
	}
	in, e := cmd.StdinPipe()
	if e != nil {
		log.Close()
		return nil, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		log.Close()
		return nil, e
	}
	if e = cmd.Start(); e != nil {
		log.Close()
		return nil, e
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 1<<20)
	return &worker{cmd: cmd, in: in, out: scan, log: log}, nil
}
func (w *worker) close() {
	w.once.Do(func() { w.in.Close(); process.KillTree(w.cmd.Process.Pid); w.cmd.Wait(); w.log.Close() })
}
func (w *worker) call(ctx context.Context, request map[string]any, result any) error {
	w.id++
	request["id"] = w.id
	request["version"] = WorkerVersion
	type response struct {
		Version int             `json:"version"`
		ID      int             `json:"id"`
		Error   string          `json:"error"`
		Result  json.RawMessage `json:"result"`
	}
	done := make(chan error, 1)
	go func() {
		b, e := json.Marshal(request)
		if e != nil {
			done <- e
			return
		}
		if len(b) > 1<<20 {
			done <- fmt.Errorf("worker request too large")
			return
		}
		if _, e = w.in.Write(append(b, '\n')); e != nil {
			done <- e
			return
		}
		if !w.out.Scan() {
			e = w.out.Err()
			if e == nil {
				e = io.ErrUnexpectedEOF
			}
			done <- e
			return
		}
		var r response
		if e = json.Unmarshal(w.out.Bytes(), &r); e != nil {
			done <- e
			return
		}
		if r.ID != w.id || r.Version != WorkerVersion {
			done <- fmt.Errorf("worker protocol mismatch")
			return
		}
		if r.Error != "" {
			done <- fmt.Errorf("Optuna: %s", r.Error)
			return
		}
		if result != nil {
			e = json.Unmarshal(r.Result, result)
		}
		done <- e
	}()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case e := <-done:
		return e
	case <-ctx.Done():
		w.close()
		<-done
		return ctx.Err()
	case <-timer.C:
		w.close()
		<-done
		return fmt.Errorf("Optuna workerの応答がタイムアウトしました")
	}
}
func workerHash() string { return params.Hash(workerSource) }
