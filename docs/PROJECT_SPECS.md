# Hylla — Simplified Project Spec

## Goal

Build **Hylla**, a small, local-first desktop ebook manager as a lightweight alternative to Calibre.

The app should focus on the core workflow:

1. Import EPUB and PDF books into a managed local library.
2. Browse and search books.
3. Track reading status.
4. Send books to Kobo over USB.
5. Later, explore local Wi-Fi transfer.

No accounts, cloud backend, plugins, conversion engine, or multi-user features.

## Stack

- **Go** — core application logic
- **Wails** — desktop shell and Go/frontend bridge
- **Svelte + TypeScript** — UI
- **SQLite** — local metadata and application state

Primary target: **macOS**

Linux and Windows support can come later.

## Distribution

The initial build is for personal use on macOS.

Hylla will use direct local builds. It will not target the Mac App Store.

Do not add Mac App Store sandbox support, App Store packaging, an installer, or automatic updates to the MVP.

Use native file and directory pickers for user-selected files and library locations.

Code signing and notarization can be added later if the developer gets an Apple Developer account. These changes must not affect the library format or core application design.

## Principles

### Keep it local

Everything related to the ebook library lives on the user's computer.

No hosted backend or account is required.

### Keep it simple

Avoid recreating Calibre's complexity.

Prefer:

- flat storage
- explicit application logic
- few dependencies
- small Go packages
- simple UI
- boring solutions

### Manage imported files

Imported books are copied into Hylla's managed library.

Hylla must not depend on the original source file after import.

The source file remains untouched.

### Keep the library portable

The whole library should be movable or backupable as one directory.

A user should be able to copy the library to another computer, point Hylla at it, and continue using it.

## Supported Formats

MVP formats:

- EPUB
- PDF

Both should be treated as first-class library items.

Hylla should not convert between formats in the MVP.

## Library Location

On first launch, Hylla asks where the library should live.

Suggested default:

```text
~/Hylla Library/
```

The user can choose another location.

Examples:

- another local directory
- directly attached external drive

Hylla must not assume that the library always resides in `~/Documents`.

The library must use a local filesystem that supports SQLite file locking.

Hylla does not support these library locations in the MVP:

- network filesystems
- synchronized directories while synchronization is active
- one library that is open on two computers

The user can copy or back up the complete library while Hylla is closed.

## Library Structure

Keep the managed library intentionally simple:

```text
Hylla/
├── library.json
├── library.lock
├── books/
│   ├── <book-id>.epub
│   └── <book-id>.pdf
├── covers/
│   └── <book-id>.jpg
├── tmp/
│   ├── imports/
│   ├── deletions/
│   └── recovery/
├── backups/
└── library.sqlite
```

Do not create author/title directory hierarchies.

The library directory should contain everything needed to restore or migrate the user's ebook library.

`library.json` identifies the directory as a Hylla library. It contains:

```json
{
  "formatVersion": 1,
  "libraryId": "<uuid>"
}
```

The library ID must not change. Hylla must not open the library for writing if `formatVersion` is newer than the version that Hylla supports.

Hylla can create a library only in a new or empty directory. It must not initialize an arbitrary nonempty directory.

Hylla must hold an operating-system lock on `library.lock` while it has a library open for writing. The presence of the file does not mean that the lock is active. A second Hylla process must not write to the same library.

## Filenames

Use the internal book ID as the managed file name:

```text
books/<book-id>.<format>
```

Store the original base file name in SQLite and show it in the UI.

Do not use user metadata in a managed file name. Metadata edits must not rename managed files.

Hylla can create a readable, safe file name when it sends a book to another device.

## Application Data

Machine-specific Hylla configuration should live outside the ebook library.

### macOS

```text
~/Library/Application Support/Hylla/
```

### Linux

```text
~/.config/hylla/
~/.cache/hylla/
```

### Windows

```text
%APPDATA%\Hylla\
```

Application data may include:

```text
config.json
logs/
temporary/cache data
```

For example:

```json
{
  "libraryPath": "/Users/example/Hylla Library"
}
```

Do not store canonical book files or library metadata in the platform application-data directory.

Rule:

> If it belongs to the user's ebook library, store it inside the selected library directory. If it is specific to the local Hylla installation, store it in the operating system's application-data location.

## MVP Features

### Library setup

On first launch:

- suggest `~/Hylla Library` as the default location
- allow another directory to be selected
- create the required directories
- initialize SQLite
- write `library.json` last, with an atomic rename
- remember the selected library path

On later launches:

- open the previous library automatically
- handle a missing or moved library gracefully
- allow the user to select an existing Hylla library
- reject a directory that is not a valid Hylla library
- report a read-only or disconnected library clearly

### Import

Allow importing through:

- file picker
- drag and drop

Supported extensions:

