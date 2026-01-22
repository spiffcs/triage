package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hal/triage/internal/triage"
)

// MarkdownFormatter formats output as Markdown
type MarkdownFormatter struct{}

// Format outputs prioritized items as Markdown
func (f *MarkdownFormatter) Format(items []triage.PrioritizedItem, w io.Writer) error {
	if len(items) == 0 {
		fmt.Fprintln(w, "No notifications found.")
		return nil
	}

	fmt.Fprintln(w, "# GitHub Notifications Priority Report")
	fmt.Fprintf(w, "\n*Generated: %s*\n\n", time.Now().Format("2006-01-02 15:04"))

	// Group by priority
	priorities := map[triage.PriorityLevel][]triage.PrioritizedItem{
		triage.PriorityUrgent:    {},
		triage.PriorityImportant: {},
		triage.PriorityQuickWin:  {},
		triage.PriorityFYI:       {},
	}

	for _, item := range items {
		priorities[item.Priority] = append(priorities[item.Priority], item)
	}

	// Output each priority
	priorityOrder := []triage.PriorityLevel{
		triage.PriorityUrgent,
		triage.PriorityImportant,
		triage.PriorityQuickWin,
		triage.PriorityFYI,
	}

	for _, p := range priorityOrder {
		pItems := priorities[p]
		if len(pItems) == 0 {
			continue
		}

		fmt.Fprintf(w, "## %s (%d)\n\n", getPriorityEmoji(p)+" "+p.Display(), len(pItems))

		for _, item := range pItems {
			f.formatItem(item, w)
		}
	}

	return nil
}

func (f *MarkdownFormatter) formatItem(item triage.PrioritizedItem, w io.Writer) {
	n := item.Notification
	d := n.Details

	// Determine URL
	url := n.Repository.HTMLURL
	if d != nil && d.HTMLURL != "" {
		url = d.HTMLURL
	}

	// Title with link
	fmt.Fprintf(w, "### [%s](%s)\n\n", n.Subject.Title, url)

	// Metadata
	fmt.Fprintf(w, "- **Repository:** %s\n", n.Repository.FullName)
	fmt.Fprintf(w, "- **Type:** %s\n", n.Subject.Type)
	fmt.Fprintf(w, "- **Reason:** %s\n", n.Reason)
	fmt.Fprintf(w, "- **Updated:** %s ago\n", formatDuration(time.Since(n.UpdatedAt)))
	fmt.Fprintf(w, "- **Action:** %s\n", item.ActionNeeded)

	if d != nil {
		fmt.Fprintf(w, "- **State:** %s\n", d.State)
		if len(d.Labels) > 0 {
			fmt.Fprintf(w, "- **Labels:** %s\n", formatLabels(d.Labels))
		}
		if d.IsPR {
			fmt.Fprintf(w, "- **Changes:** +%d/-%d (%d files)\n", d.Additions, d.Deletions, d.ChangedFiles)
			if d.ReviewState != "" {
				fmt.Fprintf(w, "- **Review Status:** %s\n", d.ReviewState)
			}
		}
	}

	fmt.Fprintln(w)
}

// FormatSummary outputs a summary as Markdown
func (f *MarkdownFormatter) FormatSummary(summary triage.Summary, w io.Writer) error {
	fmt.Fprintln(w, "# Notification Summary")
	fmt.Fprintf(w, "\n*Total: %d notifications*\n\n", summary.Total)

	fmt.Fprintln(w, "## By Priority")
	fmt.Fprintln(w, "| Priority | Count |")
	fmt.Fprintln(w, "|----------|-------|")
	for _, p := range []triage.PriorityLevel{
		triage.PriorityUrgent,
		triage.PriorityImportant,
		triage.PriorityQuickWin,
		triage.PriorityFYI,
	} {
		if count := summary.ByPriority[p]; count > 0 {
			fmt.Fprintf(w, "| %s %s | %d |\n", getPriorityEmoji(p), p.Display(), count)
		}
	}

	fmt.Fprintln(w, "\n## By Category")
	fmt.Fprintln(w, "| Category | Count |")
	fmt.Fprintln(w, "|----------|-------|")
	for cat := triage.CategoryUrgent; cat >= triage.CategoryLow; cat-- {
		if count := summary.ByCategory[cat]; count > 0 {
			fmt.Fprintf(w, "| %s | %d |\n", cat.Display(), count)
		}
	}

	if len(summary.TopUrgent) > 0 {
		fmt.Fprintln(w, "\n## Top Urgent Items")
		for i, item := range summary.TopUrgent {
			fmt.Fprintf(w, "%d. **%s** - %s (%s)\n",
				i+1,
				item.Notification.Subject.Title,
				item.Notification.Repository.FullName,
				item.ActionNeeded,
			)
		}
	}

	if len(summary.QuickWins) > 0 {
		fmt.Fprintln(w, "\n## Quick Wins")
		for i, item := range summary.QuickWins {
			fmt.Fprintf(w, "%d. **%s** - %s\n",
				i+1,
				item.Notification.Subject.Title,
				item.Notification.Repository.FullName,
			)
		}
	}

	return nil
}

func getPriorityEmoji(p triage.PriorityLevel) string {
	switch p {
	case triage.PriorityUrgent:
		return "🔴"
	case triage.PriorityImportant:
		return "🟡"
	case triage.PriorityQuickWin:
		return "🟢"
	case triage.PriorityFYI:
		return "ℹ️"
	default:
		return "📋"
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

func formatLabels(labels []string) string {
	formatted := make([]string, len(labels))
	for i, l := range labels {
		formatted[i] = "`" + l + "`"
	}
	return strings.Join(formatted, " ")
}
