package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewOrderedMap(t *testing.T) {
	keys := []string{"name", "version", "description"}
	data := map[string]any{
		"name":        "test",
		"version":     "1.0.0",
		"description": "Test description",
		"extra":       "extra field",
	}

	result := NewOrderedMap(keys, data)

	assert.Equal(t, keys, result.keys)
	assert.Equal(t, data, result.Data)
}

func TestOrderedMapToJSON(t *testing.T) {
	keys := []string{"name", "version", "description"}
	data := map[string]any{
		"name":        "test",
		"version":     "1.0.0",
		"description": "Test description",
		"extra":       "extra field",
	}

	orderedMap := NewOrderedMap(keys, data)
	jsonBytes, err := orderedMap.ToJSON()

	assert.NoError(t, err)
	assert.NotNil(t, jsonBytes)

	var parsed map[string]any
	err = json.Unmarshal(jsonBytes, &parsed)
	assert.NoError(t, err)

	assert.Equal(t, data["name"], parsed["name"])
	assert.Equal(t, data["version"], parsed["version"])
	assert.Equal(t, data["description"], parsed["description"])
	assert.Equal(t, data["extra"], parsed["extra"])
}

func TestOrderedMapMarshalJSON(t *testing.T) {
	keys := []string{"name", "version", "description"}
	data := map[string]any{
		"name":        "test",
		"version":     "1.0.0",
		"description": "Test description",
		"extra":       "extra field",
	}

	orderedMap := NewOrderedMap(keys, data)
	jsonBytes, err := json.Marshal(orderedMap)

	assert.NoError(t, err)
	assert.NotNil(t, jsonBytes)

	jsonStr := string(jsonBytes)
	nameIdx := strings.Index(jsonStr, "\"name\"")
	versionIdx := strings.Index(jsonStr, "\"version\"")
	descriptionIdx := strings.Index(jsonStr, "\"description\"")
	extraIdx := strings.Index(jsonStr, "\"extra\"")

	assert.True(t, nameIdx < versionIdx)
	assert.True(t, versionIdx < descriptionIdx)
	assert.True(t, descriptionIdx < extraIdx)
}

func TestNewOrderedMapFromJSON(t *testing.T) {
	keys := []string{"name", "version", "description"}
	jsonData := []byte(`{"name":"test","version":"1.0.0","description":"Test description","extra":"extra field"}`)

	result, err := NewOrderedMapFromJSON(keys, jsonData)

	assert.NoError(t, err)
	assert.Equal(t, keys, result.keys)
	assert.Equal(t, "test", result.Data["name"])
	assert.Equal(t, "1.0.0", result.Data["version"])
	assert.Equal(t, "Test description", result.Data["description"])
	assert.Equal(t, "extra field", result.Data["extra"])
}

func TestNewOrderedMapFromFile(t *testing.T) {
	keys := []string{"name", "version", "description"}
	jsonData := []byte(`{"name":"test","version":"1.0.0","description":"Test description","extra":"extra field"}`)

	tmpDir, err := os.MkdirTemp("", "orderedmap_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "test.json")
	err = os.WriteFile(tmpFile, jsonData, 0644)
	assert.NoError(t, err)

	result, err := NewOrderedMapFromFile(keys, tmpFile)

	assert.NoError(t, err)
	assert.Equal(t, keys, result.keys)
	assert.Equal(t, "test", result.Data["name"])
	assert.Equal(t, "1.0.0", result.Data["version"])
	assert.Equal(t, "Test description", result.Data["description"])
	assert.Equal(t, "extra field", result.Data["extra"])
}
