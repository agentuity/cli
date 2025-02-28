package tui

import (
	"fmt"

	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	messageOKColor      = lipgloss.AdaptiveColor{Light: "#009900", Dark: "#00FF00"}
	messageOKStyle      = lipgloss.NewStyle().Foreground(messageOKColor)
	messageTextColor    = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}
	messageTextStyle    = lipgloss.NewStyle().Foreground(messageTextColor)
	messageWarningColor = lipgloss.AdaptiveColor{Light: "#990000", Dark: "#FF0000"}
	messageWarningStyle = lipgloss.NewStyle().Foreground(messageWarningColor)
)

func ShowSuccess(msg string, args ...any) {
	body := messageOKStyle.Render(" ✓ ") + messageTextStyle.Render(fmt.Sprintf(msg, args...))
	fmt.Println(body)
	fmt.Println()
}

func ShowLock(msg string, args ...any) {
	fmt.Printf(" 🔒 %s", messageTextStyle.Render(fmt.Sprintf(msg, args...)))
	fmt.Println()
}

func ShowWarning(msg string, args ...any) {
	body := messageWarningStyle.Render(" ✕ ") + messageTextStyle.Render(fmt.Sprintf(msg, args...))
	fmt.Println(body)
	fmt.Println()
}

func Ask(logger logger.Logger, title string, defaultValue bool) bool {
	confirm := defaultValue

	if err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes!").
		Negative("No").
		Value(&confirm).
		Inline(false).
		Run(); err != nil {
		logger.Fatal("%s", err)
	}
	return confirm
}
