package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"uuid"

	"github.com/jsaari97/hylla/internal/storage"
)

func Create(ctx context.Context, root string) (_ *Library, err error) {
	absoluteRoot, err := absoluteRootPath(root)
	if err != nil {
		return nil, err
	}

	rootCreated, err := prepareRoot(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare library root: %w", err)
	}

	lockFile, err := acquireLock(absoluteRoot)
	if err != nil {
		return nil, err
	}

	var db *sql.DB
	complete := false

	defer func() {
		if complete {
			return
		}

		cleanupErr := cleanupFailedCreate(
			absoluteRoot,
			rootCreated,
			db,
			lockFile,
		)
		if cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	if err := createDirectories(absoluteRoot); err != nil {
		return nil, fmt.Errorf("failed to create library directories: %w", err)
	}

	databasePath := filepath.Join(absoluteRoot, "library.sqlite")

	db, err = storage.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := storage.Migrate(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	manifest := Manifest{
		FormatVersion: CurrentFormatVersion,
		LibraryID:     uuid.New().String(),
	}

	if err := writeManifest(absoluteRoot, manifest); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	result := &Library{
		Root:     absoluteRoot,
		DB:       db,
		lockFile: lockFile,
	}

	complete = true

	return result, nil
}

func prepareRoot(root string) (bool, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(root, 0o755); err != nil {
				return false, fmt.Errorf("failed to create library root directory: %w", err)
			}
			return true, nil
		}
		return false, err
	}

	if !info.IsDir() {
		return false, fmt.Errorf("library root path must be a directory")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return false, err
	}
	if len(entries) > 0 {
		return false, fmt.Errorf("library root path must be an empty directory")
	}

	return false, nil
}

func createDirectories(root string) error {
	directories := []string{
		"books",
		"covers",
		"tmp",
		filepath.Join("tmp", "imports"),
		filepath.Join("tmp", "deletions"),
		filepath.Join("tmp", "recovery"),
		"backups",
	}

	for _, dir := range directories {
		path := filepath.Join(root, dir)
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
	}

	return nil
}

func cleanupFailedCreate(
	root string,
	rootCreated bool,
	db *sql.DB,
	lockFile *os.File,
) error {
	var cleanupErrors []error

	if db != nil {
		if err := db.Close(); err != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("close library database: %w", err),
			)
		}
	}

	if err := removeIfExists(filepath.Join(root, "library.json")); err != nil {
		cleanupErrors = append(
			cleanupErrors,
			fmt.Errorf("remove library manifest: %w", err),
		)
	}

	sqliteFiles := []string{
		filepath.Join(root, "library.sqlite"),
		filepath.Join(root, "library.sqlite-journal"),
		filepath.Join(root, "library.sqlite-wal"),
		filepath.Join(root, "library.sqlite-shm"),
	}

	for _, path := range sqliteFiles {
		if err := removeIfExists(path); err != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("remove SQLite file %q: %w", path, err),
			)
		}
	}

	paths := []string{
		filepath.Join(root, "tmp", "recovery"),
		filepath.Join(root, "tmp", "deletions"),
		filepath.Join(root, "tmp", "imports"),
		filepath.Join(root, "tmp"),
		filepath.Join(root, "backups"),
		filepath.Join(root, "covers"),
		filepath.Join(root, "books"),
	}

	for _, path := range paths {
		if err := removeIfExists(path); err != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("remove library directory %q: %w", path, err),
			)
		}
	}

	if lockFile != nil {
		if err := lockFile.Close(); err != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("close library lock: %w", err),
			)
		}

		if err := removeIfExists(filepath.Join(root, "library.lock")); err != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("remove library lock: %w", err),
			)
		}
	}

	if rootCreated {
		if err := removeIfExists(root); err != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("remove library root: %w", err),
			)
		}
	}

	return errors.Join(cleanupErrors...)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
