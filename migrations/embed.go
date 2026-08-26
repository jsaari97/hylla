package migrations

import "embed"

// Files contains the database migration files.
//
//go:embed *.sql
var Files embed.FS
