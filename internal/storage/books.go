package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jsaari97/hylla/internal/books"
)

const bookColumns = `id, title, format, original_filename, file_hash,
	reading_status, created_at, updated_at`

func InsertBook(ctx context.Context, db *sql.DB, book books.Book) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO books (`+bookColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ID, book.Title, book.Format, book.OriginalFilename, book.FileHash,
		book.ReadingStatus, book.CreatedAt.Format(time.RFC3339Nano), book.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert book: %w", err)
	}
	if err := insertAuthors(ctx, tx, book.ID, book.Authors); err != nil {
		return err
	}
	return tx.Commit()
}

func BookByHash(ctx context.Context, db *sql.DB, hash string) (books.Book, error) {
	return readBook(ctx, db, `SELECT `+bookColumns+` FROM books WHERE file_hash = ?`, hash)
}

func BookByID(ctx context.Context, db *sql.DB, id string) (books.Book, error) {
	return readBook(ctx, db, `SELECT `+bookColumns+` FROM books WHERE id = ?`, id)
}

func SetReadingStatus(ctx context.Context, db *sql.DB, id string, status books.ReadingStatus) (books.Book, error) {
	switch status {
	case books.StatusUnread, books.StatusReading, books.StatusRead:
	default:
		return books.Book{}, fmt.Errorf("invalid reading status %q", status)
	}
	result, err := db.ExecContext(ctx, `UPDATE books SET reading_status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return books.Book{}, fmt.Errorf("save reading status: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return books.Book{}, err
	}
	if count == 0 {
		return books.Book{}, fmt.Errorf("book not found: %w", sql.ErrNoRows)
	}
	return BookByID(ctx, db, id)
}

func readBook(ctx context.Context, db *sql.DB, query, value string) (books.Book, error) {
	book, err := scanBook(db.QueryRowContext(ctx, query, value))
	if err != nil {
		return books.Book{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM book_authors WHERE book_id = ? ORDER BY position`, book.ID)
	if err != nil {
		return books.Book{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return books.Book{}, err
		}
		book.Authors = append(book.Authors, books.Author{Name: name})
	}
	return book, rows.Err()
}

func ListBooks(ctx context.Context, db *sql.DB) ([]books.Book, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+bookColumns+` FROM books ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	result := make([]books.Book, 0)
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, book)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	authors, err := db.QueryContext(ctx, `SELECT book_id, name FROM book_authors ORDER BY book_id, position`)
	if err != nil {
		return nil, err
	}
	defer authors.Close()
	indices := make(map[string]int, len(result))
	for index, book := range result {
		indices[book.ID] = index
	}
	for authors.Next() {
		var id, name string
		if err := authors.Scan(&id, &name); err != nil {
			return nil, err
		}
		if index, ok := indices[id]; ok {
			result[index].Authors = append(result[index].Authors, books.Author{Name: name})
		}
	}
	return result, authors.Err()
}

// The SQL guard preserves edits made after metadata was read.
func FillBookMetadata(ctx context.Context, db *sql.DB, id, fallbackTitle string, metadata books.Metadata, updated time.Time) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	title := metadata.Title
	if title == "" {
		title = fallbackTitle
	}
	result, err := tx.ExecContext(ctx, `UPDATE books SET title = ?, updated_at = ?
		WHERE id = ? AND title = ? AND created_at = updated_at
		AND NOT EXISTS (SELECT 1 FROM book_authors WHERE book_id = books.id)`, title, updated.Format(time.RFC3339Nano), id, fallbackTitle)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return false, err
	}
	if err := insertAuthors(ctx, tx, id, metadata.Authors); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func insertAuthors(ctx context.Context, tx *sql.Tx, id string, authors []books.Author) error {
	for position, author := range authors {
		if _, err := tx.ExecContext(ctx, `INSERT INTO book_authors (book_id, position, name) VALUES (?, ?, ?)`, id, position, author.Name); err != nil {
			return fmt.Errorf("save book author: %w", err)
		}
	}
	return nil
}

func scanBook(row interface{ Scan(...any) error }) (books.Book, error) {
	var book books.Book
	var created, updated string
	err := row.Scan(&book.ID, &book.Title, &book.Format, &book.OriginalFilename,
		&book.FileHash, &book.ReadingStatus, &created, &updated)
	if err != nil {
		return books.Book{}, err
	}
	book.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return books.Book{}, fmt.Errorf("read book creation date: %w", err)
	}
	book.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return books.Book{}, fmt.Errorf("read book update date: %w", err)
	}
	book.Authors = []books.Author{}
	return book, nil
}
