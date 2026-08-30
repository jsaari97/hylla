package library

import (
	"errors"
	"fmt"
)

func (library *Library) Close() error {
	if library == nil {
		return nil
	}

	library.closeOnce.Do(func() {
		var closeErrors []error

		if library.DB != nil {
			if err := library.DB.Close(); err != nil {
				closeErrors = append(
					closeErrors,
					fmt.Errorf("close library database: %w", err),
				)
			}
		}

		if library.lockFile != nil {
			if err := library.lockFile.Close(); err != nil {
				closeErrors = append(
					closeErrors,
					fmt.Errorf("close library lock: %w", err),
				)
			}
		}

		library.closeErr = errors.Join(closeErrors...)
	})

	return library.closeErr
}
