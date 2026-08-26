package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jsaari97/hylla/internal/storage"
)

func TestMigrate(t *testing.T) {
	ctx := context.Background()
	db := openTestDatabase(t)

	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() returned an error: %v", err)
	}

	var version int
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}

	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestMigrateCreatesSchema(t *testing.T) {
	ctx := context.Background()
	db := openTestDatabase(t)

	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() returned an error: %v", err)
	}

	objects := []struct {
		objectType string
		name       string
	}{
		{"table", "books"},
		{"table", "book_authors"},
		{"index", "books_title_idx"},
		{"index", "books_status_idx"},
		{"index", "book_authors_name_idx"},
	}

	for _, object := range objects {
		var count int

		err := db.QueryRowContext(
			ctx,
			`SELECT COUNT(*)
			 FROM sqlite_master
			 WHERE type = ? AND name = ?`,
			object.objectType,
			object.name,
		).Scan(&count)
		if err != nil {
			t.Fatalf("find %s %q: %v", object.objectType, object.name, err)
		}

		if count != 1 {
			t.Errorf(
				"%s %q count = %d, want 1",
				object.objectType,
				object.name,
				count,
			)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDatabase(t)

	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("first Migrate() call returned an error: %v", err)
	}

	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("second Migrate() call returned an error: %v", err)
	}

	var version int
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}

	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestMigrateRejectsNilDatabase(t *testing.T) {
	err := storage.Migrate(context.Background(), nil)
	if err == nil {
		t.Fatal("Migrate() returned no error for a nil database")
	}
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "library.sqlite")

	db, err := storage.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	return db
}
