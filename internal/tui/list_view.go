package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spiffcs/triage/internal/format"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/output"
	"github.com/spiffcs/triage/internal/triage"
)

// Column widths for orphaned view
const (
	colSignal = 26
)

// tabBarLines is the number of lines used for the tab bar (including top padding)
const tabBarLines = 3

// columnVisibility tracks which optional columns should be shown based on terminal width
type columnVisibility struct {
	showSignal bool // Orphaned pane only - first to hide
	showAuthor bool // Second to hide
	showCI     bool // Third to hide
}

// calculateColumnVisibility determines which columns to show based on available width.
// Columns are hidden in priority order: Signal (first) → Author → CI (last).
func calculateColumnVisibility(windowWidth int, hideAssignedCI, hidePriority, showAuthor bool) columnVisibility {
	vis := columnVisibility{
		showSignal: true,
		showAuthor: showAuthor,
		showCI:     true,
	}

	// Calculate base width for always-visible columns using minimum flex widths
	// cursor(2) + type(5+2) + repo(20+2) + title(30+2) + status(20+2) + age(5)
	const minTitle, minRepo = 30, 20
	baseWidth := 2 + (output.ColType + 2) + (minRepo + 2) + (minTitle + 2) + (output.ColStatus + 2) + output.ColAge

	// Add priority column if shown
	if !hidePriority {
		baseWidth += output.ColPriority + 2
	}

	// Add assigned column width for assigned/blocked/priority panes
	if !hideAssignedCI {
		baseWidth += output.ColAssigned + 2
	}

	// Calculate widths for optional columns
	ciWidth := output.ColCI + 2
	authorWidth := output.ColAuthor + 2
	signalWidth := colSignal + 2

	// Orphaned pane has Signal column
	if hideAssignedCI {
		// Hide columns in order: Signal first, then Author, then CI
		if windowWidth < baseWidth+ciWidth+authorWidth+signalWidth {
			vis.showSignal = false
		}
		if windowWidth < baseWidth+ciWidth+authorWidth {
			vis.showAuthor = false
		}
		if windowWidth < baseWidth+ciWidth {
			vis.showCI = false
		}
	} else if hidePriority && showAuthor {
		// Assigned/Blocked pane: Author, CI columns
		// Hide columns in order: Author first, then CI
		if windowWidth < baseWidth+ciWidth+authorWidth {
			vis.showAuthor = false
		}
		if windowWidth < baseWidth+ciWidth {
			vis.showCI = false
		}
	} else {
		// Priority pane: just CI column as optional
		if windowWidth < baseWidth+ciWidth {
			vis.showCI = false
		}
	}

	return vis
}

// columnWidths holds the computed flex widths for title and repo columns
type columnWidths struct {
	title int
	repo  int
}

// fixedColumnsWidth returns the total width consumed by all fixed-width columns
// (everything except title and repo, including their 2-char gaps).
func fixedColumnsWidth(vis columnVisibility, hideAssignedCI, hidePriority bool) int {
	fixed := 2 // cursor
	if !hidePriority {
		fixed += output.ColPriority + 2
	}
	fixed += output.ColType + 2
	if vis.showAuthor {
		fixed += output.ColAuthor + 2
	}
	if !hideAssignedCI {
		fixed += output.ColAssigned + 2
	}
	if vis.showCI {
		fixed += output.ColCI + 2
	}
	fixed += output.ColStatus + 2
	if hideAssignedCI && vis.showSignal {
		fixed += colSignal + 2
	}
	fixed += output.ColAge
	// Gaps for repo (+2) and title (+2)
	fixed += 4
	return fixed
}

