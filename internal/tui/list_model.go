package tui

import (
	"math"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/stats"
	"github.com/spiffcs/triage/internal/triage"
)

// pane represents which pane is active in the dual-pane TUI
type pane int

const (
	// paneAssigned is the assigned list pane (items assigned to current user)
	paneAssigned pane = iota
	// panePriority is the priority list pane (non-orphaned items)
	panePriority
	// paneOrphaned is the orphaned list pane
	paneOrphaned
	// paneBlocked is the blocked list pane (items with "blocked" label)
	paneBlocked
	// paneStats is the stats dashboard pane
	paneStats
)

// TUI layout constants
const (
	// HeaderLines is the number of lines used for the list view header.
	HeaderLines = 2

	// FooterLines is the number of lines used for the list view footer.
	FooterLines = 3
)

// SortColumn represents the available sort columns
type SortColumn string

// priorityOrder maps priority levels to their sort order (lower = higher priority)
var priorityOrder = map[triage.PriorityLevel]int{
	triage.PriorityUrgent:    0,
	triage.PriorityImportant: 1,
	triage.PriorityQuickWin:  2,
	triage.PriorityNotable:   3,
	triage.PriorityFYI:       4,
}

// ciStatusOrder maps CI status to sort order (lower = higher priority for descending)
// Success at top when descending, failure/none at bottom
var ciStatusOrder = map[string]int{
	model.CIStatusSuccess: 0,
	model.CIStatusPending: 1,
	model.CIStatusFailure: 2,
	"":                    3, // no CI status
}

// Priority pane sort columns
const (
	SortPriority SortColumn = "priority"
	SortUpdated  SortColumn = "updated"
	SortRepo     SortColumn = "repo"
)

// Orphaned pane sort columns
const (
	SortStale    SortColumn = "stale"
	SortComments SortColumn = "comments"
	SortSize     SortColumn = "size"
	SortAuthor   SortColumn = "author"
	SortCI       SortColumn = "ci"
)

// prioritySortColumns defines the cycling order for priority pane
var prioritySortColumns = []SortColumn{SortPriority, SortUpdated, SortRepo, SortSize, SortCI}

// orphanedSortColumns defines the cycling order for orphaned pane
var orphanedSortColumns = []SortColumn{SortStale, SortUpdated, SortSize, SortAuthor, SortRepo, SortCI}

// assignedSortColumns defines the cycling order for assigned pane
var assignedSortColumns = []SortColumn{SortUpdated, SortSize, SortAuthor, SortRepo, SortCI}

// blockedSortColumns defines the cycling order for blocked pane
var blockedSortColumns = []SortColumn{SortUpdated, SortSize, SortAuthor, SortRepo, SortCI}

// Default sort columns
const (
	defaultPrioritySortColumn = SortPriority
	defaultOrphanedSortColumn = SortUpdated
	defaultAssignedSortColumn = SortUpdated
	defaultBlockedSortColumn  = SortUpdated
)

// ListModel is the Bubble Tea model for the interactive notification list
type ListModel struct {
	items             []triage.PrioritizedItem
	priorityItems     []triage.PrioritizedItem // Items excluding orphaned and assigned
	orphanedItems     []triage.PrioritizedItem // Only orphaned items
	assignedItems     []triage.PrioritizedItem // Items assigned to current user
	blockedItems      []triage.PrioritizedItem // Items with "blocked" label
	activePane        pane                     // Which pane is focused
	priorityCursor    int                      // Cursor for priority pane
	orphanedCursor    int                      // Cursor for orphaned pane
	assignedCursor    int                      // Cursor for assigned pane
	blockedCursor     int                      // Cursor for blocked pane
	resolved          *resolved.Store
	windowWidth       int
	windowHeight      int
	statusMsg         string
	statusTime        time.Time
	quitting          bool
	hotTopicThreshold int
	prSizeXS          int
	prSizeS           int
	prSizeM           int
	prSizeL           int
	currentUser       string

	// Sort state per pane
	prioritySortColumn SortColumn
	prioritySortDesc   bool
	orphanedSortColumn SortColumn
	orphanedSortDesc   bool
	assignedSortColumn SortColumn
	assignedSortDesc   bool
	blockedSortColumn  SortColumn
	blockedSortDesc    bool

	// Config for persisting preferences
	config *config.Config

	// Configurable labels for blocked pane
	blockedLabels []string

	// Stats pane state
	snapshotStore     *stats.Store
	statsScrollOffset int
}

// ListOption is a functional option for configuring ListModel
type ListOption func(*ListModel)

// WithConfig provides the config for loading and saving UI preferences.
func WithConfig(cfg *config.Config) ListOption {
	return func(m *ListModel) {
		m.config = cfg
	}
}

