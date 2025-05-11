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
	log             logger.Logger
	patterns        []*regexp.Regexp
	out             chan<- ErrorEvent
	mu              sync.Mutex
	lastHash        string
	capture         bool
	bufLines        []string
	lastActivity    time.Time
	suppressedUntil time.Time // if set in future, events are ignored until then
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

// SuppressFor ignores all new error detections for the given duration.
// Useful to avoid false-positives right after an automatic code fix / reload.
func (m *Monitor) SuppressFor(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d <= 0 {
		return
	}
	until := time.Now().Add(d)
	if until.After(m.suppressedUntil) {
		m.suppressedUntil = until
	}
}

func (m *Monitor) isSuppressed(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return now.Before(m.suppressedUntil)
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
		now := time.Now()

		if m.isSuppressed(now) {
			// Clear any in-flight capture to avoid partial buffers.
			m.capture = false
			m.bufLines = nil
			continue
		}

		if m.capture {
			// continue collecting lines until blank or timeout 500ms
			if strings.TrimSpace(line) == "" || now.Sub(m.lastActivity) > 500*time.Millisecond {
				// flush buffer
				joined := strings.Join(m.bufLines, "\n")
				evt := ErrorEvent{Raw: joined, Timestamp: now, ID: hash(joined)}
				if !m.isDuplicate(evt.ID) {
					m.out <- evt
				}
				m.capture = false
				m.bufLines = nil
			} else {
				m.bufLines = append(m.bufLines, line)
				m.lastActivity = now
			}
			continue
		}

		if m.match(line) {
			// Immediate event for single-line detection
			evt := ErrorEvent{Raw: line, Timestamp: now, ID: hash(line)}
			if !m.isDuplicate(evt.ID) {
				m.out <- evt
			}

			// Start multi-line capture for additional context
			m.capture = true
			m.bufLines = []string{line}
			m.lastActivity = now
		}
	}
	if err := scanner.Err(); err != nil {
		m.log.Error("debugmon: scanner error: %s", err)
	}

	// Flush any pending buffered lines on EOF
	if m.capture && len(m.bufLines) > 0 {
		joined := strings.Join(m.bufLines, "\n")
		evt := ErrorEvent{Raw: joined, Timestamp: time.Now(), ID: hash(joined)}
		if !m.isDuplicate(evt.ID) {
			m.out <- evt
		}
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
