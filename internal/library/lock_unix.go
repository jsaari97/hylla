//go:build darwin || linux

package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquireLock(root string) (*os.File, error) {
	lockFilePath := filepath.Join(root, "library.lock")
	lockFile, err := os.OpenFile(lockFilePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLibraryInUse
		}
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	return lockFile, nil
}
