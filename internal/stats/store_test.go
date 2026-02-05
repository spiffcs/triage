package stats

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndRecent(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithPath(filepath.Join(dir, "stats.jsonl"))

	// Empty store returns nil
	got := s.Recent(10)
	if len(got) != 0 {
		t.Fatalf("expected 0 records, got %d", len(got))
	}

	// Append a snapshot
	snap := Snapshot{
		Timestamp:  time.Now(),
		TotalCount: 42,
		PRCount:    30,
		IssueCount: 12,
	}
	if err := s.Append(snap); err != nil {
		t.Fatal(err)
	}

	got = s.Recent(10)
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].TotalCount != 42 {
		t.Fatalf("expected TotalCount 42, got %d", got[0].TotalCount)
	}

	// Append another
	snap2 := Snapshot{
		Timestamp:  time.Now(),
		TotalCount: 50,
	}
	if err := s.Append(snap2); err != nil {
		t.Fatal(err)
	}

	got = s.Recent(10)
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if got[1].TotalCount != 50 {
		t.Fatalf("expected TotalCount 50, got %d", got[1].TotalCount)
	}
}

func TestRecentLimitsResults(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithPath(filepath.Join(dir, "stats.jsonl"))

	for i := range 10 {
		if err := s.Append(Snapshot{TotalCount: i}); err != nil {
			t.Fatal(err)
		}
	}

	got := s.Recent(3)
	if len(got) != 3 {
		t.Fatalf("expected 3 records, got %d", len(got))
	}
	// Should be the last 3 entries
	if got[0].TotalCount != 7 {
		t.Fatalf("expected TotalCount 7, got %d", got[0].TotalCount)
	}
	if got[2].TotalCount != 9 {
		t.Fatalf("expected TotalCount 9, got %d", got[2].TotalCount)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithPath(filepath.Join(dir, "stats.jsonl"))

	// Write maxRecords + 5 entries
	for i := range maxRecords + 5 {
		if err := s.Append(Snapshot{TotalCount: i}); err != nil {
			t.Fatal(err)
		}
	}

	got := s.Recent(maxRecords + 100)
	if len(got) != maxRecords {
		t.Fatalf("expected %d records after prune, got %d", maxRecords, len(got))
	}
	// First record should be the 6th one written (0-indexed: 5)
	if got[0].TotalCount != 5 {
		t.Fatalf("expected first record TotalCount 5, got %d", got[0].TotalCount)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.jsonl")

	// Write with one store instance
	s1 := NewStoreWithPath(path)
	if err := s1.Append(Snapshot{TotalCount: 99, MedianAgeHours: 48.5}); err != nil {
		t.Fatal(err)
	}

	// Read with a new store instance
	s2 := NewStoreWithPath(path)
	got := s2.Recent(10)
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].TotalCount != 99 {
		t.Fatalf("expected TotalCount 99, got %d", got[0].TotalCount)
	}
	if got[0].MedianAgeHours != 48.5 {
		t.Fatalf("expected MedianAgeHours 48.5, got %f", got[0].MedianAgeHours)
	}
}

func TestMissingFile(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "nonexistent", "stats.jsonl"))

	// Recent on non-existent file returns nil
	got := s.Recent(10)
	if len(got) != 0 {
		t.Fatalf("expected 0 records, got %d", len(got))
	}
}

func TestMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.jsonl")

	// Write some valid and invalid lines
	content := `{"ts":"2024-01-01T00:00:00Z","total":10}
not json at all
{"ts":"2024-01-02T00:00:00Z","total":20}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewStoreWithPath(path)
	got := s.Recent(10)
	if len(got) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(got))
	}
	if got[0].TotalCount != 10 {
		t.Fatalf("expected TotalCount 10, got %d", got[0].TotalCount)
	}
	if got[1].TotalCount != 20 {
		t.Fatalf("expected TotalCount 20, got %d", got[1].TotalCount)
	}
}
