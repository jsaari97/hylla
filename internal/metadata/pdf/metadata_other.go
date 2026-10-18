//go:build !darwin || !cgo

package pdf

import (
	"context"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/jsaari97/hylla/internal/books"
	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

func ExtractMetadata(ctx context.Context, file *os.File) (books.Metadata, error) {
	var metadata books.Metadata
	err := withPDFDocument(ctx, file, func(instance pdfium.Pdfium, document references.FPDF_DOCUMENT) error {
		for _, name := range []string{"Title", "Author"} {
			tag, err := instance.FPDF_GetMetaText(&requests.FPDF_GetMetaText{Document: document, Tag: name})
			// PDFium reports absent fields as errors. Read fields separately so
			// an absent title does not discard an available author, or vice versa.
			if err != nil {
				continue
			}
			if utf8.RuneCountInString(tag.Value) > 16384 {
				continue
			}
			value := strings.Join(strings.Fields(tag.Value), " ")
			switch tag.Tag {
			case "Title":
				metadata.Title = value
			case "Author":
				// The PDF Author field is free text, not a list of names.
				if value != "" {
					metadata.Authors = []books.Author{{Name: value}}
				}
			}
		}
		return nil
	})
	if err != nil {
		return books.Metadata{}, err
	}
	return metadata, nil
}
