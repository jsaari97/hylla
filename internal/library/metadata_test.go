package library

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/metadata/pdf"
	"github.com/jsaari97/hylla/internal/storage"
)

func epubWithMetadata(t *testing.T) []byte {
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
			data = []byte(strings.Replace(string(data), "<metadata/>", `<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>A Better Title</dc:title><dc:creator>First Author</dc:creator><dc:creator>Second Author</dc:creator></metadata>`, 1))
		}
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: entry.Name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestImportAndReopenMetadata(t *testing.T) {
	library := newImportLibrary(t)
	data := epubWithMetadata(t)
	source := sourceFile(t, "ugly_filename.epub", data)
	imported, err := library.Import(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Book.Title != "A Better Title" || len(imported.Book.Authors) != 2 {
		t.Fatalf("metadata not used: %+v", imported.Book)
	}
	duplicate, err := library.Import(context.Background(), source)
	if err != nil || !duplicate.Duplicate || len(duplicate.Book.Authors) != 2 {
		t.Fatalf("duplicate lost metadata: %+v, %v", duplicate, err)
	}
	original, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("source changed: %v", err)
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
	if err != nil || len(stored) != 1 || stored[0].Title != "A Better Title" || len(stored[0].Authors) != 2 || stored[0].Authors[0].Name != "First Author" || stored[0].Authors[1].Name != "Second Author" {
		t.Fatalf("metadata not retained: %+v, %v", stored, err)
	}
}

func TestExistingMetadataUpdatePreservesEdits(t *testing.T) {
	for _, test := range []struct {
		name, title, author   string
		edited                bool
		wantTitle, wantAuthor string
	}{
		{"untouched", "ugly_filename", "", false, "A Better Title", "First Author"},
		{"custom title", "My title", "", false, "My title", ""},
		{"edited timestamp", "ugly_filename", "", true, "ugly_filename", ""},
		{"custom author", "ugly_filename", "My author", false, "ugly_filename", "My author"},
	} {
		t.Run(test.name, func(t *testing.T) {
			library := newImportLibrary(t)
			imported, err := library.Import(context.Background(), sourceFile(t, "ugly_filename.epub", epubWithMetadata(t)))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := library.DB.Exec(`DELETE FROM book_authors WHERE book_id = ?`, imported.Book.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := library.DB.Exec(`UPDATE books SET title = ?, updated_at = created_at, reading_status = 'read' WHERE id = ?`, test.title, imported.Book.ID); err != nil {
				t.Fatal(err)
			}
			if test.edited {
				if _, err := library.DB.Exec(`UPDATE books SET updated_at = ? WHERE id = ?`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), imported.Book.ID); err != nil {
					t.Fatal(err)
				}
			}
			if test.author != "" {
				if _, err := library.DB.Exec(`INSERT INTO book_authors (book_id,position,name) VALUES (?,0,?)`, imported.Book.ID, test.author); err != nil {
					t.Fatal(err)
				}
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
			if err != nil || len(stored) != 1 {
				t.Fatalf("read books: %v", err)
			}
			book := stored[0]
			if book.Title != test.wantTitle || book.ReadingStatus != books.StatusRead {
				t.Fatalf("unexpected update: %+v", book)
			}
			if test.wantAuthor == "" && len(book.Authors) != 0 {
				t.Fatalf("authors replaced: %+v", book.Authors)
			}
			if test.wantAuthor != "" && (len(book.Authors) == 0 || book.Authors[0].Name != test.wantAuthor) {
				t.Fatalf("authors missing or replaced: %+v", book.Authors)
			}
			before := book.UpdatedAt
			again, err := reopened.Books(context.Background())
			if err != nil || !again[0].UpdatedAt.Equal(before) {
				t.Fatalf("repeated list changed metadata: %v", err)
			}
			managed := filepath.Join(reopened.Root, "books", book.ID+".epub")
			data, err := os.ReadFile(managed)
			if err != nil || !bytes.Equal(data, epubWithMetadata(t)) {
				t.Fatalf("managed EPUB changed: %v", err)
			}
		})
	}
}

func TestPDFImportMetadata(t *testing.T) {
	if !pdf.CoverSupported {
		t.Skip("native PDF metadata is unavailable")
	}
	library := newImportLibrary(t)
	source := sourceFile(t, "sql-antipatterns-volume-1_P1.0.pdf", testPDFMetadata(`<< /Title (SQL Antipatterns, Volume 1) /Author (Bill Karwin) >>`))
	imported, err := library.Import(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Book.Title != "SQL Antipatterns, Volume 1" || len(imported.Book.Authors) != 1 || imported.Book.Authors[0].Name != "Bill Karwin" {
		t.Fatalf("PDF metadata not used: %+v", imported.Book)
	}
}

func TestMetadataUpdateRollsBackAuthorsAndTitle(t *testing.T) {
	library := newImportLibrary(t)
	imported, err := library.Import(context.Background(), sourceFile(t, "plain.epub", testEPUB(t, "Text")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.DB.Exec(`CREATE TRIGGER fail_author BEFORE INSERT ON book_authors BEGIN SELECT RAISE(ABORT,'author failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = storage.FillBookMetadata(context.Background(), library.DB, imported.Book.ID, "plain", books.Metadata{Title: "New title", Authors: []books.Author{{Name: "Author"}}}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected author failure")
	}
	book, err := storage.BookByID(context.Background(), library.DB, imported.Book.ID)
	if err != nil || book.Title != "plain" || len(book.Authors) != 0 {
		t.Fatalf("partial metadata update persisted: %+v, %v", book, err)
	}
}
