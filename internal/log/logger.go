package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Verbosity levels
const (
	LevelQuiet = iota // Default: only errors and warnings
	LevelInfo         // -v: progress messages, cache hits, counts
	LevelDebug        // -vv: API calls, cache operations, timing
	LevelTrace        // -vvv: full details, write errors
)

// Custom slog levels mapped to our verbosity
const (
	slogLevelTrace = slog.Level(-8) // Below debug
)

var (
	verbosity  int
	logger     *slog.Logger
	output     io.Writer
	inProgress bool // tracks if we have an in-progress line
)

// Initialize sets up the global logger with the specified verbosity level
func Initialize(level int, w io.Writer) {
	verbosity = level
	output = w

	// Map our verbosity to slog levels
	var slogLevel slog.Level
	switch {
	case level >= LevelTrace:
		slogLevel = slogLevelTrace
	case level >= LevelDebug:
		slogLevel = slog.LevelDebug
	case level >= LevelInfo:
		slogLevel = slog.LevelInfo
	default:
		slogLevel = slog.LevelWarn
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slogLevel,
	})
	logger = slog.New(handler)
}

// Info logs at info level (-v)
func Info(msg string, args ...any) {
	if verbosity >= LevelInfo {
		clearProgress()
		logger.Info(msg, args...)
	}
}

// Debug logs at debug level (-vv)
func Debug(msg string, args ...any) {
	if verbosity >= LevelDebug {
		clearProgress()
		logger.Debug(msg, args...)
	}
}

// Trace logs at trace level (-vvv)
func Trace(msg string, args ...any) {
	if verbosity >= LevelTrace {
		clearProgress()
		logger.Log(context.Background(), slogLevelTrace, msg, args...)
	}
}

// Warn logs at warn level (always visible)
func Warn(msg string, args ...any) {
	clearProgress()
	logger.Warn(msg, args...)
}

// Error logs at error level (always visible)
func Error(msg string, args ...any) {
	clearProgress()
	logger.Error(msg, args...)
}

// Progress prints a progress message with carriage return (no newline)
// Only shown at info level or higher
func Progress(format string, args ...any) {
	if verbosity >= LevelInfo {
		inProgress = true
		_, _ = fmt.Fprintf(output, "\r"+format, args...)
	}
}

// ProgressDone completes a progress line with "done" and newline
func ProgressDone() {
	if verbosity >= LevelInfo && inProgress {
		_, _ = fmt.Fprintln(output, " done")
		inProgress = false
	}
}

// ProgressClear clears the current progress line
func ProgressClear() {
	if inProgress {
		_, _ = fmt.Fprint(output, "\r\033[K") // carriage return + clear to end of line
		inProgress = false
	}
}

// clearProgress ensures we don't write over a progress line
func clearProgress() {
	if inProgress {
		_, _ = fmt.Fprintln(output) // just add a newline to preserve the progress
		inProgress = false
	}
}

// IsInfo returns true if info-level logging is enabled
func IsInfo() bool {
	return verbosity >= LevelInfo
}

// IsDebug returns true if debug-level logging is enabled
func IsDebug() bool {
	return verbosity >= LevelDebug
}

// IsTrace returns true if trace-level logging is enabled
func IsTrace() bool {
	return verbosity >= LevelTrace
}

// Verbosity returns the current verbosity level
func Verbosity() int {
	return verbosity
}

// SetOutput changes the output writer (useful for testing)
func SetOutput(w io.Writer) {
	output = w
}

func init() {
	// Default initialization with quiet mode to stderr
	output = os.Stderr
	verbosity = LevelQuiet
	logger = slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
}
