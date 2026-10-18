package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/jsaari97/hylla/internal/books"
	"github.com/jsaari97/hylla/internal/library"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type LibraryService struct {
	mu           sync.Mutex
	current      *library.Library
	configPath   string
	startupError error
}

func (service *LibraryService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current != nil {
		return nil
	}
	service.startupError = nil
	path, err := service.configurationPath()
	if err != nil {
		service.startupError = fmt.Errorf("find application settings: %w", err)
		return nil
	}
	config, err := loadAppConfig(path)
	if err != nil {
		service.startupError = fmt.Errorf("read application settings: %w. Select a library to continue", err)
		return nil
	}
	if config.LibraryPath == "" {
		return nil
	}
	service.current, err = library.Open(ctx, config.LibraryPath)
	if err != nil {
		service.startupError = fmt.Errorf("open previous library %q: %w. Select a library to continue", config.LibraryPath, err)
	}
	// A missing or invalid library must not prevent application startup.
	return nil
}

// Use the selected library even if its path cannot be saved.
func (service *LibraryService) useLibrary(selected *library.Library) {
	service.current = selected
	service.startupError = nil
	path, err := service.configurationPath()
	if err == nil {
		err = saveAppConfig(path, appConfig{LibraryPath: selected.Root})
	}
	if err != nil {
		selected.Warnings = append(selected.Warnings, fmt.Sprintf("Cannot save the library location. Select the library again after a restart: %v", err))
	}
}

type LibraryView struct {
	Path     string
	Books    []books.Book
	Warnings []string
}

type FileImportResult struct {
	Filename  string
	BookID    string
	Title     string
	Duplicate bool
	Error     string
}

func (service *LibraryService) GetCover(ctx context.Context, id string) (string, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return "", fmt.Errorf("open a library first")
	}
	data, err := service.current.Cover(ctx, id)
	if err != nil || len(data) == 0 {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (service *LibraryService) CreateLibrary(ctx context.Context) (string, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current != nil {
		return "", fmt.Errorf("a library is already open")
	}
	path, err := chooseDirectory("Create Hylla library")
	if err != nil {
		return "", fmt.Errorf("select library directory: %w", err)
	}

	if path == "" {
		return "", nil
	}

	createdLibrary, err := library.Create(ctx, path)
	if err != nil {
		return "", fmt.Errorf("create library: %w", err)
	}

	service.useLibrary(createdLibrary)

	return createdLibrary.Root, nil
}

func (service *LibraryService) OpenLibrary(ctx context.Context) (string, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current != nil {
		return "", fmt.Errorf("a library is already open")
	}
	path, err := chooseDirectory("Open Hylla Library")
	if err != nil {
		return "", fmt.Errorf("select library directory: %w", err)
	}

	if path == "" {
		return "", nil
	}

	openedLibrary, err := library.Open(ctx, path)
	if err != nil {
		return "", fmt.Errorf("open library: %w", err)
	}

	service.useLibrary(openedLibrary)

	return openedLibrary.Root, nil
}

func (service *LibraryService) ServiceShutdown() error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return nil
	}

	return service.current.Close()
}

func (service *LibraryService) GetLibrary(ctx context.Context) (LibraryView, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		if service.startupError != nil {
			return LibraryView{}, service.startupError
		}
		return LibraryView{Books: []books.Book{}, Warnings: []string{}}, nil
	}
	stored, err := service.current.Books(ctx)
	if err != nil {
		return LibraryView{}, err
	}
	warnings := append([]string{}, service.current.Warnings...)
	return LibraryView{Path: service.current.Root, Books: stored, Warnings: warnings}, nil
}

func (service *LibraryService) UpdateBookMetadata(ctx context.Context, id, title string, authors []string) (books.Book, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return books.Book{}, fmt.Errorf("open a library first")
	}
	return service.current.UpdateBookMetadata(ctx, id, title, authors)
}

func (service *LibraryService) SetReadingStatus(ctx context.Context, id string, status books.ReadingStatus) (books.Book, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return books.Book{}, fmt.Errorf("open a library first")
	}
	return service.current.SetReadingStatus(ctx, id, status)
}

func (service *LibraryService) ImportBooks(ctx context.Context) ([]FileImportResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return nil, fmt.Errorf("open a library first")
	}
	paths, err := application.Get().Dialog.OpenFile().
		CanChooseFiles(true).CanChooseDirectories(false).
		SetTitle("Import books").AddFilter("EPUB and PDF books", "*.epub;*.pdf").
		PromptForMultipleSelection()
	if err != nil {
		return nil, fmt.Errorf("select books: %w", err)
	}
	return service.importFiles(ctx, paths)
}

func (service *LibraryService) ImportFiles(ctx context.Context, paths []string) ([]FileImportResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.current == nil {
		return nil, fmt.Errorf("open a library first")
	}
	return service.importFiles(ctx, paths)
}

func (service *LibraryService) importFiles(ctx context.Context, paths []string) ([]FileImportResult, error) {
	results := make([]FileImportResult, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		imported, err := service.current.Import(ctx, path)
		result := FileImportResult{Filename: filepath.Base(path)}
		if err != nil {
			result.Error = err.Error()
		} else {
			result.BookID = imported.Book.ID
			result.Title = imported.Book.Title
			result.Duplicate = imported.Duplicate
		}
		results = append(results, result)
	}
	return results, nil
}

func chooseDirectory(title string) (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle(title).
		PromptForSingleSelection()
}
