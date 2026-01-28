package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/spiffcs/triage/internal/github"
	"github.com/spiffcs/triage/internal/triage"
)

// Column widths
const (
	colPriority = 10
	colType     = 5
	colAssigned = 12
	colCI       = 2
	colRepo     = 26
	colTitle    = 40
	colStatus   = 20
	colAge      = 5
	colSignal   = 30
)

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// renderListView renders the complete list view
func renderListView(m ListModel) string {
	var b strings.Builder

	// Calculate available height for items
	headerLines := 2 // header + separator
	footerLines := 3 // blank + help + status
	availableHeight := m.windowHeight - headerLines - footerLines

	if len(m.items) == 0 {
		b.WriteString(renderEmptyState())
		b.WriteString("\n\n")
		b.WriteString(renderHelp())
		return b.String()
	}

	// Render header
	b.WriteString(renderHeader(m.hideAssignedCI))
	b.WriteString("\n")
	b.WriteString(renderSeparator(m.hideAssignedCI))
	b.WriteString("\n")

	// Calculate scroll window
	start, end := calculateScrollWindow(m.cursor, len(m.items), availableHeight)

	// Render visible items
	for i := start; i < end; i++ {
		selected := i == m.cursor
		b.WriteString(renderRow(m.items[i], selected, m.hotTopicThreshold, m.prSizeXS, m.prSizeS, m.prSizeM, m.prSizeL, m.currentUser, m.hideAssignedCI))
		b.WriteString("\n")
	}

	// Pad remaining space
	renderedRows := end - start
	for i := renderedRows; i < availableHeight && i < len(m.items); i++ {
		b.WriteString("\n")
	}

	// Render footer
	b.WriteString("\n")
	b.WriteString(renderHelp())

	// Status message
	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(listStatusStyle.Render(m.statusMsg))
	}

	return b.String()
}

// calculateScrollWindow determines which items to show based on cursor position
func calculateScrollWindow(cursor, total, viewHeight int) (start, end int) {
	if total <= viewHeight {
		return 0, total
	}

	start = cursor - viewHeight/2
	if start < 0 {
		start = 0
	}

	end = start + viewHeight
	if end > total {
		end = total
		start = end - viewHeight
		if start < 0 {
			start = 0
		}
	}

	return start, end
}

// renderHeader renders the table header
func renderHeader(hideAssignedCI bool) string {
	if hideAssignedCI {
		return listHeaderStyle.Render(fmt.Sprintf(
			"  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s",
			colPriority, "Priority",
			colType, "Type",
			colRepo, "Repository",
			colTitle, "Title",
			colStatus, "Status",
			colSignal, "Signal",
			"Age",
		))
	}
	return listHeaderStyle.Render(fmt.Sprintf(
		"  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s",
		colPriority, "Priority",
		colType, "Type",
		colAssigned, "Assigned",
		colCI, "CI",
		colRepo, "Repository",
		colTitle, "Title",
		colStatus, "Status",
		"Age",
	))
}

// renderSeparator renders the header separator line
func renderSeparator(hideAssignedCI bool) string {
	var width int
	if hideAssignedCI {
		width = 2 + colPriority + 2 + colType + 2 + colRepo + 2 + colTitle + 2 + colStatus + 2 + colSignal + 2 + colAge
	} else {
		width = 2 + colPriority + 2 + colType + 2 + colAssigned + 2 + colCI + 2 + colRepo + 2 + colTitle + 2 + colStatus + 2 + colAge
	}
	return listSeparatorStyle.Render(strings.Repeat("─", width))
}

