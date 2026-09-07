package library

import (
	"fmt"
	"path/filepath"
)

func absoluteRootPath(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("library root path cannot be empty")
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("get absolute library root path: %w", err)
	}

	return absoluteRoot, nil
}
