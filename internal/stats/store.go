package stats

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spiffcs/triage/internal/log"
)

// maxRecords is the maximum number of snapshots retained in the store.
const maxRecords = 1000

// Snapshot captures aggregate statistics from a single triage list run.
type Snapshot struct {
	Timestamp      time.Time `json:"ts"`
	TotalCount     int       `json:"total"`
	AssignedCount  int       `json:"assigned"`
	BlockedCount   int       `json:"blocked"`
	PriorityCount  int       `json:"priority"`
	OrphanedCount  int       `json:"orphaned"`
	PRCount        int       `json:"prs"`
	IssueCount     int       `json:"issues"`
	UrgentCount    int       `json:"urgent"`
	ImportantCount int       `json:"important"`
	QuickWinCount  int       `json:"quickWin"`
	NotableCount   int       `json:"notable"`
	FYICount       int       `json:"fyi"`
	MedianAgeHours float64   `json:"medianAgeH"`
	CISuccess      int       `json:"ciSuccess"`
	CIFailure      int       `json:"ciFailure"`
	CIPending      int       `json:"ciPending"`
}

// Store manages persistence of stats snapshots as JSON Lines.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore creates a new stats store at ~/.cache/triage/stats.jsonl.
func NewStore() (*Store, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(cacheDir, "triage")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	return &Store{
		path: filepath.Join(dir, "stats.jsonl"),
	}, nil
}

// NewStoreWithPath creates a store at the given path (for testing).
func NewStoreWithPath(path string) *Store {
	return &Store{path: path}
}

// Append adds a snapshot and prunes to the last maxRecords entries.
func (s *Store) Append(snap Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.readAll()
	if err != nil {
		log.Debug("could not read stats, starting fresh", "error", err)
		records = nil
	}

	records = append(records, snap)

	// Prune to last maxRecords
	if len(records) > maxRecords {
		records = records[len(records)-maxRecords:]
	}

	return s.writeAll(records)
}

// Recent returns the last n snapshots (or fewer if not enough exist).
func (s *Store) Recent(n int) []Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.readAll()
	if err != nil {
		return nil
	}

	if len(records) <= n {
		return records
	}
	return records[len(records)-n:]
}

// readAll reads all snapshots from disk.
func (s *Store) readAll() ([]Snapshot, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var records []Snapshot
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var snap Snapshot
		if err := json.Unmarshal(line, &snap); err != nil {
			continue // skip malformed lines
		}
		records = append(records, snap)
	}
	return records, scanner.Err()
}

// writeAll writes all snapshots to disk atomically.
func (s *Store) writeAll(records []Snapshot) error {
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, s.path)
}
