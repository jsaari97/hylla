package library

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/storage"
)

func TestMetadataEditsPersist(t *testing.T) {
	for _, format := range []string{"epub", "pdf"} {
		t.Run(format, func(t *testing.T) {
			ctx := context.Background()
			library := newImportLibrary(t)
			data := epubWithMetadata(t)
			if format == "pdf" {
				data = testPDF()
			}
			source := sourceFile(t, "original."+format, data)
			imported, err := library.Import(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			before, err := library.SetReadingStatus(ctx, imported.Book.ID, books.StatusReading)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := library.UpdateBookMetadata(ctx, before.ID, "  A new title 日本語  ", []string{" First Author ", "", "Second, Author", "  "})
			if err != nil {
				t.Fatal(err)
			}
			want := before
			want.Title = "A new title 日本語"
			want.Authors = []books.Author{{Name: "First Author"}, {Name: "Second, Author"}}
			want.UpdatedAt = updated.UpdatedAt
			if !reflect.DeepEqual(want, updated) || !updated.UpdatedAt.After(before.UpdatedAt) {
				t.Fatalf("metadata edit: %+v", updated)
			}
			if err := library.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(ctx, library.Root)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			stored, err := reopened.Books(ctx)
			if err != nil || len(stored) != 1 || !reflect.DeepEqual(stored[0], updated) {
				t.Fatalf("metadata did not persist: %+v, %v", stored, err)
			}
			for _, path := range []string{source, filepath.Join(reopened.Root, "books", before.ID+"."+format)} {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("book file changed: %s, %v", path, err)
				}
			}
		})
	}
}

func TestManualFallbackMetadataSurvivesExtraction(t *testing.T) {
	ctx := context.Background()
	library := newImportLibrary(t)
	imported, err := library.Import(ctx, sourceFile(t, "original.epub", epubWithMetadata(t)))
	if err != nil {
		t.Fatal(err)
	}
	// A user can deliberately restore the filename title and remove all authors.
	updated, err := library.UpdateBookMetadata(ctx, imported.Book.ID, "original", nil)
	if err != nil || len(updated.Authors) != 0 {
		t.Fatalf("clear authors: %+v, %v", updated, err)
	}
	changed, err := storage.FillBookMetadata(ctx, library.DB, updated.ID, "original", books.Metadata{
		Title: "Extracted title", Authors: []books.Author{{Name: "Extracted author"}},
	}, time.Now().UTC())
	if err != nil || changed {
		t.Fatalf("automatic extraction replaced manual edit: %v, %v", changed, err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, library.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	stored, err := reopened.Books(ctx)
	if err != nil || len(stored) != 1 || !reflect.DeepEqual(stored[0], updated) {
		t.Fatalf("manual metadata replaced after reopen: %+v, %v", stored, err)
	}
}

func TestFailedMetadataEditsPreserveBook(t *testing.T) {
	ctx := context.Background()
	library := newImportLibrary(t)
	imported, err := library.Import(ctx, sourceFile(t, "original.epub", epubWithMetadata(t)))
	if err != nil {
		t.Fatal(err)
	}
	before, err := storage.BookByID(ctx, library.DB, imported.Book.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"", " \t\n "} {
		if _, err := library.UpdateBookMetadata(ctx, before.ID, title, []string{"New author"}); err == nil {
			t.Fatalf("empty title accepted: %q", title)
		}
	}
	if _, err := library.UpdateBookMetadata(ctx, "missing", "Title", nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing book error: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := library.UpdateBookMetadata(cancelled, before.ID, "Title", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled edit error: %v", err)
	}
	if _, err := library.DB.Exec(`CREATE TRIGGER fail_edit_author BEFORE INSERT ON book_authors WHEN NEW.position = 1 BEGIN SELECT RAISE(ABORT,'author failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := library.UpdateBookMetadata(ctx, before.ID, "New title", []string{"New author", "Failing author"}); err == nil {
		t.Fatal("failed author write reported success")
	}
	stored, err := storage.BookByID(ctx, library.DB, before.ID)
	if err != nil || !reflect.DeepEqual(stored, before) {
		t.Fatalf("failed edit changed metadata: %+v, %v", stored, err)
	}
	moved := library.Root + "-moved"
	if err := os.Rename(library.Root, moved); err != nil {
		t.Fatal(err)
	}
	_, updateErr := library.UpdateBookMetadata(ctx, before.ID, "Title", nil)
	if err := os.Rename(moved, library.Root); err != nil {
		t.Fatal(err)
	}
	if updateErr == nil {
		t.Fatal("edit accepted while library unavailable")
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := library.UpdateBookMetadata(ctx, before.ID, "Title", nil); err == nil {
		t.Fatal("edit accepted after close")
	}
}
