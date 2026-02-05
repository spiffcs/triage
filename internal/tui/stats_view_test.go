package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/stats"
	"github.com/spiffcs/triage/internal/triage"
)

func TestRenderBar(t *testing.T) {
	tests := []struct {
		name     string
		entries  []barEntry
		maxWidth int
		wantSub  []string // substrings that should appear
	}{
		{
			name:     "empty entries",
			entries:  nil,
			maxWidth: 80,
			wantSub:  []string{"─"},
		},
		{
			name: "single entry",
			entries: []barEntry{
				{Label: "Urgent", Count: 10, Style: listUrgentStyle},
			},
			maxWidth: 80,
			wantSub:  []string{"Urgent", "10"},
		},
		{
			name: "multiple entries",
			entries: []barEntry{
				{Label: "A", Count: 5, Style: listUrgentStyle},
				{Label: "B", Count: 10, Style: listNotableStyle},
			},
			maxWidth: 80,
			wantSub:  []string{"A", "5", "B", "10"},
		},
		{
			name: "all zero counts",
			entries: []barEntry{
				{Label: "X", Count: 0, Style: listUrgentStyle},
			},
			maxWidth: 80,
			wantSub:  nil, // should be filtered out before reaching renderBars
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(renderBars(tt.entries, tt.maxWidth), "\n")
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("renderBars() = %q, want substring %q", got, sub)
				}
			}
		})
	}
}

func TestRenderSparkline(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		width  int
		want   int // expected length of output
	}{
		{
			name:   "empty",
			values: nil,
			width:  10,
			want:   0,
		},
		{
			name:   "single value",
			values: []float64{5},
			width:  10,
			want:   1,
		},
		{
			name:   "ascending values",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8},
			width:  8,
			want:   8,
		},
		{
			name:   "constant values",
			values: []float64{5, 5, 5, 5},
			width:  4,
			want:   4,
		},
		{
			name:   "more values than width",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			width:  5,
			want:   5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderSparkline(tt.values, tt.width)
			runeCount := len([]rune(got))
			if runeCount != tt.want {
				t.Errorf("renderSparkline() rune length = %d, want %d (got %q)", runeCount, tt.want, got)
			}
		})
	}
}

func TestRenderSparklineOrder(t *testing.T) {
	// Ascending values should produce ascending blocks
	values := []float64{0, 25, 50, 75, 100}
	got := []rune(renderSparkline(values, 5))

	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("expected ascending blocks, but got[%d]=%c < got[%d]=%c", i, got[i], i-1, got[i-1])
		}
	}
}

func TestComputeDistributions(t *testing.T) {
	now := time.Now()
	items := []triage.PrioritizedItem{
		{
			Item: model.Item{
				Type:       model.ItemTypePullRequest,
				CreatedAt:  now.Add(-2 * time.Hour),
				Repository: model.Repository{FullName: "org/frontend"},
				Details: &model.PRDetails{
					ReviewState: model.ReviewStateApproved,
					CIStatus:    model.CIStatusSuccess,
					Additions:   5,
					Deletions:   3,
				},
			},
			Priority: triage.PriorityUrgent,
		},
		{
			Item: model.Item{
				Type:       model.ItemTypePullRequest,
				CreatedAt:  now.Add(-48 * time.Hour),
				Repository: model.Repository{FullName: "org/backend"},
				Details: &model.PRDetails{
					ReviewState: model.ReviewStateChangesRequested,
					CIStatus:    model.CIStatusFailure,
					Additions:   100,
					Deletions:   50,
				},
			},
			Priority: triage.PriorityImportant,
		},
		{
			Item: model.Item{
				Type:       model.ItemTypeIssue,
				CreatedAt:  now.Add(-24 * 15 * time.Hour),
				Repository: model.Repository{FullName: "org/frontend"},
			},
			Priority: triage.PriorityFYI,
		},
	}

	m := ListModel{
		items:    items,
		prSizeXS: 10,
		prSizeS:  50,
		prSizeM:  200,
		prSizeL:  500,
	}

	d := computeDistributions(m)

	if d.totalCount != 3 {
		t.Errorf("totalCount = %d, want 3", d.totalCount)
	}
	if d.prCount != 2 {
		t.Errorf("prCount = %d, want 2", d.prCount)
	}
	if d.issueCount != 1 {
		t.Errorf("issueCount = %d, want 1", d.issueCount)
	}

	// Check priority distribution has non-empty entries
	if len(d.priority) == 0 {
		t.Error("expected non-empty priority distribution")
	}

	// Check age distribution
	if len(d.age) == 0 {
		t.Error("expected non-empty age distribution")
	}

	// Check review distribution
	if len(d.review) == 0 {
		t.Error("expected non-empty review distribution")
	}

	// Check CI distribution
	if len(d.ci) == 0 {
		t.Error("expected non-empty CI distribution")
	}

	// Check PR size distribution
	if len(d.prSize) == 0 {
		t.Error("expected non-empty PR size distribution")
	}

	// Check top repos (should have 2 unique repos)
	if len(d.topRepos) != 2 {
		t.Errorf("topRepos length = %d, want 2", len(d.topRepos))
	}
}

