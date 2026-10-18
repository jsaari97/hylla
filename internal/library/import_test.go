package library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/jsaari97/hylla/internal/books"
)

func newImportLibrary(t *testing.T) *Library {
	t.Helper()
	library, err := Create(context.Background(), filepath.Join(t.TempDir(), "library"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestLibrary(t, library) })
	return library
}

func sourceFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testEPUB(t *testing.T, chapter string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entries := []struct{ name, data string }{
		{"mimetype", "application/epub+zip"},
		{"META-INF/container.xml", `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="EPUB/book.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`},
		{"EPUB/book.opf", `<package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata/><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>`},
		{"EPUB/chapter.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>` + chapter + `</p></body></html>`},
	}
	for _, entry := range entries {
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func testPDF() []byte {
	return testPDFMetadata("")
}

func testPDFMetadata(info string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.7\n")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] >>",
	}
	infoReference := ""
	if info != "" {
		objects = append(objects, info)
		infoReference = fmt.Sprintf(" /Info %d 0 R", len(objects))
	}
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = buffer.Len()
		fmt.Fprintf(&buffer, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := buffer.Len()
	fmt.Fprintf(&buffer, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&buffer, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buffer, "trailer\n<< /Size %d /Root 1 0 R%s >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, infoReference, xref)
	return buffer.Bytes()
}

func assertNoImportFiles(t *testing.T, library *Library) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(library.Root, "tmp", "imports"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("incomplete import files remain: %v", entries)
	}
}

func TestImportAndReopen(t *testing.T) {
	for _, test := range []struct {
		name   string
		data   []byte
		format books.BookFormat
	}{
		{"A book.EPUB", testEPUB(t, "Text"), books.FormatEPUB},
		{"Tutkimus 日本語.pdf", testPDF(), books.FormatPDF},
	} {
		t.Run(test.name, func(t *testing.T) {
			library := newImportLibrary(t)
			source := sourceFile(t, test.name, test.data)
			imported, err := library.Import(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			book := imported.Book
			expectedTitle := strings.TrimSuffix(test.name, filepath.Ext(test.name))
			if imported.Duplicate || book.Title != expectedTitle || book.Format != test.format || book.ReadingStatus != books.StatusUnread || book.OriginalFilename != test.name {
				t.Fatalf("unexpected book: %+v", book)
			}
			hash := sha256.Sum256(test.data)
			if book.FileHash != hex.EncodeToString(hash[:]) {
				t.Fatal("incorrect content hash")
			}
			managed := filepath.Join(library.Root, "books", book.ID+"."+string(book.Format))
			copied, err := os.ReadFile(managed)
			if err != nil || !bytes.Equal(copied, test.data) {
				t.Fatalf("managed copy differs: %v", err)
			}
			original, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(original, test.data) {
				t.Fatalf("source changed: %v", err)
			}
			assertNoImportFiles(t, library)
			if err := os.Remove(source); err != nil {
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
			stored, err := reopened.Books(context.Background())
			if err != nil || len(stored) != 1 || stored[0].ID != book.ID || stored[0].FileHash != book.FileHash {
				t.Fatalf("book did not survive restart: %+v, %v", stored, err)
			}
		})
	}
}

func TestImportDuplicateAndSameFilename(t *testing.T) {
	library := newImportLibrary(t)
	source := sourceFile(t, "book.epub", testEPUB(t, "First"))
	first, err := library.Import(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := library.Import(context.Background(), source)
	if err != nil || !duplicate.Duplicate || duplicate.Book.ID != first.Book.ID {
		t.Fatalf("duplicate result: %+v, %v", duplicate, err)
	}
	secondSource := sourceFile(t, "book.epub", testEPUB(t, "Second"))
	second, err := library.Import(context.Background(), secondSource)
	if err != nil || second.Duplicate || second.Book.ID == first.Book.ID {
		t.Fatalf("same filename result: %+v, %v", second, err)
	}
	stored, err := library.Books(context.Background())
	if err != nil || len(stored) != 2 {
		t.Fatalf("stored books: %v, %v", stored, err)
	}
	entries, err := os.ReadDir(filepath.Join(library.Root, "books"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("managed files: %v, %v", entries, err)
	}
	assertNoImportFiles(t, library)
}

func TestImportRejectsInvalidFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"invalid.epub", []byte("not an EPUB")},
		{"invalid.pdf", []byte("not a PDF")},
		{"wrong.pdf", testEPUB(t, "Text")},
		{"wrong.epub", testPDF()},
		{"unsupported.txt", testPDF()},
		{"truncated.pdf", testPDF()[:50]},
	} {
		t.Run(test.name, func(t *testing.T) {
			library := newImportLibrary(t)
			if _, err := library.Import(context.Background(), sourceFile(t, test.name, test.data)); err == nil {
				t.Fatal("invalid file was imported")
			}
			assertNoImportFiles(t, library)
			entries, err := os.ReadDir(filepath.Join(library.Root, "books"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("unexpected managed files: %v, %v", entries, err)
			}
			stored, err := library.Books(context.Background())
			if err != nil || len(stored) != 0 {
				t.Fatalf("unexpected books: %v, %v", stored, err)
			}
		})
	}
}

func TestImportRollsBackOnDatabaseFailure(t *testing.T) {
	library := newImportLibrary(t)
	_, err := library.DB.Exec(`CREATE TRIGGER fail_import BEFORE INSERT ON books BEGIN SELECT RAISE(ABORT, 'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF())); err == nil {
		t.Fatal("expected database failure")
	}
	assertNoImportFiles(t, library)
	entries, err := os.ReadDir(filepath.Join(library.Root, "books"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed import left managed files: %v, %v", entries, err)
	}
}

func TestImportCancelled(t *testing.T) {
	library := newImportLibrary(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := library.Import(ctx, sourceFile(t, "book.pdf", testPDF())); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	assertNoImportFiles(t, library)
}

type fullDiskWriter struct{}

func (fullDiskWriter) Write(data []byte) (int, error) { return 0, syscall.ENOSPC }

type cancellingReader struct{ cancel context.CancelFunc }

func (reader cancellingReader) Read(data []byte) (int, error) {
	reader.cancel()
	return copy(data, "part of a book"), nil
}

func TestCopyBookStopsOnDiskFullOrCancellation(t *testing.T) {
	_, _, err := copyBook(context.Background(), fullDiskWriter{}, strings.NewReader("book"))
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("disk full error lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err = copyBook(ctx, io.Discard, cancellingReader{cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copy did not stop: %v", err)
	}
}

func TestImportRejectsUnavailableLibrary(t *testing.T) {
	library := newImportLibrary(t)
	if err := os.Rename(library.Root, library.Root+"-moved"); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF())); err == nil {
		t.Fatal("import into unavailable library succeeded")
	}
	if _, err := os.Stat(library.Root); !os.IsNotExist(err) {
		t.Fatalf("unavailable library was recreated: %v", err)
	}
}

func TestImportRejectsSymlinkDirectory(t *testing.T) {
	library := newImportLibrary(t)
	external := t.TempDir()
	booksPath := filepath.Join(library.Root, "books")
	if err := os.Remove(booksPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, booksPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF())); err == nil {
		t.Fatal("import followed a symlink")
	}
	entries, err := os.ReadDir(external)
	if err != nil || len(entries) != 0 {
		t.Fatalf("external directory changed: %v, %v", entries, err)
	}
}
