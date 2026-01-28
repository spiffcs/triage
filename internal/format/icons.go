package format

// IconType represents the type of icon to display for a notification.
type IconType int

const (
	// IconNone indicates no icon should be displayed.
	IconNone IconType = iota
	// IconHotTopic indicates a hot topic (fire emoji).
	IconHotTopic
	// IconQuickWin indicates a quick win (lightning emoji).
	IconQuickWin
)

// IconInput contains the fields needed to determine which icon to display.
type IconInput struct {
	CommentCount      int
	HotTopicThreshold int
	IsPR              bool
	LastCommenter     string
	CurrentUser       string
	IsQuickWin        bool
}

// DetermineIcon decides which icon (if any) should be displayed for an item.
// Hot topic (fire) takes precedence over quick win (lightning).
// For issues, hot topic is suppressed if the current user was the last commenter.
func DetermineIcon(input IconInput) IconType {
	// Check for hot topic first (fire takes precedence over quick win)
	if input.HotTopicThreshold > 0 && input.CommentCount > input.HotTopicThreshold {
		// Suppress for issues where current user was last commenter
		suppressForIssue := !input.IsPR && input.LastCommenter == input.CurrentUser
		if !suppressForIssue {
			return IconHotTopic
		}
	}

	// Quick win indicator (only if no fire icon)
	if input.IsQuickWin {
		return IconQuickWin
	}

	return IconNone
}

// Icon strings for display (renderers can apply their own styling)
const (
	// HotTopicIcon is the fire emoji for hot topics.
	HotTopicIcon = "\U0001F525" // 🔥

	// QuickWinIcon is the lightning emoji for quick wins.
	// Using U+26A1 + U+FE0F to force emoji presentation for consistent 2-column width.
	QuickWinIcon = "\u26A1\uFE0F" // ⚡️

	// IconWidth is the display width reserved for the icon column (emoji=2 + space=1).
	IconWidth = 3
)