// renderRow renders a single item row
func renderRow(item triage.PrioritizedItem, selected bool, hotTopicThreshold, prSizeXS, prSizeS, prSizeM, prSizeL int, currentUser string, hideAssignedCI bool) string {
	n := item.Notification

	// Cursor indicator
	cursor := "  "
	if selected {
		cursor = listCursorStyle.Render("> ")
	}

	// Type
	typeStr := "ISS"
	isPR := false
	if n.Details != nil && n.Details.IsPR {
		typeStr = "PR"
		isPR = true
	} else if n.Subject.Type == "PullRequest" {
		typeStr = "PR"
		isPR = true
	}
	typeStr = padRight(typeStr, len(typeStr), colType)

	// Priority with color - need to pad based on visible width
	priority, priorityWidth := renderPriority(item.Priority)
	priority = padRight(priority, priorityWidth, colPriority)

	// Title with indicators
	title := n.Subject.Title
	var titlePrefix string
	var titlePrefixWidth int
	if item.Priority == triage.PriorityQuickWin {
		titlePrefix = "⚡ "
		titlePrefixWidth = 3 // emoji (2) + space (1)
	}
	if n.Details != nil && hotTopicThreshold > 0 && n.Details.CommentCount > hotTopicThreshold {
		// Suppress fire emoji for issues where current user was the last commenter
		suppressForIssue := !n.Details.IsPR && n.Details.LastCommenter == currentUser
		if !suppressForIssue {
			titlePrefix = "🔥 "
			titlePrefixWidth = 3 // emoji (2) + space (1)
		}
	}
	// Truncate title to fit remaining space after prefix
	title, titleWidth := truncateToWidth(title, colTitle-titlePrefixWidth)
	title = titlePrefix + title
	titleWidth += titlePrefixWidth
	title = padRight(title, titleWidth, colTitle)

	// Repository
	repo, repoWidth := truncateToWidth(n.Repository.FullName, colRepo)
	repo = padRight(repo, repoWidth, colRepo)

	// Status with colors
	status, statusWidth := renderStatus(n, prSizeXS, prSizeS, prSizeM, prSizeL)
	if statusWidth > colStatus {
		status, statusWidth = truncateToWidth(status, colStatus)
	}
	status = padRight(status, statusWidth, colStatus)

	// Age
	age := formatAge(time.Since(n.UpdatedAt))

	var row string
	if hideAssignedCI {
		// Orphaned view: no Assigned/CI, but add Signal column
		signal, signalWidth := renderSignal(n.Details)
		signal = padRight(signal, signalWidth, colSignal)

		row = fmt.Sprintf("%s%s  %s  %s  %s  %s  %s  %s",
			cursor,
			priority,
			typeStr,
			repo,
			title,
			status,
			signal,
			age,
		)
	} else {
		// Standard view with Assigned and CI columns
		assigned, assignedWidth := renderAssigned(n.Details)
		assigned = padRight(assigned, assignedWidth, colAssigned)

		ci, ciWidth := renderCI(n.Details, isPR)
		ci = padRight(ci, ciWidth, colCI)

		row = fmt.Sprintf("%s%s  %s  %s  %s  %s  %s  %s  %s",
			cursor,
			priority,
			typeStr,
			assigned,
			ci,
			repo,
			title,
			status,
			age,
		)
	}

	if selected {
		return listSelectedStyle.Render(row)
	}
	return row
}

// renderSignal renders the signal column showing why an item needs attention
func renderSignal(d *github.ItemDetails) (string, int) {
	if d == nil {
		return "─", 1
	}

	var signals []string

	// Consecutive unanswered comments
	if d.ConsecutiveAuthorComments >= 2 {
		signals = append(signals, fmt.Sprintf("%d unanswered", d.ConsecutiveAuthorComments))
	}

	// Days since team activity
	if d.LastTeamActivityAt != nil {
		days := int(time.Since(*d.LastTeamActivityAt).Hours() / 24)
		if days > 0 {
			signals = append(signals, fmt.Sprintf("No response %dd", days))
		}
	} else if !d.CreatedAt.IsZero() {
		// No team activity at all
		days := int(time.Since(d.CreatedAt).Hours() / 24)
		if days > 0 {
			signals = append(signals, fmt.Sprintf("No response %dd", days))
		}
	}

	if len(signals) == 0 {
		return "Needs attention", 15
	}

	result := strings.Join(signals, ", ")
	return result, len(result)
}

// renderPriority renders the priority with appropriate styling
// Returns the colored string and its visible width
func renderPriority(p triage.PriorityLevel) (string, int) {
	switch p {
	case triage.PriorityUrgent:
		return listUrgentStyle.Render("Urgent"), 6
	case triage.PriorityImportant:
		return listImportantStyle.Render("Important"), 9
	case triage.PriorityQuickWin:
		return listQuickWinStyle.Render("Quick Win"), 9
	case triage.PriorityNotable:
		return listNotableStyle.Render("Notable"), 7
	default:
		return listFYIStyle.Render("FYI"), 3
	}
}

