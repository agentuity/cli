package util

import (
	"fmt"
	"regexp"
)

var safeNameTransformer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
var safePythonNameTransformer = regexp.MustCompile(`[^a-zA-Z0-9_]`)
var beginsWithNumber = regexp.MustCompile(`^[0-9]+`)
var removeStartingDashes = regexp.MustCompile(`^[-]+`)
var removeEndingDashes = regexp.MustCompile(`[-]+$`)

func SafeFilename(name string) string {
	return safeNameTransformer.ReplaceAllString(name, "-")
}

func SafePythonFilename(name string) string {
	if beginsWithNumber.MatchString(name) {
		name = beginsWithNumber.ReplaceAllString(name, "")
	}
	name = safePythonNameTransformer.ReplaceAllString(name, "_")
	if removeStartingDashes.MatchString(name) {
		name = removeStartingDashes.ReplaceAllString(name, "")
	}
	if removeEndingDashes.MatchString(name) {
		name = removeEndingDashes.ReplaceAllString(name, "")
	}
	return name
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
