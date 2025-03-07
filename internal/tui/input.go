package tui

import (
	"fmt"
	"os"

	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh"
)

var inputTheme = huh.ThemeBase16()

func Input(logger logger.Logger, title string, description string) string {
	return InputWithPlaceholder(logger, title, description, "")
}

func InputWithPlaceholder(logger logger.Logger, title string, description string, placeholder string) string {
	var value string
	if err := huh.NewInput().
		Title(title).
		Prompt("> ").
		Description(description).
		Placeholder(placeholder).
		Value(&value).
		WithTheme(inputTheme).
		Run(); err != nil {
		logger.Fatal("%s", err)
	}
	if value == "" {
		return placeholder
	}
	return value
}

func InputWithValidation(logger logger.Logger, title string, description string, maxLength int, validate func(string) error) string {
	var value string
	if err := huh.NewInput().
		Title(title).
		Prompt("> ").
		Description(description + "\n").
		CharLimit(maxLength).
		Validate(validate).
		Value(&value).
		WithTheme(inputTheme).
		Run(); err != nil {
		logger.Fatal("%s", err)
	}
	return value
}

func Password(logger logger.Logger, title string, description string) string {
	var value string
	if err := huh.NewInput().
		Title(title).
		Prompt("> ").
		Description(description + "\n").
		EchoMode(huh.EchoModePassword).
		Value(&value).
		WithTheme(inputTheme).
		Run(); err != nil {
		logger.Fatal("%s", err)
	}
	return value
}

func WaitForAnyKey() {
	WaitForAnyKeyMessage("Press any key to continue... ")
}

func WaitForAnyKeyMessage(message string) {
	fmt.Print(Secondary(message))
	buf := make([]byte, 1)
	os.Stdin.Read(buf)
}
