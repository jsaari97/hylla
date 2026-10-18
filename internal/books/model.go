package books

import "time"

type BookFormat string

const (
	FormatEPUB BookFormat = "epub"
	FormatPDF  BookFormat = "pdf"
)

type ReadingStatus string

const (
	StatusUnread  ReadingStatus = "unread"
	StatusReading ReadingStatus = "reading"
	StatusRead    ReadingStatus = "read"
)

type Author struct {
	Name string
}

type Metadata struct {
	Title   string
	Authors []Author
}

type Book struct {
	ID          string
	Title       string
	Description string
	Authors     []Author

	Language    string
	Publisher   string
	PublishedAt *time.Time
	Identifier  string

	Series      string
	SeriesIndex *float64

	Format           BookFormat
	OriginalFilename string
	FileHash         string
	PageCount        *int
	ReadingStatus    ReadingStatus

	CreatedAt time.Time
	UpdatedAt time.Time
}
