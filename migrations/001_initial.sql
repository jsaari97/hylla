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
