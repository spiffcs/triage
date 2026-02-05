package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/format"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/stats"
	"github.com/spiffcs/triage/internal/triage"
)

// barEntry represents a single segment of a horizontal bar chart.
type barEntry struct {
	Label string
	Count int
	Style lipgloss.Style
}

// sparkline characters from lowest to highest.
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Partial block characters for sub-character resolution (1/8 to 8/8).
var partialBlocks = []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

// statsDistributions holds all computed distributions for rendering.
type statsDistributions struct {
	totalCount    int
	prCount       int
	issueCount    int
	assignedCount int
	blockedCount  int
	priorityCount int
	orphanedCount int

	// Raw priority counts
	urgentCount    int
	importantCount int
	quickWinCount  int
	notableCount   int
	fyiCount       int

	// Raw CI counts
	ciSuccessCount int
	ciFailureCount int
	ciPendingCount int

	priority []barEntry
	age      []barEntry
	review   []barEntry
	ci       []barEntry
	prSize   []barEntry
	topRepos []barEntry
	// repoOthersCount is the count of repos not in the top 5.
	repoOthersCount int
	// repoOthersRepos is the number of repos not in the top 5.
	repoOthersRepos int
	orphanedStale   []barEntry
}

// computeDistributions iterates all items once to compute all bar chart data.
func computeDistributions(m ListModel) statsDistributions {
	d := statsDistributions{
		assignedCount: len(m.assignedItems),
		blockedCount:  len(m.blockedItems),
		priorityCount: len(m.priorityItems),
		orphanedCount: len(m.orphanedItems),
	}

	// Priority counts
	var urgent, important, quickWin, notable, fyi int
	// Age buckets
	var age1d, age3d, age7d, age2w, age4w, ageOld int
	// Review states
	var revApproved, revChanges, revPending, revNone int
	// CI statuses
	var ciSuccess, ciFailure, ciPending, ciNone int
	// PR sizes
	var sizeXS, sizeS, sizeM, sizeL, sizeXL int
	// Repo frequency
	repoCounts := make(map[string]int)
	// Orphaned staleness
	var stale7, stale14, stale30, staleOld int

	for _, item := range m.items {
		d.totalCount++

		isPR := item.Type == model.ItemTypePullRequest || item.Subject.Type == model.SubjectPullRequest
		if isPR {
			d.prCount++
		} else {
			d.issueCount++
		}

		// Priority
		switch item.Priority {
		case triage.PriorityUrgent:
			urgent++
		case triage.PriorityImportant:
			important++
		case triage.PriorityQuickWin:
			quickWin++
		case triage.PriorityNotable:
			notable++
		case triage.PriorityFYI:
			fyi++
		}

		// Age (since created)
		age := time.Since(item.CreatedAt)
		days := int(age.Hours() / 24)
		switch {
		case days < 1:
			age1d++
		case days < 3:
			age3d++
		case days < 7:
			age7d++
		case days < 14:
			age2w++
		case days < 28:
			age4w++
		default:
			ageOld++
		}

		// PR-specific stats
		pr := item.PRDetails()
		if isPR && pr != nil {
			// Review state
			switch pr.ReviewState {
			case model.ReviewStateApproved:
				revApproved++
			case model.ReviewStateChangesRequested:
				revChanges++
			case model.ReviewStatePending, model.ReviewStateReviewRequired, model.ReviewStateReviewed:
				revPending++
			default:
				revNone++
			}

			// CI status
			switch pr.CIStatus {
			case model.CIStatusSuccess:
				ciSuccess++
			case model.CIStatusFailure:
				ciFailure++
			case model.CIStatusPending:
				ciPending++
			default:
				ciNone++
			}

			// PR size
			totalChanges := pr.Additions + pr.Deletions
			if totalChanges > 0 {
				thresholds := format.PRSizeThresholds{
					XS: m.prSizeXS,
					S:  m.prSizeS,
					M:  m.prSizeM,
					L:  m.prSizeL,
				}
				result := format.CalculatePRSize(pr.Additions, pr.Deletions, thresholds)
				switch result.Size {
				case format.PRSizeXS:
					sizeXS++
				case format.PRSizeS:
					sizeS++
				case format.PRSizeM:
					sizeM++
				case format.PRSizeL:
					sizeL++
				case format.PRSizeXL:
					sizeXL++
				}
			}
		}

		// Repo frequency
		repoCounts[item.Repository.FullName]++

		// Orphaned staleness
		if item.Reason == model.ReasonOrphaned {
			staleDays := daysSinceTeamActivity(item)
			switch {
			case staleDays < 7:
				stale7++
			case staleDays < 14:
				stale14++
			case staleDays < 30:
				stale30++
			default:
				staleOld++
			}
		}
	}

	// Store raw counts for snapshot use
	d.urgentCount = urgent
	d.importantCount = important
	d.quickWinCount = quickWin
	d.notableCount = notable
	d.fyiCount = fyi
	d.ciSuccessCount = ciSuccess
	d.ciFailureCount = ciFailure
	d.ciPendingCount = ciPending

	// Build bar entries (only include non-zero counts)
	d.priority = filterZero([]barEntry{
		{"Urgent", urgent, listUrgentStyle},
		{"Important", important, listImportantStyle},
		{"Quick Win", quickWin, listQuickWinStyle},
		{"Notable", notable, listNotableStyle},
		{"FYI", fyi, listFYIStyle},
	})

	d.age = filterZero([]barEntry{
		{"<1d", age1d, listAgeRecentStyle},
		{"1-3d", age3d, listAgeRecentStyle},
		{"3-7d", age7d, listAgeModerateStyle},
		{"1-2w", age2w, listAgeWarningStyle},
		{"2-4w", age4w, listAgeCriticalStyle},
		{">4w", ageOld, listAgeCriticalStyle},
	})

	d.review = filterZero([]barEntry{
		{"✓ Approved", revApproved, listApprovedStyle},
		{"! Changes", revChanges, listChangesStyle},
		{"○ Pending", revPending, listReviewStyle},
		{"─ None", revNone, statsDimStyle},
	})

	d.ci = filterZero([]barEntry{
		{"✓", ciSuccess, listCISuccessStyle},
		{"✗", ciFailure, listCIFailureStyle},
		{"○", ciPending, listCIPendingStyle},
		{"─", ciNone, statsDimStyle},
	})

	d.prSize = filterZero([]barEntry{
		{"XS", sizeXS, listSizeSmallStyle},
		{"S", sizeS, listSizeSmallStyle},
		{"M", sizeM, listSizeMediumStyle},
		{"L", sizeL, listSizeMediumStyle},
		{"XL", sizeXL, listSizeLargeStyle},
	})

	// Top repos: sort by frequency descending, take top 5
	type repoFreq struct {
		name  string
		count int
	}
	var repos []repoFreq
	for name, count := range repoCounts {
		repos = append(repos, repoFreq{name, count})
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].count > repos[j].count
	})

	topN := 5
	if len(repos) < topN {
		topN = len(repos)
	}
	for i := range topN {
		d.topRepos = append(d.topRepos, barEntry{
			Label: repos[i].name,
			Count: repos[i].count,
			Style: statsRepoStyle,
		})
	}
	if len(repos) > 5 {
		for _, r := range repos[5:] {
			d.repoOthersCount += r.count
		}
		d.repoOthersRepos = len(repos) - 5
	}

	d.orphanedStale = filterZero([]barEntry{
		{"<7d", stale7, listAgeRecentStyle},
		{"7-14d", stale14, listAgeModerateStyle},
		{"14-30d", stale30, listAgeWarningStyle},
		{">30d", staleOld, listAgeCriticalStyle},
	})

	return d
}

