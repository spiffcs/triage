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

// Column widths for orphaned view
const (
	colSignal = 30
)

// TabBarLines is the number of lines used for the tab bar (including top padding)
const TabBarLines = 3

// renderListView renders the complete list view
func renderListView(m ListModel) string {
	var b strings.Builder

	// Calculate available height for items (account for tab bar)
	availableHeight := m.windowHeight - constants.HeaderLines - constants.FooterLines - TabBarLines

	// Get active pane's items and cursor
	items := m.activeItems()
	cursor := m.activeCursor()

	// Determine view flags based on active pane
	hideAssignedCI := m.activePane == PaneOrphaned
	hidePriority := m.activePane == PaneOrphaned

	// Render tab bar with top padding
	b.WriteString("\n")
	b.WriteString(renderTabBar(m.activePane, len(m.priorityItems), len(m.orphanedItems)))
	b.WriteString("\n\n")

	if len(items) == 0 {
		if m.activePane == PaneOrphaned {
			b.WriteString(renderOrphanedEmptyState())
		} else {
			b.WriteString(renderEmptyState())
		}
		b.WriteString("\n\n")
		b.WriteString(renderHelp())
		return b.String()
	}

	// Render header
	b.WriteString(renderHeader(hideAssignedCI, hidePriority))
	b.WriteString("\n")
	b.WriteString(renderSeparator(hideAssignedCI, hidePriority))
	b.WriteString("\n")

	// Calculate scroll window
	start, end := calculateScrollWindow(cursor, len(items), availableHeight)

	// Render visible items
	for i := start; i < end; i++ {
		selected := i == cursor
		b.WriteString(renderRow(items[i], selected, m.hotTopicThreshold, m.prSizeXS, m.prSizeS, m.prSizeM, m.prSizeL, m.currentUser, hideAssignedCI, hidePriority))
		b.WriteString("\n")
	}

	// Pad remaining space
	renderedRows := end - start
	for i := renderedRows; i < availableHeight && i < len(items); i++ {
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

// renderTabBar renders the tab bar at the top of the view
func renderTabBar(activePane Pane, priorityCount, orphanedCount int) string {
	priority := fmt.Sprintf("[ 1: Priority (%d) ]", priorityCount)
	orphaned := fmt.Sprintf("[ 2: Orphaned (%d) ]", orphanedCount)

	var priorityStyled, orphanedStyled string
	if activePane == PanePriority {
		priorityStyled = tabActiveStyle.Render(priority)
		orphanedStyled = tabInactiveStyle.Render(orphaned)
	} else {
		priorityStyled = tabInactiveStyle.Render(priority)
		orphanedStyled = tabActiveStyle.Render(orphaned)
	}

	return fmt.Sprintf("%s    %s", priorityStyled, orphanedStyled)
}

// renderOrphanedEmptyState renders the empty state message for the orphaned pane
func renderOrphanedEmptyState() string {
	return listEmptyStyle.Render("No orphaned contributions.\nOrphaned items are external PRs/issues that need team attention.")
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
func renderHeader(hideAssignedCI, hidePriority bool) string {
	if hideAssignedCI {
		if hidePriority {
			return listHeaderStyle.Render(fmt.Sprintf(
				"  %-*s  %-*s  %-*s  %-*s  %-*s  %s",
				constants.ColType, "Type",
				constants.ColRepo, "Repository",
				constants.ColTitle, "Title",
				constants.ColStatus, "Status",
				colSignal, "Signal",
				"Updated",
			))
		}
		return listHeaderStyle.Render(fmt.Sprintf(
			"  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s",
			constants.ColPriority, "Priority",
			constants.ColType, "Type",
			constants.ColRepo, "Repository",
			constants.ColTitle, "Title",
			constants.ColStatus, "Status",
			colSignal, "Signal",
			"Age",
		))
	}
	if hidePriority {
		return listHeaderStyle.Render(fmt.Sprintf(
			"  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s",
			constants.ColType, "Type",
			constants.ColAssigned, "Assigned",
			constants.ColCI, "CI",
			constants.ColRepo, "Repository",
			constants.ColTitle, "Title",
			constants.ColStatus, "Status",
			"Age",
		))
	}
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
func renderSeparator(hideAssignedCI, hidePriority bool) string {
	var width int
	priorityWidth := constants.ColPriority + 2 // column + spacing
	if hidePriority {
		priorityWidth = 0
	}
	if hideAssignedCI {
		width = 2 + priorityWidth + constants.ColType + 2 + constants.ColRepo + 2 + constants.ColTitle + 2 + constants.ColStatus + 2 + colSignal + 2 + constants.ColAge
	} else {
		width = 2 + priorityWidth + constants.ColType + 2 + constants.ColAssigned + 2 + constants.ColCI + 2 + constants.ColRepo + 2 + constants.ColTitle + 2 + constants.ColStatus + 2 + constants.ColAge
	}
	return listSeparatorStyle.Render(strings.Repeat("─", width))
}

// renderRow renders a single item row
func renderRow(item triage.PrioritizedItem, selected bool, hotTopicThreshold, prSizeXS, prSizeS, prSizeM, prSizeL int, currentUser string, hideAssignedCI, hidePriority bool) string {
	n := item.Notification

	// Cursor indicator
	cursor := "  "
	if selected {
		cursor = listCursorStyle.Render("> ")
	}

	// Type with color
	isPR := (n.Details != nil && n.Details.IsPR) || n.Subject.Type == "PullRequest"
	var typeStr string
	if isPR {
		typeStr = listTypePRStyle.Render("PR")
		typeStr = format.PadRight(typeStr, 2, constants.ColType)
	} else {
		typeStr = listTypeISSStyle.Render("ISS")
		typeStr = format.PadRight(typeStr, 3, constants.ColType)
	}

	// Priority with color - need to pad based on visible width
	priority := ""
	if !hidePriority {
		var priorityWidth int
		priority, priorityWidth = renderPriority(item.Priority)
		priority = format.PadRight(priority, priorityWidth, constants.ColPriority)
		priority += "  " // spacing
	}

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

	// Age using shared logic with color coding
	age := renderAge(time.Since(n.UpdatedAt))

	var row string
	if hideAssignedCI {
		// Orphaned view: no Assigned/CI, but add Signal column
		signal, signalWidth := renderSignal(n.Details)
		signal = format.PadRight(signal, signalWidth, colSignal)

		row = fmt.Sprintf("%s%s%s  %s  %s  %s  %s  %s",
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
		assigned = format.PadRight(assigned, assignedWidth, constants.ColAssigned)

		ci, ciWidth := renderCI(n.Details, isPR)
		ci = format.PadRight(ci, ciWidth, constants.ColCI)

		row = fmt.Sprintf("%s%s%s  %s  %s  %s  %s  %s  %s",
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
// Returns colored text and visible width
func renderSignal(d *github.ItemDetails) (string, int) {
	if d == nil {
		return "─", 1
	}

	var coloredParts []string
	var plainWidth int

	// Days since team activity - color based on age
	var days int
	if d.LastTeamActivityAt != nil {
		days = int(time.Since(*d.LastTeamActivityAt).Hours() / 24)
	} else if !d.CreatedAt.IsZero() {
		days = int(time.Since(d.CreatedAt).Hours() / 24)
	}

	if days > 0 {
		text := fmt.Sprintf("Last Team Response: %dd ago", days)
		var coloredText string
		if days >= 30 {
			coloredText = listSignalCriticalStyle.Render(text)
		} else if days >= 14 {
			coloredText = listSignalWarningStyle.Render(text)
		} else {
			coloredText = listSignalInfoStyle.Render(text)
		}
		coloredParts = append(coloredParts, coloredText)
		plainWidth += len(text)
	}

	// Consecutive unanswered comments - color based on count
	if d.ConsecutiveAuthorComments >= 2 {
		text := fmt.Sprintf("%d unanswered", d.ConsecutiveAuthorComments)
		var coloredText string
		if d.ConsecutiveAuthorComments >= 4 {
			coloredText = listSignalCriticalStyle.Render(text)
		} else if d.ConsecutiveAuthorComments >= 3 {
			coloredText = listSignalWarningStyle.Render(text)
		} else {
			coloredText = listSignalInfoStyle.Render(text)
		}

		if plainWidth > 0 {
			coloredParts = append(coloredParts, ", "+coloredText)
			plainWidth += 2 + len(text) // ", " + text
		} else {
			coloredParts = append(coloredParts, coloredText)
			plainWidth += len(text)
		}
	}

	if len(coloredParts) == 0 {
		return listSignalInfoStyle.Render("Needs attention"), 15
	}

	return strings.Join(coloredParts, ""), plainWidth
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

// renderAge renders the age with appropriate color coding
func renderAge(d time.Duration) string {
	ageStr := format.FormatAge(d)
	days := int(d.Hours() / 24)

	switch {
	case days >= 30:
		return listAgeCriticalStyle.Render(ageStr)
	case days >= 14:
		return listAgeWarningStyle.Render(ageStr)
	case days >= 7:
		return listAgeModerateStyle.Render(ageStr)
	default:
		return listAgeRecentStyle.Render(ageStr)
	}
}

// renderHelp renders the help text
func renderHelp() string {
	return listHelpStyle.Render("Tab/1/2: switch panes   j/k: navigate   d: mark done   enter: open   q: quit")
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

	// Tab bar styles
	tabActiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Bold(true)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280"))

	// Type column styles
	listTypePRStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")) // Blue for PRs

	listTypeISSStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FBBF24")) // Yellow/amber for issues

	// Age column styles
	listAgeRecentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22C55E")) // Green for < 7 days

	listAgeModerateStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FBBF24")) // Yellow for 7-13 days

	listAgeWarningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B")) // Orange for 14-29 days

	listAgeCriticalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EF4444")) // Red for 30+ days

	// Signal severity styles
	listSignalCriticalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EF4444")) // Red - urgent

	listSignalWarningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B")) // Orange - warning

	listSignalInfoStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FBBF24")) // Yellow - info
)
