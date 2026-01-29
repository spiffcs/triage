package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/format"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/triage"
	"golang.org/x/term"
)

// TableFormatter formats output as a terminal table
type TableFormatter struct {
	HotTopicThreshold int
	PRSizeXS          int
	PRSizeS           int
	PRSizeM           int
	PRSizeL           int
	CurrentUser       string
}

// hyperlink creates a clickable terminal hyperlink using OSC 8
// Format: \033]8;;URL\033\\TEXT\033]8;;\033\\
func hyperlink(text, url string) string {
	// Only use hyperlinks if stdout is a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return text
	}
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}

// Format outputs prioritized items as a table
func (f *TableFormatter) Format(items []triage.PrioritizedItem, w io.Writer) error {
	if len(items) == 0 {
		if _, err := fmt.Fprintln(w, "No notifications found."); err != nil {
			log.Trace("write error", "error", err)
		}
		return nil
	}

	// Header (↗ indicates column is clickable)
	if _, err := fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		constants.ColPriority, "Priority",
		constants.ColType, "Type",
		constants.ColAssigned, "Assigned",
		constants.ColRepo, "Repository ↗",
		constants.ColTitle, "Title ↗",
		constants.ColStatus, "Status",
		"Age"); err != nil {
		log.Trace("write error", "location", "header", "error", err)
	}
	separatorLen := constants.ColPriority + constants.ColType + constants.ColAssigned + constants.ColRepo + constants.ColTitle + constants.ColStatus + constants.ColAge + 14
	if _, err := fmt.Fprintln(w, strings.Repeat("-", separatorLen)); err != nil {
		log.Trace("write error", "location", "separator", "error", err)
	}

	for _, item := range items {
		n := item.Notification

		// Determine type indicator
		typeStr := "ISS"
		if (n.Details != nil && n.Details.IsPR) || n.Subject.Type == "PullRequest" {
			typeStr = "PR"
		}

		// Build title with icon prefix
		title := n.Subject.Title

		// Determine icon using shared logic
		var titleIcon string
		var iconDisplayWidth int

		iconInput := format.IconInput{
			HotTopicThreshold: f.HotTopicThreshold,
			IsQuickWin:        item.Priority == triage.PriorityQuickWin,
			CurrentUser:       f.CurrentUser,
		}
		if n.Details != nil {
			iconInput.CommentCount = n.Details.CommentCount
			iconInput.IsPR = n.Details.IsPR
			iconInput.LastCommenter = n.Details.LastCommenter
		}

		iconType := format.DetermineIcon(iconInput)
		switch iconType {
		case format.IconHotTopic:
			titleIcon = format.HotTopicIcon + " "
			iconDisplayWidth = format.IconWidth
		case format.IconQuickWin:
			titleIcon = color.YellowString(format.QuickWinIcon) + " "
			iconDisplayWidth = format.IconWidth
		default:
			titleIcon = "   " // 3 spaces
			iconDisplayWidth = format.IconWidth
		}

		// Truncate title to fit remaining space after icon
		title, visibleTitleLen := format.TruncateToWidth(title, constants.ColTitle-format.IconWidth)
		title = titleIcon + title
		visibleTitleLen += iconDisplayWidth

		// Add state indicator for closed items
		if n.Details != nil && (n.Details.State == "closed" || n.Details.Merged) {
			suffix := " [closed]"
			if n.Details.Merged {
				suffix = " [merged]"
			}
			suffixWidth := format.DisplayWidth(suffix)
			if visibleTitleLen+suffixWidth > constants.ColTitle {
				title, _ = format.TruncateToWidth(title, constants.ColTitle-suffixWidth)
			}
			title = title + suffix
			visibleTitleLen = format.DisplayWidth(title)
		}

		// Truncate repo if too long
		repo := n.Repository.FullName
		repo, visibleRepoLen := format.TruncateToWidth(repo, constants.ColRepo)

		// Create hyperlinked repo and pad it
		repoURL := n.Repository.HTMLURL
		if repoURL == "" {
			repoURL = fmt.Sprintf("https://github.com/%s", n.Repository.FullName)
		}
		linkedRepo := hyperlink(repo, repoURL)
		linkedRepo = format.PadRight(linkedRepo, visibleRepoLen, constants.ColRepo)

		// Get URL for title hyperlink
		titleURL := ""
		if n.Details != nil && n.Details.HTMLURL != "" {
			titleURL = n.Details.HTMLURL
		} else {
			titleURL = n.Repository.HTMLURL
		}

		// Create hyperlinked title and pad it
		linkedTitle := hyperlink(title, titleURL)
		linkedTitle = format.PadRight(linkedTitle, visibleTitleLen, constants.ColTitle)

		// Format priority with color and pad
		coloredPriority := colorPriority(item.Priority)
		priorityStr := format.PadRight(coloredPriority, format.DisplayWidth(coloredPriority), constants.ColPriority)

		// Format assigned column using shared logic
		assigned := formatAssigned(n.Details, constants.ColAssigned)
		assignedWidth := format.DisplayWidth(assigned)
		assigned = format.PadRight(assigned, assignedWidth, constants.ColAssigned)

		// Build status column (review state, PR size, or comment count)
		statusRes := f.formatStatus(n, item)
		statusText := statusRes.text
		statusWidth := statusRes.visibleWidth
		if statusWidth > constants.ColStatus {
			// Truncate if needed - use plain text truncation
			statusText, statusWidth = format.TruncateToWidth(statusText, constants.ColStatus)
		}
		statusText = format.PadRight(statusText, statusWidth, constants.ColStatus)

		// Calculate age using shared logic
		age := format.FormatAge(time.Since(n.UpdatedAt))

		if _, err := fmt.Fprintf(w, "%s  %-*s  %s  %s  %s  %s  %s\n",
			priorityStr,
			constants.ColType, typeStr,
			assigned,
			linkedRepo,
			linkedTitle,
			statusText,
			age,
		); err != nil {
			log.Trace("write error", "location", "row", "error", err)
		}
	}

	return nil
}