// filterZero removes entries with zero count.
func filterZero(entries []barEntry) []barEntry {
	var result []barEntry
	for _, e := range entries {
		if e.Count > 0 {
			result = append(result, e)
		}
	}
	return result
}

// maxBarChars is the maximum character width for bar segments.
// Capped to prevent bars from stretching across wide terminals.
const maxBarChars = 40

// renderBars renders a vertical list of bars (one entry per line).
// Each bar is scaled proportionally to the max count in the group,
// with all bars starting at the same column for easy comparison.
//
// Format per line:
//
//	{label padded}  {colored bar}  {count}
func renderBars(entries []barEntry, barWidth int) []string {
	if len(entries) == 0 {
		return []string{statsDimStyle.Render("  ─")}
	}

	// Find max count for scaling and max label width for alignment
	maxCount := 0
	maxLabel := 0
	for _, e := range entries {
		if e.Count > maxCount {
			maxCount = e.Count
		}
		if len(e.Label) > maxLabel {
			maxLabel = len(e.Label)
		}
	}
	if maxCount == 0 {
		return []string{statsDimStyle.Render("  ─")}
	}

	// Cap bar width
	bw := barWidth
	bw = min(bw, maxBarChars)
	bw = max(bw, 4)

	var lines []string
	for _, e := range entries {
		// Scale bar length to max count
		fracWidth := float64(e.Count) / float64(maxCount) * float64(bw)
		fullBlocks := int(fracWidth)
		remainder := fracWidth - float64(fullBlocks)

		bar := strings.Repeat("█", fullBlocks)
		if remainder >= 0.125 {
			idx := int(remainder * 8)
			idx = min(idx, 7)
			bar += partialBlocks[idx]
		}
		if bar == "" {
			bar = partialBlocks[0]
		}

		coloredBar := e.Style.Render(bar)
		label := fmt.Sprintf("%-*s", maxLabel, e.Label)
		line := fmt.Sprintf("    %s  %s  %d", label, coloredBar, e.Count)
		lines = append(lines, line)
	}

	return lines
}

