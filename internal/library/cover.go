package library

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"uuid"

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/metadata/epub"
	"github.com/jsaari97/hylla/internal/metadata/pdf"
	"github.com/jsaari97/hylla/internal/storage"
)

// Cover also fills in missing covers for books imported by earlier versions.
func (library *Library) Cover(ctx context.Context, id string) ([]byte, error) {
	library.mu.Lock()
	defer library.mu.Unlock()
	if library.DB == nil {
		return nil, fmt.Errorf("library is closed")
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("invalid book ID")
	}
	book, err := storage.BookByID(ctx, library.DB, id)
	if err != nil {
		return nil, err
	}
	if book.Format != books.FormatEPUB && book.Format != books.FormatPDF {
		return nil, nil
	}
	root, err := library.openManagedRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	coverPath := "covers/" + id + ".jpg"
	if info, err := root.Lstat(coverPath); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("cover is not a regular file")
		}
		if info.Size() <= epub.MaxCoverBytes {
			file, err := root.Open(coverPath)
			if err != nil {
				return nil, err
			}
			data, readErr := io.ReadAll(io.LimitReader(file, epub.MaxCoverBytes+1))
			file.Close()
			current := book.Format != books.FormatPDF || !pdf.CoverSupported || pdf.IsCurrentCover(data)
			if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); readErr == nil && len(data) <= epub.MaxCoverBytes && err == nil && current {
				return data, ctx.Err()
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return extractBookCover(ctx, root, id, book.Format)
}

func extractBookCover(ctx context.Context, root *os.Root, id string, format books.BookFormat) ([]byte, error) {
	bookPath := "books/" + id + "." + string(format)
	info, err := root.Lstat(bookPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("book is not a regular file")
	}
	file, err := root.Open(bookPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var data []byte
	switch format {
	case books.FormatEPUB:
		data, err = epub.ExtractCover(ctx, file, info.Size())
	case books.FormatPDF:
		data, err = pdf.RenderCover(ctx, file)
	default:
		return nil, fmt.Errorf("unsupported cover format")
	}
	if err != nil || len(data) == 0 {
		return nil, err
	}
	temporary := "tmp/imports/" + id + ".cover-" + uuid.New().String()
	output, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	defer root.Remove(temporary)
	_, writeErr := output.Write(data)
	if writeErr == nil {
		writeErr = output.Sync()
	}
	closeErr := output.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := root.Rename(temporary, "covers/"+id+".jpg"); err != nil {
		return nil, err
	}
	return data, nil
}
