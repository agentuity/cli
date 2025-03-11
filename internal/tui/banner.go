package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	bannerForegroupColor = lipgloss.AdaptiveColor{Light: "#a60853", Dark: "#F652A0"}
	bannerBorderColor    = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#AAAAAA"}
	bannerTitleColor     = lipgloss.AdaptiveColor{Light: "#36EEE0", Dark: "#00FFFF"}
	bannerMaxWidth       = 80
	bannerPadding        = 1
	bannerMargin         = 0
	bannerBorder         = lipgloss.RoundedBorder()
	bannerStyle          = lipgloss.NewStyle().
				Padding(bannerPadding).
				Margin(bannerMargin).
				AlignVertical(lipgloss.Top).
				AlignHorizontal(lipgloss.Left).
				Border(bannerBorder).
				BorderForeground(bannerBorderColor)
	bannerBodyStyle  = lipgloss.NewStyle().Width(bannerMaxWidth).Foreground(bannerForegroupColor)
	bannerTitleStyle = lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).Bold(true).Foreground(bannerTitleColor)
)

func ShowBanner(title string, body string, clearScreen bool) {
	if !HasTTY {
		return
	}
	if clearScreen {
		ClearScreen()
	}
	block := bannerTitleStyle.Render(title) + "\n\n" + bannerBodyStyle.Render(body)
	banner := bannerStyle.Render(block)
	fmt.Println(banner)
}

func BannerBodyStyle() lipgloss.Style {
	return bannerBodyStyle
}

func TitleColor() lipgloss.AdaptiveColor {
	return bannerTitleColor
}
