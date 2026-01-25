package resolved

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	if store == nil {
		t.Fatal("NewStore() returned nil")
	}
}

func TestStoreOperations(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "triage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create store with custom path
	store := &Store{
		path:    filepath.Join(tmpDir, "resolved.json"),
		entries: make(map[string]ResolvedEntry),
	}

	// Test initial count
	if count := store.Count(); count != 0 {
		t.Errorf("expected initial count 0, got %d", count)
	}

	// Test Resolve
	notifID := "test-notification-123"
	resolvedAt := time.Now()
	if err := store.Resolve(notifID, resolvedAt); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Test IsResolved
	if !store.IsResolved(notifID) {
		t.Error("expected IsResolved() to return true")
	}

	// Test Count after resolve
	if count := store.Count(); count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Test ShouldShow with older timestamp (should not show)
	olderTime := resolvedAt.Add(-time.Hour)
	if store.ShouldShow(notifID, olderTime) {
		t.Error("expected ShouldShow() to return false for older timestamp")
	}

	// Test ShouldShow with newer timestamp (should show)
	newerTime := resolvedAt.Add(time.Hour)
	if !store.ShouldShow(notifID, newerTime) {
		t.Error("expected ShouldShow() to return true for newer timestamp")
	}

	// Test ShouldShow for unknown notification
	if !store.ShouldShow("unknown-id", time.Now()) {
		t.Error("expected ShouldShow() to return true for unknown notification")
	}

	// Test Unresolve
	if err := store.Unresolve(notifID); err != nil {
		t.Fatalf("Unresolve() error: %v", err)
	}

	if store.IsResolved(notifID) {
		t.Error("expected IsResolved() to return false after Unresolve()")
	}

	if count := store.Count(); count != 0 {
		t.Errorf("expected count 0 after unresolve, got %d", count)
	}
}

func TestResolvedEntry(t *testing.T) {
	entry := ResolvedEntry{
		ResolvedAt: time.Now(),
	}
	if entry.ResolvedAt.IsZero() {
		t.Error("expected non-zero ResolvedAt")
	}
}
