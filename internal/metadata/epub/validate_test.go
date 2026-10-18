package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type fixtureEntry struct{ name, content string }

func validEntries() []fixtureEntry {
	return []fixtureEntry{
		{"mimetype", "application/epub+zip"},
		{"META-INF/container.xml", `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`},
		{"content.opf", `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="c" href="chapter.xhtml"/></manifest><spine><itemref idref="c"/></spine></package>`},
		{"chapter.xhtml", `<html><body>Example</body></html>`},
	}
}
func makeArchive(t *testing.T, entries []fixtureEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, entry := range entries {
		file, err := writer.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(file, entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func([]fixtureEntry) []fixtureEntry
		valid  bool
	}{
		{"valid without metadata", func(entries []fixtureEntry) []fixtureEntry { return entries }, true},
		{"wrong mimetype", func(entries []fixtureEntry) []fixtureEntry { entries[0].content = "application/zip"; return entries }, false},
		{"missing container", func(entries []fixtureEntry) []fixtureEntry { return append(entries[:1], entries[2:]...) }, false},
		{"missing chapter", func(entries []fixtureEntry) []fixtureEntry { return entries[:3] }, false},
		{"invalid package XML", func(entries []fixtureEntry) []fixtureEntry { entries[2].content = "<package>"; return entries }, false},
		{"XML depth limit", func(entries []fixtureEntry) []fixtureEntry {
			entries[2].content = strings.Repeat("<a>", 129) + strings.Repeat("</a>", 129)
			return entries
		}, false},
		{"XML size limit", func(entries []fixtureEntry) []fixtureEntry {
			entries[1].content = strings.Repeat("x", MaxXMLBytes+1)
			return entries
		}, false},
		{"path traversal", func(entries []fixtureEntry) []fixtureEntry {
			return append(entries, fixtureEntry{"../outside", "text"})
		}, false},
		{"duplicate entry", func(entries []fixtureEntry) []fixtureEntry { return append(entries, entries[0]) }, false},
		{"entry count limit", func(entries []fixtureEntry) []fixtureEntry {
			for len(entries) <= MaxEntries {
				entries = append(entries, fixtureEntry{strings.Repeat("x", 1) + string(rune(len(entries)+100)), ""})
			}
			return entries
		}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := makeArchive(t, test.change(validEntries()))
			err := Validate(context.Background(), bytes.NewReader(data), int64(len(data)))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}

func TestRejectsOversizedEntryBeforeDecompression(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	_, err := writer.CreateRaw(&zip.FileHeader{Name: "huge", Method: zip.Store, UncompressedSize64: MaxEntryBytes + 1, CompressedSize64: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	err = Validate(context.Background(), bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("entry size limit not applied: %v", err)
	}
}

func TestRejectsCorruptEntry(t *testing.T) {
	data := makeArchive(t, validEntries())
	index := bytes.Index(data, []byte("Example"))
	if index < 0 {
		t.Fatal("fixture content missing")
	}
	data[index] = 'X'
	if err := Validate(context.Background(), bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("corrupt archive accepted")
	}
}

func TestValidationCancellation(t *testing.T) {
	data := makeArchive(t, validEntries())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Validate(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestRejectsFalseArchiveEntryCount(t *testing.T) {
	data := makeArchive(t, validEntries())
	end := bytes.LastIndex(data, []byte{'P', 'K', 5, 6})
	if end < 0 {
		t.Fatal("missing end record")
	}
	data[end+8], data[end+9], data[end+10], data[end+11] = 1, 0, 1, 0
	if err := Validate(context.Background(), bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("inconsistent archive count accepted")
	}
}
