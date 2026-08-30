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
