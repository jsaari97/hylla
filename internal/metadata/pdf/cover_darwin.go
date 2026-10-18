//go:build darwin && cgo

package pdf

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ImageIO -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <ImageIO/ImageIO.h>
#include <math.h>
#include <stdlib.h>
#include <unistd.h>
#include <errno.h>

#include "document_darwin.h"

static CFDataRef renderPDFCover(int sourceFD, off_t size, int maxEdge) {
    CGPDFDocumentRef document = openPDFDocument(sourceFD, size);
    if (!document) return NULL;
    CFMutableDataRef data = NULL;
    CGContextRef bitmap = NULL;
    CGImageRef image = NULL;
    CGImageDestinationRef destination = NULL;
    CGColorSpaceRef colors = NULL;

    if (!CGPDFDocumentIsUnlocked(document)) goto done;
    CGPDFPageRef page = CGPDFDocumentGetPage(document, 1);
    if (!page) goto done;
    CGRect box = CGRectIntersection(CGPDFPageGetBoxRect(page, kCGPDFCropBox),
        CGPDFPageGetBoxRect(page, kCGPDFMediaBox));
    if (CGRectIsNull(box) || CGRectIsEmpty(box)) goto done;
    box = CGRectStandardize(box);
    double width = box.size.width, height = box.size.height;
    if (!isfinite(width) || !isfinite(height) || width <= 0 || height <= 0) goto done;
    int rotation = CGPDFPageGetRotationAngle(page) % 360;
    if (rotation < 0) rotation += 360;
    if (rotation == 90 || rotation == 270) {
        double swap = width; width = height; height = swap;
    }
    double scale = (double)maxEdge / fmax(width, height);
    if (!isfinite(scale) || scale <= 0) goto done;
    size_t pixelsWide = (size_t)fmax(1.0, ceil(fmin(maxEdge, width * scale)));
    size_t pixelsHigh = (size_t)fmax(1.0, ceil(fmin(maxEdge, height * scale)));
    if (pixelsWide > maxEdge || pixelsHigh > maxEdge) goto done;
    colors = CGColorSpaceCreateDeviceRGB();
    if (!colors) goto done;
    bitmap = CGBitmapContextCreate(NULL, pixelsWide, pixelsHigh, 8, pixelsWide * 4,
        colors, kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
    if (!bitmap) goto done;
    CGRect target = CGRectMake(0, 0, pixelsWide, pixelsHigh);
    CGContextSetRGBFillColor(bitmap, 1, 1, 1, 1);
    CGContextFillRect(bitmap, target);
    // The page transform only scales down. Apply the resolution scale
    // separately so vector pages also fill a higher-resolution thumbnail.
    CGContextScaleCTM(bitmap, scale, scale);
    CGRect pageTarget = CGRectMake(0, 0, pixelsWide / scale, pixelsHigh / scale);
    CGContextConcatCTM(bitmap, CGPDFPageGetDrawingTransform(page, kCGPDFCropBox, pageTarget, 0, true));
    CGContextSetInterpolationQuality(bitmap, kCGInterpolationHigh);
    CGContextDrawPDFPage(bitmap, page);
    image = CGBitmapContextCreateImage(bitmap);
    if (!image) goto done;
    data = CFDataCreateMutable(kCFAllocatorDefault, 0);
    if (!data) goto done;
    destination = CGImageDestinationCreateWithData(data, CFSTR("public.jpeg"), 1, NULL);
    if (!destination) { CFRelease(data); data = NULL; goto done; }
    CGImageDestinationAddImage(destination, image, NULL);
    if (!CGImageDestinationFinalize(destination)) { CFRelease(data); data = NULL; }

done:
    if (destination) CFRelease(destination);
    if (image) CGImageRelease(image);
    if (bitmap) CGContextRelease(bitmap);
    if (colors) CGColorSpaceRelease(colors);
    CGPDFDocumentRelease(document);
    return data;
}
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"unsafe"
)

const CoverSupported = true

// Native rendering cannot be interrupted; cancellation is checked afterward.
func RenderCover(ctx context.Context, file *os.File) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("PDF must be a regular file")
	}
	data := C.renderPDFCover(C.int(file.Fd()), C.off_t(info.Size()), C.int(MaxCoverEdge))
	runtime.KeepAlive(file)
	if data == 0 {
		return nil, fmt.Errorf("cannot render the first PDF page")
	}
	defer C.CFRelease(C.CFTypeRef(data))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	size := C.CFDataGetLength(data)
	if size <= 0 || size > MaxCoverBytes {
		return nil, fmt.Errorf("PDF cover exceeds size limit")
	}
	return versionCover(C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(size)))
}
