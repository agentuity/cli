package tui

import (
	"os"

	"github.com/mattn/go-isatty"
)

var (
	HasTTY = isatty.IsTerminal(os.Stdout.Fd())
)
