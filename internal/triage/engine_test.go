package triage

import (
	"testing"

	"github.com/spiffcs/triage/internal/github"
)

// Helper to create a test notification
func makeNotification(id string, reason github.NotificationReason, subjectType github.SubjectType, details *github.ItemDetails) github.Notification {
	return makeNotificationWithRepo(id, reason, subjectType, details, "")
}

// Helper to create a test notification with repo
func makeNotificationWithRepo(id string, reason github.NotificationReason, subjectType github.SubjectType, details *github.ItemDetails, repo string) github.Notification {
	return github.Notification{
		ID:     id,
		Reason: reason,
		Subject: github.Subject{
			Type: subjectType,
		},
		Repository: github.Repository{
			FullName: repo,
		},
		Details: details,
	}
}

// Helper to create a prioritized item
func makePrioritizedItem(id string, reason github.NotificationReason, subjectType github.SubjectType, priority PriorityLevel, details *github.ItemDetails) PrioritizedItem {
	return PrioritizedItem{
		Notification: makeNotification(id, reason, subjectType, details),
		Priority:     priority,
	}
}

func TestFilterByPriority(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("2", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil),
		makePrioritizedItem("3", github.ReasonMention, github.SubjectIssue, PriorityUrgent, nil),
		makePrioritizedItem("4", github.ReasonAuthor, github.SubjectPullRequest, PriorityImportant, nil),
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
				if item.Notification.ID != tt.wantIDs[i] {
					t.Errorf("FilterByPriority()[%d].ID = %s, want %s", i, item.Notification.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByReason(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("2", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil),
		makePrioritizedItem("3", github.ReasonMention, github.SubjectIssue, PriorityUrgent, nil),
		makePrioritizedItem("4", github.ReasonAuthor, github.SubjectPullRequest, PriorityImportant, nil),
	}

	tests := []struct {
		name    string
		reasons []github.NotificationReason
		wantIDs []string
	}{
		{
			name:    "filter by single reason",
			reasons: []github.NotificationReason{github.ReasonReviewRequested},
			wantIDs: []string{"1"},
		},
		{
			name:    "filter by multiple reasons",
			reasons: []github.NotificationReason{github.ReasonReviewRequested, github.ReasonMention},
			wantIDs: []string{"1", "3"},
		},
		{
			name:    "empty reasons returns all",
			reasons: []github.NotificationReason{},
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
				if item.Notification.ID != tt.wantIDs[i] {
					t.Errorf("FilterByReason()[%d].ID = %s, want %s", i, item.Notification.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterOutMerged(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonAuthor, github.SubjectPullRequest, PriorityImportant, &github.ItemDetails{Merged: true}),
		makePrioritizedItem("2", github.ReasonAuthor, github.SubjectPullRequest, PriorityImportant, &github.ItemDetails{Merged: false}),
		makePrioritizedItem("3", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil), // nil Details
	}

	got := FilterOutMerged(items)

	wantIDs := []string{"2", "3"}
	if len(got) != len(wantIDs) {
		t.Errorf("FilterOutMerged() returned %d items, want %d", len(got), len(wantIDs))
		return
	}
	for i, item := range got {
		if item.Notification.ID != wantIDs[i] {
			t.Errorf("FilterOutMerged()[%d].ID = %s, want %s", i, item.Notification.ID, wantIDs[i])
		}
	}
}

func TestFilterOutClosed(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonAuthor, github.SubjectPullRequest, PriorityImportant, &github.ItemDetails{State: "closed"}),
		makePrioritizedItem("2", github.ReasonAuthor, github.SubjectPullRequest, PriorityImportant, &github.ItemDetails{State: "merged"}),
		makePrioritizedItem("3", github.ReasonAuthor, github.SubjectPullRequest, PriorityImportant, &github.ItemDetails{State: "open"}),
		makePrioritizedItem("4", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil), // nil Details - should be kept
	}

	got := FilterOutClosed(items)

	wantIDs := []string{"3", "4"}
	if len(got) != len(wantIDs) {
		t.Errorf("FilterOutClosed() returned %d items, want %d", len(got), len(wantIDs))
		return
	}
	for i, item := range got {
		if item.Notification.ID != wantIDs[i] {
			t.Errorf("FilterOutClosed()[%d].ID = %s, want %s", i, item.Notification.ID, wantIDs[i])
		}
	}
}

// Helper to create a prioritized item with repo
func makePrioritizedItemWithRepo(id string, reason github.NotificationReason, subjectType github.SubjectType, priority PriorityLevel, details *github.ItemDetails, repo string) PrioritizedItem {
	return PrioritizedItem{
		Notification: makeNotificationWithRepo(id, reason, subjectType, details, repo),
		Priority:     priority,
	}
}

func TestFilterByRepo(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItemWithRepo("1", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, nil, "anchore/syft"),
		makePrioritizedItemWithRepo("2", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil, "anchore/grype"),
		makePrioritizedItemWithRepo("3", github.ReasonMention, github.SubjectPullRequest, PriorityUrgent, nil, "anchore/syft"),
		makePrioritizedItemWithRepo("4", github.ReasonAuthor, github.SubjectIssue, PriorityImportant, nil, "golang/go"),
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
				if item.Notification.ID != tt.wantIDs[i] {
					t.Errorf("FilterByRepo()[%d].ID = %s, want %s", i, item.Notification.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByType(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("2", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil),
		makePrioritizedItem("3", github.ReasonMention, github.SubjectPullRequest, PriorityUrgent, nil),
		makePrioritizedItem("4", github.ReasonAuthor, github.SubjectIssue, PriorityImportant, nil),
	}

	tests := []struct {
		name        string
		subjectType github.SubjectType
		wantIDs     []string
	}{
		{
			name:        "filter PRs only",
			subjectType: github.SubjectPullRequest,
			wantIDs:     []string{"1", "3"},
		},
		{
			name:        "filter issues only",
			subjectType: github.SubjectIssue,
			wantIDs:     []string{"2", "4"},
		},
		{
			name:        "filter releases returns empty",
			subjectType: github.SubjectRelease,
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
				if item.Notification.ID != tt.wantIDs[i] {
					t.Errorf("FilterByType()[%d].ID = %s, want %s", i, item.Notification.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByExcludedAuthors(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{Author: "dependabot[bot]"}),
		makePrioritizedItem("2", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{Author: "renovate[bot]"}),
		makePrioritizedItem("3", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{Author: "human-user"}),
		makePrioritizedItem("4", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil), // nil Details - should be kept
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
				if item.Notification.ID != tt.wantIDs[i] {
					t.Errorf("FilterByExcludedAuthors()[%d].ID = %s, want %s", i, item.Notification.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterByGreenCI(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{CIStatus: "success"}),
		makePrioritizedItem("2", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{CIStatus: "failure"}),
		makePrioritizedItem("3", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{CIStatus: "pending"}),
		makePrioritizedItem("4", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{CIStatus: ""}),
		makePrioritizedItem("5", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, nil),                 // nil Details - excluded
		makePrioritizedItem("6", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, &github.ItemDetails{CIStatus: ""}), // Issue - excluded
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
				if item.Notification.ID != tt.wantIDs[i] {
					t.Errorf("FilterByGreenCI()[%d].ID = %s, want %s", i, item.Notification.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterOutUnenriched(t *testing.T) {
	items := []PrioritizedItem{
		makePrioritizedItem("1", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, &github.ItemDetails{State: "open"}), // PR with Details - kept
		makePrioritizedItem("2", github.ReasonReviewRequested, github.SubjectPullRequest, PriorityUrgent, nil),                                // PR without Details - filtered
		makePrioritizedItem("3", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, &github.ItemDetails{State: "open"}),               // Issue with Details - kept
		makePrioritizedItem("4", github.ReasonSubscribed, github.SubjectIssue, PriorityFYI, nil),                                              // Issue without Details - filtered
		makePrioritizedItem("5", github.ReasonSubscribed, github.SubjectRelease, PriorityFYI, nil),                                            // Release without Details - kept (different type)
	}

	got := FilterOutUnenriched(items)

	wantIDs := []string{"1", "3", "5"}
	if len(got) != len(wantIDs) {
		t.Errorf("FilterOutUnenriched() returned %d items, want %d", len(got), len(wantIDs))
		return
	}
	for i, item := range got {
		if item.Notification.ID != wantIDs[i] {
			t.Errorf("FilterOutUnenriched()[%d].ID = %s, want %s", i, item.Notification.ID, wantIDs[i])
		}
	}
}
