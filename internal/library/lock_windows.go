//go:build windows

package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func acquireLock(root string) (*os.File, error) {
	lockPath := filepath.Join(root, "library.lock")

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)

	err = windows.LockFileEx(windows.Handle(lockFile.Fd()), flags, 0, 1, 0, &windows.Overlapped{})

	if err != nil {
		lockFile.Close()

		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrLibraryInUse
		}

		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	return lockFile, nil
}
