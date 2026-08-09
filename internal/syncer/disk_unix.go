//go:build !windows

package syncer

import (
	"errors"
	"os"
	"syscall"
)

func ensureSpace(path string, bytes int64) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return nil
	}
	if int64(s.Bavail)*int64(s.Bsize) < bytes+1024*1024 {
		return errors.New("espaço insuficiente no cartão")
	}
	return nil
}
