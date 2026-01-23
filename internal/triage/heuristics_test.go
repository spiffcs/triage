package triage

import (
	"testing"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/github"
)

func TestBaseScore(t *testing.T) {
	weights := config.DefaultScoreWeights()
	h := NewHeuristics("testuser", weights, config.DefaultQuickWinLabels())

	tests := []struct {
		reason github.NotificationReason
		want   int
	}{
		{github.ReasonReviewRequested, 100},
		{github.ReasonMention, 90},
		{github.ReasonTeamMention, 85},
		{github.ReasonAuthor, 70},
		{github.ReasonAssign, 60},
		{github.ReasonComment, 30},
		{github.ReasonStateChange, 25},
		{github.ReasonSubscribed, 10},
		{github.ReasonCIActivity, 5},
		// Unknown reason should default to Subscribed weight
		{github.NotificationReason("unknown"), 10},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			got := h.baseScore(tt.reason)
			if got != tt.want {
				t.Errorf("baseScore(%s) = %d, want %d", tt.reason, got, tt.want)
			}
		})
	}
}

func TestIsLowHangingFruit(t *testing.T) {
	h := NewHeuristics("testuser", config.DefaultScoreWeights(), config.DefaultQuickWinLabels())

	tests := []struct {
		name    string
		details *github.ItemDetails
		want    bool
	}{
		{
			name: "matches good first issue label",
			details: &github.ItemDetails{
				Labels: []string{"good first issue"},
			},
			want: true,
		},
		{
			name: "matches label case insensitive",
			details: &github.ItemDetails{
				Labels: []string{"Good First Issue"},
			},
			want: true,
		},
		{
			name: "matches documentation label",
			details: &github.ItemDetails{
				Labels: []string{"Documentation"},
			},
			want: true,
		},
		{
			name: "matches typo label",
			details: &github.ItemDetails{
				Labels: []string{"typo"},
			},
			want: true,
		},
		{
			name: "small PR is low hanging fruit",
			details: &github.ItemDetails{
				IsPR:         true,
				ChangedFiles: 2,
				Additions:    30,
				Deletions:    10,
			},
			want: true,
		},
		{
			name: "large PR is not low hanging fruit",
			details: &github.ItemDetails{
				IsPR:         true,
				ChangedFiles: 10,
				Additions:    500,
				Deletions:    200,
			},
			want: false,
		},
		{
			name: "no matching labels",
			details: &github.ItemDetails{
				Labels: []string{"bug", "priority:high"},
			},
			want: false,
		},
		{
			name: "empty labels and not small PR",
			details: &github.ItemDetails{
				Labels: []string{},
				IsPR:   false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.isLowHangingFruit(tt.details)
			if got != tt.want {
				t.Errorf("isLowHangingFruit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeterminePriority(t *testing.T) {
	h := NewHeuristics("testuser", config.DefaultScoreWeights(), config.DefaultQuickWinLabels())

	tests := []struct {
		name         string
		notification *github.Notification
		score        int
		want         PriorityLevel
	}{
		{
			name: "review_requested is urgent",
			notification: &github.Notification{
				Reason: github.ReasonReviewRequested,
			},
			score: 0,
			want:  PriorityUrgent,
		},
		{
			name: "mention is urgent",
			notification: &github.Notification{
				Reason: github.ReasonMention,
			},
			score: 0,
			want:  PriorityUrgent,
		},
		{
			name: "authored PR approved and mergeable is urgent",
			notification: &github.Notification{
				Reason: github.ReasonAuthor,
				Details: &github.ItemDetails{
					IsPR:        true,
					ReviewState: "approved",
					Mergeable:   true,
				},
			},
			score: 0,
			want:  PriorityUrgent,
		},
		{
			name: "authored PR with changes requested is urgent",
			notification: &github.Notification{
				Reason: github.ReasonAuthor,
				Details: &github.ItemDetails{
					IsPR:        true,
					ReviewState: "changes_requested",
				},
			},
			score: 0,
			want:  PriorityUrgent,
		},
		{
			name: "low hanging fruit is quick win",
			notification: &github.Notification{
				Reason: github.ReasonSubscribed,
				Details: &github.ItemDetails{
					Labels: []string{"good first issue"},
				},
			},
			score: 0,
			want:  PriorityQuickWin,
		},
		{
			name: "author without urgent details is important",
			notification: &github.Notification{
				Reason: github.ReasonAuthor,
				Details: &github.ItemDetails{
					IsPR:         true,
					ReviewState:  "pending",
					ChangedFiles: 10,
					Additions:    200,
					Deletions:    100,
				},
			},
			score: 0,
			want:  PriorityImportant,
		},
		{
			name: "assign is important",
			notification: &github.Notification{
				Reason: github.ReasonAssign,
			},
			score: 0,
			want:  PriorityImportant,
		},
		{
			name: "team_mention is important",
			notification: &github.Notification{
				Reason: github.ReasonTeamMention,
			},
			score: 0,
			want:  PriorityImportant,
		},
		{
			name: "subscribed is FYI",
			notification: &github.Notification{
				Reason: github.ReasonSubscribed,
			},
			score: 0,
			want:  PriorityFYI,
		},
		{
			name: "comment is FYI",
			notification: &github.Notification{
				Reason: github.ReasonComment,
			},
			score: 0,
			want:  PriorityFYI,
		},
		{
			name: "high score FYI promoted to important",
			notification: &github.Notification{
				Reason: github.ReasonSubscribed,
			},
			score: 50, // meets threshold
			want:  PriorityImportant,
		},
		{
			name: "below threshold FYI stays FYI",
			notification: &github.Notification{
				Reason: github.ReasonSubscribed,
			},
			score: 49, // below threshold
			want:  PriorityFYI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.DeterminePriority(tt.notification, tt.score)
			if got != tt.want {
				t.Errorf("DeterminePriority() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetermineAction(t *testing.T) {
	h := NewHeuristics("testuser", config.DefaultScoreWeights(), config.DefaultQuickWinLabels())

	tests := []struct {
		name         string
		notification *github.Notification
		want         string
	}{
		{
			name: "review_requested",
			notification: &github.Notification{
				Reason: github.ReasonReviewRequested,
			},
			want: "Review PR",
		},
		{
			name: "mention",
			notification: &github.Notification{
				Reason: github.ReasonMention,
			},
			want: "Respond to mention",
		},
		{
			name: "team_mention",
			notification: &github.Notification{
				Reason: github.ReasonTeamMention,
			},
			want: "Team mentioned - check if relevant",
		},
		{
			name: "assign",
			notification: &github.Notification{
				Reason: github.ReasonAssign,
			},
			want: "Work on assigned item",
		},
		{
			name: "comment",
			notification: &github.Notification{
				Reason: github.ReasonComment,
			},
			want: "Review new comments",
		},
		{
			name: "subscribed",
			notification: &github.Notification{
				Reason: github.ReasonSubscribed,
			},
			want: "Review activity (subscribed)",
		},
		{
			name: "state_change closed",
			notification: &github.Notification{
				Reason: github.ReasonStateChange,
				Details: &github.ItemDetails{
					State: "closed",
				},
			},
			want: "Acknowledge closure",
		},
		{
			name: "state_change open",
			notification: &github.Notification{
				Reason: github.ReasonStateChange,
				Details: &github.ItemDetails{
					State: "open",
				},
			},
			want: "Check state change",
		},
		{
			name: "author without details",
			notification: &github.Notification{
				Reason: github.ReasonAuthor,
			},
			want: "Check activity on your item",
		},
		{
			name: "author with approved mergeable PR",
			notification: &github.Notification{
				Reason: github.ReasonAuthor,
				Details: &github.ItemDetails{
					IsPR:        true,
					ReviewState: "approved",
					Mergeable:   true,
				},
			},
			want: "Merge PR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.DetermineAction(tt.notification)
			if got != tt.want {
				t.Errorf("DetermineAction() = %q, want %q", got, tt.want)
			}
		})
	}
}
