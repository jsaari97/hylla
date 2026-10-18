package library

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"uuid"

	"github.com/jsaari97/hylla/internal/storage"
)

// recoverImports runs with the library lock held, before user operations.
// Files moved before an interrupted database commit are kept for recovery.
func (library *Library) recoverImports(ctx context.Context) error {
	root, err := library.openManagedRoot()
	if err != nil {
		return err
	}
	defer root.Close()

	entries, err := regularEntries(root, "tmp/imports")
	if err != nil {
		return err
	}
	for _, name := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := root.Remove("tmp/imports/" + name); err != nil {
			return fmt.Errorf("remove interrupted import: %w", err)
		}
	}
	stored, err := storage.ListBooks(ctx, library.DB)
	if err != nil {
		return err
	}
	expected := make(map[string]bool, len(stored))
	for _, book := range stored {
		if _, err := uuid.Parse(book.ID); err != nil || (book.Format != "epub" && book.Format != "pdf") {
			return fmt.Errorf("invalid managed book ID or format")
		}
		name := book.ID + "." + string(book.Format)
		expected[name] = true
		info, err := root.Lstat("books/" + name)
		if os.IsNotExist(err) {
			library.Warnings = append(library.Warnings, fmt.Sprintf("Book file is missing: %s (%s)", book.Title, name))
			continue
		}
		if err != nil {
			return fmt.Errorf("check book file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("book file %q is not a regular file", name)
		}
	}
	entries, err = regularEntries(root, "books")
	if err != nil {
		return err
	}
	for _, name := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if expected[name] {
			continue
		}
		recoveryDir := "tmp/recovery/" + uuid.New().String()
		if err := root.Mkdir(recoveryDir, 0o700); err != nil {
			return fmt.Errorf("create recovery directory: %w", err)
		}
		if err := root.Rename("books/"+name, recoveryDir+"/"+name); err != nil {
			return fmt.Errorf("recover untracked book: %w", err)
		}
		id := strings.TrimSuffix(name, path.Ext(name))
		for _, ext := range []string{".jpg", ".png", ".webp"} {
			cover := "covers/" + id + ext
			info, err := root.Lstat(cover)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("cover %q is not a regular file", cover)
			}
			if err := root.Rename(cover, recoveryDir+"/"+id+ext); err != nil {
				return fmt.Errorf("recover book cover: %w", err)
			}
		}
		library.Warnings = append(library.Warnings, "An interrupted import was moved to "+recoveryDir+"/"+name)
	}
	return nil
}

func regularEntries(root *os.Root, directory string) ([]string, error) {
	file, err := root.Open(directory)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := root.Lstat(directory + "/" + entry.Name())
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("library path %q is not a regular file", directory+"/"+entry.Name())
		}
		names = append(names, entry.Name())
	}
	return names, nil
}
