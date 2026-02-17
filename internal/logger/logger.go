package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents a logging level
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// String returns the string representation of the log level
func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string into a Level
func ParseLevel(s string) Level {
	switch s {
	case "debug", "DEBUG":
		return DEBUG
	case "info", "INFO", "":
		return INFO
	case "warn", "WARN", "warning", "WARNING":
		return WARN
	case "error", "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// Logger is a structured logger with log levels
type Logger struct {
	level     Level
	component string
	output    io.Writer
	mu        sync.Mutex
}

// Config holds logger configuration
type Config struct {
	Level     string `yaml:"level"`     // debug, info, warn, error
	Component string // Component name for context
}

// defaultLogger is the package-level logger
var (
	defaultLogger = New(&Config{Level: "info", Component: "feelpulse"})
	defaultMu     sync.RWMutex
)

// New creates a new logger with the given configuration
func New(cfg *Config) *Logger {
	level := ParseLevel(cfg.Level)
	component := cfg.Component
	if component == "" {
		component = "feelpulse"
	}

	return &Logger{
		level:     level,
		component: component,
		output:    os.Stderr,
	}
}

// SetOutput sets the output writer
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = w
}

// SetLevel sets the minimum logging level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns the current logging level
func (l *Logger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// WithComponent returns a new logger with a different component name
func (l *Logger) WithComponent(component string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	return &Logger{
		level:     l.level,
		component: component,
		output:    l.output,
	}
}

// WithRequestID returns a new logger with request context
func (l *Logger) WithRequestID(requestID string) *ContextLogger {
	return &ContextLogger{
		logger:    l,
		requestID: requestID,
	}
}

// log writes a log message at the specified level
func (l *Logger) log(level Level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s [%s] %s\n", timestamp, level.String(), l.component, msg)
	l.output.Write([]byte(line))
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...any) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...any) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...any) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...any) {
	l.log(ERROR, format, args...)
}

// ContextLogger adds context (like request ID) to log messages
type ContextLogger struct {
	logger    *Logger
	requestID string
}

// log writes a log message with context
func (cl *ContextLogger) log(level Level, format string, args ...any) {
	cl.logger.mu.Lock()
	defer cl.logger.mu.Unlock()

	if level < cl.logger.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s [%s] [%s] %s\n",
		timestamp, level.String(), cl.logger.component, cl.requestID, msg)
	cl.logger.output.Write([]byte(line))
}

// Debug logs a debug message with context
func (cl *ContextLogger) Debug(format string, args ...any) {
	cl.log(DEBUG, format, args...)
}

// Info logs an info message with context
func (cl *ContextLogger) Info(format string, args ...any) {
	cl.log(INFO, format, args...)
}

// Warn logs a warning message with context
func (cl *ContextLogger) Warn(format string, args ...any) {
	cl.log(WARN, format, args...)
}

// Error logs an error message with context
func (cl *ContextLogger) Error(format string, args ...any) {
	cl.log(ERROR, format, args...)
}

// Package-level functions that use the default logger

// SetDefaultLogger sets the package-level default logger
func SetDefaultLogger(l *Logger) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultLogger = l
}

// GetDefaultLogger returns the package-level default logger
func GetDefaultLogger() *Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultLogger
}

// SetLevel sets the default logger's level
func SetLevel(level Level) {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	l.SetLevel(level)
}

// Debug logs a debug message using the default logger
func Debug(format string, args ...any) {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	l.Debug(format, args...)
}

// Info logs an info message using the default logger
func Info(format string, args ...any) {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	l.Info(format, args...)
}

// Warn logs a warning message using the default logger
func Warn(format string, args ...any) {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	l.Warn(format, args...)
}

// Error logs an error message using the default logger
func Error(format string, args ...any) {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	l.Error(format, args...)
}
