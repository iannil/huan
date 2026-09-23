//go:build linux || darwin

package build

import (
	"os"
	"path/filepath"
	"syscall"
)

func acquireMarkdownLease(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	// The OS releases flock on close or process exit. Never unlink .lock:
	// replacing its inode would allow multiple simultaneous writer leases.
	return f, nil
}
