package linkify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkifyMarkdown(t *testing.T) {
	root := t.TempDir()

	// Create dummy file
	filePath := "foo/bar.go"
	abs := root + "/" + filePath
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("package main"), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	input := "Error in " + filePath + ":10"
	out := LinkifyMarkdown(input, root)

	if !strings.Contains(out, "\x1b]8;;") {
		t.Fatalf("expected OSC-8 hyperlink, got %q", out)
	}
}
