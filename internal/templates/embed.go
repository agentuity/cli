package templates

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/agentuity/cli/internal/util"
)

func getEmbeddedFile(path string) (io.ReadCloser, error) {
	if !util.Exists(path) {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func getEmbeddedDir(path string) ([]fs.DirEntry, error) {
	if !util.Exists(path) {
		return nil, fmt.Errorf("directory not found: %s", path)
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	return files, nil
}