func TestComputeMedianAgeHours(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		items []triage.PrioritizedItem
		want  float64 // approximate (within 1 hour)
	}{
		{
			name:  "empty",
			items: nil,
			want:  0,
		},
		{
			name: "single item",
			items: []triage.PrioritizedItem{
				{Item: model.Item{CreatedAt: now.Add(-24 * time.Hour)}},
			},
			want: 24,
		},
		{
			name: "odd count",
			items: []triage.PrioritizedItem{
				{Item: model.Item{CreatedAt: now.Add(-12 * time.Hour)}},
				{Item: model.Item{CreatedAt: now.Add(-24 * time.Hour)}},
				{Item: model.Item{CreatedAt: now.Add(-48 * time.Hour)}},
			},
			want: 24,
		},
		{
			name: "even count",
			items: []triage.PrioritizedItem{
				{Item: model.Item{CreatedAt: now.Add(-12 * time.Hour)}},
				{Item: model.Item{CreatedAt: now.Add(-24 * time.Hour)}},
				{Item: model.Item{CreatedAt: now.Add(-36 * time.Hour)}},
				{Item: model.Item{CreatedAt: now.Add(-48 * time.Hour)}},
			},
			want: 30, // average of 24 and 36
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeMedianAgeHours(tt.items)
			diff := got - tt.want
			if diff < -1 || diff > 1 {
				t.Errorf("computeMedianAgeHours() = %.1f, want ~%.1f", got, tt.want)
			}
		})
	}
}

func TestFilterZero(t *testing.T) {
	entries := []barEntry{
		{Label: "A", Count: 5},
		{Label: "B", Count: 0},
		{Label: "C", Count: 3},
	}
	got := filterZero(entries)
	if len(got) != 2 {
		t.Fatalf("filterZero returned %d entries, want 2", len(got))
	}
	if got[0].Label != "A" || got[1].Label != "C" {
		t.Errorf("filterZero returned wrong labels: %v, %v", got[0].Label, got[1].Label)
	}
}

func TestResampleValues(t *testing.T) {
	// Values shorter than width should be returned as-is
	short := []float64{1, 2, 3}
	got := resampleValues(short, 10)
	if len(got) != 3 {
		t.Fatalf("expected 3 values, got %d", len(got))
	}

	// Values longer than width should be resampled
	long := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got = resampleValues(long, 5)
	if len(got) != 5 {
		t.Fatalf("expected 5 values, got %d", len(got))
	}
	// First bucket should average {1,2} = 1.5
	if got[0] != 1.5 {
		t.Errorf("expected first resampled value 1.5, got %f", got[0])
	}
}

func TestFormatAgeDays(t *testing.T) {
	tests := []struct {
		hours float64
		want  string
	}{
		{0.5, "0.5h"},
		{12, "12.0h"},
		{24, "1.0d"},
		{48, "2.0d"},
		{168, "7.0d"},
	}
	for _, tt := range tests {
		got := formatAgeDays(tt.hours)
		if got != tt.want {
			t.Errorf("formatAgeDays(%f) = %q, want %q", tt.hours, got, tt.want)
		}
	}
}

func TestRenderStatsViewBasic(t *testing.T) {
	now := time.Now()
	items := []triage.PrioritizedItem{
		{
			Item: model.Item{
				Type:       model.ItemTypePullRequest,
				CreatedAt:  now.Add(-2 * time.Hour),
				Repository: model.Repository{FullName: "org/repo"},
				Details: &model.PRDetails{
					CIStatus: model.CIStatusSuccess,
				},
			},
			Priority: triage.PriorityUrgent,
		},
	}

	m := ListModel{
		items:        items,
		windowWidth:  120,
		windowHeight: 40,
		prSizeXS:     10,
		prSizeS:      50,
		prSizeM:      200,
		prSizeL:      500,
	}

	got := renderStatsView(m)

	// Should contain key sections
	for _, want := range []string{"1 total", "1 PRs", "0 issues", "Priority", "Age", "PR Review", "CI", "Top repos"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderStatsView() missing %q", want)
		}
	}
}

func TestRenderTrends(t *testing.T) {
	snapshots := []stats.Snapshot{
		{TotalCount: 100, OrphanedCount: 50, MedianAgeHours: 48},
		{TotalCount: 110, OrphanedCount: 55, MedianAgeHours: 52},
		{TotalCount: 105, OrphanedCount: 45, MedianAgeHours: 44},
	}

	got := renderTrends(snapshots)

	for _, want := range []string{"Trends", "Total", "Orphaned", "Median age"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderTrends() missing %q", want)
		}
	}

	// Should show latest values
	if !strings.Contains(got, "105") {
		t.Error("renderTrends() should show latest total count 105")
	}
}
