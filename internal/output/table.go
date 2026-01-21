package output

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hal/github-prio/internal/github"
	"github.com/hal/github-prio/internal/priority"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TableFormatter formats output as a terminal table
type TableFormatter struct{}

// hyperlink creates a clickable terminal hyperlink using OSC 8
// Format: \033]8;;URL\033\\TEXT\033]8;;\033\\
func hyperlink(text, url string) string {
	// Only use hyperlinks if stdout is a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return text
	}
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}

// stripAnsi removes ANSI escape sequences from a string
func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// displayWidth returns the visible width of a string in terminal columns
// accounting for wide characters like emojis (which take 2 columns)
// and stripping ANSI escape sequences
func displayWidth(s string) int {
	return runewidth.StringWidth(stripAnsi(s))
}

// truncateToWidth truncates a string to fit within maxWidth display columns
// It handles ANSI escape sequences by stripping them for width calculation
func truncateToWidth(s string, maxWidth int) (string, int) {
	// Strip ANSI codes for width calculation
	plain := stripAnsi(s)
	width := runewidth.StringWidth(plain)

	// If it fits, return as-is
	if width <= maxWidth {
		return s, width
	}

	// Need to truncate - work with plain text to find cut point
	cutWidth := 0
	cutIndex := 0
	for i, r := range plain {
		rw := runewidth.RuneWidth(r)
		if cutWidth+rw > maxWidth-3 { // Leave room for "..."
			cutIndex = i
			break
		}
		cutWidth += rw
	}

	// For strings with ANSI codes, we need to be smarter about where to cut
	// Just strip and truncate the plain version, then add ellipsis
	if cutIndex > 0 && cutIndex < len(plain) {
		return plain[:cutIndex] + "...", maxWidth
	}
	return plain[:maxWidth-3] + "...", maxWidth
}

// padRight pads a string with spaces to reach the target visible width
func padRight(s string, visibleWidth, targetWidth int) string {
	if visibleWidth >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-visibleWidth)
}

// Format outputs prioritized items as a table
func (f *TableFormatter) Format(items []priority.PrioritizedItem, w io.Writer) error {
	if len(items) == 0 {
		fmt.Fprintln(w, "No notifications found.")
		return nil
	}

	// Column widths
	const (
		colPriority = 8
		colType     = 5
		colRepo     = 26
		colTitle    = 40
		colStatus   = 20
		colAge      = 5
	)

	// Header
	fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		colPriority, "Priority",
		colType, "Type",
		colRepo, "Repository",
		colTitle, "Title",
		colStatus, "Status",
		"Age")
	fmt.Fprintln(w, strings.Repeat("-", colPriority+colType+colRepo+colTitle+colStatus+colAge+12))

	for _, item := range items {
		n := item.Notification

		// Determine type indicator
		typeStr := "ISS"
		if n.Details != nil && n.Details.IsPR {
			typeStr = "PR"
		} else if n.Subject.Type == "PullRequest" {
			typeStr = "PR"
		}

		// Build title with indicators
		title := n.Subject.Title

		// Add quick win indicator
		if item.Category == priority.CategoryLowHanging {
			title = "⚡ " + title
		}

		// Add hot topic indicator (>10 comments)
		if n.Details != nil && n.Details.CommentCount > 10 {
			title = "🔥 " + title
		}

		// Truncate title if too long (using display width for emoji support)
		title, visibleTitleLen := truncateToWidth(title, colTitle)

		// Add state indicator for closed items
		if n.Details != nil && (n.Details.State == "closed" || n.Details.Merged) {
			suffix := " [closed]"
			if n.Details.Merged {
				suffix = " [merged]"
			}
			suffixWidth := displayWidth(suffix)
			if visibleTitleLen+suffixWidth > colTitle {
				title, _ = truncateToWidth(title, colTitle-suffixWidth)
			}
			title = title + suffix
			visibleTitleLen = displayWidth(title)
		}

		// Truncate repo if too long
		repo := n.Repository.FullName
		repo, _ = truncateToWidth(repo, colRepo)

		// Get URL for hyperlink
		url := ""
		if n.Details != nil && n.Details.HTMLURL != "" {
			url = n.Details.HTMLURL
		} else {
			url = n.Repository.HTMLURL
		}

		// Create hyperlinked title and pad it
		linkedTitle := hyperlink(title, url)
		linkedTitle = padRight(linkedTitle, visibleTitleLen, colTitle)

		// Format priority with color and pad
		priorityDisplay := item.Priority.Display()
		priorityStr := padRight(colorPriority(item.Priority), len(priorityDisplay), colPriority)

		// Build status column (review state, PR size, or comment count)
		statusRes := formatStatus(n, item)
		statusText := statusRes.text
		statusWidth := statusRes.visibleWidth
		if statusWidth > colStatus {
			// Truncate if needed - use plain text truncation
			statusText, statusWidth = truncateToWidth(statusText, colStatus)
		}
		statusText = padRight(statusText, statusWidth, colStatus)

		// Calculate age
		age := formatAge(time.Since(n.UpdatedAt))

		fmt.Fprintf(w, "%s  %-*s  %s  %s  %s  %s\n",
			priorityStr,
			colType, typeStr,
			padRight(repo, displayWidth(repo), colRepo),
			linkedTitle,
			statusText,
			age,
		)
	}

	// Print enhanced footer summary
	printFooterSummary(items, w)

	return nil
}

