package output

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/hal/triage/internal/github"
	"github.com/hal/triage/internal/triage"
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
		// Skip standalone variation selectors (shouldn't happen but be safe)
		if r == '\uFE0F' {
			continue
		}
		width += runewidth.RuneWidth(r)
	}
	return width
}

// truncateToWidth truncates a string to fit within maxWidth display columns
// It handles ANSI escape sequences by preserving them in the output
func truncateToWidth(s string, maxWidth int) (string, int) {
	// Use emoji-aware width calculation
	width := displayWidth(s)

	// If it fits, return as-is
	if width <= maxWidth {
		return s, width
	}

	// Need to truncate - find how many visible characters we can keep
	targetWidth := maxWidth - 3 // Leave room for "..."

	// Find all ANSI sequences and their positions in the original string
	matches := ansiRegex.FindAllStringIndex(s, -1)

	// Build result by walking through the string
	var result strings.Builder
	visibleWidth := 0
	pos := 0
	matchIdx := 0

	for pos < len(s) && visibleWidth < targetWidth {
		// Check if current position is the start of an ANSI sequence
		if matchIdx < len(matches) && pos == matches[matchIdx][0] {
			// Include the ANSI sequence without counting its width
			result.WriteString(s[matches[matchIdx][0]:matches[matchIdx][1]])
			pos = matches[matchIdx][1]
			matchIdx++
			continue
		}

		// Get the next rune
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

		// Check if adding this rune would exceed our target
		if visibleWidth+rw > targetWidth {
			break
		}

		result.WriteString(s[pos : pos+size])
		visibleWidth += rw
		pos += size
	}

	// Add ellipsis and reset code (in case we were in the middle of a color)
	result.WriteString("...\033[0m")

	return result.String(), maxWidth
}

// padRight pads a string with spaces to reach the target visible width
func padRight(s string, visibleWidth, targetWidth int) string {
	if visibleWidth >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-visibleWidth)
}

// Format outputs prioritized items as a table
func (f *TableFormatter) Format(items []triage.PrioritizedItem, w io.Writer) error {
	if len(items) == 0 {
		fmt.Fprintln(w, "No notifications found.")
		return nil
	}

	// Column widths
	const (
		colPriority = 9
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
		if item.Priority == triage.PriorityQuickWin {
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
		coloredPriority := colorPriority(item.Priority)
		priorityStr := padRight(coloredPriority, displayWidth(coloredPriority), colPriority)

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
func formatStatus(n github.Notification, _ triage.PrioritizedItem) statusResult {
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
		case "approved":
			textParts = append(textParts, color.GreenString("+ APPROVED"))
			plainParts = append(plainParts, "+ APPROVED")
		case "changes_requested":
			textParts = append(textParts, color.YellowString("! CHANGES"))
			plainParts = append(plainParts, "! CHANGES")
		case "pending", "review_required":
			textParts = append(textParts, color.CyanString("* REVIEW"))
			plainParts = append(plainParts, "* REVIEW")
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
		text := fmt.Sprintf("%d comments", d.CommentCount)
		return statusResult{text, len(text)}
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
func printFooterSummary(items []triage.PrioritizedItem, w io.Writer) {
	var urgentCount, quickWinCount, reviewCount, hotCount int

	for _, item := range items {
		n := item.Notification

		if item.Priority == triage.PriorityUrgent {
			urgentCount++
		}
		if item.Priority == triage.PriorityQuickWin {
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
			color.RedString("!"),
			color.RedString("%d", urgentCount))
	}
	if reviewCount > 0 {
		fmt.Fprintf(w, "  %s %d PRs awaiting your review\n",
			color.CyanString("*"),
			reviewCount)
	}
	if quickWinCount > 0 {
		fmt.Fprintf(w, "  ⚡ %d quick wins available\n", quickWinCount)
	}
	if hotCount > 0 {
		fmt.Fprintf(w, "  🔥 %d hot discussions\n", hotCount)
	}
}

// FormatVerbose outputs items with detailed information
func (f *TableFormatter) FormatVerbose(items []triage.PrioritizedItem, w io.Writer) error {
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

		fmt.Fprintln(w)
	}

	return nil
}

func colorPriority(p triage.PriorityLevel) string {
	switch p {
	case triage.PriorityUrgent:
		return color.RedString("Urgent")
	case triage.PriorityImportant:
		return color.YellowString("Important")
	case triage.PriorityQuickWin:
		return color.GreenString("Quick Win")
	default:
		return color.WhiteString("FYI")
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
