package library

import (
	"context"
	"fmt"

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/storage"
)

func (library *Library) SetReadingStatus(ctx context.Context, id string, status books.ReadingStatus) (books.Book, error) {
	library.mu.Lock()
	defer library.mu.Unlock()
	if library.DB == nil {
		return books.Book{}, fmt.Errorf("library is closed")
	}
	root, err := library.openManagedRoot()
	if err != nil {
		return books.Book{}, err
	}
	defer root.Close()
	return storage.SetReadingStatus(ctx, library.DB, id, status)
}
