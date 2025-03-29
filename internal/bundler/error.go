package bundler

import (
	"strings"

	"github.com/agentuity/cli/internal/util"
	"github.com/evanw/esbuild/pkg/api"
)

func formatESBuildError(projectDir string, err api.Message) string {
	if err.Location != nil && err.Location.File != "" {
		if err.Location.LineText == "" && util.Exists(err.Location.File) {
			lines, readErr := util.ReadFileLines(err.Location.File, err.Location.Line-1, err.Location.Line-1)
			if readErr == nil && len(lines) > 0 {
				err.Location.LineText = lines[0]
			}
		}

		relPath := util.GetRelativePath(projectDir, err.Location.File)
		err.Location.File = relPath
	}

	formatted := api.FormatMessages([]api.Message{err}, api.FormatMessagesOptions{
		Kind:          api.ErrorMessage,
		Color:         true,
		TerminalWidth: 120,
	})

	return strings.Join(formatted, "\n")
}
