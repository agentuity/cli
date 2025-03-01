package util

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	cstr "github.com/agentuity/go-common/string"
	"github.com/marcozac/go-jsonc"
)

type orderedMap struct {
	keys []string
	Data map[string]any
}

func NewOrderedMapFromFile(keys []string, filename string) (*orderedMap, error) {
	of, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return NewOrderedMapFromJSON(keys, of)
}

func NewOrderedMapFromJSON(keys []string, buf []byte) (*orderedMap, error) {
	var data map[string]any
	if err := jsonc.Unmarshal(buf, &data); err != nil {
		return nil, err
	}
	return NewOrderedMap(keys, data), nil
}

func NewOrderedMap(keys []string, data map[string]any) *orderedMap {
	return &orderedMap{
		keys: keys,
		Data: data,
	}
}

func (p *orderedMap) ToJSON() ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}

func (p *orderedMap) MarshalJSON() ([]byte, error) {
	var keys []string
	found := make(map[string]bool)
	for k := range p.Data {
		found[k] = false
	}
	for _, k := range p.keys {
		if _, ok := p.Data[k]; ok {
			keys = append(keys, k)
			found[k] = true
		}
	}
	for k := range p.Data {
		if !found[k] {
			keys = append(keys, k)
		}
	}
	var jsonBuf strings.Builder
	jsonBuf.WriteString("{")
	for i, k := range keys {
		var comma string
		if i < len(keys)-1 {
			comma = ","
		}
		val := cstr.JSONStringify(p.Data[k])
		jsonBuf.WriteString(fmt.Sprintf("\"%s\": %s%s", k, val, comma))
	}
	jsonBuf.WriteString("}")
	return []byte(jsonBuf.String()), nil
}
