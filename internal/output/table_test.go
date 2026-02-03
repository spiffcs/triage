package output

import (
	"strings"
	"testing"
	"time"

	"github.com/spiffcs/triage/internal/format"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/triage"
)

func TestHotTopicSuppression(t *testing.T) {
	tests := []struct {
		name           string
		isPR           bool
		commentCount   int
		lastCommenter  string
		currentUser    string
		threshold      int
		expectHotTopic bool
	}{
		{
			name:           "issue with high comments, other user last commenter - show hot topic",
			isPR:           false,
			commentCount:   10,
			lastCommenter:  "other-user",
			currentUser:    "me",
			threshold:      5,
			expectHotTopic: true,
		},
		{
			name:           "issue with high comments, current user last commenter - suppress hot topic",
			isPR:           false,
			commentCount:   10,
			lastCommenter:  "me",
			currentUser:    "me",
			threshold:      5,
			expectHotTopic: false,
		},
		{
			name:           "PR with high comments, current user last commenter - show hot topic (PRs don't suppress)",
			isPR:           true,
			commentCount:   10,
			lastCommenter:  "me",
			currentUser:    "me",
			threshold:      5,
			expectHotTopic: true,
		},
		{
			name:           "issue with comments below threshold - no hot topic",
			isPR:           false,
			commentCount:   3,
			lastCommenter:  "other-user",
			currentUser:    "me",
			threshold:      5,
			expectHotTopic: false,
		},
		{
			name:           "issue with high comments, no current user set - show hot topic",
			isPR:           false,
			commentCount:   10,
			lastCommenter:  "someone",
			currentUser:    "",
			threshold:      5,
			expectHotTopic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build test item
			item := triage.PrioritizedItem{
				Item: model.Item{
					Subject: model.Subject{
						Title: "Test issue title",
						Type:  model.SubjectIssue,
					},
					Repository: model.Repository{
						FullName: "owner/repo",
					},
					CommentCount: tt.commentCount,
				},
				Priority: triage.PriorityFYI,
			}
			if tt.isPR {
				item.Item.Type = model.ItemTypePullRequest
				item.Item.Details = &model.PRDetails{}
			} else {
				item.Item.Type = model.ItemTypeIssue
				item.Item.Details = &model.IssueDetails{
					LastCommenter: tt.lastCommenter,
				}
			}

			formatter := &TableFormatter{
				HotTopicThreshold: tt.threshold,
				CurrentUser:       tt.currentUser,
			}

			var buf strings.Builder
			err := formatter.Format([]triage.PrioritizedItem{item}, &buf)
			if err != nil {
				t.Fatalf("Format() error = %v", err)
			}

			output := buf.String()
			hasHotTopic := strings.Contains(output, "🔥")

			if hasHotTopic != tt.expectHotTopic {
				t.Errorf("hot topic indicator: got %v, want %v\nOutput:\n%s", hasHotTopic, tt.expectHotTopic, output)
			}
		})
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		// Now (sub-minute)
		{"zero", 0, "now"},
		{"30 seconds", 30 * time.Second, "now"},
		{"59 seconds", 59 * time.Second, "now"},

		// Minutes
		{"1 minute", time.Minute, "1m"},
		{"30 minutes", 30 * time.Minute, "30m"},
		{"59 minutes", 59 * time.Minute, "59m"},

		// Hours
		{"1 hour", time.Hour, "1h"},
		{"12 hours", 12 * time.Hour, "12h"},
		{"23 hours", 23 * time.Hour, "23h"},

		// Days
		{"1 day", 24 * time.Hour, "1d"},
		{"3 days", 3 * 24 * time.Hour, "3d"},
		{"6 days", 6 * 24 * time.Hour, "6d"},

		// Weeks
		{"7 days (1 week)", 7 * 24 * time.Hour, "1w"},
		{"14 days (2 weeks)", 14 * 24 * time.Hour, "2w"},
		{"21 days (3 weeks)", 21 * 24 * time.Hour, "3w"},
		{"28 days (4 weeks)", 28 * 24 * time.Hour, "4w"},
		{"29 days", 29 * 24 * time.Hour, "4w"},

		// Months - this is the bug fix: 28-29 days should NOT be "0mo"
		{"30 days (1 month)", 30 * 24 * time.Hour, "1mo"},
		{"60 days (2 months)", 60 * 24 * time.Hour, "2mo"},
		{"90 days (3 months)", 90 * 24 * time.Hour, "3mo"},
		{"365 days (12 months)", 365 * 24 * time.Hour, "12mo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := format.FormatAge(tt.duration)
			if got != tt.expected {
				t.Errorf("format.FormatAge(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestIconPrecedenceAndAlignment(t *testing.T) {
	tests := []struct {
		name       string
		isQuickWin bool
		isHotTopic bool
		expectFire bool
		expectBolt bool
	}{
		{
			name:       "quick win shows lightning bolt",
			isQuickWin: true,
			isHotTopic: false,
			expectFire: false,
			expectBolt: true,
		},
		{
			name:       "hot topic shows fire",
			isQuickWin: false,
			isHotTopic: true,
			expectFire: true,
			expectBolt: false,
		},
		{
			name:       "hot topic takes precedence over quick win",
			isQuickWin: true,
			isHotTopic: true,
			expectFire: true,
			expectBolt: false,
		},
		{
			name:       "no icon when neither condition",
			isQuickWin: false,
			isHotTopic: false,
			expectFire: false,
			expectBolt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := triage.PriorityFYI
			if tt.isQuickWin {
				priority = triage.PriorityQuickWin
			}

			commentCount := 0
			if tt.isHotTopic {
				commentCount = 10
			}

			item := triage.PrioritizedItem{
				Item: model.Item{
					Type: model.ItemTypePullRequest,
					Subject: model.Subject{
						Title: "Test title",
						Type:  model.SubjectPullRequest,
					},
					Repository: model.Repository{
						FullName: "owner/repo",
					},
					CommentCount: commentCount,
					Details:      &model.PRDetails{},
				},
				Priority: priority,
			}

			formatter := &TableFormatter{
				HotTopicThreshold: 5,
				CurrentUser:       "me",
			}

			var buf strings.Builder
			err := formatter.Format([]triage.PrioritizedItem{item}, &buf)
			if err != nil {
				t.Fatalf("Format() error = %v", err)
			}

			output := buf.String()
			hasFire := strings.Contains(output, "🔥")
			hasBolt := strings.Contains(output, "⚡")

			if hasFire != tt.expectFire {
				t.Errorf("fire icon: got %v, want %v", hasFire, tt.expectFire)
			}
			if hasBolt != tt.expectBolt {
				t.Errorf("bolt icon: got %v, want %v", hasBolt, tt.expectBolt)
			}
		})
	}
}
