package library

import (
	"database/sql"
	"os"
)

const CurrentFormatVersion = 1

type Manifest struct {
	FormatVersion int    `json:"formatVersion"`
	LibraryID     string `json:"libraryId"`
}

type Library struct {
	Root string
	DB   *sql.DB

	lockFile *os.File
}
