package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	migrationFiles "github.com/jsaari97/hylla/migrations"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("migrate database: database is nil")
	}

	currentVersion, err := readSchemaVersion(ctx, db)
	if err != nil {
		return err
	}

	entries, err := migrationFiles.Files.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migration files: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".sql" {
			continue
		}

		version, err := migrationVersion(entry.Name())
		if err != nil {
			return err
		}

		if version <= currentVersion {
			continue
		}

		if version != currentVersion+1 {
			return fmt.Errorf(
				"migration version %d follows version %d",
				version,
				currentVersion,
			)
		}

		if err := applyMigration(ctx, db, entry.Name(), version); err != nil {
			return err
		}

		currentVersion = version
	}

	return nil
}

func readSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int

	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read database schema version: %w", err)
	}

	return version, nil
}

func migrationVersion(filename string) (int, error) {
	versionText, _, found := strings.Cut(filename, "_")
	if !found {
		return 0, fmt.Errorf("invalid migration filename %q", filename)
	}

	version, err := strconv.Atoi(versionText)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("invalid migration version in %q", filename)
	}

	return version, nil
}

func applyMigration(
	ctx context.Context,
	db *sql.DB,
	filename string,
	version int,
) error {
	contents, err := migrationFiles.Files.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read migration %q: %w", filename, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %q: %w", filename, err)
	}

	if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %q: %w", filename, err)
	}

	versionStatement := fmt.Sprintf("PRAGMA user_version = %d", version)
	if _, err := tx.ExecContext(ctx, versionStatement); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("set migration version %d: %w", version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %q: %w", filename, err)
	}

	return nil
}
