package linkify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LinkifyMarkdown scans the provided markdown (already plain text) for file
// path references like "internal/handler/foo.go:42" or
// "/absolute/path/bar.ts:10" and wraps them in OSC-8 hyperlinks so that
// supporting terminals open the file in the system-default editor when
// clicked.  Only files that resolve to a location within projectRoot are
// linked – this prevents leaking arbitrary paths.
//
// It is a best-effort helper; on failure it leaves the original text intact.
func LinkifyMarkdown(md, projectRoot string) string {
	if md == "" || projectRoot == "" {
		return md
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return md
	}

	// Simple pattern for common source files followed by :<line>.
	// We purposefully keep it conservative to avoid false positives in plain text.
	re := regexp.MustCompile(`(?m)([A-Za-z0-9_./\\-]+\.(?:go|ts|tsx|js|jsx|py|rs|rb|java|c|cpp|cs|php)):(\d+)`)

	oscPrefix := "\x1b]8;;"
	oscSuffix := "\x07"

	return re.ReplaceAllStringFunc(md, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		pathPart := sub[1]
		linePart := sub[2]

		// Resolve path relative to projectRoot if not absolute.
		absPath := pathPart
		if !filepath.IsAbs(pathPart) {
			absPath = filepath.Join(absRoot, pathPart)
		}
		absPath = filepath.Clean(absPath)

		// Ensure inside project root.
		if !strings.HasPrefix(absPath, absRoot) {
			return match
		}
		// Ensure file exists (non-fatal if it doesn't).
		if _, err := os.Stat(absPath); err != nil {
			return match
		}

		uri := fmt.Sprintf("file://%s#L%s", absPath, linePart)
		return fmt.Sprintf("%s%s%s%s%s", oscPrefix, uri, oscSuffix, match, oscPrefix+oscSuffix)
	})
}
