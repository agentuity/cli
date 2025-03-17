package tui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
	if !HasTTY {
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ch := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 1)
		os.Stdin.Read(buf)
		ch <- struct{}{}
	}()
	fmt.Print(Secondary(message))
	select {
	case <-ctx.Done():
		return
	case <-ch:
	}
}
