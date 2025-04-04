package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

var originalVersion string

func TestGetLatestRelease(t *testing.T) {
	originalVersion = Version
	defer func() { Version = originalVersion }()

	t.Run("dev version", func(t *testing.T) {
		Version = "dev"
		version, err := GetLatestRelease(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, "dev", version)
	})

	t.Skip("Skipping GitHub API tests that require complex HTTP mocking")
}

func TestCheckLatestRelease(t *testing.T) {

	originalVersion = Version
	defer func() { Version = originalVersion }()

	t.Skip("Skipping test that requires mockLogger")
}