// WithBlockedLabels sets the labels used to identify blocked items.
// If empty, the blocked pane is effectively disabled.
func WithBlockedLabels(labels []string) ListOption {
	return func(m *ListModel) {
		m.blockedLabels = labels
	}
}

// WithSnapshotStore sets the stats snapshot store for historical trends.
func WithSnapshotStore(store *stats.Store) ListOption {
	return func(m *ListModel) {
		m.snapshotStore = store
	}
}

// NewListModel creates a new list model
func NewListModel(items []triage.PrioritizedItem, store *resolved.Store, weights config.ScoreWeights, currentUser string, opts ...ListOption) ListModel {
	m := ListModel{
		items:              items,
		resolved:           store,
		windowWidth:        80,
		windowHeight:       24,
		hotTopicThreshold:  weights.HotTopicThreshold,
		prSizeXS:           weights.PRSizeXS,
		prSizeS:            weights.PRSizeS,
		prSizeM:            weights.PRSizeM,
		prSizeL:            weights.PRSizeL,
		currentUser:        currentUser,
		activePane:         paneAssigned,
		priorityCursor:     0,
		orphanedCursor:     0,
		assignedCursor:     0,
		blockedCursor:      0,
		prioritySortColumn: defaultPrioritySortColumn,
		prioritySortDesc:   true, // default: descending (highest priority first)
		orphanedSortColumn: defaultOrphanedSortColumn,
		orphanedSortDesc:   true, // default: descending (most stale first)
		assignedSortColumn: defaultAssignedSortColumn,
		assignedSortDesc:   true, // default: descending (most recent first)
		blockedSortColumn:  defaultBlockedSortColumn,
		blockedSortDesc:    true, // default: descending (most recent first)
	}
	for _, opt := range opts {
		opt(&m)
	}
	// Load sort preferences from config if available
	m.loadSortPreferences()
	// Split items into priority and orphaned lists
	m.splitItems()
	return m
}

// splitItems separates items into priority, orphaned, assigned, and blocked lists
func (m *ListModel) splitItems() {
	m.priorityItems = nil
	m.orphanedItems = nil
	m.assignedItems = nil
	m.blockedItems = nil
	for _, item := range m.items {
		// Check for blocked label AND assigned to current user - blocked items don't go to other panes
		if m.hasBlockedLabel(item) && m.isAssignedToCurrentUser(item) {
			m.blockedItems = append(m.blockedItems, item)
			continue
		}
		// Check assignment - assigned items never go to orphaned
		if m.isAssignedToCurrentUser(item) {
			m.assignedItems = append(m.assignedItems, item)
		} else if m.hasAnyAssignee(item) {
			// Assigned to someone else - goes to priority (no longer orphaned)
			m.priorityItems = append(m.priorityItems, item)
		} else if item.Reason == model.ReasonOrphaned {
			// Only truly unassigned items with orphaned reason go here
			m.orphanedItems = append(m.orphanedItems, item)
		} else {
			m.priorityItems = append(m.priorityItems, item)
		}
	}
	// Sort all lists based on configured column and direction
	m.sortPriorityItems()
	m.sortOrphanedItems()
	m.sortAssignedItems()
	m.sortBlockedItems()
}

// isAssignedToCurrentUser checks if the item is assigned to the current user
func (m *ListModel) isAssignedToCurrentUser(item triage.PrioritizedItem) bool {
	if m.currentUser == "" {
		return false
	}
	for _, assignee := range item.Assignees {
		if assignee == m.currentUser {
			return true
		}
	}
	return false
}

// hasAnyAssignee checks if the item is assigned to anyone
func (m *ListModel) hasAnyAssignee(item triage.PrioritizedItem) bool {
	return len(item.Assignees) > 0
}

// hasBlockedLabel checks if the item has any of the configured blocked labels (case-insensitive).
// If blockedLabels is empty, always returns false (blocked pane is disabled).
func (m *ListModel) hasBlockedLabel(pi triage.PrioritizedItem) bool {
	if len(m.blockedLabels) == 0 {
		return false
	}
	for _, itemLabel := range pi.Labels {
		for _, blockedLabel := range m.blockedLabels {
			if strings.EqualFold(itemLabel, blockedLabel) {
				return true
			}
		}
	}
	return false
}