// calculateColumnWidths computes content-aware widths for the title and repo
// columns. It scans the visible items to find the longest content, then sizes
// each column to fit without truncation (bounded by available terminal space).
// Extra space stays at the right edge rather than padding between columns.
func calculateColumnWidths(windowWidth int, vis columnVisibility, hideAssignedCI, hidePriority bool, items []triage.PrioritizedItem) columnWidths {
	const minTitle, minRepo = 30, 20
	const maxTitle, maxRepo = 120, 50

	const rightMargin = 2
	fixed := fixedColumnsWidth(vis, hideAssignedCI, hidePriority)
	remaining := windowWidth - fixed - rightMargin

	if remaining < minTitle+minRepo {
		return columnWidths{title: minTitle, repo: minRepo}
	}

	// Scan items to find the longest title and repo content
	maxTitleWidth := 0
	maxRepoWidth := 0
	for _, item := range items {
		tw := format.DisplayWidth(item.Subject.Title) + format.IconWidth
		if tw > maxTitleWidth {
			maxTitleWidth = tw
		}
		rw := format.DisplayWidth(item.Repository.FullName)
		if rw > maxRepoWidth {
			maxRepoWidth = rw
		}
	}

	// Clamp to min/max bounds
	titleWidth := max(min(maxTitleWidth, maxTitle), minTitle)
	repoWidth := max(min(maxRepoWidth, maxRepo), minRepo)

	// If both fit within remaining space, we're done
	if titleWidth+repoWidth <= remaining {
		return columnWidths{title: titleWidth, repo: repoWidth}
	}

	// Not enough room for both at content width — shrink proportionally,
	// giving title priority (70/30 split of available space)
	titleWidth = remaining * 7 / 10
	repoWidth = remaining - titleWidth

	// Re-apply minimums
	if titleWidth < minTitle {
		titleWidth = minTitle
		repoWidth = remaining - titleWidth
	}
	if repoWidth < minRepo {
		repoWidth = minRepo
		titleWidth = remaining - repoWidth
	}

	return columnWidths{title: titleWidth, repo: repoWidth}
}

// renderListView renders the complete list view
func renderListView(m ListModel) string {
	var b strings.Builder

	// Calculate available height for items (account for tab bar)
	availableHeight := m.windowHeight - HeaderLines - FooterLines - tabBarLines
	if availableHeight < 0 {
		availableHeight = 0
	}

	// Get active pane's items and cursor
	items := m.activeItems()
	cursor := m.activeCursor()

	// Determine view flags based on active pane
	// Orphaned pane: hide Assigned/CI, show Author/Signal, hide Priority
	// Assigned pane: show Author AND Assigned, hide CI/Signal, hide Priority
	// Blocked pane: show Author AND Assigned, hide CI/Signal, hide Priority (similar to Assigned)
	// Priority pane: show Assigned/CI, hide Author, show Priority
	hideAssignedCI := m.activePane == paneOrphaned
	hidePriority := m.activePane == paneOrphaned || m.activePane == paneAssigned || m.activePane == paneBlocked
	showAuthor := m.activePane == paneOrphaned || m.activePane == paneAssigned || m.activePane == paneBlocked

	// Render tab bar with top padding
	b.WriteString("\n")
	b.WriteString(renderTabBar(m.activePane, len(m.priorityItems), len(m.orphanedItems), m.AssignedCount(), m.BlockedCount(),
		m.PrioritySortColumn(), m.PrioritySortDesc(),
		m.OrphanedSortColumn(), m.OrphanedSortDesc(),
		m.AssignedSortColumn(), m.AssignedSortDesc(),
		m.BlockedSortColumn(), m.BlockedSortDesc()))
	b.WriteString("\n\n")

	if len(items) == 0 {
		switch m.activePane {
		case paneOrphaned:
			b.WriteString(m.renderOrphanedEmptyState())
		case paneAssigned:
			b.WriteString(renderAssignedEmptyState())
		case paneBlocked:
			b.WriteString(renderBlockedEmptyState())
		default:
			b.WriteString(renderEmptyState())
		}
		b.WriteString("\n\n")
		b.WriteString(renderHelp(m.TypeFilterLabel()))
		return b.String()
	}

	// Calculate column visibility based on terminal width
	vis := calculateColumnVisibility(m.windowWidth, hideAssignedCI, hidePriority, showAuthor)
	cw := calculateColumnWidths(m.windowWidth, vis, hideAssignedCI, hidePriority, items)

	// Render header
	b.WriteString(renderHeader(hideAssignedCI, hidePriority, vis, cw))
	b.WriteString("\n")
	b.WriteString(renderSeparator(m.windowWidth))
	b.WriteString("\n")

	// Calculate scroll window
	start, end := calculateScrollWindow(cursor, len(items), availableHeight)

	// Render visible items
	for i := start; i < end; i++ {
		selected := i == cursor
		b.WriteString(renderRow(items[i], selected, m.hotTopicThreshold, m.prSizeXS, m.prSizeS, m.prSizeM, m.prSizeL, m.currentUser, hideAssignedCI, hidePriority, vis, cw, m.windowWidth))
		b.WriteString("\n")
	}

	// Pad remaining space so the help text stays pinned to the bottom
	renderedRows := end - start
	for i := renderedRows; i < availableHeight; i++ {
		b.WriteString("\n")
	}

	// Render footer
	b.WriteString("\n")
	b.WriteString(renderHelp(m.TypeFilterLabel()))

	// Status message
	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(listStatusStyle.Render(m.statusMsg))
	}

	return b.String()
}

