package library

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jsaari97/hylla/internal/storage"
)

func Open(ctx context.Context, root string) (*Library, error) {
	absoluteRoot, err := absoluteRootPath(root)
	if err != nil {
		return nil, err
	}

	if _, err := readManifest(absoluteRoot); err != nil {
		return nil, err
	}

	lockFile, err := acquireLock(absoluteRoot)
	if err != nil {
		return nil, err
	}

	databasePath := filepath.Join(absoluteRoot, "library.sqlite")
	checkedRoot, err := (&Library{Root: absoluteRoot}).openManagedRoot()
	if err != nil {
		_ = lockFile.Close()
		return nil, err
	}
	_ = checkedRoot.Close()

	db, err := storage.Open(ctx, databasePath)
	if err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("open library database: %w", err)
	}

	result := &Library{
		Root:     absoluteRoot,
		DB:       db,
		lockFile: lockFile,
	}

	if err := storage.Migrate(ctx, db); err != nil {
		_ = result.Close()
		return nil, fmt.Errorf("migrate library database: %w", err)
	}
	if err := result.recoverImports(ctx); err != nil {
		_ = result.Close()
		return nil, fmt.Errorf("recover library imports: %w", err)
	}

	return result, nil
}
