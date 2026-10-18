package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"path"
	"strings"
)

const (
	MaxCoverBytes      = 12 << 20
	MaxCoverPixels     = 20_000_000
	MaxCoverEdge       = 1600
	maxCoverReferences = 16
)

// ExtractCover supports EPUB 3 cover-image, EPUB 2 cover metadata, and guide
// cover pages. It reads local archive entries only and returns a safe JPEG.
// See https://www.w3.org/TR/epub-33/#sec-cover-image.
func ExtractCover(ctx context.Context, file io.ReaderAt, size int64) ([]byte, error) {
	if err := checkDirectory(ctx, file, size); err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]*zip.File, len(archive.File))
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	var container struct {
		Roots []struct {
			Path string `xml:"full-path,attr"`
			Type string `xml:"media-type,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := decodeXML(entries["META-INF/container.xml"], &container); err != nil {
		return nil, err
	}
	packagePath := ""
	for _, root := range container.Roots {
		if root.Type == "application/oebps-package+xml" {
			packagePath = root.Path
			break
		}
	}
	var pkg struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"manifest>item"`
		Meta []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"metadata>meta"`
		Guide []struct {
			Type string `xml:"type,attr"`
			Href string `xml:"href,attr"`
		} `xml:"guide>reference"`
	}
	if err := decodeXML(entries[packagePath], &pkg); err != nil {
		return nil, err
	}
	candidates := []string{}
	itemsByID := make(map[string]string, len(pkg.Items))
	for _, item := range pkg.Items {
		itemsByID[item.ID] = item.Href
		if hasWord(item.Properties, "cover-image") {
			candidates = append(candidates, item.Href)
		}
	}
	for _, meta := range pkg.Meta {
		if meta.Name != "cover" {
			continue
		}
		if href := itemsByID[meta.Content]; href != "" {
			candidates = append(candidates, href)
		}
	}
	for _, reference := range pkg.Guide {
		if hasWord(reference.Type, "cover") {
			candidates = append(candidates, reference.Href)
		}
	}
	attempts := 0
	for _, candidate := range candidates {
		if attempts >= maxCoverReferences {
			break
		}
		attempts++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name, err := resourcePath(packagePath, candidate)
		if err != nil {
			continue
		}
		data, err := readEntry(entries[name], MaxCoverBytes)
		if err != nil {
			continue
		}
		if jpeg, err := coverJPEG(ctx, data); err == nil {
			return jpeg, nil
		}
		// A guide may refer to XHTML or SVG that wraps a local raster image.
		if len(data) > MaxXMLBytes {
			continue
		}
		decoder := xml.NewDecoder(bytes.NewReader(data))
		depth := 0
		for {
			if attempts >= maxCoverReferences {
				break
			}
			token, err := decoder.Token()
			if err != nil {
				break
			}
			if _, end := token.(xml.EndElement); end {
				depth--
				continue
			}
			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			depth++
			if depth > 128 {
				break
			}
			if start.Name.Local != "img" && start.Name.Local != "image" {
				continue
			}
			for _, attr := range start.Attr {
				if (start.Name.Local == "img" && attr.Name.Local != "src") || (start.Name.Local == "image" && attr.Name.Local != "href") {
					continue
				}
				if attempts >= maxCoverReferences {
					break
				}
				attempts++
				imagePath, err := resourcePath(name, attr.Value)
				if err != nil {
					continue
				}
				imageData, err := readEntry(entries[imagePath], MaxCoverBytes)
				if err != nil {
					continue
				}
				if jpeg, err := coverJPEG(ctx, imageData); err == nil {
					return jpeg, nil
				}
			}
		}
	}
	return nil, ctx.Err()
}

func hasWord(value, word string) bool {
	for _, field := range strings.Fields(value) {
		if field == word {
			return true
		}
	}
	return false
}

func resourcePath(base, reference string) (string, error) {
	parsed, err := url.Parse(reference)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" || path.IsAbs(parsed.Path) || strings.Contains(parsed.Path, `\`) {
		return "", fmt.Errorf("cover reference must be local")
	}
	name := path.Join(path.Dir(base), parsed.Path)
	if name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("cover reference leaves the archive")
	}
	return name, nil
}

func coverJPEG(ctx context.Context, data []byte) ([]byte, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width < 1 || config.Height < 1 || config.Width > MaxCoverPixels/config.Height {
		return nil, fmt.Errorf("cover dimensions exceed limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	width, height := config.Width, config.Height
	if edge := max(width, height); edge > MaxCoverEdge {
		width = max(1, width*MaxCoverEdge/edge)
		height = max(1, height*MaxCoverEdge/edge)
	}
	thumbnail := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < width; x++ {
			r, g, b, a := source.At(bounds.Min.X+x*config.Width/width, bounds.Min.Y+y*config.Height/height).RGBA()
			thumbnail.SetRGBA(x, y, color.RGBA{uint8((r + 65535 - a) / 257), uint8((g + 65535 - a) / 257), uint8((b + 65535 - a) / 257), 255})
		}
	}
	var result bytes.Buffer
	if err := jpeg.Encode(&result, thumbnail, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return result.Bytes(), ctx.Err()
}
