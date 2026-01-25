package log

import (
	"bytes"
	"testing"
)

func TestInitialize(t *testing.T) {
	var buf bytes.Buffer
	Initialize(LevelInfo, &buf)

	if Verbosity() != LevelInfo {
		t.Errorf("expected verbosity %d, got %d", LevelInfo, Verbosity())
	}
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer

	// Test at debug level so all messages are captured
	Initialize(LevelTrace, &buf)

	// These should not panic
	Info("test info", "key", "value")
	Debug("test debug", "key", "value")
	Trace("test trace", "key", "value")
	Warn("test warn", "key", "value")
	Error("test error", "key", "value")

	if buf.Len() == 0 {
		t.Error("expected log output, got none")
	}
}

func TestLogLevelChecks(t *testing.T) {
	var buf bytes.Buffer

	Initialize(LevelDebug, &buf)

	if !IsInfo() {
		t.Error("expected IsInfo() to be true at debug level")
	}
	if !IsDebug() {
		t.Error("expected IsDebug() to be true at debug level")
	}
	if IsTrace() {
		t.Error("expected IsTrace() to be false at debug level")
	}
}

func TestProgress(t *testing.T) {
	var buf bytes.Buffer
	Initialize(LevelInfo, &buf)

	// These should not panic
	Progress("Loading %d%%", 50)
	ProgressDone()

	Progress("Another progress")
	ProgressClear()
}

func TestSetOutput(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	Initialize(LevelInfo, &buf1)
	Info("message 1")

	SetOutput(&buf2)
	Info("message 2")

	if buf1.Len() == 0 {
		t.Error("expected output in first buffer")
	}
}

func TestVerbosityLevels(t *testing.T) {
	tests := []struct {
		level    int
		isInfo   bool
		isDebug  bool
		isTrace  bool
	}{
		{LevelQuiet, false, false, false},
		{LevelInfo, true, false, false},
		{LevelDebug, true, true, false},
		{LevelTrace, true, true, true},
	}

	var buf bytes.Buffer
	for _, tt := range tests {
		Initialize(tt.level, &buf)

		if IsInfo() != tt.isInfo {
			t.Errorf("at level %d: expected IsInfo()=%v, got %v", tt.level, tt.isInfo, IsInfo())
		}
		if IsDebug() != tt.isDebug {
			t.Errorf("at level %d: expected IsDebug()=%v, got %v", tt.level, tt.isDebug, IsDebug())
		}
		if IsTrace() != tt.isTrace {
			t.Errorf("at level %d: expected IsTrace()=%v, got %v", tt.level, tt.isTrace, IsTrace())
		}
	}
}