// renderTabBar renders the tab bar at the top of the view
func renderTabBar(activePane pane, priorityCount, orphanedCount, assignedCount, blockedCount int,
	prioritySortCol SortColumn, prioritySortDesc bool,
	orphanedSortCol SortColumn, orphanedSortDesc bool,
	assignedSortCol SortColumn, assignedSortDesc bool,
	blockedSortCol SortColumn, blockedSortDesc bool) string {

	// Format sort indicator
	priorityDir := "▼"
	if !prioritySortDesc {
		priorityDir = "▲"
	}
	orphanedDir := "▼"
	if !orphanedSortDesc {
		orphanedDir = "▲"
	}
	assignedDir := "▼"
	if !assignedSortDesc {
		assignedDir = "▲"
	}
	blockedDir := "▼"
	if !blockedSortDesc {
		blockedDir = "▲"
	}

	assigned := fmt.Sprintf("[ 1: Assigned (%d) %s%s ]", assignedCount, assignedDir, assignedSortCol)
	blocked := fmt.Sprintf("[ 2: Blocked (%d) %s%s ]", blockedCount, blockedDir, blockedSortCol)
	priority := fmt.Sprintf("[ 3: Priority (%d) %s%s ]", priorityCount, priorityDir, prioritySortCol)
	orphaned := fmt.Sprintf("[ 4: Orphaned (%d) %s%s ]", orphanedCount, orphanedDir, orphanedSortCol)

	var assignedStyled, priorityStyled, orphanedStyled, blockedStyled string
	switch activePane {
	case paneAssigned:
		assignedStyled = tabActiveStyle.Render(assigned)
		priorityStyled = tabInactiveStyle.Render(priority)
		orphanedStyled = tabInactiveStyle.Render(orphaned)
		blockedStyled = tabInactiveStyle.Render(blocked)
	case panePriority:
		assignedStyled = tabInactiveStyle.Render(assigned)
		priorityStyled = tabActiveStyle.Render(priority)
		orphanedStyled = tabInactiveStyle.Render(orphaned)
		blockedStyled = tabInactiveStyle.Render(blocked)
	case paneOrphaned:
		assignedStyled = tabInactiveStyle.Render(assigned)
		priorityStyled = tabInactiveStyle.Render(priority)
		orphanedStyled = tabActiveStyle.Render(orphaned)
		blockedStyled = tabInactiveStyle.Render(blocked)
	case paneBlocked:
		assignedStyled = tabInactiveStyle.Render(assigned)
		priorityStyled = tabInactiveStyle.Render(priority)
		orphanedStyled = tabInactiveStyle.Render(orphaned)
		blockedStyled = tabActiveStyle.Render(blocked)
	}

	return fmt.Sprintf("%s    %s    %s    %s", assignedStyled, blockedStyled, priorityStyled, orphanedStyled)
}

