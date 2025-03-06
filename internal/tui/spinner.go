package tui

import (
	"context"
	"sync"

	"github.com/charmbracelet/huh/spinner"
)

var (
	currentSpinnerCtx  context.Context
	currentSpinnerDone context.CancelFunc
)

// ShowSpinner will display a spinner while the action is being performed
func ShowSpinner(title string, action func()) {
	if !HasTTY {
		action()
		return
	}
	var wg sync.WaitGroup
	if currentSpinnerDone != nil {
		currentSpinnerDone()
	}
	currentSpinnerCtx, currentSpinnerDone = context.WithCancel(context.Background())
	s := spinner.New()
	s = s.Context(currentSpinnerCtx)
	wg.Add(1)
	s.Title(title).Action(func() {
		defer wg.Done()
		action()
		if currentSpinnerDone != nil {
			currentSpinnerDone()
			currentSpinnerCtx = nil
			currentSpinnerDone = nil
		}
	}).Run()
	wg.Wait()
}

// CancelSpinner will cancel the current spinner
func CancelSpinner() {
	if currentSpinnerDone != nil {
		currentSpinnerDone()
		currentSpinnerCtx = nil
		currentSpinnerDone = nil
	}
}
