package pdf

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"
)

func metadataFixture(title, author string) []byte {
	encode := func(value string) string {
		encoded := []byte{0xfe, 0xff}
		for _, unit := range utf16.Encode([]rune(value)) {
			encoded = append(encoded, byte(unit>>8), byte(unit))
		}
		return "<" + hex.EncodeToString(encoded) + ">"
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 300] >>",
		"<< /Title " + encode(title) + " /Author " + encode(author) + " >>",
	}
	var result bytes.Buffer
	result.WriteString("%PDF-1.7\n")
	offsets := []int{}
	for i, object := range objects {
		offsets = append(offsets, result.Len())
		fmt.Fprintf(&result, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := result.Len()
	fmt.Fprintf(&result, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&result, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&result, "trailer\n<< /Size %d /Root 1 0 R /Info 4 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return result.Bytes()
}

func TestExtractMetadata(t *testing.T) {
	for _, test := range []struct{ title, author string }{
		{"SQL Antipatterns, Volume 1", "Bill Karwin"},
		{"日本語の本", "Åsa Writer"},
		{"  A   book  ", "Last, First; Other Author"},
		{"", ""},
	} {
		metadata, err := ExtractMetadata(context.Background(), openFixture(t, metadataFixture(test.title, test.author)))
		if err != nil || metadata.Title != strings.Join(strings.Fields(test.title), " ") {
			t.Fatalf("metadata=%+v, error=%v", metadata, err)
		}
		if test.author != "" && (len(metadata.Authors) != 1 || metadata.Authors[0].Name != test.author) {
			t.Fatalf("author=%+v", metadata.Authors)
		}
		if test.author == "" && len(metadata.Authors) != 0 {
			t.Fatal("unexpected author")
		}
	}
}

func TestMetadataMissingInvalidAndCancelled(t *testing.T) {
	metadata, err := ExtractMetadata(context.Background(), openFixture(t, renderFixture(0)))
	if err != nil || metadata.Title != "" || len(metadata.Authors) != 0 {
		t.Fatalf("missing metadata failed: %+v, %v", metadata, err)
	}
	if _, err := ExtractMetadata(context.Background(), openFixture(t, []byte("not a PDF"))); err == nil {
		t.Fatal("invalid PDF accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExtractMetadata(ctx, openFixture(t, renderFixture(0))); err != context.Canceled {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestMetadataSingleField(t *testing.T) {
	for _, test := range []struct{ field, title, author string }{
		{"Title", "A title", ""},
		{"Author", "", "An author"},
	} {
		t.Run(test.field, func(t *testing.T) {
			// Keep offsets valid by replacing the unused key with an unknown key.
			data := metadataFixture(test.title, test.author)
			missing := "Title"
			if test.field == "Title" {
				missing = "Author"
			}
			data = bytes.Replace(data, []byte("/"+missing+" "), []byte("/"+strings.Repeat("X", len(missing))+" "), 1)
			metadata, err := ExtractMetadata(context.Background(), openFixture(t, data))
			if err != nil || metadata.Title != test.title {
				t.Fatalf("metadata=%+v, error=%v", metadata, err)
			}
			if test.author != "" && (len(metadata.Authors) != 1 || metadata.Authors[0].Name != test.author) {
				t.Fatalf("authors=%+v", metadata.Authors)
			}
			if test.author == "" && len(metadata.Authors) != 0 {
				t.Fatalf("unexpected authors=%+v", metadata.Authors)
			}
		})
	}
}
