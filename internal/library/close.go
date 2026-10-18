package library

import "errors"

func (library *Library) Close() error {
	if library == nil {
		return nil
	}
	library.mu.Lock()
	defer library.mu.Unlock()

	var closeErrors []error

	if library.DB != nil {
		closeErrors = append(closeErrors, library.DB.Close())
		library.DB = nil
	}

	if library.lockFile != nil {
		closeErrors = append(closeErrors, library.lockFile.Close())
		library.lockFile = nil
	}

	return errors.Join(closeErrors...)
}
