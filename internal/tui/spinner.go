package tui

import (
	"github.com/charmbracelet/huh/spinner"
)

// ShowSpinner will display a spinner while the action is being performed
func ShowSpinner(title string, action func()) {
	spinner.New().Title(title).Action(action).Run()
}
