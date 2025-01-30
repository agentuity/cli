package autodetect

import (
	"os"
	"path/filepath"

	"github.com/shopmonkeyus/go-common/logger"
)

type Detector func(logger logger.Logger, dir string, state map[string]any) (string, error)

var detectors = []Detector{}

func register(detector Detector) {
	detectors = append(detectors, detector)
}

func Detect(logger logger.Logger, dir string) (string, error) {
	state := map[string]any{}
	for _, detector := range detectors {
		result, err := detector(logger, dir, state)
		if err != nil {
			return "", err
		}
		if result != "" {
			return result, nil
		}
	}
	return "", nil
}

func readPyProject(dir string, state map[string]any) (string, error) {
	if val, ok := state["pyproject"].(string); ok {
		return val, nil
	}
	fn := filepath.Join(dir, "pyproject.toml")
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return "", nil
	}
	content, err := os.ReadFile(fn)
	if err != nil {
		return "", err
	}
	state["pyproject"] = string(content)
	return string(content), nil
}
