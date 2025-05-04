package dev

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

type TuiLogger struct {
	logLevel logger.LogLevel
	ui       *DevModeUI
}

func NewTUILogger(logLevel logger.LogLevel, ui *DevModeUI) *TuiLogger {
	return &TuiLogger{logLevel: logLevel, ui: ui}
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
	val := tui.Muted("[TRACE] " + fmt.Sprintf(msg, args...))
	l.ui.AddLog("%s", val)
}

// Debug level logging
func (l *TuiLogger) Debug(msg string, args ...interface{}) {
	if logger.LevelDebug < l.logLevel {
		return
	}
	val := tui.Muted("[TRACE] " + fmt.Sprintf(msg, args...))
	l.ui.AddLog("%s", val)
}

// Info level loggi	ng
func (l *TuiLogger) Info(msg string, args ...interface{}) {
	if logger.LevelInfo < l.logLevel {
		return
	}
	val := tui.Text("[INFO] " + fmt.Sprintf(msg, args...))
	l.ui.AddLog("%s", val)
}

// Warning level logging
func (l *TuiLogger) Warn(msg string, args ...interface{}) {
	if logger.LevelWarn < l.logLevel {
		return
	}
	val := tui.Title("[WARN] " + fmt.Sprintf(msg, args...))
	l.ui.AddLog("%s", val)
}

// Error level logging
func (l *TuiLogger) Error(msg string, args ...interface{}) {
	if logger.LevelError < l.logLevel {
		return
	}
	val := tui.Bold("[ERROR] " + fmt.Sprintf(msg, args...))
	l.ui.AddLog("%s", val)
}

// Fatal level logging and exit with code 1
func (l *TuiLogger) Fatal(msg string, args ...interface{}) {
	val := tui.Bold("[FATAL] " + fmt.Sprintf(msg, args...))
	l.ui.AddLog("%s", val)
	os.Exit(1)
}

// Stack will return a new logger that logs to the given logger as well as the current logger
func (l *TuiLogger) Stack(next logger.Logger) logger.Logger {
	return l
}

var eol = []byte("\n")
var ansiColorStripper = regexp.MustCompile("\x1b\\[[0-9;]*[mK]")

func (l *TuiLogger) Write(p []byte) (n int, err error) {
	trimmed := bytes.Split(p, eol)
	for _, line := range trimmed {
		if len(line) == 0 {
			continue
		}
		log := string(line)
		if len(log) > 20 {
			prefix := ansiColorStripper.ReplaceAllString(log[:20], "")
			if logger.LevelTrace < l.logLevel && strings.HasPrefix(prefix, "[TRACE]") {
				continue
			}
			if logger.LevelDebug < l.logLevel && strings.HasPrefix(prefix, "[DEBUG]") {
				continue
			}
			if logger.LevelInfo < l.logLevel && strings.HasPrefix(prefix, "[INFO]") {
				continue
			}
			if logger.LevelWarn < l.logLevel && strings.HasPrefix(prefix, "[WARN]") {
				continue
			}
			if logger.LevelError < l.logLevel && strings.HasPrefix(prefix, "[ERROR]") {
				continue
			}
			continue
		}
		l.ui.AddLog("%s", log)
	}
	return len(p), nil
}
