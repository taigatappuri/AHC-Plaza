package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// KillTree は別プロセスグループのcase-execの子孫も停止します（Linux）。
func KillTree(pid int) {
	parents := map[int][]int{}
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		id, e := strconv.Atoi(entry.Name())
		if e != nil {
			continue
		}
		b, e := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if e != nil {
			continue
		}
		end := strings.LastIndex(string(b), ")")
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(b)[end+1:])
		if len(fields) < 2 {
			continue
		}
		ppid, e := strconv.Atoi(fields[1])
		if e == nil {
			parents[ppid] = append(parents[ppid], id)
		}
	}
	var kill func(int)
	kill = func(id int) {
		for _, child := range parents[id] {
			kill(child)
		}
		_ = syscall.Kill(-id, syscall.SIGKILL)
		_ = syscall.Kill(id, syscall.SIGKILL)
	}
	kill(pid)
}