// statusResult holds both the display string and its visible width
type statusResult struct {
	text        string
	visibleWidth int
}

// formatStatus builds the status column showing review state, PR size, or activity
// Returns the formatted string and its visible width (excluding ANSI codes)
func formatStatus(n github.Notification, _ priority.PrioritizedItem) statusResult {
	if n.Details == nil {
		reason := string(n.Reason)
		return statusResult{reason, len(reason)}
	}

	d := n.Details

	// For PRs, show review state and size
	if d.IsPR {
		var textParts []string
		var plainParts []string

		// Review state with color
		switch d.ReviewState {
		case "approved":
			textParts = append(textParts, color.GreenString("✓ APPROVED"))
			plainParts = append(plainParts, "✓ APPROVED")
		case "changes_requested":
			textParts = append(textParts, color.YellowString("△ CHANGES"))
			plainParts = append(plainParts, "△ CHANGES")
		case "pending", "review_required":
			textParts = append(textParts, color.CyanString("○ REVIEW"))
			plainParts = append(plainParts, "○ REVIEW")
		}

		// PR size (compact format)
		totalChanges := d.Additions + d.Deletions
		if totalChanges > 0 {
			sizeText, sizePlain := formatPRSize(d.Additions, d.Deletions)
			textParts = append(textParts, sizeText)
			plainParts = append(plainParts, sizePlain)
		}

		if len(textParts) > 0 {
			text := strings.Join(textParts, " ")
			plain := strings.Join(plainParts, " ")
			return statusResult{text, runewidth.StringWidth(plain)}
		}
	}

	// For issues or PRs without specific status, show comment activity
	if d.CommentCount > 0 {
		text := fmt.Sprintf("💬 %d comments", d.CommentCount)
		return statusResult{text, runewidth.StringWidth(text)}
	}

	reason := string(n.Reason)
	return statusResult{reason, len(reason)}
}

// formatPRSize returns a compact representation of PR size
// Returns both the colored string and the plain string for width calculation
func formatPRSize(additions, deletions int) (colored string, plain string) {
	total := additions + deletions

	// T-shirt sizing based on total changes
	var sizeColored, sizePlain string
	switch {
	case total <= 10:
		sizePlain = "XS"
		sizeColored = color.GreenString(sizePlain)
	case total <= 50:
		sizePlain = "S"
		sizeColored = color.GreenString(sizePlain)
	case total <= 200:
		sizePlain = "M"
		sizeColored = color.YellowString(sizePlain)
	case total <= 500:
		sizePlain = "L"
		sizeColored = color.YellowString(sizePlain)
	default:
		sizePlain = "XL"
		sizeColored = color.RedString(sizePlain)
	}

	plain = fmt.Sprintf("%s+%d/-%d", sizePlain, additions, deletions)
	colored = fmt.Sprintf("%s+%d/-%d", sizeColored, additions, deletions)
	return colored, plain
}

// printFooterSummary prints an enhanced summary footer
func printFooterSummary(items []priority.PrioritizedItem, w io.Writer) {
	var urgentCount, quickWinCount, reviewCount, hotCount int

	for _, item := range items {
		n := item.Notification

		if item.Priority == priority.PriorityUrgent {
			urgentCount++
		}
		if item.Category == priority.CategoryLowHanging {
			quickWinCount++
		}
		if n.Reason == "review_requested" {
			reviewCount++
		}
		if n.Details != nil && n.Details.CommentCount > 10 {
			hotCount++
		}
	}

	// Only print if there's something actionable
	if urgentCount == 0 && quickWinCount == 0 && reviewCount == 0 && hotCount == 0 {
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("━", 60))

	if urgentCount > 0 {
		fmt.Fprintf(w, "  %s %s urgent items need attention\n",
			color.RedString("●"),
			color.RedString("%d", urgentCount))
	}
	if reviewCount > 0 {
		fmt.Fprintf(w, "  %s %d PRs awaiting your review\n",
			color.CyanString("○"),
			reviewCount)
	}
	if quickWinCount > 0 {
		fmt.Fprintf(w, "  ⚡ %d quick wins available\n", quickWinCount)
	}
	if hotCount > 0 {
		fmt.Fprintf(w, "  🔥 %d hot discussions\n", hotCount)
	}
}

