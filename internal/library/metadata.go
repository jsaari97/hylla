package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/metadata/epub"
	"github.com/jsaari97/hylla/internal/metadata/pdf"
	"github.com/jsaari97/hylla/internal/storage"
)

func filenameTitle(filename string) string {
	title := strings.TrimSuffix(filename, filepath.Ext(filename))
	if strings.TrimSpace(title) == "" {
		return filename
	}
	return title
}

func readBookMetadata(ctx context.Context, root *os.Root, id string, format books.BookFormat) (books.Metadata, error) {
	if _, err := uuid.Parse(id); err != nil {
		return books.Metadata{}, err
	}
	if format != books.FormatEPUB && format != books.FormatPDF {
		return books.Metadata{}, fmt.Errorf("unsupported metadata format")
	}
	name := "books/" + id + "." + string(format)
	info, err := root.Lstat(name)
	if err != nil {
		return books.Metadata{}, err
	}
	if !info.Mode().IsRegular() {
		return books.Metadata{}, fmt.Errorf("book must be a regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return books.Metadata{}, err
	}
	defer file.Close()
	if format == books.FormatEPUB {
		return epub.ExtractMetadata(ctx, file, info.Size())
	}
	return pdf.ExtractMetadata(ctx, file)
}

// Called with the library mutex held. Only untouched, filename-based records
// are eligible. A failed extraction is retried at the next library session.
func (library *Library) fillMissingMetadata(ctx context.Context, stored []books.Book) ([]books.Book, error) {
	if library.metadataChecked == nil {
		library.metadataChecked = make(map[string]bool)
	}
	var root *os.Root
	defer func() {
		if root != nil {
			root.Close()
		}
	}()
	for index, book := range stored {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fallback := filenameTitle(book.OriginalFilename)
		if library.metadataChecked[book.ID] || book.Title != fallback || len(book.Authors) != 0 || !book.CreatedAt.Equal(book.UpdatedAt) {
			continue
		}
		if root == nil {
			var err error
			root, err = library.openManagedRoot()
			if err != nil {
				return stored, nil
			}
		}
		metadata, err := readBookMetadata(ctx, root, book.ID, book.Format)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		library.metadataChecked[book.ID] = true
		if err != nil || (metadata.Title == "" && len(metadata.Authors) == 0) {
			continue
		}
		updated := time.Now().UTC()
		changed, err := storage.FillBookMetadata(ctx, library.DB, book.ID, fallback, metadata, updated)
		if err != nil {
			return nil, fmt.Errorf("save imported metadata: %w", err)
		}
		if changed {
			if metadata.Title != "" {
				stored[index].Title = metadata.Title
			}
			stored[index].Authors = metadata.Authors
			stored[index].UpdatedAt = updated
		}
	}
	return stored, nil
}
