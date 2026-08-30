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
	Root string
	DB   *sql.DB

	lockFile  *os.File
	closeOnce sync.Once
	closeErr  error
}
