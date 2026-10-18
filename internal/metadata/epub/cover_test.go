package epub

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func coverImage(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 40; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 30, G: 100, B: 200, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestExtractCoverReferences(t *testing.T) {
	for _, test := range []struct{ name, metadata, properties, guide, wrapper, href string }{
		{name: "EPUB 3", properties: "cover-image"},
		{name: "EPUB 3 property list", properties: "scripted cover-image"},
		{name: "EPUB 2", metadata: `<meta name="cover" content="cover"/>`},
		{name: "guide XHTML", guide: `<reference type="cover" href="cover.xhtml"/>`, wrapper: `<html><body><img src="images/front%20cover.png"/></body></html>`},
		{name: "guide SVG wrapper", guide: `<reference type="cover" href="cover.xhtml"/>`, wrapper: `<html xmlns:xlink="http://www.w3.org/1999/xlink"><svg><image xlink:href="images/front%20cover.png"/></svg></html>`},
		{name: "cover page from metadata", metadata: `<meta name="cover" content="cover"/>`, href: "cover.xhtml", wrapper: `<html><body><img src="images/front%20cover.png"/></body></html>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := validEntries()
			href := test.href
			if href == "" {
				href = "images/front%20cover.png"
			}
			entries[2].content = `<package xmlns="http://www.idpf.org/2007/opf"><metadata>` + test.metadata + `</metadata><manifest><item id="cover" href="` + href + `" properties="` + test.properties + `"/></manifest><guide>` + test.guide + `</guide></package>`
			entries = append(entries, fixtureEntry{"images/front cover.png", string(coverImage(t))})
			if test.wrapper != "" {
				entries = append(entries, fixtureEntry{"cover.xhtml", test.wrapper})
			}
			data := makeArchive(t, entries)
			cover, err := ExtractCover(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil || len(cover) == 0 {
				t.Fatalf("cover missing: %v", err)
			}
			config, err := jpeg.DecodeConfig(bytes.NewReader(cover))
			if err != nil || config.Width != 40 || config.Height != 60 {
				t.Fatalf("invalid JPEG cover: %+v, %v", config, err)
			}
		})
	}
}

func TestCoverFallback(t *testing.T) {
	for _, test := range []struct{ name, href, data string }{
		{"no cover", "", ""},
		{"missing image", "missing.png", ""},
		{"invalid image", "cover.png", "not an image"},
		{"remote cover", "https://example.com/cover.png", ""},
		{"outside archive", "../cover.png", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := validEntries()
			if test.href != "" {
				entries[2].content = strings.Replace(entries[2].content, "</manifest>", `<item id="cover" href="`+test.href+`" properties="cover-image"/></manifest>`, 1)
			}
			if test.data != "" {
				entries = append(entries, fixtureEntry{"cover.png", test.data})
			}
			data := makeArchive(t, entries)
			cover, err := ExtractCover(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil || len(cover) != 0 {
				t.Fatalf("expected no cover: %d bytes, %v", len(cover), err)
			}
		})
	}
}

func TestCoverPixelLimit(t *testing.T) {
	data := coverImage(t)
	binary.BigEndian.PutUint32(data[16:20], 100000)
	binary.BigEndian.PutUint32(data[20:24], 100000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := coverJPEG(context.Background(), data); err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("large image accepted: %v", err)
	}
}

func TestCoverResize(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 10, MaxCoverEdge+100))); err != nil {
		t.Fatal(err)
	}
	cover, err := coverJPEG(context.Background(), data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(cover))
	if err != nil || config.Height != MaxCoverEdge || config.Width < 1 {
		t.Fatalf("cover was not resized: %+v, %v", config, err)
	}
}

func TestCoverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := makeArchive(t, validEntries())
	if _, err := ExtractCover(ctx, bytes.NewReader(data), int64(len(data))); err != context.Canceled {
		t.Fatalf("cancellation lost: %v", err)
	}
}