// renderCI renders the CI status column
// Returns the colored string and its visible width
func renderCI(d *github.ItemDetails, isPR bool) (string, int) {
	if !isPR {
		return "─", 1 // dash for non-PRs
	}
	if d == nil {
		return "─", 1 // dash if no details
	}
	switch d.CIStatus {
	case "success":
		return listCISuccessStyle.Render("✓"), 1
	case "failure":
		return listCIFailureStyle.Render("✗"), 1
	case "pending":
		return listCIPendingStyle.Render("○"), 1
	default:
		return "─", 1 // dash for no CI
	}
}

// renderAssigned renders the Assigned column
// For PRs: shows requested reviewer if available, otherwise assignee
// For Issues: shows assignee
// Returns the string and its visible width
// Priority: assignee > latest reviewer > requested reviewer
func renderAssigned(d *github.ItemDetails) (string, int) {
	if d == nil {
		return "─", 1
	}

	var assigned string
	// Prefer assignees first
	if len(d.Assignees) > 0 {
		assigned = d.Assignees[0]
	} else if d.IsPR && d.LatestReviewer != "" {
		// For PRs without assignee, show the most recent reviewer
		assigned = d.LatestReviewer
	} else if d.IsPR && len(d.RequestedReviewers) > 0 {
		// Fall back to requested reviewers
		assigned = d.RequestedReviewers[0]
	}

	if assigned == "" {
		return "─", 1
	}

	// Truncate if needed
	if len(assigned) > colAssigned {
		assigned = assigned[:colAssigned-1] + "…"
	}

	return assigned, len(assigned)
}

// renderStatus renders the status column with colors
// Returns the colored string and its visible width
func renderStatus(n github.Notification, sizeXS, sizeS, sizeM, sizeL int) (string, int) {
	if n.Details == nil {
		reason := string(n.Reason)
		return reason, len(reason)
	}

	d := n.Details

	if d.IsPR {
		var coloredParts []string
		var plainWidth int

		switch d.ReviewState {
		case "approved":
			coloredParts = append(coloredParts, listApprovedStyle.Render("+ APPROVED"))
			plainWidth += 10
		case "changes_requested":
			coloredParts = append(coloredParts, listChangesStyle.Render("! CHANGES"))
			plainWidth += 9
		case "pending", "review_required", "reviewed":
			coloredParts = append(coloredParts, listReviewStyle.Render("* REVIEW"))
			plainWidth += 8
		}

		totalChanges := d.Additions + d.Deletions
		if totalChanges > 0 {
			sizeColored, sizePlain := getPRSizeColored(totalChanges, sizeXS, sizeS, sizeM, sizeL)
			sizeStr := fmt.Sprintf("%s+%d/-%d", sizeColored, d.Additions, d.Deletions)
			sizePlainStr := fmt.Sprintf("%s+%d/-%d", sizePlain, d.Additions, d.Deletions)
			coloredParts = append(coloredParts, sizeStr)
			if plainWidth > 0 {
				plainWidth += 1 // space
			}
			plainWidth += len(sizePlainStr)
		}

		if len(coloredParts) > 0 {
			return strings.Join(coloredParts, " "), plainWidth
		}
	}

	if d.CommentCount > 0 {
		text := fmt.Sprintf("%d comments", d.CommentCount)
		return text, len(text)
	}

	reason := string(n.Reason)
	return reason, len(reason)
}

// getPRSizeColored returns colored and plain t-shirt size
func getPRSizeColored(total, sizeXS, sizeS, sizeM, sizeL int) (colored string, plain string) {
	switch {
	case total <= sizeXS:
		return listSizeSmallStyle.Render("XS"), "XS"
	case total <= sizeS:
		return listSizeSmallStyle.Render("S"), "S"
	case total <= sizeM:
		return listSizeMediumStyle.Render("M"), "M"
	case total <= sizeL:
		return listSizeMediumStyle.Render("L"), "L"
	default:
		return listSizeLargeStyle.Render("XL"), "XL"
	}
}

