package dev

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

type ioType int

const (
	StdErr ioType = iota
	Stdout
)

type TuiLogger struct {
	logLevel logger.LogLevel
	ui       *DevModeUI
	ioType   ioType
	pending  bytes.Buffer
	mu       sync.Mutex
}

func NewTUILogger(logLevel logger.LogLevel, ui *DevModeUI, ioType ioType) *TuiLogger {
	return &TuiLogger{logLevel: logLevel, ui: ui, ioType: ioType}
}

var _ logger.Logger = (*TuiLogger)(nil)
var _ io.Writer = (*TuiLogger)(nil)

// With will return a new logger using metadata as the base context
func (l *TuiLogger) With(metadata map[string]interface{}) logger.Logger {
	return l
}

// WithPrefix will return a new logger with a prefix prepended to the message
func (l *TuiLogger) WithPrefix(prefix string) logger.Logger {
	return l
}

// WithContext will return a new logger with the given context
func (l *TuiLogger) WithContext(ctx context.Context) logger.Logger {
	return l
}

// Trace level logging
func (l *TuiLogger) Trace(msg string, args ...interface{}) {
	if logger.LevelTrace < l.logLevel {
		return
	}
	l.ui.AddLog(logger.LevelTrace, "[TRACE] %s", fmt.Sprintf(msg, args...))
}

// Debug level logging
func (l *TuiLogger) Debug(msg string, args ...interface{}) {
	if logger.LevelDebug < l.logLevel {
		return
	}
	l.ui.AddLog(logger.LevelDebug, "[DEBUG] %s", fmt.Sprintf(msg, args...))
}

// Info level loggi	ng
func (l *TuiLogger) Info(msg string, args ...interface{}) {
	if logger.LevelInfo < l.logLevel {
		return
	}
	l.ui.AddLog(logger.LevelInfo, "[INFO] %s", fmt.Sprintf(msg, args...))
}

// Warning level logging
func (l *TuiLogger) Warn(msg string, args ...interface{}) {
	if logger.LevelWarn < l.logLevel {
		return
	}
	l.ui.AddLog(logger.LevelWarn, "[WARN] %s", fmt.Sprintf(msg, args...))
}

// Error level logging
func (l *TuiLogger) Error(msg string, args ...interface{}) {
	if logger.LevelError < l.logLevel {
		return
	}
	l.ui.AddLog(logger.LevelError, "[ERROR] %s", fmt.Sprintf(msg, args...))
}

// Fatal level logging and exit with code 1
func (l *TuiLogger) Fatal(msg string, args ...interface{}) {
	val := tui.Bold("[FATAL] " + fmt.Sprintf(msg, args...))
	l.ui.AddLog(logger.LevelError, "%s", val)
	os.Exit(1)
}

// Stack will return a new logger that logs to the given logger as well as the current logger
func (l *TuiLogger) Stack(next logger.Logger) logger.Logger {
	return l
}

var eol = []byte("\n")
var ansiColorStripper = regexp.MustCompile("\x1b\\[[0-9;]*[mK]")

// Map prefix to severity level
var prefixToLevel = map[string]logger.LogLevel{
	"[TRACE]": logger.LevelTrace,
	"[DEBUG]": logger.LevelDebug,
	"[INFO]":  logger.LevelInfo,
	"[INFO ]": logger.LevelInfo, // python
	"[WARNI]": logger.LevelWarn, // python
	"[WARN]":  logger.LevelWarn,
	"[ERROR]": logger.LevelError,
	"[FATAL]": logger.LevelError,
}

func (l *TuiLogger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending.Write(p)

	if !bytes.HasSuffix(l.pending.Bytes(), eol) {
		return len(p), nil
	}

	trimmed := bytes.Split(l.pending.Bytes(), eol)
	l.pending.Reset()

	for _, line := range trimmed {
		if len(line) == 0 {
			continue
		}
		log := ansiColorStripper.ReplaceAllString(string(line), "")
		severity := logger.LevelTrace
		var prefix string
		if len(log) > 9 {
			bracket := strings.Index(log, "] ")
			if bracket == -1 {
				// No prefix – treat the entire line as the message
				prefix = ""
			} else {
				prefix = strings.TrimSpace(log[:bracket+2])
				log = strings.TrimPrefix(log[bracket+2:], " ")
			}
			// Find matching prefix
			for p, level := range prefixToLevel {
				if strings.HasPrefix(prefix, p) {
					severity = level
					if level < l.logLevel {
						continue
					}
					break
				}
			}
		}
		if l.ioType == Stdout {
			l.ui.AddLog(severity, "%s %s", prefix, log)
		} else {
			l.ui.AddErrorLog("%s", log)
		}
	}
	return len(p), nil
}
