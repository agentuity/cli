package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"no special characters", "filename", "filename"},
		{"with spaces", "file name", "file-name"},
		{"with special characters", "file@#$%^&*()name", "file---------name"},
		{"mixed case", "FileName", "FileName"},
		{"with numbers", "file123name", "file123name"},
		{"with underscores", "file_name", "file_name"},
		{"with hyphens", "file-name", "file-name"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := SafeFilename(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		singular string
		plural   string
		expected string
	}{
		{"zero items", 0, "item", "items", "no items"},
		{"one item", 1, "item", "items", "1 item"},
		{"multiple items", 2, "item", "items", "2 items"},
		{"different words", 1, "person", "people", "1 person"},
		{"different words plural", 2, "person", "people", "2 people"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Pluralize(test.count, test.singular, test.plural)
			assert.Equal(t, test.expected, result)
		})
	}
}
