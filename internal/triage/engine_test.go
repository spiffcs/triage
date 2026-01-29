package triage

import (
	"testing"

	"github.com/spiffcs/triage/internal/model"
)

// Helper to create a test item
func makeItem(id string, reason model.ItemReason, subjectType model.SubjectType, details *model.ItemDetails) model.Item {
	return makeItemWithRepo(id, reason, subjectType, details, "")
}

// Helper to create a test item with repo
func makeItemWithRepo(id string, reason model.ItemReason, subjectType model.SubjectType, details *model.ItemDetails, repo string) model.Item {
	return model.Item{
		ID:     id,
		Reason: reason,
		Subject: model.Subject{
			Type: subjectType,
		},
		Repository: model.Repository{
			FullName: repo,
		},
		Details: details,
	}
}

// Helper to create a prioritized item
func makePrioritizedItem(id string, reason model.ItemReason, subjectType model.SubjectType, priority PriorityLevel, details *model.ItemDetails) PrioritizedItem {
	return PrioritizedItem{
		Item:     makeItem(id, reason, subjectType, details),
		Priority: priority,
	}
}

func TestFilterByPriority(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("2", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil),
		makePrioritizedItem("3", model.ReasonMention, model.SubjectIssue, PriorityUrgent, nil),
		makePrioritizedItem("4", model.ReasonAuthor, model.SubjectPullRequest, PriorityImportant, nil),
	}

	tests := []struct {
		name     string
		priority PriorityLevel
		wantIDs  []string
	}{
		{
			name:     "filter urgent only",
			priority: PriorityUrgent,
			wantIDs:  []string{"1", "3"},
		},
		{
			name:     "filter FYI only",
			priority: PriorityFYI,
			wantIDs:  []string{"2"},
		},
		{
			name:     "filter important only",
			priority: PriorityImportant,
			wantIDs:  []string{"4"},
		},
		{
			name:     "no matches returns empty",
			priority: PriorityQuickWin,
			wantIDs:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByPriority(items, tt.priority)
			if len(got) != len(tt.wantIDs) {
				t.Errorf("FilterByPriority() returned %d items, want %d", len(got), len(tt.wantIDs))
				return
			}
			for i, item := range got {
				if item.Item.ID != tt.wantIDs[i] {
					t.Errorf("FilterByPriority()[%d].ID = %s, want %s", i, item.Item.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByReason(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("2", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil),
		makePrioritizedItem("3", model.ReasonMention, model.SubjectIssue, PriorityUrgent, nil),
		makePrioritizedItem("4", model.ReasonAuthor, model.SubjectPullRequest, PriorityImportant, nil),
	}

	tests := []struct {
		name    string
		reasons []model.ItemReason
		wantIDs []string
	}{
		{
			name:    "filter by single reason",
			reasons: []model.ItemReason{model.ReasonReviewRequested},
			wantIDs: []string{"1"},
		},
		{
			name:    "filter by multiple reasons",
			reasons: []model.ItemReason{model.ReasonReviewRequested, model.ReasonMention},
			wantIDs: []string{"1", "3"},
		},
		{
			name:    "empty reasons returns all",
			reasons: []model.ItemReason{},
			wantIDs: []string{"1", "2", "3", "4"},
		},
		{
			name:    "nil reasons returns all",
			reasons: nil,
			wantIDs: []string{"1", "2", "3", "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByReason(items, tt.reasons)
			if len(got) != len(tt.wantIDs) {
				t.Errorf("FilterByReason() returned %d items, want %d", len(got), len(tt.wantIDs))
				return
			}
			for i, item := range got {
				if item.Item.ID != tt.wantIDs[i] {
					t.Errorf("FilterByReason()[%d].ID = %s, want %s", i, item.Item.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterOutMerged(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonAuthor, model.SubjectPullRequest, PriorityImportant, &model.ItemDetails{Merged: true}),
		makePrioritizedItem("2", model.ReasonAuthor, model.SubjectPullRequest, PriorityImportant, &model.ItemDetails{Merged: false}),
		makePrioritizedItem("3", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil), // nil Details
	}

	got := FilterOutMerged(items)

	wantIDs := []string{"2", "3"}
	if len(got) != len(wantIDs) {
		t.Errorf("FilterOutMerged() returned %d items, want %d", len(got), len(wantIDs))
		return
	}
	for i, item := range got {
		if item.Item.ID != wantIDs[i] {
			t.Errorf("FilterOutMerged()[%d].ID = %s, want %s", i, item.Item.ID, wantIDs[i])
		}
	}
}

func TestFilterOutClosed(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonAuthor, model.SubjectPullRequest, PriorityImportant, &model.ItemDetails{State: "closed"}),
		makePrioritizedItem("2", model.ReasonAuthor, model.SubjectPullRequest, PriorityImportant, &model.ItemDetails{State: "merged"}),
		makePrioritizedItem("3", model.ReasonAuthor, model.SubjectPullRequest, PriorityImportant, &model.ItemDetails{State: "open"}),
		makePrioritizedItem("4", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil), // nil Details - should be kept
	}

	got := FilterOutClosed(items)

	wantIDs := []string{"3", "4"}
	if len(got) != len(wantIDs) {
		t.Errorf("FilterOutClosed() returned %d items, want %d", len(got), len(wantIDs))
		return
	}
	for i, item := range got {
		if item.Item.ID != wantIDs[i] {
			t.Errorf("FilterOutClosed()[%d].ID = %s, want %s", i, item.Item.ID, wantIDs[i])
		}
	}
}

// Helper to create a prioritized item with repo
func makePrioritizedItemWithRepo(id string, reason model.ItemReason, subjectType model.SubjectType, priority PriorityLevel, details *model.ItemDetails, repo string) PrioritizedItem {
	return PrioritizedItem{
		Item:     makeItemWithRepo(id, reason, subjectType, details, repo),
		Priority: priority,
	}
}

func TestFilterByRepo(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItemWithRepo("1", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, nil, "anchore/syft"),
		makePrioritizedItemWithRepo("2", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil, "anchore/grype"),
		makePrioritizedItemWithRepo("3", model.ReasonMention, model.SubjectPullRequest, PriorityUrgent, nil, "anchore/syft"),
		makePrioritizedItemWithRepo("4", model.ReasonAuthor, model.SubjectIssue, PriorityImportant, nil, "golang/go"),
	}

	tests := []struct {
		name    string
		repo    string
		wantIDs []string
	}{
		{
			name:    "filter by specific repo",
			repo:    "anchore/syft",
			wantIDs: []string{"1", "3"},
		},
		{
			name:    "filter by different repo",
			repo:    "anchore/grype",
			wantIDs: []string{"2"},
		},
		{
			name:    "empty repo returns all",
			repo:    "",
			wantIDs: []string{"1", "2", "3", "4"},
		},
		{
			name:    "non-matching repo returns empty",
			repo:    "nonexistent/repo",
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByRepo(items, tt.repo)
			if len(got) != len(tt.wantIDs) {
				t.Errorf("FilterByRepo() returned %d items, want %d", len(got), len(tt.wantIDs))
				return
			}
			for i, item := range got {
				if item.Item.ID != tt.wantIDs[i] {
					t.Errorf("FilterByRepo()[%d].ID = %s, want %s", i, item.Item.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByType(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("2", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil),
		makePrioritizedItem("3", model.ReasonMention, model.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("4", model.ReasonAuthor, model.SubjectIssue, PriorityImportant, nil),
	}

	tests := []struct {
		name        string
		subjectType model.SubjectType
		wantIDs     []string
	}{
		{
			name:        "filter PRs only",
			subjectType: model.SubjectPullRequest,
			wantIDs:     []string{"1", "3"},
		},
		{
			name:        "filter issues only",
			subjectType: model.SubjectIssue,
			wantIDs:     []string{"2", "4"},
		},
		{
			name:        "filter releases returns empty",
			subjectType: model.SubjectRelease,
			wantIDs:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByType(items, tt.subjectType)
			if len(got) != len(tt.wantIDs) {
				t.Errorf("FilterByType() returned %d items, want %d", len(got), len(tt.wantIDs))
				return
			}
			for i, item := range got {
				if item.Item.ID != tt.wantIDs[i] {
					t.Errorf("FilterByType()[%d].ID = %s, want %s", i, item.Item.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByExcludedAuthors(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{Author: "dependabot[bot]"}),
		makePrioritizedItem("2", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{Author: "renovate[bot]"}),
		makePrioritizedItem("3", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{Author: "human-user"}),
		makePrioritizedItem("4", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil), // nil Details - should be kept
	}

	tests := []struct {
		name            string
		excludedAuthors []string
		wantIDs         []string
	}{
		{
			name:            "filter dependabot",
			excludedAuthors: []string{"dependabot[bot]"},
			wantIDs:         []string{"2", "3", "4"},
		},
		{
			name:            "filter multiple bots",
			excludedAuthors: []string{"dependabot[bot]", "renovate[bot]"},
			wantIDs:         []string{"3", "4"},
		},
		{
			name:            "empty exclude list returns all",
			excludedAuthors: []string{},
			wantIDs:         []string{"1", "2", "3", "4"},
		},
		{
			name:            "nil exclude list returns all",
			excludedAuthors: nil,
			wantIDs:         []string{"1", "2", "3", "4"},
		},
		{
			name:            "non-matching author returns all",
			excludedAuthors: []string{"unknown-bot"},
			wantIDs:         []string{"1", "2", "3", "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByExcludedAuthors(items, tt.excludedAuthors)
			if len(got) != len(tt.wantIDs) {
				t.Errorf("FilterByExcludedAuthors() returned %d items, want %d", len(got), len(tt.wantIDs))
				return
			}
			for i, item := range got {
				if item.Item.ID != tt.wantIDs[i] {
					t.Errorf("FilterByExcludedAuthors()[%d].ID = %s, want %s", i, item.Item.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByGreenCI(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{CIStatus: "success"}),
		makePrioritizedItem("2", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{CIStatus: "failure"}),
		makePrioritizedItem("3", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{CIStatus: "pending"}),
		makePrioritizedItem("4", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{CIStatus: ""}),
		makePrioritizedItem("5", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, nil),                // nil Details - excluded
		makePrioritizedItem("6", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, &model.ItemDetails{CIStatus: ""}), // Issue - excluded
	}

	tests := []struct {
		name    string
		wantIDs []string
	}{
		{
			name:    "keeps only PRs with success CI",
			wantIDs: []string{"1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByGreenCI(items)
			if len(got) != len(tt.wantIDs) {
				t.Errorf("FilterByGreenCI() returned %d items, want %d", len(got), len(tt.wantIDs))
				return
			}
			for i, item := range got {
				if item.Item.ID != tt.wantIDs[i] {
					t.Errorf("FilterByGreenCI()[%d].ID = %s, want %s", i, item.Item.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterOutUnenriched(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, &model.ItemDetails{State: "open"}), // PR with Details - kept
		makePrioritizedItem("2", model.ReasonReviewRequested, model.SubjectPullRequest, PriorityUrgent, nil),                               // PR without Details - filtered
		makePrioritizedItem("3", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, &model.ItemDetails{State: "open"}),               // Issue with Details - kept
		makePrioritizedItem("4", model.ReasonSubscribed, model.SubjectIssue, PriorityFYI, nil),                                             // Issue without Details - filtered
		makePrioritizedItem("5", model.ReasonSubscribed, model.SubjectRelease, PriorityFYI, nil),                                           // Release without Details - kept (different type)
	}

	got := FilterOutUnenriched(items)

	wantIDs := []string{"1", "3", "5"}
	if len(got) != len(wantIDs) {
		t.Errorf("FilterOutUnenriched() returned %d items, want %d", len(got), len(wantIDs))
		return
	}
	for i, item := range got {
		if item.Item.ID != wantIDs[i] {
			t.Errorf("FilterOutUnenriched()[%d].ID = %s, want %s", i, item.Item.ID, wantIDs[i])
		}
	}
}
