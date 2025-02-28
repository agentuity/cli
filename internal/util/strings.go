package util

import (
	"fmt"
	"regexp"
)

var safeNameTransformer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func SafeFilename(name string) string {
	return safeNameTransformer.ReplaceAllString(name, "-")
}

func Pluralize(count int, singular string, plural string) string {
	if count == 0 {
		return fmt.Sprintf("no %s", plural)
	}
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}
