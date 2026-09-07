package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"uuid"
)

func TestCreateCreatesLibrary(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "library")

	createdLibrary, err := Create(ctx, root)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}
	t.Cleanup(func() { closeTestLibrary(t, createdLibrary) })

	if createdLibrary.Root != root {
		t.Errorf("library root = %q, want %q", createdLibrary.Root, root)
	}

	expectedDirectories := []string{
		"books",
		"covers",
		"tmp",
		filepath.Join("tmp", "imports"),
		filepath.Join("tmp", "deletions"),
		filepath.Join("tmp", "recovery"),
		"backups",
	}

	for _, directory := range expectedDirectories {
		path := filepath.Join(root, directory)

		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("stat directory %q: %v", path, err)
			continue
		}

		if !info.IsDir() {
			t.Errorf("%q is not a directory", path)
		}
	}

	expectedFiles := []string{
		"library.json",
		"library.lock",
		"library.sqlite",
	}

	for _, filename := range expectedFiles {
		path := filepath.Join(root, filename)

		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("stat file %q: %v", path, err)
			continue
		}

		if !info.Mode().IsRegular() {
			t.Errorf("%q is not a regular file", path)
		}
	}

	assertManifest(t, root)
	assertSchemaVersion(t, ctx, createdLibrary, 1)
	assertNoTemporaryManifest(t, root)
	assertLibraryIsLocked(t, root)
}

func TestCreateUsesExistingEmptyDirectory(t *testing.T) {
	root := t.TempDir()

	createdLibrary, err := Create(context.Background(), root)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}
	t.Cleanup(func() { closeTestLibrary(t, createdLibrary) })

	if createdLibrary.Root != root {
		t.Errorf("library root = %q, want %q", createdLibrary.Root, root)
	}

	if _, err := os.Stat(filepath.Join(root, "library.json")); err != nil {
		t.Errorf("stat library manifest: %v", err)
	}
}

func TestCreateRejectsEmptyPath(t *testing.T) {
	createdLibrary, err := Create(context.Background(), "")
	if err == nil {
		if createdLibrary != nil {
			closeTestLibrary(t, createdLibrary)
		}
		t.Fatal("Create() returned no error for an empty path")
	}

	if createdLibrary != nil {
		closeTestLibrary(t, createdLibrary)
		t.Error("Create() returned a library for an empty path")
	}
}

func TestCreateRejectsNonemptyDirectory(t *testing.T) {
	root := t.TempDir()
	markerPath := filepath.Join(root, "keep.txt")
	markerContents := []byte("keep this file")

	if err := os.WriteFile(markerPath, markerContents, 0o600); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	createdLibrary, err := Create(context.Background(), root)
	if err == nil {
		if createdLibrary != nil {
			closeTestLibrary(t, createdLibrary)
		}
		t.Fatal("Create() returned no error for a nonempty directory")
	}

	if createdLibrary != nil {
		closeTestLibrary(t, createdLibrary)
		t.Error("Create() returned a library for a nonempty directory")
	}

	gotContents, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker file: %v", err)
	}

	if string(gotContents) != string(markerContents) {
		t.Errorf("marker contents = %q, want %q", gotContents, markerContents)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read rejected directory: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != "keep.txt" {
		t.Errorf("directory entries after rejection = %v, want only keep.txt", entryNames(entries))
	}
}

func TestCreateRejectsFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-directory")

	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write root file: %v", err)
	}

	createdLibrary, err := Create(context.Background(), root)
	if err == nil {
		if createdLibrary != nil {
			closeTestLibrary(t, createdLibrary)
		}
		t.Fatal("Create() returned no error for a file path")
	}

	if createdLibrary != nil {
		closeTestLibrary(t, createdLibrary)
		t.Error("Create() returned a library for a file path")
	}
}

func TestCreateCleansNewRootAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := filepath.Join(t.TempDir(), "library")
	createdLibrary, err := Create(ctx, root)
	if err == nil {
		if createdLibrary != nil {
			closeTestLibrary(t, createdLibrary)
		}
		t.Fatal("Create() returned no error for a canceled context")
	}

	if createdLibrary != nil {
		closeTestLibrary(t, createdLibrary)
		t.Error("Create() returned a library after cancellation")
	}

	_, statErr := os.Lstat(root)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("new library root remains after failure; stat error = %v", statErr)
	}
}

func TestCreateCleansExistingRootAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := t.TempDir()
	createdLibrary, err := Create(ctx, root)
	if err == nil {
		if createdLibrary != nil {
			closeTestLibrary(t, createdLibrary)
		}
		t.Fatal("Create() returned no error for a canceled context")
	}

	if createdLibrary != nil {
		closeTestLibrary(t, createdLibrary)
		t.Error("Create() returned a library after cancellation")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read existing root after failure: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("existing root entries after failure = %v, want none", entryNames(entries))
	}
}

func TestCleanupFailedCreateContinuesAfterError(t *testing.T) {
	root := t.TempDir()
	booksPath := filepath.Join(root, "books")

	if err := os.Mkdir(booksPath, 0o755); err != nil {
		t.Fatalf("create books directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(booksPath, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	lockFile, err := acquireLock(root)
	if err != nil {
		t.Fatalf("acquire library lock: %v", err)
	}

	err = cleanupFailedCreate(root, false, nil, lockFile)
	if err == nil {
		t.Fatal("cleanupFailedCreate() returned no error for a nonempty directory")
	}

	_, statErr := os.Stat(filepath.Join(root, "library.lock"))
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("library lock remains after cleanup; stat error = %v", statErr)
	}
}

func TestLibraryCloseReleasesLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")

	createdLibrary, err := Create(context.Background(), root)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	t.Cleanup(func() {
		if err := createdLibrary.Close(); err != nil {
			t.Errorf("close library during cleanup: %v", err)
		}
	})

	if err := createdLibrary.Close(); err != nil {
		t.Fatalf("first Close() returned an error: %v", err)
	}

	lockfile, err := acquireLock(root)
	if err != nil {
		t.Fatalf("acquireLock() returned an error after library Close(): %v", err)
	}

	if createdLibrary.DB != nil {
		t.Fatal("library database was not cleared after Close()")
	}

	t.Cleanup(func() {
		if err := lockfile.Close(); err != nil {
			t.Errorf("close lockfile: %v", err)
		}
	})
}

func TestLibraryCloseIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")

	createdLibrary, err := Create(context.Background(), root)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	if err := createdLibrary.Close(); err != nil {
		t.Fatalf("first Close() returned an error: %v", err)
	}

	if err := createdLibrary.Close(); err != nil {
		t.Fatalf("second Close() returned an error: %v", err)
	}
}

func assertManifest(t *testing.T, root string) {
	t.Helper()

	manifestData, err := os.ReadFile(filepath.Join(root, "library.json"))
	if err != nil {
		t.Fatalf("read library manifest: %v", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode library manifest: %v", err)
	}

	if manifest.FormatVersion != CurrentFormatVersion {
		t.Errorf(
			"format version = %d, want %d",
			manifest.FormatVersion,
			CurrentFormatVersion,
		)
	}

	if _, err := uuid.Parse(manifest.LibraryID); err != nil {
		t.Errorf("library ID %q is not a valid UUID: %v", manifest.LibraryID, err)
	}
}

func assertSchemaVersion(
	t *testing.T,
	ctx context.Context,
	createdLibrary *Library,
	want int,
) {
	t.Helper()

	var got int
	if err := createdLibrary.DB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&got); err != nil {
		t.Fatalf("read schema version: %v", err)
	}

	if got != want {
		t.Errorf("schema version = %d, want %d", got, want)
	}
}

func assertNoTemporaryManifest(t *testing.T, root string) {
	t.Helper()

	temporaryFiles, err := filepath.Glob(filepath.Join(root, ".library.json.*"))
	if err != nil {
		t.Fatalf("find temporary manifest files: %v", err)
	}

	if len(temporaryFiles) != 0 {
		t.Errorf("temporary manifest files remain: %v", temporaryFiles)
	}
}

func assertLibraryIsLocked(t *testing.T, root string) {
	t.Helper()

	secondLock, err := acquireLock(root)
	if err == nil {
		_ = secondLock.Close()
		t.Fatal("acquireLock() acquired a second lock")
	}

	if !errors.Is(err, ErrLibraryInUse) {
		t.Errorf("acquireLock() error = %v, want ErrLibraryInUse", err)
	}
}

func closeTestLibrary(t *testing.T, createdLibrary *Library) {
	t.Helper()

	if err := createdLibrary.Close(); err != nil {
		t.Errorf("close library: %v", err)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
