package ignore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRules(t *testing.T) {
	rules := Empty()
	rules.AddDefaults()
	assert.True(t, rules.Ignore("/Users/foobar/example/src/agents/my-agent/.index.ts.swp", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/agents/my-agent/.index.ts~", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.venv/lib/foo.py", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.gitignore", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/README.md", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/README", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/LICENSE.md", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/LICENSE", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/Makefile", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.editorconfig", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.agentuity/config.json", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.cursor/file1", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.env.local", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.github/workflows/ci.yml", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.vscode/settings.json", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/__pycache__/foo.pyc", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/__tests__/test_foo.py", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/node_modules/lodash/index.js", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/agents/my-agent/.index.pyc", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/agents/my-agent/.index.tar.gz", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/agents/my-agent/.index.zip", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/agents/my-agent/.index.tar", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.git/objects/pack/pack-123.pack", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.git", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.foo.swp", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/src/__test__/test_bar.py", nil))
	assert.True(t, rules.Ignore("/Users/foobar/example/.agentuity-12345", nil))
}