// loadSortPreferences loads sort preferences from config
func (m *ListModel) loadSortPreferences() {
	if m.config == nil || m.config.UI == nil {
		return
	}
	ui := m.config.UI
	if ui.PrioritySortColumn != "" {
		m.prioritySortColumn = SortColumn(ui.PrioritySortColumn)
	}
	if ui.PrioritySortDesc != nil {
		m.prioritySortDesc = *ui.PrioritySortDesc
	}
	if ui.OrphanedSortColumn != "" {
		m.orphanedSortColumn = SortColumn(ui.OrphanedSortColumn)
	}
	if ui.OrphanedSortDesc != nil {
		m.orphanedSortDesc = *ui.OrphanedSortDesc
	}
	if ui.AssignedSortColumn != "" {
		m.assignedSortColumn = SortColumn(ui.AssignedSortColumn)
	}
	if ui.AssignedSortDesc != nil {
		m.assignedSortDesc = *ui.AssignedSortDesc
	}
	if ui.BlockedSortColumn != "" {
		m.blockedSortColumn = SortColumn(ui.BlockedSortColumn)
	}
	if ui.BlockedSortDesc != nil {
		m.blockedSortDesc = *ui.BlockedSortDesc
	}
}

// saveSortPreferences saves sort preferences to config
func (m *ListModel) saveSortPreferences() {
	if m.config == nil {
		return
	}
	if m.config.UI == nil {
		m.config.UI = &config.UIPreferences{}
	}
	m.config.UI.PrioritySortColumn = string(m.prioritySortColumn)
	m.config.UI.PrioritySortDesc = &m.prioritySortDesc
	m.config.UI.OrphanedSortColumn = string(m.orphanedSortColumn)
	m.config.UI.OrphanedSortDesc = &m.orphanedSortDesc
	m.config.UI.AssignedSortColumn = string(m.assignedSortColumn)
	m.config.UI.AssignedSortDesc = &m.assignedSortDesc
	m.config.UI.BlockedSortColumn = string(m.blockedSortColumn)
	m.config.UI.BlockedSortDesc = &m.blockedSortDesc
	// Save async to avoid blocking UI
	go func() {
		_ = m.config.SaveUIPreferences()
	}()
}

// sortPriorityItems sorts the priority items by the configured column and direction.
func (m *ListModel) sortPriorityItems() {
	if len(m.priorityItems) == 0 {
		return
	}

	column := m.prioritySortColumn
	desc := m.prioritySortDesc

	sort.Slice(m.priorityItems, func(i, j int) bool {
		a, b := m.priorityItems[i], m.priorityItems[j]
		var less bool

		switch column {
		case SortPriority:
			// Sort by priority level first, then by score within level
			// Lower ordinal = higher priority (Urgent=0, Important=1, QuickWin=2, Notable=3, FYI=4)
			if a.Priority != b.Priority {
				less = priorityOrder[a.Priority] > priorityOrder[b.Priority]
			} else {
				less = a.Score < b.Score
			}
		case SortUpdated:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		case SortRepo:
			// Inverted so that descending (▼) gives A-Z order
			// Case insensitive comparison
			less = strings.ToLower(a.Repository.FullName) > strings.ToLower(b.Repository.FullName)
		case SortSize:
			// Custom sorting: PRs with review data come first, then everything else by comments
			// Direction is handled within this case (not using standard less+invert pattern)
			prA := a.PRDetails()
			prB := b.PRDetails()
			aSize, bSize := 0, 0
			if prA != nil {
				aSize = prA.Additions + prA.Deletions
			}
			if prB != nil {
				bSize = prB.Additions + prB.Deletions
			}
			aHasReviewData := prA != nil && aSize > 0
			bHasReviewData := prB != nil && bSize > 0

			// PRs with review data always come before everything else
			if aHasReviewData && !bHasReviewData {
				return true
			}
			if !aHasReviewData && bHasReviewData {
				return false
			}

			// Both have review data: sort by lines changed
			if aHasReviewData && bHasReviewData {
				// desc (▼): smallest to largest; asc (▲): largest to smallest
				if desc {
					return aSize < bSize
				}
				return aSize > bSize
			}

			// Neither has review data: sort by comment count
			// desc (▼): most to least; asc (▲): least to most
			if desc {
				return a.CommentCount > b.CommentCount
			}
			return a.CommentCount < b.CommentCount
		case SortCI:
			// Sort by CI status: success > pending > failure > none
			// Non-PRs are treated as having no CI status
			prA := a.PRDetails()
			prB := b.PRDetails()
			var ciA, ciB string
			if prA != nil {
				ciA = prA.CIStatus
			}
			if prB != nil {
				ciB = prB.CIStatus
			}
			orderA := ciStatusOrder[ciA]
			orderB := ciStatusOrder[ciB]
			// Lower order value = higher priority (success first when descending)
			less = orderA > orderB
		default:
			// Default to priority
			if a.Priority != b.Priority {
				less = priorityOrder[a.Priority] > priorityOrder[b.Priority]
			} else {
				less = a.Score < b.Score
			}
		}

		// Invert for descending order
		if desc {
			return !less
		}
		return less
	})
}

