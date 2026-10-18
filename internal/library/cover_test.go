package library

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsaari97/hylla/internal/metadata/pdf"
)

func epubWithCover(t *testing.T, imageData []byte) []byte {
	t.Helper()
	original := testEPUB(t, "Text")
	source, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, entry := range source.File {
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(input)
		input.Close()
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name == "EPUB/book.opf" {
			data = []byte(strings.Replace(string(data), "</manifest>", `<item id="cover" href="cover.png" properties="cover-image"/></manifest>`, 1))
		}
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: entry.Name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	writer, err := archive.Create("EPUB/cover.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(imageData); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func testCover(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 40, 60))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestImportStoresCoverAndFillsMissingCover(t *testing.T) {
	library := newImportLibrary(t)
	imported, err := library.Import(context.Background(), sourceFile(t, "book.epub", epubWithCover(t, testCover(t))))
	if err != nil {
		t.Fatal(err)
	}
	coverPath := filepath.Join(library.Root, "covers", imported.Book.ID+".jpg")
	data, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatalf("cover not stored at import: %v", err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(coverPath); err != nil {
		t.Fatal(err)
	}
	data, err = library.Cover(context.Background(), imported.Book.ID)
	if err != nil || len(data) == 0 {
		t.Fatalf("existing book cover missing: %v", err)
	}
	if _, err := os.Stat(coverPath); err != nil {
		t.Fatalf("cover not cached: %v", err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), library.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cached, err := reopened.Cover(context.Background(), imported.Book.ID)
	if err != nil || !bytes.Equal(data, cached) {
		t.Fatalf("cover did not survive restart: %v", err)
	}
	assertNoImportFiles(t, reopened)
}

func TestBrokenCoverDoesNotBlockImport(t *testing.T) {
	library := newImportLibrary(t)
	imported, err := library.Import(context.Background(), sourceFile(t, "book.epub", epubWithCover(t, []byte("broken image"))))
	if err != nil {
		t.Fatalf("cover error stopped import: %v", err)
	}
	data, err := library.Cover(context.Background(), imported.Book.ID)
	if err != nil || len(data) != 0 {
		t.Fatalf("fallback failed: %v", err)
	}
	stored, err := library.Books(context.Background())
	if err != nil || len(stored) != 1 {
		t.Fatalf("book was lost: %v", err)
	}
}

func TestFailedImportRemovesCover(t *testing.T) {
	library := newImportLibrary(t)
	if _, err := library.DB.Exec(`CREATE TRIGGER fail_cover_import BEFORE INSERT ON books BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Import(context.Background(), sourceFile(t, "book.epub", epubWithCover(t, testCover(t)))); err == nil {
		t.Fatal("expected database failure")
	}
	entries, err := os.ReadDir(filepath.Join(library.Root, "covers"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("cover remains after failed import: %v, %v", entries, err)
	}
	assertNoImportFiles(t, library)
}

func TestCoverRejectsSymlink(t *testing.T) {
	library := newImportLibrary(t)
	imported, err := library.Import(context.Background(), sourceFile(t, "book.epub", epubWithCover(t, testCover(t))))
	if err != nil {
		t.Fatal(err)
	}
	target := sourceFile(t, "external.jpg", []byte("keep this file"))
	coverPath := filepath.Join(library.Root, "covers", imported.Book.ID+".jpg")
	if err := os.Remove(coverPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, coverPath); err != nil {
		t.Skip(err)
	}
	if _, err := library.Cover(context.Background(), imported.Book.ID); err == nil {
		t.Fatal("cover followed a symbolic link")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep this file" {
		t.Fatalf("external file changed: %v", err)
	}
}

func TestPDFCoverStoredAndFilled(t *testing.T) {
	if !pdf.CoverSupported {
		t.Skip("native PDF rendering is not available")
	}
	library := newImportLibrary(t)
	imported, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF()))
	if err != nil {
		t.Fatal(err)
	}
	coverPath := filepath.Join(library.Root, "covers", imported.Book.ID+".jpg")
	data, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatalf("PDF cover was not stored: %v", err)
	}
	if !pdf.IsCurrentCover(data) {
		t.Fatal("PDF cover has no renderer version")
	}
	// A JPEG from an older renderer must be replaced, even if it is readable.
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var oldCover bytes.Buffer
	if err := jpeg.Encode(&oldCover, decoded, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverPath, oldCover.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err = library.Cover(context.Background(), imported.Book.ID)
	if err != nil || !pdf.IsCurrentCover(data) {
		t.Fatalf("old PDF cover was not replaced: %v", err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || max(config.Width, config.Height) > pdf.MaxCoverEdge {
		t.Fatalf("invalid PDF cover: %+v, %v", config, err)
	}
	if err := os.Remove(coverPath); err != nil {
		t.Fatal(err)
	}
	data, err = library.Cover(context.Background(), imported.Book.ID)
	if err != nil || len(data) == 0 {
		t.Fatalf("older PDF book did not get a cover: %v", err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), library.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cached, err := reopened.Cover(context.Background(), imported.Book.ID)
	if err != nil || !bytes.Equal(cached, data) {
		t.Fatalf("PDF cover did not survive restart: %v", err)
	}
	assertNoImportFiles(t, reopened)
}

func TestPDFCoverFailureDoesNotBlockImport(t *testing.T) {
	library := newImportLibrary(t)
	// This has a valid container but refers to a missing page object.
	broken := bytes.Replace(testPDF(), []byte("/Kids [3 0 R]"), []byte("/Kids [9 0 R]"), 1)
	imported, err := library.Import(context.Background(), sourceFile(t, "book.pdf", broken))
	if err != nil {
		t.Fatalf("optional PDF cover blocked import: %v", err)
	}
	stored, err := library.Books(context.Background())
	if err != nil || len(stored) != 1 || stored[0].ID != imported.Book.ID {
		t.Fatalf("book was lost: %v", err)
	}
	assertNoImportFiles(t, library)
}

func TestFailedPDFImportRemovesCover(t *testing.T) {
	library := newImportLibrary(t)
	if _, err := library.DB.Exec(`CREATE TRIGGER fail_pdf_import BEFORE INSERT ON books BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Import(context.Background(), sourceFile(t, "book.pdf", testPDF())); err == nil {
		t.Fatal("expected database failure")
	}
	entries, err := os.ReadDir(filepath.Join(library.Root, "covers"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("PDF cover remains after failed import: %v, %v", entries, err)
	}
	assertNoImportFiles(t, library)
}
