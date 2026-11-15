//go:build windows

package storage

import "errors"

func storageDevice(string) (uint64, error) {
	return 0, errors.New("storage: persistent-mount check is not supported on windows")
}
