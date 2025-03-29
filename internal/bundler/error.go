package bundler

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agentuity/cli/internal/util"
	"github.com/charmbracelet/lipgloss"
	"github.com/evanw/esbuild/pkg/api"
)

func FormatBuildError(projectDir string, err api.Message) string {
	var builder strings.Builder

	var locationInfo string
	if err.Location != nil {
		relPath := util.GetRelativePath(projectDir, err.Location.File)

		if err.Location.Column > 0 {
			locationInfo = fmt.Sprintf("%s:%d:%d", relPath, err.Location.Line, err.Location.Column)
		} else {
			locationInfo = fmt.Sprintf("%s:%d", relPath, err.Location.Line)
		}

		locationStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0000ff", Dark: "#6699ff"}).Bold(true)
		locationInfo = locationStyle.Render(locationInfo)

		errorHeaderStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#ff6666"}).Bold(true)
		msgStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"})
		builder.WriteString(errorHeaderStyle.Render("error: ") + msgStyle.Render(err.Text) + "\n")
		builder.WriteString("  " + locationStyle.Render("-->") + " " + locationInfo + "\n")

		if util.Exists(err.Location.File) {
			startLine := err.Location.Line - 4
			if startLine < 0 {
				startLine = 0
			}
			endLine := err.Location.Line + 3 // Show 3 lines after the error line

			lines, readErr := util.ReadFileLines(err.Location.File, startLine, endLine)
			if readErr == nil && len(lines) > 0 {
				builder.WriteString("\n")
				
				maxLineNum := startLine + len(lines)
				lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))
				
				lineNumberStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"})
				normalTextStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"})
				errorPointerStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#ff6666"}).Bold(true)
				
				language := "javascript"
				if filepath.Ext(err.Location.File) == ".ts" {
					language = "typescript"
				}
				
				errorLineIndex := -1
				for i, _ := range lines {
					lineNum := startLine + i + 1
					if lineNum == err.Location.Line {
						errorLineIndex = i
						break
					}
				}
				
				for i, line := range lines {
					lineNum := startLine + i + 1
					
					builder.WriteString(lineNumberStyle.Render(fmt.Sprintf("%*d | ", lineNumWidth, lineNum)))
					builder.WriteString(normalTextStyle.Render(line) + "\n")
					
					if i == errorLineIndex - 1 && err.Location.Column > 0 {
						pointerIndent := strings.Repeat(" ", err.Location.Column)
						builder.WriteString(lineNumberStyle.Render(fmt.Sprintf("%*s │ ", lineNumWidth, "")))
						builder.WriteString(errorPointerStyle.Render(fmt.Sprintf("%s^", pointerIndent)) + "\n")
						
						if err.Text != "" {
							builder.WriteString(lineNumberStyle.Render(fmt.Sprintf("%*s │ ", lineNumWidth, "")))
							builder.WriteString(errorPointerStyle.Render(fmt.Sprintf("%s%s", pointerIndent, err.Text)) + "\n")
						}
					}
				}
			}
			builder.WriteString("\n")
		}
	} else {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#ff6666"}).Bold(true)
		builder.WriteString(errorStyle.Render("error: ") + err.Text + "\n")
	}

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0066cc", Dark: "#66ccff"})
	builder.WriteString(helpStyle.Render("note: JavaScript build failed\n"))

	return builder.String()
}