// daysSinceTeamActivity calculates how many days since the last team activity on an item.
// Falls back to CreatedAt if LastTeamActivityAt is not set (matching display logic).
func daysSinceTeamActivity(item triage.PrioritizedItem) int {
	n := item.Item
	if n.LastTeamActivityAt != nil {
		return int(time.Since(*n.LastTeamActivityAt).Hours() / 24)
	}
	if !n.CreatedAt.IsZero() {
		return int(time.Since(n.CreatedAt).Hours() / 24)
	}
	return 0
}

// sortOrphanedItems sorts the orphaned items by the configured column and direction.
func (m *ListModel) sortOrphanedItems() {
	if len(m.orphanedItems) == 0 {
		return
	}

	column := m.orphanedSortColumn
	desc := m.orphanedSortDesc

	sort.Slice(m.orphanedItems, func(i, j int) bool {
		a, b := m.orphanedItems[i], m.orphanedItems[j]
		var less bool

		switch column {
		case SortUpdated:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		case SortAuthor:
			// Inverted so that descending (▼) gives A-Z order
			// Case insensitive comparison
			less = strings.ToLower(a.Author) > strings.ToLower(b.Author)
		case SortRepo:
			// Inverted so that descending (▼) gives A-Z order
			// Case insensitive comparison
			less = strings.ToLower(a.Repository.FullName) > strings.ToLower(b.Repository.FullName)
		case SortComments:
			less = a.CommentCount < b.CommentCount
		case SortStale:
			// Calculate days since last team activity
			staleA := daysSinceTeamActivity(a)
			staleB := daysSinceTeamActivity(b)
			less = staleA > staleB
		case SortSize:
			// Custom sorting: PRs with review data come first, then everything else by comments
			// Direction is handled within this case (not using standard less+invert pattern)
			prA := a.PRDetails()
			prB := b.PRDetails()
			aSize, bSize := 0, 0
			if prA != nil {
				aSize = prA.Additions + prA.Deletions
			}
			if prB != nil {
				bSize = prB.Additions + prB.Deletions
			}
			aHasReviewData := prA != nil && aSize > 0
			bHasReviewData := prB != nil && bSize > 0

			// PRs with review data always come before everything else
			if aHasReviewData && !bHasReviewData {
				return true
			}
			if !aHasReviewData && bHasReviewData {
				return false
			}

			// Both have review data: sort by lines changed
			if aHasReviewData && bHasReviewData {
				// desc (▼): smallest to largest; asc (▲): largest to smallest
				if desc {
					return aSize < bSize
				}
				return aSize > bSize
			}

			// Neither has review data: sort by comment count
			// desc (▼): most to least; asc (▲): least to most
			if desc {
				return a.CommentCount > b.CommentCount
			}
			return a.CommentCount < b.CommentCount
		case SortCI:
			// Sort by CI status: success > pending > failure > none
			// Non-PRs are treated as having no CI status
			prA := a.PRDetails()
			prB := b.PRDetails()
			var ciA, ciB string
			if prA != nil {
				ciA = prA.CIStatus
			}
			if prB != nil {
				ciB = prB.CIStatus
			}
			orderA := ciStatusOrder[ciA]
			orderB := ciStatusOrder[ciB]
			// Lower order value = higher priority (success first when descending)
			less = orderA > orderB
		default:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		}

		// Invert for descending order
		if desc {
			return !less
		}
		return less
	})
}

