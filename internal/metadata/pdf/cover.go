package pdf

import (
	"bytes"
	"fmt"
)

const (
	MaxCoverBytes = 12 << 20
	MaxCoverEdge  = 1600
)

// A JPEG comment identifies the renderer version without a database change or
// extra cache files. Change this value when the rendered output changes.
const coverVersion = "Hylla PDF cover 1"

var coverSignature = append([]byte{0xff, 0xd8, 0xff, 0xfe, 0, byte(len(coverVersion) + 2)}, []byte(coverVersion)...)

func IsCurrentCover(data []byte) bool {
	return bytes.HasPrefix(data, coverSignature)
}

func versionCover(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, fmt.Errorf("PDF renderer did not return a JPEG")
	}
	if len(data)+len(coverSignature)-2 > MaxCoverBytes {
		return nil, fmt.Errorf("PDF cover exceeds size limit")
	}
	result := make([]byte, 0, len(data)+len(coverSignature)-2)
	result = append(result, coverSignature...)
	return append(result, data[2:]...), nil
}
