package tui

import (
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh"
)

type Option struct {
	ID       string
	Text     string
	Selected bool
}

// MultiSelect will display a multi-select list of items and return the selected items
func MultiSelect(logger logger.Logger, title string, description string, items []Option) []string {
	var selected []string
	var options []huh.Option[string]

	for _, item := range items {
		options = append(options, huh.NewOption(item.Text, item.ID).Selected(item.Selected))
	}

	if description == "" {
		description = "Toggle selection by pressing the spacebar\nPress enter to confirm"
	}

	if err := huh.NewMultiSelect[string]().
		Options(
			options...,
		).
		Title(title).
		Description(description + "\n").
		Value(&selected).
		WithHeight(100).
		Run(); err != nil {
		logger.Fatal("%s", err)
	}

	return selected
}
