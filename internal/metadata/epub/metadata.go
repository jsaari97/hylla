package epub

import (
	"archive/zip"
	"context"
	"io"
	"strings"

	"github.com/jsaari97/hylla/internal/books"
)

// ExtractMetadata reads package metadata without changing or extracting files.
func ExtractMetadata(ctx context.Context, file io.ReaderAt, size int64) (books.Metadata, error) {
	if err := checkDirectory(ctx, file, size); err != nil {
		return books.Metadata{}, err
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return books.Metadata{}, err
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
		return books.Metadata{}, err
	}
	packagePath := ""
	for _, root := range container.Roots {
		if root.Type == "application/oebps-package+xml" {
			packagePath = root.Path
			break
		}
	}
	var pkg struct {
		Metadata struct {
			Titles []struct {
				ID   string `xml:"id,attr"`
				Text string `xml:",chardata"`
			} `xml:"http://purl.org/dc/elements/1.1/ title"`
			Creators []struct {
				ID   string `xml:"id,attr"`
				Role string `xml:"http://www.idpf.org/2007/opf role,attr"`
				Text string `xml:",chardata"`
			} `xml:"http://purl.org/dc/elements/1.1/ creator"`
			Meta []struct {
				Refines  string `xml:"refines,attr"`
				Property string `xml:"property,attr"`
				Text     string `xml:",chardata"`
			} `xml:"meta"`
		} `xml:"metadata"`
	}
	if err := decodeXML(entries[packagePath], &pkg); err != nil {
		return books.Metadata{}, err
	}
	result := books.Metadata{}
	mainTitles := make(map[string]bool)
	roles := make(map[string]string)
	for _, meta := range pkg.Metadata.Meta {
		if !strings.HasPrefix(meta.Refines, "#") || len(meta.Refines) < 2 {
			continue
		}
		id := strings.TrimPrefix(meta.Refines, "#")
		if meta.Property == "title-type" && strings.TrimSpace(meta.Text) == "main" {
			mainTitles[id] = true
		}
		if meta.Property == "role" {
			if roles[id] != "aut" {
				roles[id] = strings.TrimSpace(meta.Text)
			}
		}
	}
	for _, title := range pkg.Metadata.Titles {
		value := strings.Join(strings.Fields(title.Text), " ")
		if value == "" {
			continue
		}
		if result.Title == "" {
			result.Title = value
		}
		if mainTitles[title.ID] {
			result.Title = value
			break
		}
	}
	seen := make(map[string]bool)
	for _, creator := range pkg.Metadata.Creators {
		role := strings.TrimSpace(creator.Role)
		if refined, ok := roles[creator.ID]; ok {
			role = refined
		}
		if role != "" && role != "aut" {
			continue
		}
		name := strings.Join(strings.Fields(creator.Text), " ")
		if name != "" && !seen[name] {
			result.Authors = append(result.Authors, books.Author{Name: name})
			seen[name] = true
		}
	}
	return result, ctx.Err()
}