// renderSparkline renders a sparkline from a series of float64 values.
// Width is the number of characters to use (values will be sampled/averaged to fit).
func renderSparkline(values []float64, width int) string {
	if len(values) == 0 || width <= 0 {
		return ""
	}

	// Find min/max for scaling
	minVal, maxVal := values[0], values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Resample values to fit width
	resampled := resampleValues(values, width)

	valRange := maxVal - minVal
	if valRange == 0 {
		// All values are the same, show a flat middle line
		return strings.Repeat(string(sparkBlocks[3]), len(resampled))
	}

	var b strings.Builder
	for _, v := range resampled {
		normalized := (v - minVal) / valRange
		idx := int(normalized * float64(len(sparkBlocks)-1))
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		b.WriteRune(sparkBlocks[idx])
	}

	return b.String()
}

// resampleValues resamples a slice of float64 to the target width using averaging.
func resampleValues(values []float64, width int) []float64 {
	if len(values) <= width {
		return values
	}

	result := make([]float64, width)
	step := float64(len(values)) / float64(width)

	for i := range width {
		start := int(float64(i) * step)
		end := int(float64(i+1) * step)
		if end > len(values) {
			end = len(values)
		}
		if start >= end {
			if start < len(values) {
				result[i] = values[start]
			}
			continue
		}
		sum := 0.0
		for j := start; j < end; j++ {
			sum += values[j]
		}
		result[i] = sum / float64(end-start)
	}

	return result
}

// statsSparklineWidth is the target character width for sparklines.
const statsSparklineWidth = 50

// separatorWidth is the fixed width for section separator lines.
const separatorWidth = 60

// statsContentHeight returns the number of lines the stats view will render.
// This avoids a full render just to count newlines (used for scroll clamping).
func statsContentHeight(m ListModel) int {
	d := computeDistributions(m)

	// sectionHeight: separator + label + bar lines (min 1 for empty placeholder)
	sectionHeight := func(entries []barEntry) int {
		n := len(entries)
		if n == 0 {
			n = 1
		}
		return 2 + n
	}

	lines := 2 // summary + pane counts
	lines += sectionHeight(d.priority)
	lines += sectionHeight(d.age)
	lines += sectionHeight(d.review)
	lines += sectionHeight(d.ci)
	lines += sectionHeight(d.prSize)

	if len(d.topRepos) > 0 {
		lines += sectionHeight(d.topRepos)
		if d.repoOthersRepos > 0 {
			lines++
		}
	}
	if len(d.orphanedStale) > 0 {
		lines += sectionHeight(d.orphanedStale)
	}
	if m.snapshotStore != nil {
		snapshots := m.snapshotStore.Recent(statsSparklineWidth)
		if len(snapshots) >= 2 {
			lines += 5 // separator + header + 3 sparklines
		}
	}
	return lines
}

