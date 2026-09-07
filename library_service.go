package main

import (
	"context"
	"fmt"

	"github.com/jsaari97/hylla/internal/library"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type LibraryService struct {
	current *library.Library
}

func (service *LibraryService) CreateLibrary(ctx context.Context) (string, error) {
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

	service.current = createdLibrary

	return createdLibrary.Root, nil
}

func (service *LibraryService) OpenLibrary(ctx context.Context) (string, error) {
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

	service.current = openedLibrary

	return openedLibrary.Root, nil
}

func (service *LibraryService) ServiceShutdown() error {
	if service.current == nil {
		return nil
	}

	return service.current.Close()
}

func chooseDirectory(title string) (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle(title).
		PromptForSingleSelection()
}
