package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/format"
	"github.com/spiffcs/triage/internal/github"
	"github.com/spiffcs/triage/internal/triage"
)

// renderListView renders the complete list view
func renderListView(m ListModel) string {
	var b strings.Builder

	// Calculate available height for items
	availableHeight := m.windowHeight - constants.HeaderLines - constants.FooterLines

	if len(m.items) == 0 {
		b.WriteString(renderEmptyState())
		b.WriteString("\n\n")
		b.WriteString(renderHelp())
		return b.String()
	}

	// Render header
	b.WriteString(renderHeader())
	b.WriteString("\n")
	b.WriteString(renderSeparator())
	b.WriteString("\n")

	// Calculate scroll window
	start, end := calculateScrollWindow(m.cursor, len(m.items), availableHeight)

	// Render visible items
	for i := start; i < end; i++ {
		selected := i == m.cursor
		b.WriteString(renderRow(m.items[i], selected, m.hotTopicThreshold, m.prSizeXS, m.prSizeS, m.prSizeM, m.prSizeL, m.currentUser))
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
func renderHeader() string {
	return listHeaderStyle.Render(fmt.Sprintf(
		"  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s",
		constants.ColPriority, "Priority",
		constants.ColType, "Type",
		constants.ColAssigned, "Assigned",
		constants.ColCI, "CI",
		constants.ColRepo, "Repository",
		constants.ColTitle, "Title",
		constants.ColStatus, "Status",
		"Age",
	))
}

// renderSeparator renders the header separator line
func renderSeparator() string {
	width := 2 + constants.ColPriority + 2 + constants.ColType + 2 + constants.ColAssigned + 2 + constants.ColCI + 2 + constants.ColRepo + 2 + constants.ColTitle + 2 + constants.ColStatus + 2 + constants.ColAge
	return listSeparatorStyle.Render(strings.Repeat("─", width))
}

// renderRow renders a single item row
func renderRow(item triage.PrioritizedItem, selected bool, hotTopicThreshold, prSizeXS, prSizeS, prSizeM, prSizeL int, currentUser string) string {
	n := item.Notification

	// Cursor indicator
	cursor := "  "
	if selected {
		cursor = listCursorStyle.Render("> ")
	}

	// Type
	typeStr := "ISS"
	isPR := (n.Details != nil && n.Details.IsPR) || n.Subject.Type == "PullRequest"
	if isPR {
		typeStr = "PR"
	}
	typeStr = format.PadRight(typeStr, len(typeStr), constants.ColType)

	// Assigned using shared logic
	assigned, assignedWidth := renderAssigned(n.Details)
	assigned = format.PadRight(assigned, assignedWidth, constants.ColAssigned)

	// CI status
	ci, ciWidth := renderCI(n.Details, isPR)
	ci = format.PadRight(ci, ciWidth, constants.ColCI)

	// Priority with color - need to pad based on visible width
	priority, priorityWidth := renderPriority(item.Priority)
	priority = format.PadRight(priority, priorityWidth, constants.ColPriority)

	// Title with icon prefix using shared logic
	title := n.Subject.Title

	var titleIcon string
	var iconDisplayWidth int

	iconInput := format.IconInput{
		HotTopicThreshold: hotTopicThreshold,
		IsQuickWin:        item.Priority == triage.PriorityQuickWin,
		CurrentUser:       currentUser,
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
		titleIcon = listQuickWinIconStyle.Render(format.QuickWinIcon) + " "
		iconDisplayWidth = format.IconWidth
	default:
		titleIcon = "   " // 3 spaces
		iconDisplayWidth = format.IconWidth
	}

	// Truncate title to fit remaining space after icon
	title, titleWidth := format.TruncateToWidth(title, constants.ColTitle-format.IconWidth)
	title = titleIcon + title
	titleWidth += iconDisplayWidth
	title = format.PadRight(title, titleWidth, constants.ColTitle)

	// Repository
	repo, repoWidth := format.TruncateToWidth(n.Repository.FullName, constants.ColRepo)
	repo = format.PadRight(repo, repoWidth, constants.ColRepo)

	// Status with colors
	status, statusWidth := renderStatus(n, prSizeXS, prSizeS, prSizeM, prSizeL)
	if statusWidth > constants.ColStatus {
		status, statusWidth = format.TruncateToWidth(status, constants.ColStatus)
	}
	status = format.PadRight(status, statusWidth, constants.ColStatus)

	// Age using shared logic
	age := format.FormatAge(time.Since(n.UpdatedAt))

	row := fmt.Sprintf("%s%s  %s  %s  %s  %s  %s  %s  %s",
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

	if selected {
		return listSelectedStyle.Render(row)
	}
	return row
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
	case constants.CIStatusSuccess:
		return listCISuccessStyle.Render("✓"), 1
	case constants.CIStatusFailure:
		return listCIFailureStyle.Render("✗"), 1
	case constants.CIStatusPending:
		return listCIPendingStyle.Render("○"), 1
	default:
		return "─", 1 // dash for no CI
	}
}

// renderAssigned renders the Assigned column using shared logic
// Returns the string and its visible width
func renderAssigned(d *github.ItemDetails) (string, int) {
	if d == nil {
		return "─", 1
	}

	input := format.AssignedInput{
		Assignees:          d.Assignees,
		IsPR:               d.IsPR,
		LatestReviewer:     d.LatestReviewer,
		RequestedReviewers: d.RequestedReviewers,
	}

	assigned := format.GetAssignedUser(input)
	if assigned == "" {
		return "─", 1
	}

	// Truncate if needed
	assigned = format.TruncateUsername(assigned, constants.ColAssigned)

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
		case constants.ReviewStateApproved:
			coloredParts = append(coloredParts, listApprovedStyle.Render("+ APPROVED"))
			plainWidth += 10
		case constants.ReviewStateChangesRequested:
			coloredParts = append(coloredParts, listChangesStyle.Render("! CHANGES"))
			plainWidth += 9
		case constants.ReviewStatePending, constants.ReviewStateReviewRequired, constants.ReviewStateReviewed:
			coloredParts = append(coloredParts, listReviewStyle.Render("* REVIEW"))
			plainWidth += 8
		}

		totalChanges := d.Additions + d.Deletions
		if totalChanges > 0 {
			thresholds := format.PRSizeThresholds{
				XS: sizeXS,
				S:  sizeS,
				M:  sizeM,
				L:  sizeL,
			}
			sizeResult := format.CalculatePRSize(d.Additions, d.Deletions, thresholds)
			sizeColored := colorPRSizeTUI(sizeResult.Size)
			sizeStr := fmt.Sprintf("%s+%d/-%d", sizeColored, d.Additions, d.Deletions)
			coloredParts = append(coloredParts, sizeStr)
			if plainWidth > 0 {
				plainWidth += 1 // space
			}
			plainWidth += len(sizeResult.Formatted)
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

// colorPRSizeTUI returns a styled string for the PR size using lipgloss
func colorPRSizeTUI(size format.PRSize) string {
	switch size {
	case format.PRSizeXS, format.PRSizeS:
		return listSizeSmallStyle.Render(string(size))
	case format.PRSizeM, format.PRSizeL:
		return listSizeMediumStyle.Render(string(size))
	default: // XL
		return listSizeLargeStyle.Render(string(size))
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

	listQuickWinIconStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FBBF24")) // Yellow for lightning bolt

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