// FormatSummary outputs a summary
func (f *TableFormatter) FormatSummary(summary priority.Summary, w io.Writer) error {
	fmt.Fprintf(w, "Total notifications: %d\n\n", summary.Total)

	fmt.Fprintln(w, "By Priority:")
	for p := priority.PriorityUrgent; p >= priority.PriorityLow; p-- {
		count := summary.ByPriority[p]
		if count > 0 {
			fmt.Fprintf(w, "  %s: %d\n", colorPriority(p), count)
		}
	}

	fmt.Fprintln(w, "\nBy Category:")
	categories := []priority.Category{
		priority.CategoryUrgent,
		priority.CategoryImportant,
		priority.CategoryLowHanging,
		priority.CategoryFYI,
	}
	for _, cat := range categories {
		count := summary.ByCategory[cat]
		if count > 0 {
			fmt.Fprintf(w, "  %s: %d\n", cat.Display(), count)
		}
	}

	if len(summary.TopUrgent) > 0 {
		fmt.Fprintln(w, "\nTop Urgent Items:")
		for i, item := range summary.TopUrgent {
			fmt.Fprintf(w, "  %d. [%s] %s - %s\n",
				i+1,
				item.Notification.Repository.FullName,
				truncate(item.Notification.Subject.Title, 50),
				item.ActionNeeded,
			)
		}
	}

	if len(summary.QuickWins) > 0 {
		fmt.Fprintln(w, "\nQuick Wins:")
		for i, item := range summary.QuickWins {
			fmt.Fprintf(w, "  %d. [%s] %s\n",
				i+1,
				item.Notification.Repository.FullName,
				truncate(item.Notification.Subject.Title, 50),
			)
		}
	}

	return nil
}

// FormatWithAnalysis outputs items with LLM analysis
func (f *TableFormatter) FormatWithAnalysis(items []priority.PrioritizedItem, w io.Writer) error {
	for i, item := range items {
		n := item.Notification

		fmt.Fprintf(w, "%s #%d: %s\n", colorPriority(item.Priority), i+1, n.Subject.Title)
		fmt.Fprintf(w, "  Repository: %s\n", n.Repository.FullName)
		fmt.Fprintf(w, "  Type: %s | Reason: %s | Age: %s\n",
			n.Subject.Type, n.Reason, formatAge(time.Since(n.UpdatedAt)))
		fmt.Fprintf(w, "  Category: %s\n", item.Category.Display())
		fmt.Fprintf(w, "  Action: %s\n", item.ActionNeeded)

		if n.Details != nil {
			d := n.Details
			fmt.Fprintf(w, "  State: %s | Comments: %d\n", d.State, d.CommentCount)
			if d.IsPR {
				fmt.Fprintf(w, "  PR: +%d/-%d (%d files) | Review: %s\n",
					d.Additions, d.Deletions, d.ChangedFiles, d.ReviewState)
			}
			if len(d.Labels) > 0 {
				fmt.Fprintf(w, "  Labels: %s\n", strings.Join(d.Labels, ", "))
			}
		}

		if item.Analysis != nil {
			a := item.Analysis
			fmt.Fprintf(w, "\n  AI Analysis:\n")
			fmt.Fprintf(w, "    Summary: %s\n", a.Summary)
			fmt.Fprintf(w, "    Action: %s\n", a.ActionNeeded)
			fmt.Fprintf(w, "    Effort: %s\n", a.EffortEstimate)
			if len(a.Tags) > 0 {
				fmt.Fprintf(w, "    Tags: %s\n", strings.Join(a.Tags, ", "))
			}
			if len(a.Blockers) > 0 {
				fmt.Fprintf(w, "    Blockers: %s\n", strings.Join(a.Blockers, ", "))
			}
		}

		fmt.Fprintln(w)
	}

	return nil
}

func colorPriority(p priority.PriorityLevel) string {
	switch p {
	case priority.PriorityUrgent:
		return color.RedString("URGENT")
	case priority.PriorityHigh:
		return color.YellowString("HIGH")
	case priority.PriorityMedium:
		return color.CyanString("MEDIUM")
	default:
		return color.WhiteString("LOW")
	}
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
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
	weeks := days / 7
	if weeks < 4 {
		return fmt.Sprintf("%dw", weeks)
	}
	months := days / 30
	return fmt.Sprintf("%dmo", months)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
