package library

import (
	"database/sql"
	"os"
	"sync"
)

const CurrentFormatVersion = 1

type Manifest struct {
	FormatVersion int    `json:"formatVersion"`
	LibraryID     string `json:"libraryId"`
}

type Library struct {
	Root     string
	DB       *sql.DB
	Warnings []string

	mu              sync.Mutex
	metadataChecked map[string]bool
	lockFile        *os.File
}
