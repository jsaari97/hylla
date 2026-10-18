package epub

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExtractMetadata(t *testing.T) {
	for _, test := range []struct {
		name, metadata, title string
		authors               []string
	}{
		{"basic", `<dc:title> Better  Book </dc:title><dc:creator>Ada Writer</dc:creator>`, "Better Book", []string{"Ada Writer"}},
		{"multiple authors", `<dc:title>日本語の本</dc:title><dc:creator>Ada Writer</dc:creator><dc:creator>Other Writer</dc:creator><dc:creator>Ada Writer</dc:creator>`, "日本語の本", []string{"Ada Writer", "Other Writer"}},
		{"EPUB 2 roles", `<dc:title>Book</dc:title><dc:creator opf:role="edt">An Editor</dc:creator><dc:creator opf:role="aut">An Author</dc:creator>`, "Book", []string{"An Author"}},
		{"EPUB 3 roles", `<dc:title>Book</dc:title><dc:creator id="editor">An Editor</dc:creator><dc:creator id="author">An Author</dc:creator><meta refines="#editor" property="role">edt</meta><meta refines="#author" property="role">aut</meta>`, "Book", []string{"An Author"}},
		{"main title", `<dc:title id="subtitle">Subtitle</dc:title><dc:title id="main">Main title</dc:title><meta refines="#main" property="title-type">main</meta>`, "Main title", nil},
		{"preserve labels", `<dc:title>Book (for True Epub)</dc:title>`, "Book (for True Epub)", nil},
		{"missing", `<dc:publisher>Publisher</dc:publisher>`, "", nil},
		{"empty", `<dc:title> </dc:title><dc:creator> </dc:creator>`, "", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := validEntries()
			entries[2].content = strings.Replace(entries[2].content, "<manifest>", `<metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">`+test.metadata+`</metadata><manifest>`, 1)
			data := makeArchive(t, entries)
			metadata, err := ExtractMetadata(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil || metadata.Title != test.title || len(metadata.Authors) != len(test.authors) {
				t.Fatalf("metadata=%+v, error=%v", metadata, err)
			}
			for i, author := range metadata.Authors {
				if author.Name != test.authors[i] {
					t.Fatalf("author=%q, expected %q", author.Name, test.authors[i])
				}
			}
		})
	}
}

func TestMetadataInvalidAndCancelled(t *testing.T) {
	data := []byte("not an EPUB")
	if _, err := ExtractMetadata(context.Background(), bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("invalid archive accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExtractMetadata(ctx, bytes.NewReader(data), int64(len(data))); err != context.Canceled {
		t.Fatalf("cancellation lost: %v", err)
	}
}
