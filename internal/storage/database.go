package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, databasePath string) (*sql.DB, error) {
	if databasePath == "" {
		return nil, fmt.Errorf("database path is empty")
	}

	absolutePath, err := filepath.Abs(databasePath)
	if err != nil {
		return nil, fmt.Errorf("get absolute database path: %w", err)
	}

	uriPath := filepath.ToSlash(absolutePath)

	if filepath.VolumeName(absolutePath) != "" {
		uriPath = "/" + uriPath
	}

	databaseURL := &url.URL{
		Scheme: "file",
		Path:   uriPath,
	}

	parameters := databaseURL.Query()
	parameters.Set("_foreign_keys", "on")
	parameters.Set("_journal_mode", "DELETE")
	parameters.Set("_busy_timeout", "5000")
	databaseURL.RawQuery = parameters.Encode()

	db, err := sql.Open("sqlite", databaseURL.String())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	return db, nil
}
