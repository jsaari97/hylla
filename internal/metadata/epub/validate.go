package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

const (
	MaxEntries        = 10000
	MaxEntryBytes     = 64 << 20
	MaxArchiveBytes   = 512 << 20
	MaxXMLBytes       = 2 << 20
	MaxDirectoryBytes = 16 << 20
)

// Validate reads ZIP entries directly. It never extracts archive paths.
func Validate(ctx context.Context, file io.ReaderAt, size int64) error {
	if err := checkDirectory(ctx, file, size); err != nil {
		return err
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return fmt.Errorf("open EPUB archive: %w", err)
	}
	if len(archive.File) == 0 || len(archive.File) > MaxEntries {
		return fmt.Errorf("EPUB must contain between 1 and %d entries", MaxEntries)
	}
	entries := make(map[string]*zip.File, len(archive.File))
	var total uint64
	for _, entry := range archive.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.Contains(name, `\`) {
			return fmt.Errorf("invalid EPUB entry path")
		}
		if _, exists := entries[entry.Name]; exists {
			return fmt.Errorf("duplicate EPUB entry %q", entry.Name)
		}
		entries[entry.Name] = entry
		if entry.UncompressedSize64 > MaxEntryBytes {
			return fmt.Errorf("EPUB entry exceeds %d bytes", MaxEntryBytes)
		}
		total += entry.UncompressedSize64
		if total > MaxArchiveBytes {
			return fmt.Errorf("EPUB exceeds %d uncompressed bytes", MaxArchiveBytes)
		}
		if err := checkEntry(ctx, entry); err != nil {
			return fmt.Errorf("read EPUB entry %q: %w", entry.Name, err)
		}
	}
	first := archive.File[0]
	if first.Name != "mimetype" || first.Method != zip.Store {
		return fmt.Errorf("EPUB must start with an uncompressed mimetype entry")
	}
	mimetype, err := readEntry(entries["mimetype"], 64)
	if err != nil || string(mimetype) != "application/epub+zip" {
		return fmt.Errorf("invalid EPUB mimetype")
	}
	var container struct {
		XMLName   xml.Name `xml:"urn:oasis:names:tc:opendocument:xmlns:container container"`
		Rootfiles []struct {
			Path      string `xml:"full-path,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := decodeXML(entries["META-INF/container.xml"], &container); err != nil {
		return fmt.Errorf("read EPUB container: %w", err)
	}
	packagePath := ""
	for _, root := range container.Rootfiles {
		if root.MediaType == "application/oebps-package+xml" {
			packagePath = root.Path
			break
		}
	}
	var pkg struct {
		XMLName xml.Name `xml:"http://www.idpf.org/2007/opf package"`
		Items   []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"manifest>item"`
		Spine []struct {
			ID string `xml:"idref,attr"`
		} `xml:"spine>itemref"`
	}
	if err := decodeXML(entries[packagePath], &pkg); err != nil {
		return fmt.Errorf("read EPUB package: %w", err)
	}
	if len(pkg.Spine) == 0 {
		return fmt.Errorf("EPUB has no reading order")
	}
	items := make(map[string]string)
	for _, item := range pkg.Items {
		items[item.ID] = item.Href
	}
	for _, item := range pkg.Spine {
		href, err := url.Parse(items[item.ID])
		if err != nil || href.IsAbs() || href.Host != "" || href.Path == "" || entries[path.Join(path.Dir(packagePath), href.Path)] == nil {
			return fmt.Errorf("EPUB reading order refers to missing content")
		}
	}
	return ctx.Err()
}

// Check the central directory before archive/zip allocates a model for every
// entry. Do not trust the entry count declared in the end record.
func checkDirectory(ctx context.Context, file io.ReaderAt, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if size < 22 {
		return fmt.Errorf("EPUB archive is too short")
	}
	tail := make([]byte, min(size, 65535+22))
	if _, err := file.ReadAt(tail, size-int64(len(tail))); err != nil {
		return err
	}
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if bytes.Equal(tail[i:i+4], []byte{'P', 'K', 5, 6}) && i+22+int(binary.LittleEndian.Uint16(tail[i+20:i+22])) == len(tail) {
			end = i
			break
		}
	}
	if end < 0 {
		return fmt.Errorf("EPUB archive end record is missing")
	}
	record := tail[end:]
	declared := binary.LittleEndian.Uint16(record[10:12])
	if declared == 0 || declared > MaxEntries {
		return fmt.Errorf("EPUB entry count exceeds limit or is empty")
	}
	if binary.LittleEndian.Uint16(record[4:6]) != 0 || binary.LittleEndian.Uint16(record[6:8]) != 0 {
		return fmt.Errorf("multi-disk EPUB archives are not supported")
	}
	directorySize := int64(binary.LittleEndian.Uint32(record[12:16]))
	position := int64(binary.LittleEndian.Uint32(record[16:20]))
	if directorySize > MaxDirectoryBytes || position > size-directorySize {
		return fmt.Errorf("EPUB archive directory exceeds limit or is outside the file")
	}
	directoryEnd := position + directorySize
	count := 0
	var header [46]byte
	for position < directoryEnd {
		if err := ctx.Err(); err != nil {
			return err
		}
		if directoryEnd-position < int64(len(header)) {
			return fmt.Errorf("invalid EPUB archive directory")
		}
		if _, err := file.ReadAt(header[:], position); err != nil {
			return err
		}
		if !bytes.Equal(header[:4], []byte{'P', 'K', 1, 2}) {
			return fmt.Errorf("invalid EPUB archive entry")
		}
		count++
		if count > MaxEntries {
			return fmt.Errorf("EPUB entry count exceeds limit")
		}
		position += 46 + int64(binary.LittleEndian.Uint16(header[28:30])) + int64(binary.LittleEndian.Uint16(header[30:32])) + int64(binary.LittleEndian.Uint16(header[32:34]))
		if position > directoryEnd {
			return fmt.Errorf("EPUB archive entry exceeds directory bounds")
		}
	}
	if count != int(declared) {
		return fmt.Errorf("EPUB archive entry count is inconsistent")
	}
	return nil
}

func checkEntry(ctx context.Context, entry *zip.File) error {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, MaxEntryBytes+1)
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := limited.Read(buffer)
		total += int64(n)
		if total > MaxEntryBytes {
			return fmt.Errorf("entry exceeds read limit")
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func readEntry(entry *zip.File, limit int64) ([]byte, error) {
	if entry == nil {
		return nil, fmt.Errorf("required entry is missing")
	}
	if entry.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("entry exceeds read limit")
	}
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("entry exceeds read limit")
	}
	return data, nil
}

func decodeXML(entry *zip.File, target any) error {
	data, err := readEntry(entry, MaxXMLBytes)
	if err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch token.(type) {
		case xml.StartElement:
			depth++
			if depth > 128 {
				return fmt.Errorf("XML nesting exceeds limit")
			}
		case xml.EndElement:
			depth--
		}
	}
	return xml.Unmarshal(data, target)
}
