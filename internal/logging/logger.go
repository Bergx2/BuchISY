// Package logging provides a simple logging wrapper with levels and file rotation.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Level represents a log level.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// Logger wraps the standard logger with levels and file output.
type Logger struct {
	level      Level
	logger     *log.Logger
	file       *os.File
	filePrefix string
}

// New creates a new logger that writes to both stdout and a log file.
// logDir is the directory where log files are stored.
func New(logDir string, level Level) (*Logger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02")
	logFile := filepath.Join(logDir, fmt.Sprintf("buchisy_%s.log", timestamp))

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create multi-writer (stdout + file)
	multiWriter := io.MultiWriter(os.Stdout, file)

	logger := &Logger{
		level:      level,
		logger:     log.New(multiWriter, "", log.LstdFlags),
		file:       file,
		filePrefix: logFile,
	}

	return logger, nil
}

// Close closes the log file.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Debug logs a debug message.
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= DEBUG {
		l.log("DEBUG", format, args...)
	}
}

// Info logs an info message.
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= INFO {
		l.log("INFO", format, args...)
	}
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= WARN {
		l.log("WARN", format, args...)
	}
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= ERROR {
		l.log("ERROR", format, args...)
	}
}

// log formats and writes a log message.
func (l *Logger) log(levelStr, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.logger.Printf("[%s] %s", levelStr, message)
}

// SetLevel sets the log level.
func (l *Logger) SetLevel(level Level) {
	l.level = level
}
