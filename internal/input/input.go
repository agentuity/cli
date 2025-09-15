package input

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// WaitForAnyKey prints the message "Press any key to continue..." and waits for any key press
func WaitForAnyKey() {
	WaitForAnyKeyMessage("Press any key to continue... ")
}

// WaitForAnyKeyMessage prints a message and waits for any key press using raw terminal mode
func WaitForAnyKeyMessage(message string) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Print(message)
		buf := make([]byte, 1)
		os.Stdin.Read(buf)
		return
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	ch := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 1)
		os.Stdin.Read(buf)
		fmt.Print(ansi.CursorBackward(1)) // remove the char from the screen output
		ch <- struct{}{}
	}()

	fmt.Print(message)
	select {
	case <-ctx.Done():
		fmt.Println()
		os.Exit(1)
		return
	case <-ch:
		select {
		case <-ctx.Done():
			fmt.Println()
			os.Exit(1)
		default:
			return
		}
	}
}