```text
.epub
.pdf
```

Import flow:

1. Generate an internal book ID.
2. Copy the source to a unique file in `tmp/imports/`.
3. Calculate the SHA-256 hash during the copy.
4. Validate the file contents and format.
5. Check whether the same hash is already present.
6. Extract available metadata from the temporary file.
7. Move the temporary file to `books/<book-id>.<format>`.
8. Extract a cover when the format handler supports it.
9. Add the metadata to SQLite in a transaction.

The managed copy becomes Hylla's canonical copy.

An import must not fail only because metadata or cover extraction fails. Use fallback metadata and a standard format cover.

If the hash already exists, remove the temporary file, stop the import, and show the existing book. Do not create a second database row.

If the database transaction fails, remove the new managed file and cover. Hylla must remove incomplete temporary files when it starts.

The import handler must protect the application from malformed files. Read EPUB entries directly from the archive. Do not extract the archive to the filesystem. Define and test limits for archive entry count and bytes read from an entry.

## Metadata

### EPUB

Extract where available:

- title
- author(s)
- description
- language
- publisher
- publication date
- ISBN / identifier
- series
- series number
- cover

### PDF

Extract where available:

- title
- author
- subject / description
- creation/publication date
- page count

PDF metadata may be poor or missing.

Fallbacks:

- filename as title
- no author value

Use a standard PDF cover in the first release. Add a first-page thumbnail later only if users need it.

Users should be able to edit imported metadata manually.

Metadata edits must not modify the managed EPUB or PDF in the MVP.

## Library Browsing

Provide a simple main view containing:

- cover
- title
- author
- format
- reading status

Support:

- search by title
- search by author
- sort by title, author, or import date
- filter by format or reading status
- selecting a book
- deleting a book

Use simple substring search. Do not add fuzzy search or full-text search in the MVP.

Deleting a book removes:

- managed book file
- extracted or generated cover
- database entry

Require confirmation before deletion.

Deletion must be recoverable if the operation stops before completion. Move files to `tmp/deletions/<book-id>/` before the database transaction. At the next start, restore or remove these files according to the database state.

Hylla must never delete a file outside the selected library.

## Reading Status

Reading state should be a first-class field rather than a custom boolean.

Supported statuses:

```go
type ReadingStatus string

const (
    StatusUnread  ReadingStatus = "unread"
    StatusReading ReadingStatus = "reading"
    StatusRead    ReadingStatus = "read"
)
```

Store:

```text
reading_status
```

The UI should expose simple filters:

```text
All
Unread
Reading
Read
```

Users should be able to change status quickly from the library and book detail view.

Do not implement page-level reading progress in the MVP.

## Book Detail View

Display:

- cover
- title
- author(s)
- description
- series
- publication metadata
- file format
- filename
- file size
- page count where available
- reading status

Actions:

- change reading status
- edit metadata
- reveal file in Finder/file manager
- delete book
- send/remove from Kobo once device support exists

## Core Book Model

The core application model should represent a generic book, not an EPUB-specific object.

Example:

```go
type BookFormat string

const (
    FormatEPUB BookFormat = "epub"
    FormatPDF  BookFormat = "pdf"
)

type Book struct {
    ID            string
    Title         string
    Authors       []Author
    Format        BookFormat
    ReadingStatus ReadingStatus

    OriginalFilename string
    FileHash         string

    // Additional shared metadata...
}
```

Format-specific behavior belongs in dedicated packages.

## SQLite Model

Keep the initial schema small.

```sql
CREATE TABLE books (
    id TEXT PRIMARY KEY,

    title TEXT NOT NULL,
    description TEXT,
    language TEXT,
    publisher TEXT,
    published_at TEXT,
    identifier TEXT,
    series TEXT,
    series_index REAL,

    format TEXT NOT NULL CHECK (format IN ('epub', 'pdf')),

    original_filename TEXT NOT NULL,
    file_hash TEXT NOT NULL UNIQUE,

    page_count INTEGER CHECK (page_count IS NULL OR page_count >= 0),

    reading_status TEXT NOT NULL DEFAULT 'unread'
        CHECK (reading_status IN ('unread', 'reading', 'read')),

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE book_authors (
    book_id TEXT NOT NULL,
    position INTEGER NOT NULL,
    name TEXT NOT NULL,

    PRIMARY KEY (book_id, position),
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);

CREATE INDEX books_title_idx ON books(title COLLATE NOCASE);
CREATE INDEX books_status_idx ON books(reading_status);
CREATE INDEX book_authors_name_idx ON book_authors(name COLLATE NOCASE);
```

Use migrations from the beginning.

Enable SQLite foreign keys on every database connection. Store all timestamps as UTC RFC 3339 values.

Use the SQLite `DELETE` journal mode. The MVP has one writer and does not need write-ahead logging files.