// renderOrphanedEmptyState renders the empty state message for the orphaned pane
func (m ListModel) renderOrphanedEmptyState() string {
	// Check if orphaned repos are configured
	if m.config == nil || m.config.Orphaned == nil || len(m.config.Orphaned.Repos) == 0 {
		return listEmptyStyle.Render(
			"Orphaned pane not configured.\n\n" +
				"Add repos to monitor in ~/.config/triage/config.yaml:\n\n" +
				"  orphaned:\n" +
				"    repos:\n" +
				"      - owner/repo\n" +
				"    stale_days: 7")
	}
	return listEmptyStyle.Render("No orphaned contributions.\nOrphaned items are external PRs/issues that need team attention.")
}

// renderAssignedEmptyState renders the empty state message for the assigned pane
func renderAssignedEmptyState() string {
	return listEmptyStyle.Render("No items assigned to you.\nItems where you are an assignee will appear here.")
}

// renderBlockedEmptyState renders the empty state message for the blocked pane
func renderBlockedEmptyState() string {
	return listEmptyStyle.Render("No blocked items.\nItems with the 'blocked' label will appear here.")
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
func renderHeader(hideAssignedCI, hidePriority bool, vis columnVisibility, cw columnWidths) string {
	var parts []string

	// Cursor space
	parts = append(parts, "  ")

	// Priority column (Priority pane only)
	if !hidePriority {
		parts = append(parts, fmt.Sprintf("%-*s  ", output.ColPriority, "Priority"))
	}

	// Type column (always visible)
	parts = append(parts, fmt.Sprintf("%-*s  ", output.ColType, "Type"))

	// Author column (Orphaned/Assigned/Blocked panes, if visible)
	if vis.showAuthor {
		parts = append(parts, fmt.Sprintf("%-*s  ", output.ColAuthor, "Author"))
	}

	// Assigned column (Assigned/Blocked/Priority panes)
	if !hideAssignedCI {
		parts = append(parts, fmt.Sprintf("%-*s  ", output.ColAssigned, "Assigned"))
	}

	// CI column (if visible)
	if vis.showCI {
		parts = append(parts, fmt.Sprintf("%-*s  ", output.ColCI, "CI"))
	}

	// Repository column (always visible)
	parts = append(parts, fmt.Sprintf("%-*s  ", cw.repo, "Repository"))

	// Title column (always visible)
	parts = append(parts, fmt.Sprintf("%-*s  ", cw.title, "Title"))

	// Status column (always visible)
	parts = append(parts, fmt.Sprintf("%-*s  ", output.ColStatus, "Status"))

	// Signal column (Orphaned pane only, if visible)
	if hideAssignedCI && vis.showSignal {
		parts = append(parts, fmt.Sprintf("%-*s  ", colSignal, "Signal"))
	}

	// Age column (always visible, no trailing space)
	parts = append(parts, fmt.Sprintf("%-*s", output.ColAge, "Age"))

	return listHeaderStyle.Render(strings.Join(parts, ""))
}

// renderSeparator renders the header separator line spanning the full window width
func renderSeparator(width int) string {
	return listSeparatorStyle.Render(strings.Repeat("─", width))
}

// renderRow renders a single item row
func renderRow(item triage.PrioritizedItem, selected bool, hotTopicThreshold, prSizeXS, prSizeS, prSizeM, prSizeL int, currentUser string, hideAssignedCI, hidePriority bool, vis columnVisibility, cw columnWidths, windowWidth int) string {
	n := item.Item

	// Cursor indicator
	cursor := "  "
	if selected {
		cursor = applyStyle(listCursorStyle, "> ", selected)
	}

	// Type with color
	isPR := n.Type == model.ItemTypePullRequest || n.Subject.Type == model.SubjectPullRequest
	var typeStr string
	if isPR {
		typeStr = applyStyle(listTypePRStyle, "PR", selected)
		typeStr = format.PadRight(typeStr, 2, output.ColType)
	} else {
		typeStr = applyStyle(listTypeISSStyle, "ISS", selected)
		typeStr = format.PadRight(typeStr, 3, output.ColType)
	}

	// Priority with color - need to pad based on visible width
	priority := ""
	if !hidePriority {
		var priorityWidth int
		priority, priorityWidth = renderPriority(item.Priority, selected)
		priority = format.PadRight(priority, priorityWidth, output.ColPriority)
		priority += "  " // spacing
	}

	// Title with icon prefix using shared logic
	title := n.Subject.Title

	var titleIcon string
	var iconDisplayWidth int

	iconInput := format.IconOptions{
		HotTopicThreshold: hotTopicThreshold,
		IsQuickWin:        item.Priority == triage.PriorityQuickWin,
		CurrentUser:       currentUser,
		CommentCount:      n.CommentCount,
		IsPR:              isPR,
	}
	if issueDetails := n.IssueDetails(); issueDetails != nil {
		iconInput.LastCommenter = issueDetails.LastCommenter
	}

	iconType := format.Icon(iconInput)
	switch iconType {
	case format.IconHotTopic:
		titleIcon = format.HotTopicIcon + " "
		iconDisplayWidth = format.IconWidth
	case format.IconQuickWin:
		titleIcon = applyStyle(listQuickWinIconStyle, format.QuickWinIcon, selected) + " "
		iconDisplayWidth = format.IconWidth
	default:
		titleIcon = "   " // 3 spaces
		iconDisplayWidth = format.IconWidth
	}

	// Truncate title to fit remaining space after icon
	title, titleWidth := format.TruncateToWidth(title, cw.title-format.IconWidth)
	title = titleIcon + title
	titleWidth += iconDisplayWidth
	title = format.PadRight(title, titleWidth, cw.title)

	// Repository
	repo, repoWidth := format.TruncateToWidth(n.Repository.FullName, cw.repo)
	repo = format.PadRight(repo, repoWidth, cw.repo)

	// Status with colors
	status, statusWidth := renderStatus(n, prSizeXS, prSizeS, prSizeM, prSizeL, selected)
	if statusWidth > output.ColStatus {
		status, statusWidth = format.TruncateToWidth(status, output.ColStatus)
	}
	status = format.PadRight(status, statusWidth, output.ColStatus)

	// Age using shared logic with color coding
	age, ageWidth := renderAge(time.Since(n.UpdatedAt), selected)
	age = format.PadRight(age, ageWidth, output.ColAge)

	// Build row dynamically based on pane type and column visibility
	var parts []string
	parts = append(parts, cursor)

	// Priority column (Priority pane only)
	if !hidePriority {
		parts = append(parts, priority)
	}

	// Type column (always visible)
	parts = append(parts, typeStr+"  ")

	// Author column (Orphaned/Assigned/Blocked panes, if visible)
	if vis.showAuthor {
		author := "─"
		if n.Author != "" {
			author, _ = format.TruncateToWidth(n.Author, output.ColAuthor)
		}
		author = format.PadRight(author, len(author), output.ColAuthor)
		parts = append(parts, author+"  ")
	}

	// Assigned column (non-orphaned panes)
	if !hideAssignedCI {
		assigned, assignedWidth := renderAssigned(&n, selected)
		assigned = format.PadRight(assigned, assignedWidth, output.ColAssigned)
		parts = append(parts, assigned+"  ")
	}

	// CI column (if visible)
	if vis.showCI {
		ci, ciWidth := renderCI(&n, isPR, selected)
		ci = format.PadRight(ci, ciWidth, output.ColCI)
		parts = append(parts, ci+"  ")
	}

	// Repository column (always visible)
	parts = append(parts, repo+"  ")

	// Title column (always visible)
	parts = append(parts, title+"  ")

	// Status column (always visible)
	parts = append(parts, status+"  ")

	// Signal column (Orphaned pane only, if visible)
	if hideAssignedCI && vis.showSignal {
		signal, signalWidth := renderSignal(&n, selected)
		signal = format.PadRight(signal, signalWidth, colSignal)
		parts = append(parts, signal+"  ")
	}

	// Age column (always visible)
	parts = append(parts, age)

	row := strings.Join(parts, "")

	if selected {
		return listSelectedStyle.Width(windowWidth).Render(row)
	}
	return row
}

// renderSignal renders the signal column showing why an item needs attention
// Returns colored text and visible width
func renderSignal(n *model.Item, selected bool) (string, int) {
	var coloredParts []string
	var plainWidth int

	// Days since team activity - color based on age
	var days int
	if n.LastTeamActivityAt != nil {
		days = int(time.Since(*n.LastTeamActivityAt).Hours() / 24)
	} else if !n.CreatedAt.IsZero() {
		days = int(time.Since(n.CreatedAt).Hours() / 24)
	}

	if days > 0 {
		text := fmt.Sprintf("Stale %dd", days)
		var coloredText string
		if days >= 30 {
			coloredText = applyStyle(listSignalCriticalStyle, text, selected)
		} else if days >= 14 {
			coloredText = applyStyle(listSignalWarningStyle, text, selected)
		} else {
			coloredText = applyStyle(listSignalInfoStyle, text, selected)
		}
		coloredParts = append(coloredParts, coloredText)
		plainWidth += len(text)
	}

	// Consecutive unanswered comments - color based on count
	if n.ConsecutiveAuthorComments >= 2 {
		text := fmt.Sprintf("%d waiting", n.ConsecutiveAuthorComments)
		var coloredText string
		if n.ConsecutiveAuthorComments >= 4 {
			coloredText = applyStyle(listSignalCriticalStyle, text, selected)
		} else if n.ConsecutiveAuthorComments >= 3 {
			coloredText = applyStyle(listSignalWarningStyle, text, selected)
		} else {
			coloredText = applyStyle(listSignalInfoStyle, text, selected)
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
		return applyStyle(listSignalInfoStyle, "Needs attention", selected), 15
	}

	return strings.Join(coloredParts, ""), plainWidth
}

// renderPriority renders the priority with appropriate styling
// Returns the colored string and its visible width
func renderPriority(p triage.PriorityLevel, selected bool) (string, int) {
	switch p {
	case triage.PriorityUrgent:
		return applyStyle(listUrgentStyle, "Urgent", selected), 6
	case triage.PriorityImportant:
		return applyStyle(listImportantStyle, "Important", selected), 9
	case triage.PriorityQuickWin:
		return applyStyle(listQuickWinStyle, "Quick Win", selected), 9
	case triage.PriorityNotable:
		return applyStyle(listNotableStyle, "Notable", selected), 7
	default:
		return applyStyle(listFYIStyle, "FYI", selected), 3
	}
}

// renderCI renders the CI status column
// Returns the colored string and its visible width
func renderCI(n *model.Item, isPR bool, selected bool) (string, int) {
	if !isPR {
		return "─", 1 // dash for non-PRs
	}
	pr := n.PRDetails()
	if pr == nil {
		return "─", 1 // dash if no details
	}
	switch pr.CIStatus {
	case model.CIStatusSuccess:
		return applyStyle(listCISuccessStyle, "✓", selected), 1
	case model.CIStatusFailure:
		return applyStyle(listCIFailureStyle, "✗", selected), 1
	case model.CIStatusPending:
		return applyStyle(listCIPendingStyle, "○", selected), 1
	default:
		return "─", 1 // dash for no CI
	}
}

// renderAssigned renders the Assigned column using shared logic
// Returns the string and its visible width
func renderAssigned(n *model.Item, selected bool) (string, int) {
	pr := n.PRDetails()

	input := format.AssignedOptions{
		Assignees: n.Assignees,
		IsPR:      n.IsPR(),
	}
	if pr != nil {
		input.LatestReviewer = pr.LatestReviewer
		input.RequestedReviewers = pr.RequestedReviewers
	}

	assigned := format.Assigned(input)
	if assigned == "" {
		return "─", 1
	}

	// Truncate if needed
	assigned = format.TruncateUsername(assigned, output.ColAssigned)

	return assigned, len(assigned)
}

// renderStatus renders the status column with colors
// Returns the colored string and its visible width
func renderStatus(n model.Item, sizeXS, sizeS, sizeM, sizeL int, selected bool) (string, int) {
	pr := n.PRDetails()

	if n.IsPR() && pr != nil {
		var coloredParts []string
		var plainWidth int

		switch pr.ReviewState {
		case model.ReviewStateApproved:
			coloredParts = append(coloredParts, applyStyle(listApprovedStyle, "+ APPROVED", selected))
			plainWidth += 10
		case model.ReviewStateChangesRequested:
			coloredParts = append(coloredParts, applyStyle(listChangesStyle, "! CHANGES", selected))
			plainWidth += 9
		case model.ReviewStatePending, model.ReviewStateReviewRequired, model.ReviewStateReviewed:
			coloredParts = append(coloredParts, applyStyle(listReviewStyle, "* REVIEW", selected))
			plainWidth += 8
		}

		totalChanges := pr.Additions + pr.Deletions
		if totalChanges > 0 {
			thresholds := format.PRSizeThresholds{
				XS: sizeXS,
				S:  sizeS,
				M:  sizeM,
				L:  sizeL,
			}
			sizeResult := format.CalculatePRSize(pr.Additions, pr.Deletions, thresholds)
			sizeColored := colorPRSizeTUI(sizeResult.Size, selected)
			sizeStr := fmt.Sprintf("%s+%d/-%d", sizeColored, pr.Additions, pr.Deletions)
			coloredParts = append(coloredParts, sizeStr)
			if plainWidth > 0 {
				plainWidth += 1 // space
			}
			// Calculate actual visible width of size string
			plainWidth += len(fmt.Sprintf("%s+%d/-%d", string(sizeResult.Size), pr.Additions, pr.Deletions))
		}

		if len(coloredParts) > 0 {
			return strings.Join(coloredParts, " "), plainWidth
		}
	}

	if n.CommentCount > 0 {
		text := fmt.Sprintf("%d comments", n.CommentCount)
		return text, len(text)
	}

	reason := string(n.Reason)
	return reason, len(reason)
}

// colorPRSizeTUI returns a styled string for the PR size using lipgloss
func colorPRSizeTUI(size format.PRSize, selected bool) string {
	switch size {
	case format.PRSizeXS, format.PRSizeS:
		return applyStyle(listSizeSmallStyle, string(size), selected)
	case format.PRSizeM, format.PRSizeL:
		return applyStyle(listSizeMediumStyle, string(size), selected)
	default: // XL
		return applyStyle(listSizeLargeStyle, string(size), selected)
	}
}

// renderAge renders the age with appropriate color coding
// Returns the colored string and its visible width
func renderAge(d time.Duration, selected bool) (string, int) {
	ageStr := format.FormatAge(d)
	days := int(d.Hours() / 24)
	width := len(ageStr)

	switch {
	case days >= 30:
		return applyStyle(listAgeCriticalStyle, ageStr, selected), width
	case days >= 14:
		return applyStyle(listAgeWarningStyle, ageStr, selected), width
	case days >= 7:
		return applyStyle(listAgeModerateStyle, ageStr, selected), width
	default:
		return applyStyle(listAgeRecentStyle, ageStr, selected), width
	}
}

// renderHelp renders the help text with the current type filter label
func renderHelp(filterLabel string) string {
	return listHelpStyle.Render("Tab/1-4: panes   j/k: nav   s/S: sort   r: reset   t: " + filterLabel + "   d: done   enter: open   q: quit")
}

// renderEmptyState renders the empty state message
func renderEmptyState() string {
	return listEmptyStyle.Render("All caught up! No items to triage.")
}

// applyStyle renders text with the given style when not selected.
// When selected, returns plain text to avoid ANSI reset codes that would
// interrupt the selected row's background highlight.
func applyStyle(s lipgloss.Style, text string, selected bool) string {
	if selected {
		return text
	}
	return s.Render(text)
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
				Background(lipgloss.Color("#334155")).
				Foreground(lipgloss.Color("#F1F5F9")).
				Bold(true)

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
