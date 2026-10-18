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

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/storage"
)

func TestReadingStatusPersistsAfterReopen(t *testing.T) {
	for _, status := range []books.ReadingStatus{books.StatusReading, books.StatusRead, books.StatusUnread} {
		t.Run(string(status), func(t *testing.T) {
			ctx := context.Background()
			library := newImportLibrary(t)
			data := epubWithMetadata(t)
			source := sourceFile(t, "book.epub", data)
			imported, err := library.Import(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := library.SetReadingStatus(ctx, imported.Book.ID, books.StatusRead); err != nil {
				t.Fatal(err)
			}
			updated, err := library.SetReadingStatus(ctx, imported.Book.ID, status)
			if err != nil || updated.ReadingStatus != status || !updated.UpdatedAt.After(imported.Book.UpdatedAt) {
				t.Fatalf("status update: %+v, %v", updated, err)
			}
			want := imported.Book
			want.ReadingStatus = status
			want.UpdatedAt = updated.UpdatedAt
			if !reflect.DeepEqual(want, updated) {
				t.Fatalf("status update changed other fields: %+v", updated)
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
				t.Fatalf("status did not persist: %+v, %v", stored, err)
			}
			for _, path := range []string{source, filepath.Join(reopened.Root, "books", updated.ID+".epub")} {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("book file changed: %s, %v", path, err)
				}
			}
		})
	}
}

func TestReadingStatusErrorsPreserveBook(t *testing.T) {
	ctx := context.Background()
	library := newImportLibrary(t)
	imported, err := library.Import(ctx, sourceFile(t, "book.epub", testEPUB(t, "Text")))
	if err != nil {
		t.Fatal(err)
	}
	before, err := storage.BookByID(ctx, library.DB, imported.Book.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []books.ReadingStatus{"", "finished", "READ"} {
		if _, err := library.SetReadingStatus(ctx, imported.Book.ID, status); err == nil {
			t.Fatalf("invalid status accepted: %q", status)
		}
	}
	if _, err := library.SetReadingStatus(ctx, "missing", books.StatusRead); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing book error: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := library.SetReadingStatus(cancelled, imported.Book.ID, books.StatusRead); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled update error: %v", err)
	}
	if _, err := library.DB.Exec(`CREATE TRIGGER fail_status BEFORE UPDATE OF reading_status ON books BEGIN SELECT RAISE(ABORT,'status failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := library.SetReadingStatus(ctx, imported.Book.ID, books.StatusRead); err == nil {
		t.Fatal("failed write reported success")
	}
	stored, err := storage.BookByID(ctx, library.DB, imported.Book.ID)
	if err != nil || !reflect.DeepEqual(stored, before) {
		t.Fatalf("failed update changed book: %+v, %v", stored, err)
	}
}

func TestReadingStatusRequiresAvailableLibrary(t *testing.T) {
	ctx := context.Background()
	library := newImportLibrary(t)
	imported, err := library.Import(ctx, sourceFile(t, "book.epub", testEPUB(t, "Text")))
	if err != nil {
		t.Fatal(err)
	}
	moved := library.Root + "-moved"
	if err := os.Rename(library.Root, moved); err != nil {
		t.Fatal(err)
	}
	_, updateErr := library.SetReadingStatus(ctx, imported.Book.ID, books.StatusRead)
	if err := os.Rename(moved, library.Root); err != nil {
		t.Fatal(err)
	}
	if updateErr == nil {
		t.Fatal("update accepted while library unavailable")
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := library.SetReadingStatus(ctx, imported.Book.ID, books.StatusRead); err == nil {
		t.Fatal("update accepted after close")
	}
}
