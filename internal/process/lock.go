package process

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockProject はCLIとGUIの同時所有を防ぎます。ロックファイルは削除しません。
func LockProject(root string) (func(), error) {
	dir := filepath.Join(root, "ahc-plaza")
	if e := os.MkdirAll(dir, 0755); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "execution.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("このプロジェクトは別のAHC Plazaで使用中です: %w", e)
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
