package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

type ImportResult struct {
	Book      books.Book
	Duplicate bool
}

// Import copies a source file. The source is never changed.
func (library *Library) Import(ctx context.Context, source string) (_ ImportResult, err error) {
	library.mu.Lock()
	defer library.mu.Unlock()
	if library.DB == nil {
		return ImportResult{}, fmt.Errorf("library is closed")
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}

	extension := strings.ToLower(filepath.Ext(source))
	if extension != ".epub" && extension != ".pdf" {
		return ImportResult{}, fmt.Errorf("select an EPUB or PDF file")
	}
	root, err := library.openManagedRoot()
	if err != nil {
		return ImportResult{}, err
	}
	defer root.Close()

	input, err := os.Open(source)
	if err != nil {
		return ImportResult{}, fmt.Errorf("open source file: %w", err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return ImportResult{}, err
	}
	if !info.Mode().IsRegular() {
		return ImportResult{}, fmt.Errorf("source must be a regular file")
	}

	id := uuid.New().String()
	stagedPath := "tmp/imports/" + id
	managedPath := "books/" + id + extension
	staged, err := root.OpenFile(stagedPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return ImportResult{}, fmt.Errorf("create import file: %w", err)
	}
	managed := false
	complete := false
	defer func() {
		_ = staged.Close()
		if !complete {
			cleanupPath := stagedPath
			if managed {
				cleanupPath = managedPath
			}
			if cleanupErr := root.Remove(cleanupPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove incomplete import: %w", cleanupErr))
			}
			if cleanupErr := root.Remove("covers/" + id + ".jpg"); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove incomplete cover: %w", cleanupErr))
			}
		}
	}()

	fileHash, size, err := copyBook(ctx, staged, input)
	if err != nil {
		return ImportResult{}, fmt.Errorf("copy book: %w", err)
	}
	if extension == ".epub" {
		err = epub.Validate(ctx, staged, size)
	} else {
		err = pdf.Validate(staged, size)
	}
	if err != nil {
		return ImportResult{}, fmt.Errorf("validate book: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}

	existing, err := storage.BookByHash(ctx, library.DB, fileHash)
	if err == nil {
		return ImportResult{Book: existing, Duplicate: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ImportResult{}, fmt.Errorf("check duplicate: %w", err)
	}

	if err := staged.Sync(); err != nil {
		return ImportResult{}, fmt.Errorf("sync imported file: %w", err)
	}
	if err := staged.Close(); err != nil {
		return ImportResult{}, fmt.Errorf("close imported file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}
	if err := root.Rename(stagedPath, managedPath); err != nil {
		return ImportResult{}, fmt.Errorf("store book: %w", err)
	}
	managed = true
	// Cover extraction is optional. A valid book must remain importable.
	_, _ = extractBookCover(ctx, root, id, books.BookFormat(extension[1:]))
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}

	now := time.Now().UTC()
	filename := filepath.Base(source)
	title := filenameTitle(filename)
	book := books.Book{
		ID: id, Title: title, Authors: []books.Author{},
		Format: books.BookFormat(extension[1:]), OriginalFilename: filename,
		FileHash: fileHash, ReadingStatus: books.StatusUnread, CreatedAt: now, UpdatedAt: now,
	}
	if metadata, metadataErr := readBookMetadata(ctx, root, book.ID, book.Format); metadataErr == nil {
		if metadata.Title != "" {
			book.Title = metadata.Title
		}
		book.Authors = metadata.Authors
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}
	if err := storage.InsertBook(ctx, library.DB, book); err != nil {
		return ImportResult{}, fmt.Errorf("save book: %w", err)
	}
	complete = true
	if library.metadataChecked == nil {
		library.metadataChecked = make(map[string]bool)
	}
	library.metadataChecked[id] = true
	return ImportResult{Book: book}, nil
}

func (library *Library) Books(ctx context.Context) ([]books.Book, error) {
	library.mu.Lock()
	defer library.mu.Unlock()
	if library.DB == nil {
		return nil, fmt.Errorf("library is closed")
	}
	stored, err := storage.ListBooks(ctx, library.DB)
	if err != nil {
		return nil, err
	}
	return library.fillMissingMetadata(ctx, stored)
}

// openManagedRoot confines all managed file operations to the selected library.
// Managed directories and files must not be symbolic links.
func (library *Library) openManagedRoot() (*os.Root, error) {
	root, err := os.OpenRoot(library.Root)
	if err != nil {
		return nil, fmt.Errorf("open library directory: %w", err)
	}
	for _, name := range []string{"books", "covers", "tmp", "tmp/imports", "tmp/deletions", "tmp/recovery", "backups", "library.json", "library.sqlite"} {
		info, err := root.Lstat(name)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			root.Close()
			return nil, fmt.Errorf("library path %q is unavailable or unsafe", name)
		}
		isFile := name == "library.json" || name == "library.sqlite"
		if info.IsDir() == isFile {
			root.Close()
			return nil, fmt.Errorf("invalid library path %q", name)
		}
	}
	return root, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func copyBook(ctx context.Context, destination io.Writer, source io.Reader) (string, int64, error) {
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(destination, hash), contextReader{ctx, source})
	if err != nil {
		return "", size, err
	}
	if err := ctx.Err(); err != nil {
		return "", size, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
