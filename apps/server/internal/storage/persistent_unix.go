//go:build !windows

package storage

import (
	"fmt"
	"os"
	"syscall"
)

func storageDevice(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("storage: unsupported stat type for %q", path)
	}
	return uint64(stat.Dev), nil
}
