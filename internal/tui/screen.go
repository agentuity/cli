package tui

import tm "github.com/buger/goterm"

// ClearScreen clears the screen and moves the cursor to the top left corner
func ClearScreen() {
	if !HasTTY {
		return
	}
	tm.Clear()
	tm.MoveCursor(1, 1)
	tm.Flush()
}
