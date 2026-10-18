package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func renderFixture(rotation int) []byte {
	return renderFixtureBox(rotation, "")
}

func renderFixtureBox(rotation int, crop string) []byte {
	var result bytes.Buffer
	result.WriteString("%PDF-1.7\n")
	first := "1 0 0 rg 0 150 200 150 re f\n"
	second := "0 0 1 rg 0 0 200 300 re f\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 /Resources << >> >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 300] %s /Rotate %d /Contents 4 0 R >>", crop, rotation),
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(first), first),
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 300] /Contents 6 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(second), second),
	}
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = result.Len()
		fmt.Fprintf(&result, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := result.Len()
	fmt.Fprintf(&result, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&result, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&result, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return result.Bytes()
}

func openFixture(t *testing.T, data []byte) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func TestRenderCoverFirstPageAndOrientation(t *testing.T) {
	original := renderFixture(0)
	file := openFixture(t, original)
	cover, err := RenderCover(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	image, err := jpeg.Decode(bytes.NewReader(cover))
	if err != nil {
		t.Fatal(err)
	}
	bounds := image.Bounds()
	if bounds.Dy() != MaxCoverEdge || bounds.Dx() > MaxCoverEdge || len(cover) > MaxCoverBytes {
		t.Fatalf("cover dimensions or size invalid: %v, %d", bounds, len(cover))
	}
	r, g, b, _ := image.At(bounds.Dx()/2, bounds.Dy()/4).RGBA()
	if r < 60000 || g > 3000 || b > 3000 {
		t.Fatalf("first page top should be red: %d %d %d", r, g, b)
	}
	r, g, b, _ = image.At(bounds.Dx()/2, 3*bounds.Dy()/4).RGBA()
	if r < 60000 || g < 60000 || b < 60000 {
		t.Fatalf("page bottom should be white: %d %d %d", r, g, b)
	}
	stored, err := os.ReadFile(file.Name())
	if err != nil || !bytes.Equal(original, stored) {
		t.Fatalf("PDF source changed: %v", err)
	}
}

func TestRenderCoverRotation(t *testing.T) {
	for _, rotation := range []int{90, 270} {
		t.Run(fmt.Sprint(rotation), func(t *testing.T) {
			cover, err := RenderCover(context.Background(), openFixture(t, renderFixture(rotation)))
			if err != nil {
				t.Fatal(err)
			}
			config, err := jpeg.DecodeConfig(bytes.NewReader(cover))
			if err != nil || config.Width != MaxCoverEdge || config.Height >= config.Width {
				t.Fatalf("rotation not applied: %+v, %v", config, err)
			}
		})
	}
}

func TestRenderCoverCropBox(t *testing.T) {
	cover, err := RenderCover(context.Background(), openFixture(t, renderFixtureBox(0, "/CropBox [0 150 200 300]")))
	if err != nil {
		t.Fatal(err)
	}
	image, err := jpeg.Decode(bytes.NewReader(cover))
	if err != nil {
		t.Fatal(err)
	}
	bounds := image.Bounds()
	if bounds.Dx() != MaxCoverEdge || bounds.Dy() != 1200 {
		t.Fatalf("crop dimensions incorrect: %v", bounds)
	}
	for _, y := range []int{bounds.Dy() / 4, 3 * bounds.Dy() / 4} {
		r, g, b, _ := image.At(bounds.Dx()/2, y).RGBA()
		if r < 60000 || g > 3000 || b > 3000 {
			t.Fatalf("crop included the white area: %d %d %d", r, g, b)
		}
	}
}

func TestRenderCoverInvalidAndCancelled(t *testing.T) {
	if _, err := RenderCover(context.Background(), openFixture(t, []byte("not a PDF"))); err == nil {
		t.Fatal("invalid PDF was rendered")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RenderCover(ctx, openFixture(t, renderFixture(0))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestPDFReadersPreserveOffsetAndRejectDirectory(t *testing.T) {
	file := openFixture(t, metadataFixture("A title", "An author"))
	for _, read := range []func(context.Context, *os.File) error{
		func(ctx context.Context, f *os.File) error { _, err := RenderCover(ctx, f); return err },
		func(ctx context.Context, f *os.File) error { _, err := ExtractMetadata(ctx, f); return err },
	} {
		if _, err := file.Seek(7, 0); err != nil {
			t.Fatal(err)
		}
		if err := read(context.Background(), file); err != nil {
			t.Fatal(err)
		}
		offset, err := file.Seek(0, 1)
		if err != nil || offset != 7 {
			t.Fatalf("offset=%d, error=%v", offset, err)
		}
		dir, err := os.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		err = read(context.Background(), dir)
		dir.Close()
		if err == nil {
			t.Fatal("directory accepted as PDF")
		}
	}
}
