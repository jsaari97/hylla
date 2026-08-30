package library

import "errors"

var ErrLibraryInUse = errors.New("library is already open")
