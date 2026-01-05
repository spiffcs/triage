package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hal/github-prio/internal/priority"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

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

// displayWidth returns the visible width of a string in terminal columns
// accounting for wide characters like emojis (which take 2 columns)
func displayWidth(s string) int {
	return runewidth.StringWidth(s)
}

// truncateToWidth truncates a string to fit within maxWidth display columns
func truncateToWidth(s string, maxWidth int) (string, int) {
	width := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if width+rw > maxWidth-3 { // Leave room for "..."
			return s[:i] + "...", maxWidth
		}
		width += rw
	}
	return s, width
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
		colCategory = 12
		colRepo     = 30
		colTitle    = 50
		colReason   = 18
	)

	// Header
	fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		colPriority, "Priority",
		colCategory, "Category",
		colRepo, "Repository",
		colTitle, "Title",
		colReason, "Reason",
		"Age")
	fmt.Fprintln(w, strings.Repeat("-", colPriority+colCategory+colRepo+colTitle+colReason+20))

	for _, item := range items {
		n := item.Notification

		// Truncate title if too long (using display width for emoji support)
		title := n.Subject.Title
		title, visibleTitleLen := truncateToWidth(title, colTitle)

		// Add state indicator for closed items
		if n.Details != nil && n.Details.State == "closed" {
			suffix := " [closed]"
			suffixWidth := displayWidth(suffix)
			if visibleTitleLen+suffixWidth > colTitle {
				title, _ = truncateToWidth(n.Subject.Title, colTitle-suffixWidth)
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

		// Reason
		reason := string(n.Reason)
		reason, reasonWidth := truncateToWidth(reason, colReason)
		reason = padRight(reason, reasonWidth, colReason)

		// Calculate age
		age := formatAge(time.Since(n.UpdatedAt))

		fmt.Fprintf(w, "%s  %-*s  %s  %s  %s  %s\n",
			priorityStr,
			colCategory, item.Category.Display(),
			padRight(repo, displayWidth(repo), colRepo),
			linkedTitle,
			reason,
			age,
		)
	}

	// Print action summary if there are urgent items
	urgentCount := 0
	for _, item := range items {
		if item.Priority == priority.PriorityUrgent {
			urgentCount++
		}
	}

	if urgentCount > 0 {
		fmt.Fprintf(w, "\n%s urgent items need your attention.\n", color.RedString("%d", urgentCount))
	}

	return nil
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
