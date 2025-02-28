package tui

import (
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh/spinner"
)

// ShowSpinner will display a spinner while the action is being performed
func ShowSpinner(logger logger.Logger, title string, action func()) {
	if err := spinner.New().Title(title).Action(action).Run(); err != nil {
		logger.Fatal("%s", err)
	}
}
