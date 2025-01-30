package autodetect

import (
	"strings"

	"github.com/shopmonkeyus/go-common/logger"
)

func detectCrewAI(logger logger.Logger, dir string, state map[string]any) (string, error) {
	buf, err := readPyProject(dir, state)
	if err != nil {
		return "", err
	}
	if buf != "" {
		if strings.Contains(buf, "crewai") {
			return "crewai", nil
		}
	}
	return "", nil
}

func init() {
	register(detectCrewAI)
}
