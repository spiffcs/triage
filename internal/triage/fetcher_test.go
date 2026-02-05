package triage

import (
	"fmt"
	"testing"

	"github.com/spiffcs/triage/internal/model"
)

func makeFetchItem(repo string, number int, subjectType model.SubjectType, url string, hasDetails bool) model.Item {
	item := model.Item{
		Number: number,
		Repository: model.Repository{
			FullName: repo,
		},
		Subject: model.Subject{
			Type: subjectType,
			URL:  url,
		},
	}
	if hasDetails {
		switch subjectType {
		case model.SubjectPullRequest:
			item.Type = model.ItemTypePullRequest
			item.Details = &model.PRDetails{}
		case model.SubjectIssue:
			item.Type = model.ItemTypeIssue
			item.Details = &model.IssueDetails{}
		}
	}
	return item
}

func TestFetchResult_TotalFetched(t *testing.T) {
	tests := []struct {
		name   string
		result *FetchResult
		want   int
	}{
		{
			name: "sums all sources",
			result: &FetchResult{
				Notifications:  make([]model.Item, 3),
				ReviewPRs:      make([]model.Item, 2),
				AuthoredPRs:    make([]model.Item, 1),
				AssignedIssues: make([]model.Item, 4),
				AssignedPRs:    make([]model.Item, 0),
				Orphaned:       make([]model.Item, 5),
			},
			want: 15,
		},
		{
			name:   "empty result",
			result: &FetchResult{},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.TotalFetched(); got != tt.want {
				t.Errorf("TotalFetched() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFetchResult_Merge(t *testing.T) {
	tests := []struct {
		name      string
		result    *FetchResult
		wantLen   int
		wantStats MergeStats
	}{
		{
			name: "deduplicates by repo and number",
			result: &FetchResult{
				Notifications: []model.Item{
					makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
				},
				ReviewPRs: []model.Item{
					makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
					makeFetchItem("org/repo", 2, model.SubjectPullRequest, "url2", true),
				},
			},
			wantLen:   2,
			wantStats: MergeStats{ReviewPRsAdded: 1},
		},
		{
			name: "adds from multiple sources",
			result: &FetchResult{
				Notifications: []model.Item{
					makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
				},
				AuthoredPRs: []model.Item{
					makeFetchItem("org/repo", 10, model.SubjectPullRequest, "url10", true),
				},
				AssignedIssues: []model.Item{
					makeFetchItem("org/repo", 20, model.SubjectIssue, "url20", true),
				},
				AssignedPRs: []model.Item{
					makeFetchItem("org/repo", 30, model.SubjectPullRequest, "url30", true),
				},
			},
			wantLen: 4,
			wantStats: MergeStats{
				AuthoredPRsAdded:    1,
				AssignedIssuesAdded: 1,
				AssignedPRsAdded:    1,
			},
		},
		{
			name: "all sources combined",
			result: &FetchResult{
				Notifications: []model.Item{
					makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
				},
				ReviewPRs: []model.Item{
					makeFetchItem("org/repo", 2, model.SubjectPullRequest, "url2", true),
				},
				AuthoredPRs: []model.Item{
					makeFetchItem("org/repo", 3, model.SubjectPullRequest, "url3", true),
				},
				AssignedIssues: []model.Item{
					makeFetchItem("org/repo", 4, model.SubjectIssue, "url4", true),
				},
				AssignedPRs: []model.Item{
					makeFetchItem("org/repo", 5, model.SubjectPullRequest, "url5", true),
				},
				Orphaned: []model.Item{
					makeFetchItem("org/repo", 6, model.SubjectIssue, "url6", true),
				},
			},
			wantLen: 6,
			wantStats: MergeStats{
				ReviewPRsAdded:      1,
				AuthoredPRsAdded:    1,
				AssignedIssuesAdded: 1,
				AssignedPRsAdded:    1,
				OrphanedAdded:       1,
			},
		},
		{
			name:      "empty result",
			result:    &FetchResult{},
			wantLen:   0,
			wantStats: MergeStats{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, stats := tt.result.Merge()
			if len(merged) != tt.wantLen {
				t.Errorf("len(merged) = %d, want %d", len(merged), tt.wantLen)
			}
			if stats != tt.wantStats {
				t.Errorf("stats = %+v, want %+v", stats, tt.wantStats)
			}
		})
	}

	t.Run("skips nil details", func(t *testing.T) {
		r := &FetchResult{
			Notifications: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
			},
			ReviewPRs: []model.Item{
				makeFetchItem("org/repo", 99, model.SubjectPullRequest, "url99", false),
			},
		}

		merged, stats := r.Merge()
		if len(merged) != 1 {
			t.Errorf("len(merged) = %d, want 1", len(merged))
		}
		if stats.ReviewPRsAdded != 0 {
			t.Errorf("ReviewPRsAdded = %d, want 0", stats.ReviewPRsAdded)
		}
	})

	t.Run("does not mutate original notifications", func(t *testing.T) {
		r := &FetchResult{
			Notifications: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
			},
			ReviewPRs: []model.Item{
				makeFetchItem("org/repo", 2, model.SubjectPullRequest, "url2", true),
			},
		}

		merged, _ := r.Merge()
		if len(merged) != 2 {
			t.Errorf("len(merged) = %d, want 2", len(merged))
		}
		if len(r.Notifications) != 1 {
			t.Errorf("Merge() mutated Notifications: len = %d, want 1", len(r.Notifications))
		}
	})

	t.Run("orphaned dedup against all subject types", func(t *testing.T) {
		r := &FetchResult{
			Notifications: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
				makeFetchItem("org/repo", 2, model.SubjectIssue, "url2", true),
			},
			Orphaned: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
				makeFetchItem("org/repo", 2, model.SubjectIssue, "url2", true),
				makeFetchItem("org/repo", 3, model.SubjectIssue, "url3", true),
			},
		}

		merged, stats := r.Merge()
		if len(merged) != 3 {
			t.Errorf("len(merged) = %d, want 3", len(merged))
		}
		if stats.OrphanedAdded != 1 {
			t.Errorf("OrphanedAdded = %d, want 1", stats.OrphanedAdded)
		}
	})
}

