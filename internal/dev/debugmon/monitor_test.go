package debugmon

import (
	"strings"
	"testing"
	"time"

	"github.com/agentuity/go-common/env"
)

func TestMonitorSingleLine(t *testing.T) {
	log := env.NewLogger(nil)
	ch := make(chan ErrorEvent, 1)
	mon := New(log, ch)
	go mon.Run(strings.NewReader("panic: something bad\n"))

	select {
	case evt := <-ch:
		if !strings.Contains(evt.Raw, "panic") {
			t.Fatalf("unexpected raw: %s", evt.Raw)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMonitorMultiLine(t *testing.T) {
	log := env.NewLogger(nil)
	ch := make(chan ErrorEvent, 1)
	input := "panic: boom\nstack line1\nstack line2\n\nnext output\n"
	mon := New(log, ch)
	go mon.Run(strings.NewReader(input))

	select {
	case evt := <-ch:
		if !strings.Contains(evt.Raw, "stack line2") {
			t.Fatalf("multiline capture failed: %s", evt.Raw)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMonitorDuplicate(t *testing.T) {
	log := env.NewLogger(nil)
	ch := make(chan ErrorEvent, 2)
	input := "panic: bad\npanic: bad\n"
	mon := New(log, ch)
	go mon.Run(strings.NewReader(input))

	count := 0
	for {
		select {
		case <-ch:
			count++
		case <-time.After(500 * time.Millisecond):
			if count != 1 {
				t.Fatalf("expected 1 event, got %d", count)
			}
			return
		}
	}
}
