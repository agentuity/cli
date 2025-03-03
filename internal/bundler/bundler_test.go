package bundler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPyProject(t *testing.T) {
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test-name"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test1"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test name"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test-name-1"`))

	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-alpha"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-beta"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-rc"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-dev"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-alpha.1"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-beta.1"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-rc.1"`))

	assert.Equal(t, "test", pyProjectNameRegex.FindStringSubmatch(`name = "test"`)[1])
	assert.Equal(t, "test name", pyProjectNameRegex.FindStringSubmatch(`name = "test name"`)[1])
	assert.Equal(t, "test1", pyProjectNameRegex.FindStringSubmatch(`name = "test1"`)[1])
	assert.Equal(t, "test-name", pyProjectNameRegex.FindStringSubmatch(`name = "test-name"`)[1])
	assert.Equal(t, "test-name-1", pyProjectNameRegex.FindStringSubmatch(`name = "test-name-1"`)[1])

	assert.Equal(t, "1.0.0", pyProjectVersionRegex.FindStringSubmatch(`version = "1.0.0"`)[1])
	assert.Equal(t, "1.0.0-alpha", pyProjectVersionRegex.FindStringSubmatch(`version = "1.0.0-alpha"`)[1])
	assert.Equal(t, "1.0.0-beta", pyProjectVersionRegex.FindStringSubmatch(`version = "1.0.0-beta"`)[1])
}
