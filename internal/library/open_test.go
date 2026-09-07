package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenExistingLibrary(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "library")

	createdLibrary, err := Create(ctx, root)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	if err := createdLibrary.Close(); err != nil {
		t.Fatalf("close created library: %v", err)
	}

	openedLibrary, err := Open(ctx, root)
	if err != nil {
		t.Fatalf("Open() returned an error: %v", err)
	}
	t.Cleanup(func() {
		closeTestLibrary(t, openedLibrary)
	})

	if openedLibrary.Root != root {
		t.Errorf(
			"library root = %q, want %q",
			openedLibrary.Root,
			root,
		)
	}

	if err := openedLibrary.DB.PingContext(ctx); err != nil {
		t.Errorf("opened database is not available: %v", err)
	}

	assertLibraryIsLocked(t, root)
}

func TestOpenRejectsLibraryInUse(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")

	firstLibrary, err := Create(context.Background(), root)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}
	t.Cleanup(func() {
		closeTestLibrary(t, firstLibrary)
	})

	secondLibrary, err := Open(context.Background(), root)
	if secondLibrary != nil {
		closeTestLibrary(t, secondLibrary)
	}

	if !errors.Is(err, ErrLibraryInUse) {
		t.Errorf(
			"Open() error = %v, want ErrLibraryInUse",
			err,
		)
	}
}

func TestOpenRejectsMissingDatabase(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")

	createdLibrary, err := Create(context.Background(), root)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	if err := createdLibrary.Close(); err != nil {
		t.Fatalf("close created library: %v", err)
	}

	databasePath := filepath.Join(root, "library.sqlite")
	if err := os.Remove(databasePath); err != nil {
		t.Fatalf("remove test database: %v", err)
	}

	openedLibrary, err := Open(context.Background(), root)
	if openedLibrary != nil {
		closeTestLibrary(t, openedLibrary)
	}

	if err == nil {
		t.Fatal("Open() returned no error for a missing database")
	}
}