// statusResult holds both the display string and its visible width
type statusResult struct {
	text         string
	visibleWidth int
}

// formatStatus builds the status column showing review state, PR size, or activity
// Returns the formatted string and its visible width (excluding ANSI codes)
func (f *TableFormatter) formatStatus(n model.Item, _ triage.PrioritizedItem) statusResult {
	if n.Details == nil {
		reason := string(n.Reason)
		return statusResult{reason, len(reason)}
	}

	d := n.Details

	// For PRs, show review state and size
	if d.IsPR {
		var textParts []string
		var plainParts []string

		// Review state with color (using ASCII symbols for consistent terminal width)
		switch d.ReviewState {
		case constants.ReviewStateApproved:
			textParts = append(textParts, color.GreenString("+ APPROVED"))
			plainParts = append(plainParts, "+ APPROVED")
		case constants.ReviewStateChangesRequested:
			textParts = append(textParts, color.YellowString("! CHANGES"))
			plainParts = append(plainParts, "! CHANGES")
		case constants.ReviewStatePending, constants.ReviewStateReviewRequired, constants.ReviewStateReviewed:
			textParts = append(textParts, color.CyanString("* REVIEW"))
			plainParts = append(plainParts, "* REVIEW")
		}

		// PR size (compact format) using shared logic
		totalChanges := d.Additions + d.Deletions
		if totalChanges > 0 {
			thresholds := format.PRSizeThresholds{
				XS: f.PRSizeXS,
				S:  f.PRSizeS,
				M:  f.PRSizeM,
				L:  f.PRSizeL,
			}
			sizeResult := format.CalculatePRSize(d.Additions, d.Deletions, thresholds)
			sizeColored := colorPRSize(sizeResult.Size)
			sizeText := fmt.Sprintf("%s+%d/-%d", sizeColored, d.Additions, d.Deletions)
			textParts = append(textParts, sizeText)
			plainParts = append(plainParts, sizeResult.Formatted)
		}

		if len(textParts) > 0 {
			text := strings.Join(textParts, " ")
			plain := strings.Join(plainParts, " ")
			return statusResult{text, len(plain)}
		}
	}

	// For issues or PRs without specific status, show comment activity
	if d.CommentCount > 0 {
		text := fmt.Sprintf("%d comments", d.CommentCount)
		return statusResult{text, len(text)}
	}

	// For items with assignees but no comments, show "assign"
	if len(d.Assignees) > 0 {
		return statusResult{"assign", 6}
	}

	reason := string(n.Reason)
	return statusResult{reason, len(reason)}
}

// colorPRSize returns a colored string for the PR size
func colorPRSize(size format.PRSize) string {
	switch size {
	case format.PRSizeXS, format.PRSizeS:
		return color.GreenString(string(size))
	case format.PRSizeM, format.PRSizeL:
		return color.YellowString(string(size))
	default: // XL
		return color.RedString(string(size))
	}
}

func colorPriority(p triage.PriorityLevel) string {
	switch p {
	case triage.PriorityUrgent:
		return color.RedString("Urgent")
	case triage.PriorityImportant:
		return color.YellowString("Important")
	case triage.PriorityQuickWin:
		return color.GreenString("Quick Win")
	case triage.PriorityNotable:
		return color.CyanString("Notable")
	default:
		return color.WhiteString("FYI")
	}
}

// formatAssigned returns the assigned user for display
// Priority: assignee > latest reviewer > requested reviewer
func formatAssigned(d *model.ItemDetails, maxWidth int) string {
	if d == nil {
		return "─"
	}

	input := format.AssignedInput{
		Assignees:          d.Assignees,
		IsPR:               d.IsPR,
		LatestReviewer:     d.LatestReviewer,
		RequestedReviewers: d.RequestedReviewers,
	}

	assigned := format.GetAssignedUser(input)
	if assigned == "" {
		return "─"
	}

	return format.TruncateUsername(assigned, maxWidth)
}