// sortAssignedItems sorts the assigned items by the configured column and direction.
func (m *ListModel) sortAssignedItems() {
	if len(m.assignedItems) == 0 {
		return
	}

	column := m.assignedSortColumn
	desc := m.assignedSortDesc

	sort.Slice(m.assignedItems, func(i, j int) bool {
		a, b := m.assignedItems[i], m.assignedItems[j]
		var less bool

		switch column {
		case SortUpdated:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		case SortSize:
			// Custom sorting: PRs with review data come first, then everything else by comments
			// Direction is handled within this case (not using standard less+invert pattern)
			prA := a.PRDetails()
			prB := b.PRDetails()
			aSize, bSize := 0, 0
			if prA != nil {
				aSize = prA.Additions + prA.Deletions
			}
			if prB != nil {
				bSize = prB.Additions + prB.Deletions
			}
			aHasReviewData := prA != nil && aSize > 0
			bHasReviewData := prB != nil && bSize > 0

			// PRs with review data always come before everything else
			if aHasReviewData && !bHasReviewData {
				return true
			}
			if !aHasReviewData && bHasReviewData {
				return false
			}

			// Both have review data: sort by lines changed
			if aHasReviewData && bHasReviewData {
				// desc (▼): smallest to largest; asc (▲): largest to smallest
				if desc {
					return aSize < bSize
				}
				return aSize > bSize
			}

			// Neither has review data: sort by comment count
			// desc (▼): most to least; asc (▲): least to most
			if desc {
				return a.CommentCount > b.CommentCount
			}
			return a.CommentCount < b.CommentCount
		case SortAuthor:
			// Inverted so that descending (▼) gives A-Z order
			// Case insensitive comparison
			less = strings.ToLower(a.Author) > strings.ToLower(b.Author)
		case SortRepo:
			// Inverted so that descending (▼) gives A-Z order
			// Case insensitive comparison
			less = strings.ToLower(a.Repository.FullName) > strings.ToLower(b.Repository.FullName)
		case SortCI:
			// Sort by CI status: success > pending > failure > none
			// Non-PRs are treated as having no CI status
			prA := a.PRDetails()
			prB := b.PRDetails()
			var ciA, ciB string
			if prA != nil {
				ciA = prA.CIStatus
			}
			if prB != nil {
				ciB = prB.CIStatus
			}
			orderA := ciStatusOrder[ciA]
			orderB := ciStatusOrder[ciB]
			// Lower order value = higher priority (success first when descending)
			less = orderA > orderB
		default:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		}

		// Invert for descending order
		if desc {
			return !less
		}
		return less
	})
}

// sortBlockedItems sorts the blocked items by the configured column and direction.
func (m *ListModel) sortBlockedItems() {
	if len(m.blockedItems) == 0 {
		return
	}

	column := m.blockedSortColumn
	desc := m.blockedSortDesc

	sort.Slice(m.blockedItems, func(i, j int) bool {
		a, b := m.blockedItems[i], m.blockedItems[j]
		var less bool

		switch column {
		case SortUpdated:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		case SortSize:
			// Custom sorting: PRs with review data come first, then everything else by comments
			// Direction is handled within this case (not using standard less+invert pattern)
			prA := a.PRDetails()
			prB := b.PRDetails()
			aSize, bSize := 0, 0
			if prA != nil {
				aSize = prA.Additions + prA.Deletions
			}
			if prB != nil {
				bSize = prB.Additions + prB.Deletions
			}
			aHasReviewData := prA != nil && aSize > 0
			bHasReviewData := prB != nil && bSize > 0

			// PRs with review data always come before everything else
			if aHasReviewData && !bHasReviewData {
				return true
			}
			if !aHasReviewData && bHasReviewData {
				return false
			}

			// Both have review data: sort by lines changed
			if aHasReviewData && bHasReviewData {
				// desc (▼): smallest to largest; asc (▲): largest to smallest
				if desc {
					return aSize < bSize
				}
				return aSize > bSize
			}

			// Neither has review data: sort by comment count
			// desc (▼): most to least; asc (▲): least to most
			if desc {
				return a.CommentCount > b.CommentCount
			}
			return a.CommentCount < b.CommentCount
		case SortAuthor:
			// Inverted so that descending (▼) gives A-Z order
			// Case insensitive comparison
			less = strings.ToLower(a.Author) > strings.ToLower(b.Author)
		case SortRepo:
			// Inverted so that descending (▼) gives A-Z order
			// Case insensitive comparison
			less = strings.ToLower(a.Repository.FullName) > strings.ToLower(b.Repository.FullName)
		case SortCI:
			// Sort by CI status: success > pending > failure > none
			// Non-PRs are treated as having no CI status
			prA := a.PRDetails()
			prB := b.PRDetails()
			var ciA, ciB string
			if prA != nil {
				ciA = prA.CIStatus
			}
			if prB != nil {
				ciB = prB.CIStatus
			}
			orderA := ciStatusOrder[ciA]
			orderB := ciStatusOrder[ciB]
			// Lower order value = higher priority (success first when descending)
			less = orderA > orderB
		default:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		}

		// Invert for descending order
		if desc {
			return !less
		}
		return less
	})
}

// activeItems returns the items for the active pane
func (m *ListModel) activeItems() []triage.PrioritizedItem {
	switch m.activePane {
	case paneOrphaned:
		return m.orphanedItems
	case paneAssigned:
		return m.assignedItems
	case paneBlocked:
		return m.blockedItems
	case paneStats:
		return nil
	default:
		return m.priorityItems
	}
}

// activeCursor returns the cursor position for the active pane
func (m *ListModel) activeCursor() int {
	switch m.activePane {
	case paneOrphaned:
		return m.orphanedCursor
	case paneAssigned:
		return m.assignedCursor
	case paneBlocked:
		return m.blockedCursor
	case paneStats:
		return 0
	default:
		return m.priorityCursor
	}
}

