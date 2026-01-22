package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hal/priority/internal/priority"
)

// MarkdownFormatter formats output as Markdown
type MarkdownFormatter struct{}

// Format outputs prioritized items as Markdown
func (f *MarkdownFormatter) Format(items []priority.PrioritizedItem, w io.Writer) error {
	if len(items) == 0 {
		fmt.Fprintln(w, "No notifications found.")
		return nil
	}

	fmt.Fprintln(w, "# GitHub Notifications Priority Report")
	fmt.Fprintf(w, "\n*Generated: %s*\n\n", time.Now().Format("2006-01-02 15:04"))

	// Group by category
	categories := map[priority.Category][]priority.PrioritizedItem{
		priority.CategoryUrgent:    {},
		priority.CategoryImportant: {},
		priority.CategoryLowHanging: {},
		priority.CategoryFYI:        {},
	}

	for _, item := range items {
		categories[item.Category] = append(categories[item.Category], item)
	}

	// Output each category
	categoryOrder := []priority.Category{
		priority.CategoryUrgent,
		priority.CategoryImportant,
		priority.CategoryLowHanging,
		priority.CategoryFYI,
	}

	for _, cat := range categoryOrder {
		catItems := categories[cat]
		if len(catItems) == 0 {
			continue
		}

		fmt.Fprintf(w, "## %s (%d)\n\n", getCategoryEmoji(cat)+" "+cat.Display(), len(catItems))

		for _, item := range catItems {
			f.formatItem(item, w)
		}
	}

	return nil
}

func (f *MarkdownFormatter) formatItem(item priority.PrioritizedItem, w io.Writer) {
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
func (f *MarkdownFormatter) FormatSummary(summary priority.Summary, w io.Writer) error {
	fmt.Fprintln(w, "# Notification Summary")
	fmt.Fprintf(w, "\n*Total: %d notifications*\n\n", summary.Total)

	fmt.Fprintln(w, "## By Priority")
	fmt.Fprintln(w, "| Priority | Count |")
	fmt.Fprintln(w, "|----------|-------|")
	for p := priority.PriorityUrgent; p >= priority.PriorityLow; p-- {
		if count := summary.ByPriority[p]; count > 0 {
			fmt.Fprintf(w, "| %s | %d |\n", p.Display(), count)
		}
	}

	fmt.Fprintln(w, "\n## By Category")
	fmt.Fprintln(w, "| Category | Count |")
	fmt.Fprintln(w, "|----------|-------|")
	for _, cat := range []priority.Category{
		priority.CategoryUrgent,
		priority.CategoryImportant,
		priority.CategoryLowHanging,
		priority.CategoryFYI,
	} {
		if count := summary.ByCategory[cat]; count > 0 {
			fmt.Fprintf(w, "| %s %s | %d |\n", getCategoryEmoji(cat), cat.Display(), count)
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

func getCategoryEmoji(cat priority.Category) string {
	switch cat {
	case priority.CategoryUrgent:
		return "🔴"
	case priority.CategoryImportant:
		return "🟡"
	case priority.CategoryLowHanging:
		return "🟢"
	case priority.CategoryFYI:
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
