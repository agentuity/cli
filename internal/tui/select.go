package tui

import (
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh"
)

func Select(logger logger.Logger, title string, description string, items []Option) string {
	var selected string

	var opts []huh.Option[string]
	for _, item := range items {
		opts = append(opts, huh.NewOption(item.Text, item.ID).Selected(item.Selected))
	}

	descriptionText := description
	if description != "" && description != "\n" {
		descriptionText += "\n"
	}

	if err := huh.NewSelect[string]().
		Title(title).
		Description(descriptionText).
		Options(opts...).
		Value(&selected).Run(); err != nil {
		logger.Fatal("%s", err)
	}

	return selected
}