// setActiveCursor sets the cursor position for the active pane
func (m *ListModel) setActiveCursor(pos int) {
	switch m.activePane {
	case paneOrphaned:
		m.orphanedCursor = pos
	case paneAssigned:
		m.assignedCursor = pos
	case paneBlocked:
		m.blockedCursor = pos
	default:
		m.priorityCursor = pos
	}
}

// clampStatsScroll ensures the stats scroll offset is within valid bounds.
func (m ListModel) clampStatsScroll() ListModel {
	content := renderStatsView(m)
	contentLines := strings.Count(content, "\n") + 1
	availableHeight := m.windowHeight - tabBarLines - FooterLines
	availableHeight = max(availableHeight, 1)
	maxScroll := max(contentLines-availableHeight, 0)
	if m.statsScrollOffset > maxScroll {
		m.statsScrollOffset = maxScroll
	}
	return m
}

// Init implements tea.Model
func (m ListModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		return m, nil

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil
	}

	return m, nil
}

// handleKey processes keyboard input
func (m ListModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "tab":
		// Cycle through panes: Assigned -> Blocked -> Priority -> Orphaned -> Stats -> Assigned
		switch m.activePane {
		case paneAssigned:
			m.activePane = paneBlocked
		case paneBlocked:
			m.activePane = panePriority
		case panePriority:
			m.activePane = paneOrphaned
		case paneOrphaned:
			m.activePane = paneStats
		case paneStats:
			m.activePane = paneAssigned
		}
		return m, nil

	case "1":
		m.activePane = paneAssigned
		return m, nil

	case "2":
		m.activePane = paneBlocked
		return m, nil

	case "3":
		m.activePane = panePriority
		return m, nil

	case "4":
		m.activePane = paneOrphaned
		return m, nil

	case "5":
		m.activePane = paneStats
		return m, nil

	case "d", "enter", "s", "S", "r":
		if m.activePane == paneStats {
			return m, nil
		}
		switch msg.String() {
		case "d":
			return m.markDone()
		case "enter":
			return m.openInBrowser()
		case "s":
			return m.cycleSortColumn()
		case "S":
			return m.toggleSortDirection()
		case "r":
			return m.resetSort()
		}

	case "j", "down":
		if m.activePane == paneStats {
			m.statsScrollOffset++
			m = m.clampStatsScroll()
			return m, nil
		}
		items := m.activeItems()
		cursor := m.activeCursor()
		if cursor < len(items)-1 {
			m.setActiveCursor(cursor + 1)
		}
		return m, nil

	case "k", "up":
		if m.activePane == paneStats {
			if m.statsScrollOffset > 0 {
				m.statsScrollOffset--
			}
			return m, nil
		}
		cursor := m.activeCursor()
		if cursor > 0 {
			m.setActiveCursor(cursor - 1)
		}
		return m, nil

	case "g", "home":
		if m.activePane == paneStats {
			m.statsScrollOffset = 0
			return m, nil
		}
		m.setActiveCursor(0)
		return m, nil

	case "G", "end":
		if m.activePane == paneStats {
			m.statsScrollOffset = math.MaxInt
			m = m.clampStatsScroll()
			return m, nil
		}
		items := m.activeItems()
		if len(items) > 0 {
			m.setActiveCursor(len(items) - 1)
		}
		return m, nil
	}

	return m, nil
}

// markDone marks the current item as done
func (m ListModel) markDone() (tea.Model, tea.Cmd) {
	items := m.activeItems()
	cursor := m.activeCursor()

	if len(items) == 0 {
		return m, nil
	}

	item := items[cursor]
	n := item.Item

	// Resolve using the item's UpdatedAt time
	if err := m.resolved.Resolve(n.ID, n.UpdatedAt); err != nil {
		m.statusMsg = "Error: " + err.Error()
		m.statusTime = time.Now()
		return m, clearStatusAfter(2 * time.Second)
	}

	// Remove from the active pane's list
	switch m.activePane {
	case paneOrphaned:
		m.orphanedItems = append(m.orphanedItems[:cursor], m.orphanedItems[cursor+1:]...)
		if m.orphanedCursor >= len(m.orphanedItems) && m.orphanedCursor > 0 {
			m.orphanedCursor = len(m.orphanedItems) - 1
		}
	case paneAssigned:
		m.assignedItems = append(m.assignedItems[:cursor], m.assignedItems[cursor+1:]...)
		if m.assignedCursor >= len(m.assignedItems) && m.assignedCursor > 0 {
			m.assignedCursor = len(m.assignedItems) - 1
		}
	case paneBlocked:
		m.blockedItems = append(m.blockedItems[:cursor], m.blockedItems[cursor+1:]...)
		if m.blockedCursor >= len(m.blockedItems) && m.blockedCursor > 0 {
			m.blockedCursor = len(m.blockedItems) - 1
		}
	default:
		m.priorityItems = append(m.priorityItems[:cursor], m.priorityItems[cursor+1:]...)
		if m.priorityCursor >= len(m.priorityItems) && m.priorityCursor > 0 {
			m.priorityCursor = len(m.priorityItems) - 1
		}
	}

	m.statusMsg = "Marked as done"
	m.statusTime = time.Now()

	return m, clearStatusAfter(2 * time.Second)
}

