package debugmon

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/agentuity/go-common/logger"
)

// ErrorEvent represents a detected runtime error from the dev server output.
// Raw contains the entire captured snippet (potentially multi-line) that
// triggered the detection.
// Timestamp is when the first triggering line was seen.
// ID is a simple hash to deduplicate identical consecutive errors.
// Future versions may include parsed stack information.

type ErrorEvent struct {
	Raw       string
	Timestamp time.Time
	ID        string
}

// Monitor watches an io.Reader of process output and emits ErrorEvents to the
// provided channel when a line matches error patterns.

type Monitor struct {
	log      logger.Logger
	patterns []*regexp.Regexp
	out      chan<- ErrorEvent
	mu       sync.Mutex
	lastHash string
}

// New creates a monitor with a preconfigured set of regex patterns.
func New(log logger.Logger, out chan<- ErrorEvent) *Monitor {
	defaultPatterns := []*regexp.Regexp{
		regexp.MustCompile(`panic:`),
		regexp.MustCompile(`\berror\b`),
		regexp.MustCompile(`\bERROR\b`),
		regexp.MustCompile(`unhandled .*exception`),
	}
	return &Monitor{
		log:      log,
		patterns: defaultPatterns,
		out:      out,
	}
}

// Run begins streaming the reader and blocks until it returns EOF. Should be
// called in a goroutine if non-blocking behaviour is desired.
func (m *Monitor) Run(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Increase buffer for long lines (stack traces)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1<<20) // 1 MiB

	for scanner.Scan() {
		line := scanner.Text()
		if m.match(line) {
			evt := ErrorEvent{
				Raw:       line,
				Timestamp: time.Now(),
				ID:        hash(line),
			}
			if m.isDuplicate(evt.ID) {
				continue
			}
			m.out <- evt
		}
	}
	if err := scanner.Err(); err != nil {
		m.log.Error("debugmon: scanner error: %s", err)
	}
}

func (m *Monitor) match(line string) bool {
	l := strings.TrimSpace(line)
	for _, re := range m.patterns {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

func (m *Monitor) isDuplicate(h string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h == m.lastHash {
		return true
	}
	m.lastHash = h
	return false
}

// Very lightweight string hash (fnv1a) to deduplicate identical error lines.
func hash(s string) string {
	var h uint64 = 14695981039346656037 // offset
	const prime uint64 = 1099511628211
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return fmt.Sprintf("%x", h)
}