// renderHelp renders the help text
func renderHelp() string {
	return listHelpStyle.Render("j/k: navigate   d: mark done   enter: open in browser   q: quit")
}

// renderEmptyState renders the empty state message
func renderEmptyState() string {
	return listEmptyStyle.Render("All caught up! No items to triage.")
}

// stripAnsi removes ANSI escape sequences from a string
func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// displayWidth returns the visible width of a string in terminal columns
// accounting for wide characters like emojis and stripping ANSI codes
func displayWidth(s string) int {
	plain := stripAnsi(s)
	width := 0
	runes := []rune(plain)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// Check for emoji presentation sequence: base emoji + U+FE0F (VS16)
		// These display as 2 columns in modern terminals
		if i+1 < len(runes) && runes[i+1] == '\uFE0F' {
			width += 2
			i++ // skip the variation selector
			continue
		}
		// Skip standalone variation selectors
		if r == '\uFE0F' {
			continue
		}
		width += runewidth.RuneWidth(r)
	}
	return width
}

// truncateToWidth truncates a string to fit within maxWidth display columns
// handling ANSI codes and emoji presentation sequences
func truncateToWidth(s string, maxWidth int) (string, int) {
	width := displayWidth(s)
	if width <= maxWidth {
		return s, width
	}

	targetWidth := maxWidth - 3 // Leave room for "..."
	if targetWidth < 0 {
		targetWidth = 0
	}

	// Find all ANSI sequences and their positions
	matches := ansiRegex.FindAllStringIndex(s, -1)

	var result strings.Builder
	visibleWidth := 0
	pos := 0
	matchIdx := 0

	for pos < len(s) && visibleWidth < targetWidth {
		// Check if current position is the start of an ANSI sequence
		if matchIdx < len(matches) && pos == matches[matchIdx][0] {
			result.WriteString(s[matches[matchIdx][0]:matches[matchIdx][1]])
			pos = matches[matchIdx][1]
			matchIdx++
			continue
		}

		r, size := utf8.DecodeRuneInString(s[pos:])

		// Check for emoji presentation sequence: base + U+FE0F (VS16)
		nextPos := pos + size
		if nextPos < len(s) {
			nextR, nextSize := utf8.DecodeRuneInString(s[nextPos:])
			if nextR == '\uFE0F' {
				// Emoji + VS16 = 2 columns
				if visibleWidth+2 > targetWidth {
					break
				}
				result.WriteString(s[pos : nextPos+nextSize])
				visibleWidth += 2
				pos = nextPos + nextSize
				continue
			}
		}

		// Skip standalone variation selectors
		if r == '\uFE0F' {
			pos += size
			continue
		}

		rw := runewidth.RuneWidth(r)
		if visibleWidth+rw > targetWidth {
			break
		}

		result.WriteString(s[pos : pos+size])
		visibleWidth += rw
		pos += size
	}

	result.WriteString("...")
	return result.String(), maxWidth
}

// padRight pads a string with spaces to reach target visible width
func padRight(s string, visibleWidth, targetWidth int) string {
	if visibleWidth >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-visibleWidth)
}

// formatAge formats a duration as a human-readable age string
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}
	if days < 30 {
		return fmt.Sprintf("%dw", days/7)
	}
	return fmt.Sprintf("%dmo", days/30)
}

// List view styles - balanced palette (vibrant but not harsh)
var (
	// Neutral UI
	listHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#CBD5E1"))

	listSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#475569"))

	listSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E293B")).
				Foreground(lipgloss.Color("#E5E7EB"))

	listCursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Bold(true)

	// Priority
	listUrgentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444"))

	listImportantStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))

	listQuickWinStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22C55E"))

	listNotableStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#60A5FA"))

	listFYIStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))

	// Status
	listApprovedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22C55E"))

	listChangesStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))

	listReviewStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#93C5FD"))

	// PR size
	listSizeSmallStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22C55E"))

	listSizeMediumStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))

	listSizeLargeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EF4444"))

	// CI status
	listCISuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22C55E")) // Green

	listCIFailureStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EF4444")) // Red

	listCIPendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B")) // Orange

	listHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748B"))

	listStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA"))

	listEmptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Italic(true)
)
