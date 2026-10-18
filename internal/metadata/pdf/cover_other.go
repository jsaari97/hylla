//go:build !darwin || !cgo

package pdf

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

const CoverSupported = true

// Keep the compiled module for the process lifetime. Close each instance to
// release its memory.
var pdfiumPool = sync.OnceValues(func() (pdfium.Pool, error) {
	config := wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
	var cache wazero.CompilationCache
	if dir, err := os.UserCacheDir(); err == nil {
		cache, err = wazero.NewCompilationCacheWithDir(filepath.Join(dir, "hylla", "pdfium"))
		if err == nil {
			config = config.WithCompilationCache(cache)
		}
	}
	pool, err := webassembly.Init(webassembly.Config{
		MaxTotal: 1,
		// An empty filesystem prevents PDFium from opening unrelated local files.
		FSConfig:      wazero.NewFSConfig(),
		RuntimeConfig: config,
	})
	if err != nil && cache != nil {
		cache.Close(context.Background())
	}
	return pool, err
})

func withPDFDocument(ctx context.Context, file *os.File, use func(pdfium.Pdfium, references.FPDF_DOCUMENT) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("PDF must be a regular file")
	}
	pool, err := pdfiumPool()
	if err != nil {
		return fmt.Errorf("initialize PDF renderer: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	instance, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return canceled
		}
		return err
	}
	defer instance.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	// SectionReader preserves the caller's file offset.
	document, err := instance.OpenDocument(&requests.OpenDocument{
		FileReader:     io.NewSectionReader(file, 0, info.Size()),
		FileReaderSize: info.Size(),
	})
	if err != nil {
		return err
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: document.Document})
	err = use(instance, document.Document)
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	return err
}

// Active rendering cannot be interrupted; cancellation is checked afterward.
func RenderCover(ctx context.Context, file *os.File) ([]byte, error) {
	var data []byte
	err := withPDFDocument(ctx, file, func(instance pdfium.Pdfium, document references.FPDF_DOCUMENT) error {
		result, err := instance.RenderToFile(&requests.RenderToFile{
			RenderPageInPixels: &requests.RenderPageInPixels{
				Page:  requests.Page{ByIndex: &requests.PageByIndex{Document: document, Index: 0}},
				Width: MaxCoverEdge, Height: MaxCoverEdge,
			},
			OutputFormat:  requests.RenderToFileOutputFormatJPG,
			OutputTarget:  requests.RenderToFileOutputTargetBytes,
			OutputQuality: 85,
			MaxFileSize:   MaxCoverBytes,
		})
		if err != nil {
			return err
		}
		if result.ImageBytes == nil {
			return fmt.Errorf("PDF renderer returned no cover")
		}
		data, err = versionCover(*result.ImageBytes)
		return err
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}
