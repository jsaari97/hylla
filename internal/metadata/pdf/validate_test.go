package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func fixture(version, section string) []byte {
	prefix := "%PDF-" + version + "\n1 0 obj\n<< /Type /Catalog >>\nendobj\n"
	return []byte(fmt.Sprintf("%s%s\nstartxref\n%d\n%%%%EOF\n", prefix, section, len(prefix)))
}

func TestValidate(t *testing.T) {
	table := "xref\n0 2\n0000000000 65535 f \n0000000009 00000 n \ntrailer\n<< /Size 2 /Root 1 0 R >>"
	stream := "2 0 obj\n<< /Type /XRef /Size 2 /W [1 2 1] /Length 4 >>\nstream\nxxxx\nendstream\nendobj"
	longIndex := "/Index [" + strings.Repeat("1 1 ", 1200) + "] "
	longStream := strings.Replace(stream, "/Type", longIndex+"/Type", 1)
	valid := fixture("1.7", table)
	for _, test := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"table", valid, true},
		{"CR line endings", fixture("1.7", strings.ReplaceAll(table, "\n", "\r")), true},
		{"PDF 2.0", fixture("2.0", table), true},
		{"xref stream", fixture("1.7", stream), true},
		{"xref stream with long Index before Type", fixture("1.7", longStream), true},
		{"xref stream with long Index after Type", fixture("1.7", strings.Replace(stream, "/Size", longIndex+"/Size", 1)), true},
		{"long stream with wrong type", fixture("1.7", strings.Replace(longStream, "/XRef", "/Other", 1)), false},
		{"xref header exceeds limit", fixture("1.7", strings.Replace(stream, "/Size", "/Index ["+strings.Repeat("1 1 ", maxXRefHeaderBytes/4)+"] /Size", 1)), false},
		{"empty", nil, false},
		{"header only", []byte("%PDF-1.7\nthis is not a PDF\n%%EOF"), false},
		{"truncated", valid[:len(valid)-8], false},
		{"bad xref", fixture("1.7", "random text"), false},
		{"empty xref", fixture("1.7", "xref\n0 0\n"), false},
		{"outside offset", []byte(strings.Replace(string(valid), fmt.Sprintf("startxref\n%d", bytes.Index(valid, []byte("xref"))), "startxref\n99999999", 1)), false},
		{"unsupported header", fixture("9.9", table), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(bytes.NewReader(test.data), int64(len(test.data)))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}
