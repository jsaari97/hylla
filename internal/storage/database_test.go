package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsaari97/hylla/internal/storage"
)

func TestOpenCreatesDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "library.sqlite")

	db, err := storage.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("Open() returned an error: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatalf("stat database file: %v", err)
	}

	if !info.Mode().IsRegular() {
		t.Errorf("database path is not a regular file")
	}
}

func TestOpenConfiguresDatabase(t *testing.T) {
	db, err := storage.Open(
		context.Background(),
		filepath.Join(t.TempDir(), "library.sqlite"),
	)
	if err != nil {
		t.Fatalf("Open() returned an error: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	}()

	var foreignKeys int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys)
	if err != nil {
		t.Fatalf("read foreign_keys setting: %v", err)
	}

	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("read journal_mode setting: %v", err)
	}

	if !strings.EqualFold(journalMode, "delete") {
		t.Errorf("journal_mode = %q, want %q", journalMode, "delete")
	}

	var busyTimeout int
	err = db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
	if err != nil {
		t.Fatalf("read busy_timeout setting: %v", err)
	}

	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	db, err := storage.Open(context.Background(), "")
	if err == nil {
		if db != nil {
			_ = db.Close()
		}
		t.Fatal("Open() returned no error for an empty path")
	}

	if db != nil {
		_ = db.Close()
		t.Error("Open() returned a database for an empty path")
	}
}
