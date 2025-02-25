package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type AgentConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func parseAttribute(contents string, attribute string) (string, error) {
	var attributeRegex = regexp.MustCompile(fmt.Sprintf(`["']?%s["']?:\s+["'](.*?)["'](,\s)?`, attribute))
	if attributeRegex.MatchString(contents) {
		matches := attributeRegex.FindStringSubmatch(contents)
		// fmt.Println(attribute, matches, len(matches))
		if len(matches) == 3 {
			return strings.TrimSpace(matches[1]), nil
		}
	}
	// fmt.Println("parseAttribute >>" + contents + "<<")
	return "", fmt.Errorf("attribute '%s' not found in %s", attribute, contents)
}

// ParseAgentConfig parses an agent config from a file.
func ParseAgentConfig(projectID string, filename string) (*AgentConfig, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	contents := string(buf)
	index := strings.Index(contents, "export const config")
	if index == -1 {
		return nil, fmt.Errorf("export const config not found in %s", filename)
	}

	contents = contents[index+20:]
	endSemi := strings.Index(contents, "}")
	if endSemi == -1 {
		return nil, fmt.Errorf("couldn't find config end in%s", filename)
	}
	contents = contents[:endSemi]
	name, err := parseAttribute(contents, "name")
	if err != nil {
		return nil, err
	}
	comma := strings.Index(contents, ",")
	contents = strings.TrimSpace(contents[comma+1:])
	description, err := parseAttribute(contents, "description")
	if err != nil {
		return nil, err
	}

	id := fmt.Sprintf("%s:%s", projectID, name)
	hash := sha256.Sum256([]byte(id))

	return &AgentConfig{ID: hex.EncodeToString(hash[:]), Name: name, Description: description}, nil
}
