package bundler

import (
	"strings"

	"github.com/agentuity/cli/internal/util"
	"github.com/charmbracelet/lipgloss"
	"github.com/evanw/esbuild/pkg/api"
)

func FormatBuildError(projectDir string, err api.Message) string {
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

	result := strings.Join(formatted, "\n")

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0066cc", Dark: "#66ccff"})
	result += "\n\n" + helpStyle.Render("note: JavaScript build failed\n")

	return result
}
