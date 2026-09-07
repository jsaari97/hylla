package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func writeManifest(root string, manifest Manifest) error {
	finalPath := filepath.Join(root, "library.json")

	temporaryFile, err := os.CreateTemp(root, ".library.json.*")
	if err != nil {
		return fmt.Errorf("create temporary manifest file: %w", err)
	}

	temporaryPath := temporaryFile.Name()
	fileClosed := false
	manifestWritten := false

	defer func() {
		if !fileClosed {
			temporaryFile.Close()
		}

		if !manifestWritten {
			os.Remove(temporaryPath)
		}
	}()

	encoder := json.NewEncoder(temporaryFile)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("write manifest to temporary file: %w", err)
	}

	if err := temporaryFile.Sync(); err != nil {
		return fmt.Errorf("sync temporary manifest file: %w", err)
	}

	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary manifest file: %w", err)
	}

	fileClosed = true

	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return fmt.Errorf("rename temporary manifest file to final path: %w", err)
	}

	manifestWritten = true

	return nil
}

func readManifest(root string) (Manifest, error) {
	path := filepath.Join(root, "library.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest file: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("unmarshal manifest JSON: %w", err)
	}

	if manifest.FormatVersion < 1 || manifest.FormatVersion > CurrentFormatVersion {
		return Manifest{}, fmt.Errorf("unsupported manifest version: %d", manifest.FormatVersion)
	}

	if manifest.LibraryID == "" {
		return Manifest{}, fmt.Errorf("manifest is missing library ID")
	}

	return manifest, nil
}