// renderStatsView renders the full stats dashboard.
func renderStatsView(m ListModel) string {
	d := computeDistributions(m)

	var lines []string

	// Summary line
	summaryLine := fmt.Sprintf("  %d total: %d PRs, %d issues",
		d.totalCount, d.prCount, d.issueCount)
	paneCounts := fmt.Sprintf("Assigned %d  Blocked %d  Priority %d  Orphaned %d",
		d.assignedCount, d.blockedCount, d.priorityCount, d.orphanedCount)
	lines = append(lines, summaryLine)
	lines = append(lines, "  "+statsDimStyle.Render(paneCounts))

	// Separator
	sep := "  " + statsSepStyle.Render(strings.Repeat("─", separatorWidth))

	// Section rendering helper: header line + bar lines
	renderSection := func(label string, entries []barEntry) {
		lines = append(lines, sep)
		lines = append(lines, "  "+statsLabelStyle.Render(label))
		barLines := renderBars(entries, maxBarChars)
		lines = append(lines, barLines...)
	}

	// Priority + Age
	renderSection("Priority", d.priority)
	renderSection("Age", d.age)

	// PR Review + CI + Size
	renderSection("PR Review", d.review)
	renderSection("CI", d.ci)
	renderSection("Size", d.prSize)

	// Top repos
	if len(d.topRepos) > 0 {
		renderSection("Top repos", d.topRepos)
		if d.repoOthersRepos > 0 {
			others := fmt.Sprintf("    (%d others: %d)", d.repoOthersRepos, d.repoOthersCount)
			lines = append(lines, statsDimStyle.Render(others))
		}
	}

	// Orphaned staleness (only show if orphaned items exist)
	if len(d.orphanedStale) > 0 {
		renderSection("Orphaned staleness", d.orphanedStale)
	}

	// Sparkline trends section
	if m.snapshotStore != nil {
		snapshots := m.snapshotStore.Recent(statsSparklineWidth)
		if len(snapshots) >= 2 {
			lines = append(lines, sep)
			lines = append(lines, renderTrends(snapshots))
		}
	}

	return strings.Join(lines, "\n")
}

// renderTrends renders the sparkline trends section.
func renderTrends(snapshots []stats.Snapshot) string {
	const labelCol = 14

	var lines []string

	countLabel := fmt.Sprintf("(%d runs)", len(snapshots))
	lines = append(lines, "  "+statsLabelStyle.Render("Trends")+"  "+statsDimStyle.Render(countLabel))

	sparkW := statsSparklineWidth

	// Total count sparkline
	totals := make([]float64, len(snapshots))
	for i, s := range snapshots {
		totals[i] = float64(s.TotalCount)
	}
	totalSpark := renderSparkline(totals, sparkW)
	latestTotal := snapshots[len(snapshots)-1].TotalCount
	lines = append(lines, fmt.Sprintf("    %-*s%s  %s",
		labelCol, "Total",
		statsSparkStyle.Render(totalSpark),
		statsDimStyle.Render(fmt.Sprintf("%d", latestTotal))))

	// Orphaned count sparkline
	orphaned := make([]float64, len(snapshots))
	for i, s := range snapshots {
		orphaned[i] = float64(s.OrphanedCount)
	}
	orphanedSpark := renderSparkline(orphaned, sparkW)
	latestOrphaned := snapshots[len(snapshots)-1].OrphanedCount
	lines = append(lines, fmt.Sprintf("    %-*s%s  %s",
		labelCol, "Orphaned",
		statsSparkStyle.Render(orphanedSpark),
		statsDimStyle.Render(fmt.Sprintf("%d", latestOrphaned))))

	// Median age sparkline
	ages := make([]float64, len(snapshots))
	for i, s := range snapshots {
		ages[i] = s.MedianAgeHours
	}
	ageSpark := renderSparkline(ages, sparkW)
	latestAge := snapshots[len(snapshots)-1].MedianAgeHours
	ageDisplay := formatAgeDays(latestAge)
	lines = append(lines, fmt.Sprintf("    %-*s%s  %s",
		labelCol, "Median age",
		statsSparkStyle.Render(ageSpark),
		statsDimStyle.Render(ageDisplay)))

	return strings.Join(lines, "\n")
}

