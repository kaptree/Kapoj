package storage

import (
	"os"
	"path/filepath"
)

// IsPersistentMount reports whether dir sits under a filesystem mount that is
// not the filesystem root. In a container, the root filesystem is usually an
// ephemeral overlay; a bind mount, named volume, or Kubernetes PVC appears as
// a separate mount. Writing uploads to the root overlay means they disappear
// when the container is recreated, even though the database rows survive.
//
// This is a deployment invariant check, not a substitute for backups. It is
// intentionally conservative: a directory that is merely a normal subdirectory
// of the root filesystem (including the container writable layer) returns false.
func IsPersistentMount(dir string) (bool, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(abs); err != nil {
		return false, err
	}
	return hasMountBoundary(abs, storageDevice)
}

// hasMountBoundary walks from path toward the filesystem root and reports
// whether any ancestor is a mount point. Comparing device IDs is portable
// across Linux and macOS and does not require parsing /proc.
func hasMountBoundary(path string, device func(string) (uint64, error)) (bool, error) {
	current := filepath.Clean(path)
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}
		currentDevice, err := device(current)
		if err != nil {
			return false, err
		}
		parentDevice, err := device(parent)
		if err != nil {
			return false, err
		}
		if currentDevice != parentDevice {
			return true, nil
		}
		current = parent
	}
}