## Go Structure

Keep the backend intentionally small.

```text
/
├── internal/
│   ├── app/
│   ├── books/
│   ├── library/
│   ├── metadata/
│   │   ├── epub/
│   │   └── pdf/
│   └── storage/
├── frontend/
├── migrations/
└── main.go
```

### `books`

Core models and book-related operations.

### `metadata/epub`

EPUB parsing and metadata extraction.

### `metadata/pdf`

PDF metadata extraction.

### `library`

Managed library filesystem operations:

- creating/opening libraries
- copying books
- recovering incomplete operations
- deleting files
- cover storage

### `storage`

SQLite access and migrations.

Avoid adding repository/service/interface layers unless they solve a real problem.

Do not create packages for future features. Add `internal/kobo` when Kobo work starts. Add a general device abstraction only when a second device type requires it.

## Kobo USB Support

Implement after the basic library works.

Initial functionality:

- detect mounted Kobo devices
- show device name
- show available storage
- determine whether a library book exists on the device
- send EPUB files
- send PDF files
- remove transferred books

Hylla should transfer the original managed file without conversion.

Hylla must copy books only to a Hylla-owned directory on the Kobo. The transferred file name must contain the Hylla book ID. Hylla must remove only the file that matches that ID inside the Hylla-owned directory.

Hylla must not change the Kobo database in the first implementation.

In the UI:

```text
Kobo Libra H2O

Dune                  EPUB    On device ✓
Some Research Book    PDF     Send
```

The user should not need a separate complicated device mode.

## Wi-Fi Support

Not part of the MVP.

Future investigation:

- OPDS server
- mDNS / Bonjour discovery
- KOReader integration
- Calibre wireless-device protocol

The intended architecture remains:

```text
Desktop Hylla
      │
      │ local Wi-Fi
      ▼
     Kobo
```

No cloud intermediary.

## Non-Goals

Do not implement initially:

- EPUB ↔ PDF conversion
- MOBI/AZW support
- DRM handling
- Kindle support
- cloud sync
- Mac App Store distribution
- automatic updates
- user accounts
- plugins
- metadata scraping
- annotations
- page/chapter progress
- advanced collections
- custom metadata fields
- Calibre library import
- EPUB editing
- PDF editing
- OCR
- embedded EPUB reader
- embedded PDF reader

Hylla is initially a **library and device-management application**, not a reading application.

## Delivery Milestones

### Milestone 1: Complete local workflow

The first usable version must support this complete workflow:

1. Launch Hylla.
2. Select or create a library directory.
3. Import an EPUB or PDF.
4. Validate the file and copy it into the managed library.
5. Use the file name as the title and use a standard format cover.
6. Store the book in SQLite.
7. Display and search the book.
8. Change its reading status.
9. Edit its basic metadata.
10. Delete it safely.
11. Restart Hylla and retain all state.

### Milestone 2: EPUB metadata

Add best-effort EPUB metadata and cover extraction. A metadata error must not stop a valid import.

### Milestone 3: PDF metadata

Add best-effort PDF metadata and page count extraction. Keep the standard PDF cover.

Milestones 1 through 3 make the MVP. A PDF first-page thumbnail is not part of the MVP.

Do not start Kobo integration until all MVP milestones are reliable.

## Recovery and Integrity

Hylla must check the library after it gets the library lock and before it accepts user operations.

The check must:

- remove incomplete import files from `tmp/imports/`
- complete or reverse interrupted deletions according to the database state
- report a database row that has no managed book file
- move a managed book file and its matching cover to `tmp/recovery/` if it has no database row, and report it
- stop with a clear error if a migration fails

Create a backup in `backups/` before a schema migration. Keep the backup if the migration fails. Keep at least the most recent pre-migration backup.

Do not follow symbolic links during library recovery or deletion. Stop write operations if an external drive becomes unavailable.

## Acceptance Cases

Test at least these cases:

- a valid EPUB import
- a valid PDF import
- a file with a supported extension and invalid contents
- two imports of the same file
- two different files with the same original file name
- an interrupted import
- a canceled import
- an import that stops because the disk is full
- an interrupted deletion
- metadata and cover extraction failures
- an unavailable external drive
- a read-only library
- a moved library
- a second Hylla process that opens the same library
- file names with Unicode characters and long names
- an EPUB with too many entries or an entry that exceeds the read limit

## Development Priorities

This is also a Go learning project.

Prefer idiomatic Go:

- explicit error handling
- small packages
- standard library where practical
- `context.Context` for operations that may block
- table-driven tests
- clear ownership of filesystem/database operations
- minimal abstractions
- minimal dependencies

The codebase should remain small enough that a new contributor can quickly understand how importing, storage, metadata, and device transfer work.
