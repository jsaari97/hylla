# Hylla

Hylla is a desktop application for a personal book library. It keeps books,
covers, and metadata in one local directory. The primary platform is macOS.

## Current features

- Create or open a library.
- Open the previous library automatically after a restart.
- Import EPUB and PDF books through a file dialog or file drop.
- Read embedded titles and authors, generate covers, and detect duplicate files.
- Search by title, author, or file name, and view book details.
- Change reading status to Unread, Reading, or Read.
- Filter by format or reading status, and sort by title, author, or date added.
- Switch between a cover grid and a list with small cover thumbnails.

Imports keep the source files unchanged. Missing metadata or covers do not stop
an import. Interrupted imports are checked when the library opens.

Hylla is in early development. Metadata editing and book deletion are not
available yet. If the previous library is unavailable,
select its new location or create another library.

## Development

Built with Go, Svelte, and Wails v3.

| Task | Command |
| --- | --- |
| Run with live reload | `wails3 dev` |
| Build | `wails3 build` |
| Run Go tests | `go test ./...` |

See the [project specification](docs/PROJECT_SPECS.md) for design and planned features.
