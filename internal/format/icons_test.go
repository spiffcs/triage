package format

import (
	"testing"
)

func TestIcon(t *testing.T) {
	tests := []struct {
		name     string
		input    IconOptions
		expected IconType
	}{
		{
			name:     "no icon when no conditions met",
			input:    IconOptions{},
			expected: IconNone,
		},
		{
			name: "hot topic for PR with high comments",
			input: IconOptions{
				CommentCount:      10,
				HotTopicThreshold: 5,
				IsPR:              true,
			},
			expected: IconHotTopic,
		},
		{
			name: "hot topic for issue with high comments, other commenter",
			input: IconOptions{
				CommentCount:      10,
				HotTopicThreshold: 5,
				IsPR:              false,
				LastCommenter:     "other-user",
				CurrentUser:       "me",
			},
			expected: IconHotTopic,
		},
		{
			name: "suppress hot topic for issue when current user is last commenter",
			input: IconOptions{
				CommentCount:      10,
				HotTopicThreshold: 5,
				IsPR:              false,
				LastCommenter:     "me",
				CurrentUser:       "me",
			},
			expected: IconNone,
		},
		{
			name: "PR does not suppress hot topic even when current user is last commenter",
			input: IconOptions{
				CommentCount:      10,
				HotTopicThreshold: 5,
				IsPR:              true,
				LastCommenter:     "me",
				CurrentUser:       "me",
			},
			expected: IconHotTopic,
		},
		{
			name: "quick win icon",
			input: IconOptions{
				IsQuickWin: true,
			},
			expected: IconQuickWin,
		},
		{
			name: "hot topic takes precedence over quick win",
			input: IconOptions{
				CommentCount:      10,
				HotTopicThreshold: 5,
				IsPR:              true,
				IsQuickWin:        true,
			},
			expected: IconHotTopic,
		},
		{
			name: "below threshold shows no icon",
			input: IconOptions{
				CommentCount:      3,
				HotTopicThreshold: 5,
			},
			expected: IconNone,
		},
		{
			name: "exactly at threshold shows no icon",
			input: IconOptions{
				CommentCount:      5,
				HotTopicThreshold: 5,
			},
			expected: IconNone,
		},
		{
			name: "threshold of 0 disables hot topic",
			input: IconOptions{
				CommentCount:      10,
				HotTopicThreshold: 0,
				IsPR:              true,
			},
			expected: IconNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Icon(tt.input)
			if got != tt.expected {
				t.Errorf("Icon() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIconConstants(t *testing.T) {
	// Verify icon constants are set correctly
	if HotTopicIcon != "🔥" {
		t.Errorf("HotTopicIcon = %q, want 🔥", HotTopicIcon)
	}

	if QuickWinIcon != "⚡\uFE0F" {
		t.Errorf("QuickWinIcon = %q, want ⚡️", QuickWinIcon)
	}

	if IconWidth != 3 {
		t.Errorf("IconWidth = %d, want 3", IconWidth)
	}
}
