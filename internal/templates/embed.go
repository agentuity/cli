package templates

import (
	"embed"
	"io"
	"io/fs"
	"strings"
)

//go:embed data
var embeddedData embed.FS

func getEmbeddedFile(path string) (io.Reader, error) {
	// Ensure the path starts with "data/"
	if !strings.HasPrefix(path, "data/") {
		path = "data/" + path
	}
	return embeddedData.Open(path)
}

func getEmbeddedDir(path string) ([]fs.DirEntry, error) {
	// Ensure the path starts with "data/"
	if !strings.HasPrefix(path, "data/") {
		path = "data/" + path
	}
	return embeddedData.ReadDir(path)
}
