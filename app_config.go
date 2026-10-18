package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type appConfig struct {
	LibraryPath string `json:"libraryPath"`
}

func (service *LibraryService) configurationPath() (string, error) {
	if service.configPath != "" {
		return service.configPath, nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	name := "Hylla"
	if runtime.GOOS == "linux" {
		name = "hylla"
	}
	return filepath.Join(directory, name, "config.json"), nil
}

func loadAppConfig(path string) (appConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return appConfig{}, nil
	}
	if err != nil {
		return appConfig{}, err
	}
	var config appConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return appConfig{}, err
	}
	if config.LibraryPath != "" && !filepath.IsAbs(config.LibraryPath) {
		return appConfig{}, fmt.Errorf("saved library path must be absolute")
	}
	return config, nil
}

func saveAppConfig(path string, config appConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
