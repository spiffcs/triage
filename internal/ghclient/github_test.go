package ghclient

import (
	"context"
	"testing"
	"time"

	"github.com/spiffcs/triage/internal/model"
)

func TestItemTypes(t *testing.T) {
	// Test that notification types can be created
	n := model.Item{
		ID:        "123",
		Reason:    model.ReasonMention,
		Unread:    true,
		UpdatedAt: time.Now(),
		Repository: model.Repository{
			ID:       1,
			Name:     "test-repo",
			FullName: "owner/test-repo",
			Private:  false,
			HTMLURL:  "https://github.com/owner/test-repo",
		},
		Subject: model.Subject{
			Title: "Test Issue",
			URL:   "https://api.github.com/repos/owner/test-repo/issues/1",
			Type:  model.SubjectIssue,
		},
		URL: "https://api.github.com/notifications/threads/123",
	}

	if n.ID != "123" {
		t.Errorf("expected ID '123', got %q", n.ID)
	}
	if n.Reason != model.ReasonMention {
		t.Errorf("expected reason %q, got %q", model.ReasonMention, n.Reason)
	}
}

func TestItemWithPRDetails(t *testing.T) {
	item := model.Item{
		Type:         model.ItemTypePullRequest,
		Number:       42,
		State:        "open",
		HTMLURL:      "https://github.com/owner/repo/pull/42",
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		UpdatedAt:    time.Now(),
		Author:       "testuser",
		Assignees:    []string{"user1", "user2"},
		Labels:       []string{"bug", "help wanted"},
		CommentCount: 5,
		Details: &model.PRDetails{
			Additions:    100,
			Deletions:    50,
			ChangedFiles: 3,
			ReviewState:  "approved",
			Draft:        false,
		},
	}

	if item.Number != 42 {
		t.Errorf("expected number 42, got %d", item.Number)
	}
	if !item.IsPR() {
		t.Error("expected IsPR() to be true")
	}
	pr := item.PRDetails()
	if pr == nil {
		t.Fatal("expected PRDetails to be non-nil")
	}
	if pr.Additions != 100 {
		t.Errorf("expected additions 100, got %d", pr.Additions)
	}
}

func TestItemReasons(t *testing.T) {
	reasons := []model.ItemReason{
		model.ReasonMention,
		model.ReasonReviewRequested,
		model.ReasonAuthor,
		model.ReasonAssign,
		model.ReasonComment,
		model.ReasonSubscribed,
		model.ReasonTeamMention,
		model.ReasonStateChange,
		model.ReasonCIActivity,
		model.ReasonManual,
	}

	for _, reason := range reasons {
		if reason == "" {
			t.Error("notification reason should not be empty")
		}
	}
}

func TestSubjectTypes(t *testing.T) {
	types := []model.SubjectType{
		model.SubjectIssue,
		model.SubjectPullRequest,
		model.SubjectRelease,
		model.SubjectDiscussion,
	}

	for _, st := range types {
		if st == "" {
			t.Error("subject type should not be empty")
		}
	}
}

func TestRepoFromURL(t *testing.T) {
	tests := []struct {
		url           string
		expectedOwner string
		expectedRepo  string
	}{
		{"https://api.github.com/repos/owner/repo", "owner", "repo"},
		{"https://api.github.com/repos/org/project/issues/1", "org", "project"},
		{"", "", ""},
		{"https://api.github.com/repos/", "", ""},
		{"https://api.github.com/repos/owner", "", ""},
		{"https://other.com/repos/owner/repo", "", ""},
	}

	for _, tt := range tests {
		owner, repo := repoFromURL(tt.url)
		if owner != tt.expectedOwner || repo != tt.expectedRepo {
			t.Errorf("repoFromURL(%q): expected (%q, %q), got (%q, %q)",
				tt.url, tt.expectedOwner, tt.expectedRepo, owner, repo)
		}
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	// Don't actually modify env in tests - just test with empty string
	_, err := NewClient(context.Background(), "")
	if err == nil {
		t.Error("expected error when creating client without token")
	}
}

func TestRepository(t *testing.T) {
	repo := model.Repository{
		ID:       12345,
		Name:     "my-repo",
		FullName: "owner/my-repo",
		Private:  true,
		HTMLURL:  "https://github.com/owner/my-repo",
	}

	if repo.Name != "my-repo" {
		t.Errorf("expected name 'my-repo', got %q", repo.Name)
	}
	if !repo.Private {
		t.Error("expected Private to be true")
	}
}

func TestSubject(t *testing.T) {
	subject := model.Subject{
		Title: "Fix critical bug",
		URL:   "https://api.github.com/repos/owner/repo/issues/123",
		Type:  model.SubjectIssue,
	}

	if subject.Title != "Fix critical bug" {
		t.Errorf("expected title 'Fix critical bug', got %q", subject.Title)
	}
	if subject.Type != model.SubjectIssue {
		t.Errorf("expected type %q, got %q", model.SubjectIssue, subject.Type)
	}
}