// openInBrowser opens the current item in the default browser
func (m ListModel) openInBrowser() (tea.Model, tea.Cmd) {
	items := m.activeItems()
	cursor := m.activeCursor()

	if len(items) == 0 {
		return m, nil
	}

	item := items[cursor]
	url := ""

	if item.HTMLURL != "" {
		url = item.HTMLURL
	} else if item.Repository.HTMLURL != "" {
		url = item.Repository.HTMLURL
	}

	if url == "" {
		m.statusMsg = "No URL available"
		m.statusTime = time.Now()
		return m, clearStatusAfter(2 * time.Second)
	}

	return m, openURL(url)
}

// cycleSortColumn cycles to the next sort column for the active pane
func (m ListModel) cycleSortColumn() (tea.Model, tea.Cmd) {
	// Get current item to preserve cursor position
	items := m.activeItems()
	var currentItem *triage.PrioritizedItem
	cursor := m.activeCursor()
	if len(items) > 0 && cursor < len(items) {
		currentItem = &items[cursor]
	}

	var columns []SortColumn
	var currentCol *SortColumn

	switch m.activePane {
	case paneOrphaned:
		columns = orphanedSortColumns
		currentCol = &m.orphanedSortColumn
	case paneAssigned:
		columns = assignedSortColumns
		currentCol = &m.assignedSortColumn
	case paneBlocked:
		columns = blockedSortColumns
		currentCol = &m.blockedSortColumn
	default:
		columns = prioritySortColumns
		currentCol = &m.prioritySortColumn
	}

	// Find current column index and cycle to next
	currentIdx := 0
	for i, col := range columns {
		if col == *currentCol {
			currentIdx = i
			break
		}
	}
	nextIdx := (currentIdx + 1) % len(columns)
	*currentCol = columns[nextIdx]

	// Re-sort the items
	switch m.activePane {
	case paneOrphaned:
		m.sortOrphanedItems()
	case paneAssigned:
		m.sortAssignedItems()
	case paneBlocked:
		m.sortBlockedItems()
	default:
		m.sortPriorityItems()
	}

	// Preserve cursor position on the same item
	m.preserveCursorPosition(currentItem)

	// Save preferences
	m.saveSortPreferences()

	// Show status message
	direction := "▼"
	switch m.activePane {
	case paneOrphaned:
		if !m.orphanedSortDesc {
			direction = "▲"
		}
	case paneAssigned:
		if !m.assignedSortDesc {
			direction = "▲"
		}
	case paneBlocked:
		if !m.blockedSortDesc {
			direction = "▲"
		}
	default:
		if !m.prioritySortDesc {
			direction = "▲"
		}
	}
	m.statusMsg = "Sorted by " + string(*currentCol) + " " + direction
	m.statusTime = time.Now()

	return m, clearStatusAfter(2 * time.Second)
}

// toggleSortDirection toggles the sort direction for the active pane
func (m ListModel) toggleSortDirection() (tea.Model, tea.Cmd) {
	// Get current item to preserve cursor position
	items := m.activeItems()
	var currentItem *triage.PrioritizedItem
	cursor := m.activeCursor()
	if len(items) > 0 && cursor < len(items) {
		currentItem = &items[cursor]
	}

	switch m.activePane {
	case paneOrphaned:
		m.orphanedSortDesc = !m.orphanedSortDesc
		m.sortOrphanedItems()
	case paneAssigned:
		m.assignedSortDesc = !m.assignedSortDesc
		m.sortAssignedItems()
	case paneBlocked:
		m.blockedSortDesc = !m.blockedSortDesc
		m.sortBlockedItems()
	default:
		m.prioritySortDesc = !m.prioritySortDesc
		m.sortPriorityItems()
	}

	// Preserve cursor position on the same item
	m.preserveCursorPosition(currentItem)

	// Save preferences
	m.saveSortPreferences()

	// Show status message
	var col SortColumn
	var desc bool
	switch m.activePane {
	case paneOrphaned:
		col = m.orphanedSortColumn
		desc = m.orphanedSortDesc
	case paneAssigned:
		col = m.assignedSortColumn
		desc = m.assignedSortDesc
	case paneBlocked:
		col = m.blockedSortColumn
		desc = m.blockedSortDesc
	default:
		col = m.prioritySortColumn
		desc = m.prioritySortDesc
	}
	direction := "▼"
	if !desc {
		direction = "▲"
	}
	m.statusMsg = "Sorted by " + string(col) + " " + direction
	m.statusTime = time.Now()

	return m, clearStatusAfter(2 * time.Second)
}

