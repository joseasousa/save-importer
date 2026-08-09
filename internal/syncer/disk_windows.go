//go:build windows

package syncer

import "os"

func ensureSpace(path string, _ int64) error { return os.MkdirAll(path, 0755) }
