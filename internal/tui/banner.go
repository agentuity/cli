package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	bannerForegroupColor = lipgloss.AdaptiveColor{Light: "#071330", Dark: "#F652A0"}
	bannerBorderColor    = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#AAAAAA"}
	bannerTitleColor     = lipgloss.AdaptiveColor{Light: "#36EEE0", Dark: "#00FFFF"}
	bannerMaxWidth       = 80
	bannerPadding        = 1
	bannerMargin         = 1
	bannerBorder         = lipgloss.RoundedBorder()
	bannerStyle          = lipgloss.NewStyle().
				Width(bannerMaxWidth).
				Padding(bannerPadding).
				Margin(bannerMargin).
				AlignVertical(lipgloss.Top).
				AlignHorizontal(lipgloss.Left).
				Border(bannerBorder).
				BorderForeground(bannerBorderColor).
				Foreground(bannerForegroupColor)
	bannerTitleStyle = lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).Bold(true).Foreground(bannerTitleColor)
)

func ShowBanner(title string, body string, clearScreen bool) {
	if clearScreen {
		ClearScreen()
	}
	block := bannerTitleStyle.Render(title) + "\n\n" + body
	banner := bannerStyle.Render(block)
	fmt.Println(banner)
}

func TitleColor() lipgloss.AdaptiveColor {
	return bannerTitleColor
}
