package util

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestSafePythonFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cases
		{"empty string", "", ""},
		{"simple name", "module", "module"},
		{"with underscores", "my_module", "my_module"},
		{"with numbers", "module123", "module123"},
		{"mixed case", "MyModule", "MyModule"},

		// Special characters
		{"with spaces", "my module", "my_module"},
		{"with hyphens", "my-module", "my_module"},
		{"with dots", "my.module", "my_module"},
		{"with multiple special chars", "my@#$%^&*()module", "my_________module"},

		// Numbers at start
		{"starts with number", "123module", "module"},
		{"starts with number and special chars", "123@#$module", "___module"},
		{"starts with number and spaces", "123 module", "_module"},

		// Edge cases
		{"all special chars", "@#$%^&*()", "_________"},
		{"all numbers", "12345", ""},
		{"single underscore", "_", "_"},
		{"multiple underscores", "___", "___"},

		// Python keywords
		{"python keyword", "import", "import"},
		{"python keyword with special chars", "def@#$", "def___"},

		// Unicode
		{"unicode characters", "módulé", "m_dul_"},
		{"unicode with numbers", "módulé123", "m_dul_123"},

		// Length considerations
		{"very long name", "a" + strings.Repeat("b", 100) + "c", "a" + strings.Repeat("b", 100) + "c"},
		{"single character", "a", "a"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := SafeProjectFilename(test.input, true)
			assert.Equal(t, test.expected, result)
		})
	}
}

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
			result := SafeProjectFilename(test.input, false)
			assert.Equal(t, test.expected, result)
		})
	}
}
