package dev

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

type pendingLog struct {
	level logger.LogLevel
	msg   string
}

type PendingLogger struct {
	pending  []pendingLog
	logLevel logger.LogLevel
	logger   logger.Logger
	mutex    sync.RWMutex
}

var _ logger.Logger = (*PendingLogger)(nil)

func NewPendingLogger(logLevel logger.LogLevel) *PendingLogger {
	return &PendingLogger{
		pending:  make([]pendingLog, 0),
		logLevel: logLevel,
	}
}

func (l *PendingLogger) drain(ui *DevModeUI, logger logger.Logger) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, val := range l.pending {
		ui.AddLog(val.level, "%s", val.msg)
	}
	l.logger = logger
	l.pending = nil
}

// With will return a new logger using metadata as the base context
func (l *PendingLogger) With(metadata map[string]interface{}) logger.Logger {
	return l
}

// WithPrefix will return a new logger with a prefix prepended to the message
func (l *PendingLogger) WithPrefix(prefix string) logger.Logger {
	return l
}

// WithContext will return a new logger with the given context
func (l *PendingLogger) WithContext(ctx context.Context) logger.Logger {
	return l
}

// Trace level logging
func (l *PendingLogger) Trace(msg string, args ...interface{}) {
	if logger.LevelTrace < l.logLevel {
		return
	}
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.logger != nil {
		l.logger.Trace(msg, args...)
		return
	}
	val := pendingLog{
		level: logger.LevelTrace,
		msg:   fmt.Sprintf(msg, args...),
	}
	l.pending = append(l.pending, val)
}

// Debug level logging
func (l *PendingLogger) Debug(msg string, args ...interface{}) {
	if logger.LevelDebug < l.logLevel {
		return
	}
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.logger != nil {
		l.logger.Debug(msg, args...)
		return
	}
	val := pendingLog{
		level: logger.LevelDebug,
		msg:   fmt.Sprintf(msg, args...),
	}
	l.pending = append(l.pending, val)
}

// Info level loggi	ng
func (l *PendingLogger) Info(msg string, args ...interface{}) {
	if logger.LevelInfo < l.logLevel {
		return
	}
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.logger != nil {
		l.logger.Info(msg, args...)
		return
	}
	val := pendingLog{
		level: logger.LevelInfo,
		msg:   fmt.Sprintf(msg, args...),
	}
	l.pending = append(l.pending, val)
}

// Warning level logging
func (l *PendingLogger) Warn(msg string, args ...interface{}) {
	if logger.LevelWarn < l.logLevel {
		return
	}
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.logger != nil {
		l.logger.Warn(msg, args...)
		return
	}
	val := pendingLog{
		level: logger.LevelWarn,
		msg:   fmt.Sprintf(msg, args...),
	}
	l.pending = append(l.pending, val)
}

// Error level logging
func (l *PendingLogger) Error(msg string, args ...interface{}) {
	if logger.LevelError < l.logLevel {
		return
	}
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.logger != nil {
		l.logger.Error(msg, args...)
		return
	}
	val := pendingLog{
		level: logger.LevelError,
		msg:   fmt.Sprintf(msg, args...),
	}
	l.pending = append(l.pending, val)
}

// Fatal level logging and exit with code 1
func (l *PendingLogger) Fatal(msg string, args ...interface{}) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.logger != nil {
		l.logger.Fatal(msg, args...)
		return
	}
	val := tui.Bold("[FATAL] " + fmt.Sprintf(msg, args...))
	fmt.Println(val)
	os.Exit(1)
}

// Stack will return a new logger that logs to the given logger as well as the current logger
func (l *PendingLogger) Stack(next logger.Logger) logger.Logger {
	return l
}