// resetSort resets the sort to defaults for the active pane
func (m ListModel) resetSort() (tea.Model, tea.Cmd) {
	// Get current item to preserve cursor position
	items := m.activeItems()
	var currentItem *triage.PrioritizedItem
	cursor := m.activeCursor()
	if len(items) > 0 && cursor < len(items) {
		currentItem = &items[cursor]
	}

	switch m.activePane {
	case paneOrphaned:
		m.orphanedSortColumn = defaultOrphanedSortColumn
		m.orphanedSortDesc = true
		m.sortOrphanedItems()
	case paneAssigned:
		m.assignedSortColumn = defaultAssignedSortColumn
		m.assignedSortDesc = true
		m.sortAssignedItems()
	case paneBlocked:
		m.blockedSortColumn = defaultBlockedSortColumn
		m.blockedSortDesc = true
		m.sortBlockedItems()
	default:
		m.prioritySortColumn = defaultPrioritySortColumn
		m.prioritySortDesc = true
		m.sortPriorityItems()
	}

	// Preserve cursor position on the same item
	m.preserveCursorPosition(currentItem)

	// Save preferences
	m.saveSortPreferences()

	// Show status message
	var col SortColumn
	switch m.activePane {
	case paneOrphaned:
		col = m.orphanedSortColumn
	case paneAssigned:
		col = m.assignedSortColumn
	case paneBlocked:
		col = m.blockedSortColumn
	default:
		col = m.prioritySortColumn
	}
	m.statusMsg = "Reset to default sort: " + string(col) + " ▼"
	m.statusTime = time.Now()

	return m, clearStatusAfter(2 * time.Second)
}

// preserveCursorPosition finds the item after sorting and sets the cursor to it
func (m *ListModel) preserveCursorPosition(item *triage.PrioritizedItem) {
	if item == nil {
		return
	}

	items := m.activeItems()
	for i, it := range items {
		if it.ID == item.ID {
			m.setActiveCursor(i)
			return
		}
	}
	// Item not found, keep cursor in bounds
	if m.activeCursor() >= len(items) && len(items) > 0 {
		m.setActiveCursor(len(items) - 1)
	}
}

// PrioritySortColumn returns the current priority sort column for rendering
func (m ListModel) PrioritySortColumn() SortColumn {
	return m.prioritySortColumn
}

// PrioritySortDesc returns whether priority sort is descending
func (m ListModel) PrioritySortDesc() bool {
	return m.prioritySortDesc
}

// OrphanedSortColumn returns the current orphaned sort column for rendering
func (m ListModel) OrphanedSortColumn() SortColumn {
	return m.orphanedSortColumn
}

// OrphanedSortDesc returns whether orphaned sort is descending
func (m ListModel) OrphanedSortDesc() bool {
	return m.orphanedSortDesc
}

// AssignedSortColumn returns the current assigned sort column for rendering
func (m ListModel) AssignedSortColumn() SortColumn {
	return m.assignedSortColumn
}

// AssignedSortDesc returns whether assigned sort is descending
func (m ListModel) AssignedSortDesc() bool {
	return m.assignedSortDesc
}

// AssignedCount returns the number of assigned items
func (m ListModel) AssignedCount() int {
	return len(m.assignedItems)
}

// BlockedSortColumn returns the current blocked sort column for rendering
func (m ListModel) BlockedSortColumn() SortColumn {
	return m.blockedSortColumn
}

// BlockedSortDesc returns whether blocked sort is descending
func (m ListModel) BlockedSortDesc() bool {
	return m.blockedSortDesc
}

// BlockedCount returns the number of blocked items
func (m ListModel) BlockedCount() int {
	return len(m.blockedItems)
}

// View implements tea.Model
func (m ListModel) View() string {
	if m.quitting {
		return ""
	}

	if m.activePane == paneStats {
		return renderStatsPane(m)
	}

	return renderListView(m)
}

// clearStatusMsg is a message to clear the status
type clearStatusMsg struct{}

// clearStatusAfter returns a command that clears the status after a delay
func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// openURL opens a URL in the default browser
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd

		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "linux":
			cmd = exec.Command("xdg-open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			return nil
		}

		_ = cmd.Start()
		return nil
	}
}
