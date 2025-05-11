package debugmon

import (
	"strings"
	"testing"
	"time"

	"github.com/agentuity/go-common/env"
	"github.com/spf13/cobra"
)

func TestMonitorSingleLine(t *testing.T) {
	log := env.NewLogger(&cobra.Command{})
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