func TestMergeItems(t *testing.T) {
	tests := []struct {
		name          string
		notifications []model.Item
		newItems      []model.Item
		subjectType   model.SubjectType
		wantLen       int
		wantAdded     int
	}{
		{
			name: "dedup by URL within same subject type",
			notifications: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
			},
			newItems: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
			},
			subjectType: model.SubjectPullRequest,
			wantLen:     1,
			wantAdded:   0,
		},
		{
			name: "does not dedup across subject types",
			notifications: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectIssue, "url1", true),
			},
			newItems: []model.Item{
				makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
			},
			subjectType: model.SubjectPullRequest,
			wantLen:     2,
			wantAdded:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, added := mergeItems(tt.notifications, tt.newItems, tt.subjectType)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			if added != tt.wantAdded {
				t.Errorf("added = %d, want %d", added, tt.wantAdded)
			}
		})
	}
}

func TestMergeOrphaned(t *testing.T) {
	notifications := []model.Item{
		makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true),
		makeFetchItem("org/repo", 2, model.SubjectIssue, "url2", true),
	}

	orphaned := []model.Item{
		makeFetchItem("org/repo", 1, model.SubjectPullRequest, "url1", true), // dup
		makeFetchItem("org/repo", 3, model.SubjectIssue, "url3", true),       // unique
	}

	got, added := mergeOrphaned(notifications, orphaned)
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
	if added != 1 {
		t.Errorf("added = %d, want 1", added)
	}

	// Verify the added item is the right one
	last := got[len(got)-1]
	want := fmt.Sprintf("%s#%d", "org/repo", 3)
	gotKey := fmt.Sprintf("%s#%d", last.Repository.FullName, last.Number)
	if gotKey != want {
		t.Errorf("last item = %s, want %s", gotKey, want)
	}
}
