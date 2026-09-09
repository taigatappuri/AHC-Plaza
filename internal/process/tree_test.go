package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCancelStopsSeparateDescendantProcessGroup(t *testing.T) {
	setsid, e := exec.LookPath("setsid")
	if e != nil {
		t.Skip("setsid unavailable")
	}
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "child.pid")
	childPath := filepath.Join(dir, "child.sh")
	parentPath := filepath.Join(dir, "parent.sh")
	os.WriteFile(childPath, []byte(`echo $$ > "$1"; sleep 30`), 0600)
	os.WriteFile(parentPath, []byte(`"$1" /bin/sh "$2" "$3" & wait`), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, e := Run(ctx, Request{Command: []string{"/bin/sh", parentPath, setsid, childPath, pidPath}, StdoutPath: filepath.Join(dir, "stdout"), StderrPath: filepath.Join(dir, "stderr")})
		done <- e
	}()
	deadline := time.Now().Add(3 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		if b, e := os.ReadFile(pidPath); e == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		cancel()
		<-done
		t.Fatal("child did not start")
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	for i := 0; i < 50; i++ {
		b, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
		if os.IsNotExist(e) {
			return
		}
		if e == nil && strings.Contains(string(b), ") Z ") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("descendant remained alive", pid)
}
