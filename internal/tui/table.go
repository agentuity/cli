package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

var (
	tableBorderColor = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#AAAAAA"}
	tableBorderStyle = lipgloss.NewStyle().Foreground(tableBorderColor)
)

func Table(headers []string, rows [][]string) {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(tableBorderStyle).
		Headers(headers...).
		Rows(rows...)
	fmt.Println(t.String())
}
