package resolved

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spiffcs/triage/internal/log"
)

// ResolvedEntry represents when an item was marked as resolved
type ResolvedEntry struct {
	ResolvedAt time.Time `json:"resolvedAt"`
}

// Store manages persistence of resolved items
type Store struct {
	path    string
	entries map[string]ResolvedEntry
	mu      sync.RWMutex
}

// NewStore creates a new resolved items store
func NewStore() (*Store, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(cacheDir, "triage")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "resolved.json")
	s := &Store{
		path:    path,
		entries: make(map[string]ResolvedEntry),
	}

	if err := s.load(); err != nil {
		log.Debug("could not load resolved store, starting fresh", "error", err)
	}

	return s, nil
}

// load reads the resolved entries from disk
func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.entries)
}

// save writes the resolved entries to disk
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// Resolve marks an item as resolved with the given updatedAt timestamp
func (s *Store) Resolve(notificationID string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[notificationID] = ResolvedEntry{
		ResolvedAt: updatedAt,
	}

	return s.save()
}

// Unresolve removes an item from the resolved list
func (s *Store) Unresolve(notificationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, notificationID)
	return s.save()
}

// ShouldShow returns true if the item should be shown (not resolved or has new activity)
func (s *Store) ShouldShow(notificationID string, currentUpdatedAt time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.entries[notificationID]
	if !exists {
		return true
	}

	// Show if item has been updated since it was resolved
	return currentUpdatedAt.After(entry.ResolvedAt)
}

// IsResolved returns true if the item is currently marked as resolved
func (s *Store) IsResolved(notificationID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.entries[notificationID]
	return exists
}

// Count returns the number of resolved items
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.entries)
}