// formatAgeDays formats hours as a human-readable age string.
func formatAgeDays(hours float64) string {
	days := hours / 24
	if days < 1 {
		return fmt.Sprintf("%.1fh", hours)
	}
	return fmt.Sprintf("%.1fd", days)
}

// computeMedianAgeHours calculates the median age in hours across all items.
func computeMedianAgeHours(items []triage.PrioritizedItem) float64 {
	if len(items) == 0 {
		return 0
	}

	ages := make([]float64, len(items))
	for i, item := range items {
		ages[i] = time.Since(item.CreatedAt).Hours()
	}
	sort.Float64s(ages)

	mid := len(ages) / 2
	if len(ages)%2 == 0 {
		return (ages[mid-1] + ages[mid]) / 2
	}
	return ages[mid]
}

// ComputeSnapshotFromItems builds a Snapshot from items without requiring a full ListModel.
// This is called from cmd/list.go before launching the TUI.
func ComputeSnapshotFromItems(items []triage.PrioritizedItem, cfg *config.Config, currentUser string) stats.Snapshot {
	weights := cfg.GetScoreWeights()
	blockedLabels := cfg.GetBlockedLabels()

	// Build a temporary ListModel to reuse splitItems and computeDistributions
	m := NewListModel(items, nil, weights, currentUser,
		WithConfig(cfg),
		WithBlockedLabels(blockedLabels),
	)

	d := computeDistributions(m)

	return stats.Snapshot{
		Timestamp:      time.Now(),
		TotalCount:     d.totalCount,
		AssignedCount:  d.assignedCount,
		BlockedCount:   d.blockedCount,
		PriorityCount:  d.priorityCount,
		OrphanedCount:  d.orphanedCount,
		PRCount:        d.prCount,
		IssueCount:     d.issueCount,
		UrgentCount:    d.urgentCount,
		ImportantCount: d.importantCount,
		QuickWinCount:  d.quickWinCount,
		NotableCount:   d.notableCount,
		FYICount:       d.fyiCount,
		MedianAgeHours: math.Round(computeMedianAgeHours(m.items)*10) / 10,
		CISuccess:      d.ciSuccessCount,
		CIFailure:      d.ciFailureCount,
		CIPending:      d.ciPendingCount,
	}
}

// renderStatsPane wraps the stats view with tab bar, scroll, and help text.
func renderStatsPane(m ListModel) string {
	var b strings.Builder

	// Tab bar with top padding
	b.WriteString("\n")
	b.WriteString(renderTabBar(m.activePane, len(m.priorityItems), len(m.orphanedItems), m.AssignedCount(), m.BlockedCount(),
		m.PrioritySortColumn(), m.PrioritySortDesc(),
		m.OrphanedSortColumn(), m.OrphanedSortDesc(),
		m.AssignedSortColumn(), m.AssignedSortDesc(),
		m.BlockedSortColumn(), m.BlockedSortDesc()))
	b.WriteString("\n\n")

	// Render stats content
	content := renderStatsView(m)
	lines := strings.Split(content, "\n")

	// Apply scrolling
	availableHeight := m.windowHeight - tabBarLines - FooterLines
	availableHeight = max(availableHeight, 1)

	// Clamp scroll offset to valid range
	maxScroll := len(lines) - availableHeight
	maxScroll = max(maxScroll, 0)
	offset := min(m.statsScrollOffset, maxScroll)

	start := offset
	end := min(start+availableHeight, len(lines))

	for _, line := range lines[start:end] {
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Pad remaining space
	rendered := end - start
	for range max(availableHeight-rendered, 0) {
		b.WriteString("\n")
	}

	// Help text
	b.WriteString(renderStatsHelp())

	// Status message
	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(listStatusStyle.Render(m.statusMsg))
	}

	return b.String()
}

// renderStatsHelp renders help text specific to the stats pane.
func renderStatsHelp() string {
	return listHelpStyle.Render("Tab/1-5: panes   j/k: scroll   g/G: top/bottom   q: quit")
}

// Stats view styles
var (
	statsLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CBD5E1")).
			Bold(true)

	statsDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	statsSepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#334155"))

	statsRepoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA"))

	statsSparkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA"))
)
