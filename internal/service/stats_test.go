package service

import (
	"testing"
	"time"
)

func TestFetchStats_AnyFromCache(t *testing.T) {
	tests := []struct {
		name  string
		stats FetchStats
		want  bool
	}{
		{"none cached", FetchStats{}, false},
		{"notif cached", FetchStats{NotifFromCache: true}, true},
		{"review cached", FetchStats{ReviewFromCache: true}, true},
		{"authored cached", FetchStats{AuthoredFromCache: true}, true},
		{"assigned cached", FetchStats{AssignedFromCache: true}, true},
		{"assigned PRs cached", FetchStats{AssignedPRsFromCache: true}, true},
		{"orphaned cached", FetchStats{OrphanedFromCache: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stats.AnyFromCache(); got != tt.want {
				t.Errorf("AnyFromCache() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecordCachedAt(t *testing.T) {
	t.Run("first call sets the value", func(t *testing.T) {
		svc := New(nil, nil, "testuser", time.Now())
		ts := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

		svc.recordCachedAt(ts)

		got := svc.Stats().OldestCachedAt
		if !got.Equal(ts) {
			t.Errorf("OldestCachedAt = %v, want %v", got, ts)
		}
	})

	t.Run("older time updates the value", func(t *testing.T) {
		svc := New(nil, nil, "testuser", time.Now())
		first := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
		older := first.Add(-1 * time.Hour)

		svc.recordCachedAt(first)
		svc.recordCachedAt(older)

		got := svc.Stats().OldestCachedAt
		if !got.Equal(older) {
			t.Errorf("OldestCachedAt = %v, want %v", got, older)
		}
	})

	t.Run("newer time does not update the value", func(t *testing.T) {
		svc := New(nil, nil, "testuser", time.Now())
		first := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
		older := first.Add(-1 * time.Hour)
		newer := first.Add(1 * time.Hour)

		svc.recordCachedAt(first)
		svc.recordCachedAt(older)
		svc.recordCachedAt(newer)

		got := svc.Stats().OldestCachedAt
		if !got.Equal(older) {
			t.Errorf("OldestCachedAt = %v, want oldest %v", got, older)
		}
	})
}

func TestFetchStats_CacheAge(t *testing.T) {
	t.Run("zero when not cached", func(t *testing.T) {
		s := FetchStats{}
		if got := s.CacheAge(); got != 0 {
			t.Errorf("CacheAge() = %v, want 0", got)
		}
	})

	t.Run("returns age when cached", func(t *testing.T) {
		s := FetchStats{
			OldestCachedAt: time.Now().Add(-5 * time.Minute),
		}
		age := s.CacheAge()
		if age < 4*time.Minute || age > 6*time.Minute {
			t.Errorf("CacheAge() = %v, want ~5m", age)
		}
	})
}
