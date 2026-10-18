package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsaari97/hylla/internal/library"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestPreviousLibraryOpensAfterRestart(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "Books 日本語")
	selected, err := library.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "settings", "config.json")
	first := &LibraryService{configPath: configPath}
	first.useLibrary(selected)
	if err := first.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	second := &LibraryService{configPath: configPath}
	defer second.ServiceShutdown()
	if err := second.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	view, err := second.GetLibrary(ctx)
	if err != nil || view.Path != root {
		t.Fatalf("restored library = %+v, %v", view, err)
	}
	// Startup must be safe if the service already has an open library.
	if err := second.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Open(ctx, root); err == nil {
		t.Fatal("restored library is not locked")
	}
}

func TestStartupWithoutPreviousLibrary(t *testing.T) {
	for _, contents := range []string{"", "{}", `{"libraryPath":""}`} {
		t.Run(contents, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if contents != "" {
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			service := &LibraryService{configPath: path}
			if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatal(err)
			}
			view, err := service.GetLibrary(context.Background())
			if err != nil || view.Path != "" || len(view.Books) != 0 {
				t.Fatalf("first launch = %+v, %v", view, err)
			}
		})
	}
}

func TestStartupFailureAllowsAnotherLibrary(t *testing.T) {
	for _, failure := range []string{"missing", "invalid library", "locked", "invalid config", "relative path"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "previous")
			path := filepath.Join(t.TempDir(), "config.json")
			switch failure {
			case "invalid library":
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			case "locked":
				locked, err := library.Create(ctx, root)
				if err != nil {
					t.Fatal(err)
				}
				defer locked.Close()
			case "relative path":
				root = "relative-library"
			}
			if err := saveAppConfig(path, appConfig{LibraryPath: root}); err != nil {
				t.Fatal(err)
			}
			if failure == "invalid config" {
				if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			service := &LibraryService{configPath: path}
			defer service.ServiceShutdown()
			if err := service.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
				t.Fatalf("startup must continue: %v", err)
			}
			if _, err := service.GetLibrary(ctx); err == nil || !strings.Contains(err.Error(), "Select a library") {
				t.Fatalf("missing startup error: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatalf("failed startup changed settings: %v", err)
			}
			replacement, err := library.Create(ctx, filepath.Join(t.TempDir(), "replacement"))
			if err != nil {
				t.Fatal(err)
			}
			service.useLibrary(replacement)
			view, err := service.GetLibrary(ctx)
			if err != nil || view.Path != replacement.Root {
				t.Fatalf("replacement library = %+v, %v", view, err)
			}
			config, err := loadAppConfig(path)
			if err != nil || config.LibraryPath != replacement.Root {
				t.Fatalf("replacement settings = %+v, %v", config, err)
			}
		})
	}
}

func TestConfigSaveFailureKeepsLibraryOpen(t *testing.T) {
	ctx := context.Background()
	selected, err := library.Create(ctx, filepath.Join(t.TempDir(), "library"))
	if err != nil {
		t.Fatal(err)
	}
	// A directory at the destination makes the final rename fail.
	path := t.TempDir()
	service := &LibraryService{configPath: path}
	defer service.ServiceShutdown()
	service.useLibrary(selected)
	view, err := service.GetLibrary(ctx)
	if err != nil || view.Path != selected.Root || len(view.Warnings) != 1 {
		t.Fatalf("library after save failure = %+v, %v", view, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".config-") {
			t.Fatalf("temporary configuration was not removed: %s", entry.Name())
		}
	}
}
