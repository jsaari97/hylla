package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRecoversInterruptedImports(t *testing.T) {
	library := newImportLibrary(t)
	imported, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF()))
	if err != nil {
		t.Fatal(err)
	}
	// A crash after the managed-file rename but before commit leaves no row.
	if _, err := library.DB.Exec("DELETE FROM books WHERE id = ?", imported.Book.ID); err != nil {
		t.Fatal(err)
	}
	cover := filepath.Join(library.Root, "covers", imported.Book.ID+".jpg")
	if err := os.WriteFile(cover, []byte("cover"), 0o600); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(library.Root, "tmp", "imports", "incomplete")
	if err := os.WriteFile(staged, []byte("partial copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), library.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertNoImportFiles(t, reopened)
	if len(reopened.Warnings) != 1 {
		t.Fatalf("recovery warning missing: %v", reopened.Warnings)
	}
	matches, err := filepath.Glob(filepath.Join(library.Root, "tmp", "recovery", "*", imported.Book.ID+".pdf"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("orphan was not recovered: %v, %v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil || string(data) != string(testPDF()) {
		t.Fatalf("recovered book changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(matches[0]), imported.Book.ID+".jpg")); err != nil {
		t.Fatalf("cover not recovered: %v", err)
	}
	if _, err := os.Stat(filepath.Join(library.Root, "books", imported.Book.ID+".pdf")); !os.IsNotExist(err) {
		t.Fatal("orphan remains in books")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(context.Background(), library.Root)
	if err != nil {
		t.Fatalf("recovery is not repeatable: %v", err)
	}
	defer again.Close()
}

func TestOpenReportsMissingBook(t *testing.T) {
	library := newImportLibrary(t)
	imported, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF()))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(library.Root, "books", imported.Book.ID+".pdf")); err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), library.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.Warnings) != 1 || !strings.Contains(reopened.Warnings[0], imported.Book.ID) {
		t.Fatalf("missing file not reported: %v", reopened.Warnings)
	}
	stored, err := reopened.Books(context.Background())
	if err != nil || len(stored) != 1 {
		t.Fatalf("missing book record was lost: %v, %v", stored, err)
	}
}

func TestRecoveryRejectsSymlink(t *testing.T) {
	library := newImportLibrary(t)
	outside := sourceFile(t, "keep.pdf", []byte("keep this file"))
	link := filepath.Join(library.Root, "tmp", "imports", "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), library.Root)
	if reopened != nil {
		defer reopened.Close()
	}
	if err == nil {
		t.Fatal("recovery accepted symlink")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "keep this file" {
		t.Fatalf("external file changed: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("link was removed: %v", err)
	}
}

func TestImportRejectsReadOnlyStagingDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories")
	}
	library := newImportLibrary(t)
	staging := filepath.Join(library.Root, "tmp", "imports")
	if err := os.Chmod(staging, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(staging, 0o700)
	if _, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF())); err == nil {
		t.Fatal("import into read-only directory succeeded")
	}
	assertNoImportFiles(t, library)
}
