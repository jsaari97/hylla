//go:build darwin && cgo

package pdf

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include "document_darwin.h"

typedef struct { char *title; char *author; int valid; } BookMetadata;

static char *metadataText(CGPDFDictionaryRef info, const char *key) {
    CGPDFStringRef value = NULL;
    if (!info || !CGPDFDictionaryGetString(info, key, &value)) return NULL;
    CFStringRef text = CGPDFStringCopyTextString(value);
    if (!text) return NULL;
    CFIndex length = CFStringGetLength(text);
    if (length > 16384) { CFRelease(text); return NULL; }
    CFIndex capacity = CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1;
    char *buffer = malloc(capacity);
    if (buffer && !CFStringGetCString(text, buffer, capacity, kCFStringEncodingUTF8)) {
        free(buffer); buffer = NULL;
    }
    CFRelease(text);
    return buffer;
}
static BookMetadata readBookMetadata(int fd, off_t size) {
    BookMetadata result = {NULL, NULL, 0};
    CGPDFDocumentRef document = openPDFDocument(fd, size);
    if (!document) return result;
    if (CGPDFDocumentIsUnlocked(document)) {
        result.valid = 1;
        CGPDFDictionaryRef info = CGPDFDocumentGetInfo(document);
        result.title = metadataText(info, "Title");
        result.author = metadataText(info, "Author");
    }
    CGPDFDocumentRelease(document);
    return result;
}
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"

	"github.com/jsaari97/hylla/internal/books"
)

func ExtractMetadata(ctx context.Context, file *os.File) (books.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return books.Metadata{}, err
	}
	info, err := file.Stat()
	if err != nil {
		return books.Metadata{}, err
	}
	if !info.Mode().IsRegular() {
		return books.Metadata{}, fmt.Errorf("PDF must be a regular file")
	}
	data := C.readBookMetadata(C.int(file.Fd()), C.off_t(info.Size()))
	runtime.KeepAlive(file)
	defer C.free(unsafe.Pointer(data.title))
	defer C.free(unsafe.Pointer(data.author))
	if data.valid == 0 {
		return books.Metadata{}, fmt.Errorf("cannot read PDF metadata")
	}
	result := books.Metadata{Title: strings.Join(strings.Fields(C.GoString(data.title)), " ")}
	author := strings.Join(strings.Fields(C.GoString(data.author)), " ")
	// The PDF Author field is free text, not a list of names.
	if author != "" {
		result.Authors = []books.Author{{Name: author}}
	}
	return result, ctx.Err()
}
