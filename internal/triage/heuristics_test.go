package triage

import (
	"testing"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/model"
)

func TestBaseScore(t *testing.T) {
	weights := config.DefaultScoreWeights()
	h := NewHeuristics("testuser", weights, config.DefaultQuickWinLabels())

	tests := []struct {
		reason model.NotificationReason
		want   int
	}{
		{model.ReasonReviewRequested, 100},
		{model.ReasonMention, 90},
		{model.ReasonTeamMention, 85},
		{model.ReasonAuthor, 70},
		{model.ReasonAssign, 60},
		{model.ReasonComment, 30},
		{model.ReasonStateChange, 25},
		{model.ReasonSubscribed, 10},
		{model.ReasonCIActivity, 5},
		// Unknown reason should default to Subscribed weight
		{model.NotificationReason("unknown"), 10},
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
		details *model.ItemDetails
		want    bool
	}{
		{
			name: "matches good first issue label",
			details: &model.ItemDetails{
				Labels: []string{"good first issue"},
			},
			want: true,
		},
		{
			name: "matches label case insensitive",
			details: &model.ItemDetails{
				Labels: []string{"Good First Issue"},
			},
			want: true,
		},
		{
			name: "matches hyphenated label with space-separated config",
			details: &model.ItemDetails{
				Labels: []string{"good-first-issue"},
			},
			want: true,
		},
		{
			name: "matches help-wanted with hyphens",
			details: &model.ItemDetails{
				Labels: []string{"Help-Wanted"},
			},
			want: true,
		},
		{
			name: "matches documentation label",
			details: &model.ItemDetails{
				Labels: []string{"Documentation"},
			},
			want: true,
		},
		{
			name: "matches typo label",
			details: &model.ItemDetails{
				Labels: []string{"typo"},
			},
			want: true,
		},
		{
			name: "small PR is low hanging fruit",
			details: &model.ItemDetails{
				IsPR:         true,
				ChangedFiles: 2,
				Additions:    30,
				Deletions:    10,
			},
			want: true,
		},
		{
			name: "large PR is not low hanging fruit",
			details: &model.ItemDetails{
				IsPR:         true,
				ChangedFiles: 10,
				Additions:    500,
				Deletions:    200,
			},
			want: false,
		},
		{
			name: "no matching labels",
			details: &model.ItemDetails{
				Labels: []string{"bug", "priority:high"},
			},
			want: false,
		},
		{
			name: "empty labels and not small PR",
			details: &model.ItemDetails{
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
		notification *model.Item
		score        int
		want         PriorityLevel
	}{
		{
			name: "review_requested is urgent",
			notification: &model.Item{
				Reason: model.ReasonReviewRequested,
			},
			score: 0,
			want:  PriorityUrgent,
		},
		{
			name: "mention is FYI by default (urgency trigger disabled)",
			notification: &model.Item{
				Reason: model.ReasonMention,
			},
			score: 0,
			want:  PriorityFYI,
		},
		{
			name: "authored PR approved and mergeable is urgent",
			notification: &model.Item{
				Reason: model.ReasonAuthor,
				Details: &model.ItemDetails{
					IsPR:         true,
					ReviewState:  "approved",
					Mergeable:    true,
					ChangedFiles: 10,
					Additions:    200,
					Deletions:    100,
				},
			},
			score: 0,
			want:  PriorityUrgent,
		},
		{
			name: "authored PR with changes requested is important by default (urgency trigger disabled)",
			notification: &model.Item{
				Reason: model.ReasonAuthor,
				Details: &model.ItemDetails{
					IsPR:         true,
					ReviewState:  "changes_requested",
					ChangedFiles: 10,
					Additions:    200,
					Deletions:    100,
				},
			},
			score: 0,
			want:  PriorityImportant,
		},
		{
			name: "low hanging fruit is quick win",
			notification: &model.Item{
				Reason: model.ReasonSubscribed,
				Details: &model.ItemDetails{
					Labels: []string{"good first issue"},
				},
			},
			score: 0,
			want:  PriorityQuickWin,
		},
		{
			name: "author without urgent details is important",
			notification: &model.Item{
				Reason: model.ReasonAuthor,
				Details: &model.ItemDetails{
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
			notification: &model.Item{
				Reason: model.ReasonAssign,
			},
			score: 0,
			want:  PriorityImportant,
		},
		{
			name: "team_mention is important",
			notification: &model.Item{
				Reason: model.ReasonTeamMention,
			},
			score: 0,
			want:  PriorityImportant,
		},
		{
			name: "subscribed is FYI",
			notification: &model.Item{
				Reason: model.ReasonSubscribed,
			},
			score: 0,
			want:  PriorityFYI,
		},
		{
			name: "comment is FYI",
			notification: &model.Item{
				Reason: model.ReasonComment,
			},
			score: 0,
			want:  PriorityFYI,
		},
		{
			name: "high score promoted to important (notable threshold)",
			notification: &model.Item{
				Reason: model.ReasonSubscribed,
			},
			score: 60, // meets notable promotion threshold
			want:  PriorityImportant,
		},
		{
			name: "medium score promoted to notable (fyi threshold)",
			notification: &model.Item{
				Reason: model.ReasonSubscribed,
			},
			score: 35, // meets fyi promotion threshold
			want:  PriorityNotable,
		},
		{
			name: "below threshold FYI stays FYI",
			notification: &model.Item{
				Reason: model.ReasonSubscribed,
			},
			score: 34, // below fyi promotion threshold
			want:  PriorityFYI,
		},
		{
			name: "very high score promoted to urgent (important threshold)",
			notification: &model.Item{
				Reason: model.ReasonSubscribed,
			},
			score: 100, // meets important promotion threshold
			want:  PriorityUrgent,
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

func TestDeterminePriorityWithDisabledUrgencyTriggers(t *testing.T) {
	// Create weights with urgency triggers disabled
	weights := config.DefaultScoreWeights()
	weights.ReviewRequestedIsUrgent = false
	weights.MentionIsUrgent = false
	weights.ApprovedMergeablePRIsUrgent = false
	weights.ChangesRequestedPRIsUrgent = false

	h := NewHeuristics("testuser", weights, config.DefaultQuickWinLabels())

	tests := []struct {
		name         string
		notification *model.Item
		score        int
		want         PriorityLevel
	}{
		{
			name: "review_requested falls through when disabled (low score)",
			notification: &model.Item{
				Reason: model.ReasonReviewRequested,
			},
			score: 0,
			want:  PriorityFYI, // Falls through to score-based, low score = FYI
		},
		{
			name: "review_requested promoted by high score when disabled",
			notification: &model.Item{
				Reason: model.ReasonReviewRequested,
			},
			score: 100, // meets important promotion threshold
			want:  PriorityUrgent,
		},
		{
			name: "mention falls through when disabled (low score)",
			notification: &model.Item{
				Reason: model.ReasonMention,
			},
			score: 0,
			want:  PriorityFYI,
		},
		{
			name: "mention promoted by score when disabled",
			notification: &model.Item{
				Reason: model.ReasonMention,
			},
			score: 60, // meets notable promotion threshold
			want:  PriorityImportant,
		},
		{
			name: "authored PR approved+mergeable falls through when disabled",
			notification: &model.Item{
				Reason: model.ReasonAuthor,
				Details: &model.ItemDetails{
					IsPR:         true,
					ReviewState:  "approved",
					Mergeable:    true,
					ChangedFiles: 10, // Large PR to avoid quick-win
					Additions:    500,
					Deletions:    100,
				},
			},
			score: 0,
			want:  PriorityImportant, // Falls through to reason-based (Author = Important)
		},
		{
			name: "authored PR changes_requested falls through when disabled",
			notification: &model.Item{
				Reason: model.ReasonAuthor,
				Details: &model.ItemDetails{
					IsPR:         true,
					ReviewState:  "changes_requested",
					ChangedFiles: 10, // Large PR to avoid quick-win
					Additions:    500,
					Deletions:    100,
				},
			},
			score: 0,
			want:  PriorityImportant, // Falls through to reason-based (Author = Important)
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

func TestDeterminePriorityWithPartialUrgencyOverrides(t *testing.T) {
	// Test with some triggers modified from defaults
	weights := config.DefaultScoreWeights()
	weights.MentionIsUrgent = true              // Enable mention (default: false)
	weights.ApprovedMergeablePRIsUrgent = false // Disable approved PR (default: true)

	h := NewHeuristics("testuser", weights, config.DefaultQuickWinLabels())

	t.Run("mention urgent when enabled", func(t *testing.T) {
		notification := &model.Item{
			Reason: model.ReasonMention,
		}
		got := h.DeterminePriority(notification, 0)
		if got != PriorityUrgent {
			t.Errorf("DeterminePriority() = %v, want %v (mention should be urgent when enabled)", got, PriorityUrgent)
		}
	})

	t.Run("review_requested still urgent (default)", func(t *testing.T) {
		notification := &model.Item{
			Reason: model.ReasonReviewRequested,
		}
		got := h.DeterminePriority(notification, 0)
		if got != PriorityUrgent {
			t.Errorf("DeterminePriority() = %v, want %v (review_requested should be urgent)", got, PriorityUrgent)
		}
	})

	t.Run("approved PR falls through when disabled", func(t *testing.T) {
		notification := &model.Item{
			Reason: model.ReasonAuthor,
			Details: &model.ItemDetails{
				IsPR:         true,
				ReviewState:  "approved",
				Mergeable:    true,
				ChangedFiles: 10,
				Additions:    200,
				Deletions:    100,
			},
		}
		got := h.DeterminePriority(notification, 0)
		if got != PriorityImportant {
			t.Errorf("DeterminePriority() = %v, want %v (approved PR should fall through to Important when disabled)", got, PriorityImportant)
		}
	})

	t.Run("changes_requested PR not urgent (still disabled)", func(t *testing.T) {
		notification := &model.Item{
			Reason: model.ReasonAuthor,
			Details: &model.ItemDetails{
				IsPR:         true,
				ReviewState:  "changes_requested",
				ChangedFiles: 10,
				Additions:    200,
				Deletions:    100,
			},
		}
		got := h.DeterminePriority(notification, 0)
		if got != PriorityImportant {
			t.Errorf("DeterminePriority() = %v, want %v (changes_requested PR should fall through to Important)", got, PriorityImportant)
		}
	})
}

func TestDetermineAction(t *testing.T) {
	h := NewHeuristics("testuser", config.DefaultScoreWeights(), config.DefaultQuickWinLabels())

	tests := []struct {
		name         string
		notification *model.Item
		want         string
	}{
		{
			name: "review_requested",
			notification: &model.Item{
				Reason: model.ReasonReviewRequested,
			},
			want: "Review PR",
		},
		{
			name: "mention",
			notification: &model.Item{
				Reason: model.ReasonMention,
			},
			want: "Respond to mention",
		},
		{
			name: "team_mention",
			notification: &model.Item{
				Reason: model.ReasonTeamMention,
			},
			want: "Team mentioned - check if relevant",
		},
		{
			name: "assign",
			notification: &model.Item{
				Reason: model.ReasonAssign,
			},
			want: "Work on assigned item",
		},
		{
			name: "comment",
			notification: &model.Item{
				Reason: model.ReasonComment,
			},
			want: "Review new comments",
		},
		{
			name: "subscribed",
			notification: &model.Item{
				Reason: model.ReasonSubscribed,
			},
			want: "Review activity (subscribed)",
		},
		{
			name: "state_change closed",
			notification: &model.Item{
				Reason: model.ReasonStateChange,
				Details: &model.ItemDetails{
					State: "closed",
				},
			},
			want: "Acknowledge closure",
		},
		{
			name: "state_change open",
			notification: &model.Item{
				Reason: model.ReasonStateChange,
				Details: &model.ItemDetails{
					State: "open",
				},
			},
			want: "Check state change",
		},
		{
			name: "author without details",
			notification: &model.Item{
				Reason: model.ReasonAuthor,
			},
			want: "Check activity on your item",
		},
		{
			name: "author with approved mergeable PR",
			notification: &model.Item{
				Reason: model.ReasonAuthor,
				Details: &model.ItemDetails{
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
