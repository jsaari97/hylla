package pdf

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

var header = regexp.MustCompile(`^%PDF-(1\.[0-7]|2\.0)[\r\n]`)
var xrefStream = regexp.MustCompile(`(?s)^\d+\s+\d+\s+obj\s*<<.*?/Type\s*/XRef\b`)
var xrefTable = regexp.MustCompile(`^xref[\t ]*[\r\n]+(\d+)[\t ]+(\d+)[\t ]*[\r\n]+\d{10} \d{5} [nf](?:[ \r\n]|$)`)

// Cross-reference stream dictionaries can contain large /Index arrays.
const maxXRefHeaderBytes = 64 << 10

// Validate checks PDF container structure, not page contents or conformance.
func Validate(file io.ReaderAt, size int64) error {
	if size < 32 {
		return fmt.Errorf("PDF is too short")
	}
	first := make([]byte, 9)
	if _, err := file.ReadAt(first, 0); err != nil || !header.Match(first) {
		return fmt.Errorf("invalid PDF header")
	}
	tail := make([]byte, min(size, 4096))
	if _, err := file.ReadAt(tail, size-int64(len(tail))); err != nil {
		return fmt.Errorf("read PDF trailer: %w", err)
	}
	tail = bytes.TrimSpace(tail)
	if !bytes.HasSuffix(tail, []byte("%%EOF")) {
		return fmt.Errorf("PDF has no final EOF marker")
	}
	start := bytes.LastIndex(tail, []byte("startxref"))
	if start < 0 {
		return fmt.Errorf("PDF has no final cross-reference offset")
	}
	fields := bytes.Fields(tail[start+len("startxref"):])
	if len(fields) != 2 || string(fields[1]) != "%%EOF" {
		return fmt.Errorf("invalid PDF cross-reference offset")
	}
	offset, err := strconv.ParseInt(string(fields[0]), 10, 64)
	if err != nil || offset < 9 || offset >= size {
		return fmt.Errorf("PDF cross-reference offset is outside the file")
	}
	section := make([]byte, min(size-offset, maxXRefHeaderBytes))
	if _, err := file.ReadAt(section, offset); err != nil {
		return fmt.Errorf("read PDF cross-reference section: %w", err)
	}
	if bytes.HasPrefix(section, []byte("xref")) {
		match := xrefTable.FindSubmatch(section)
		if match == nil {
			return fmt.Errorf("invalid PDF cross-reference table")
		}
		_, firstErr := strconv.ParseInt(string(match[1]), 10, 64)
		count, countErr := strconv.ParseInt(string(match[2]), 10, 64)
		if firstErr != nil || countErr != nil || count < 1 {
			return fmt.Errorf("invalid PDF cross-reference entries")
		}
		return nil
	}
	// In PDF 1.5 and later, the cross-reference data can be a stream.
	if stream := bytes.Index(section, []byte("stream")); stream >= 0 && xrefStream.Match(section[:stream]) {
		return nil
	}
	return fmt.Errorf("PDF cross-reference section is missing")
}
