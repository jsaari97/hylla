package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/library"
)

func TestSetReadingStatusRequiresLibrary(t *testing.T) {
	service := &LibraryService{}
	if _, err := service.SetReadingStatus(context.Background(), "book", books.StatusRead); err == nil {
		t.Fatal("status update without library succeeded")
	}
}

func TestImportFilesRequiresLibrary(t *testing.T) {
	service := &LibraryService{}
	if _, err := service.ImportFiles(context.Background(), []string{"book.pdf"}); err == nil {
		t.Fatal("import without library succeeded")
	}
}

func TestImportFilesReportsEachFailure(t *testing.T) {
	ctx := context.Background()
	library, err := library.Create(ctx, filepath.Join(t.TempDir(), "library"))
	if err != nil {
		t.Fatal(err)
	}
	service := &LibraryService{current: library}
	defer service.ServiceShutdown()
	source := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(source, []byte("not a PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	results, err := service.ImportFiles(ctx, []string{source, source + ".epub"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Error == "" || results[1].Error == "" {
		t.Fatalf("per-file failures missing: %+v", results)
	}
	view, err := service.GetLibrary(ctx)
	if err != nil || view.Path != library.Root || len(view.Books) != 0 {
		t.Fatalf("library view: %+v, %v", view, err)
	}
}
