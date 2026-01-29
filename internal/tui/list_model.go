package tui

import (
	"os/exec"
	"runtime"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/triage"
)

// pane represents which pane is active in the dual-pane TUI
type pane int

const (
	// panePriority is the priority list pane (non-orphaned items)
	panePriority pane = iota
	// paneOrphaned is the orphaned list pane
	paneOrphaned
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
)

// prioritySortColumns defines the cycling order for priority pane
var prioritySortColumns = []SortColumn{SortPriority, SortUpdated, SortRepo}

// orphanedSortColumns defines the cycling order for orphaned pane
var orphanedSortColumns = []SortColumn{SortStale, SortUpdated, SortComments, SortSize, SortAuthor, SortRepo}

// Default sort columns
const (
	defaultPrioritySortColumn = SortPriority
	defaultOrphanedSortColumn = SortUpdated
)

// ListModel is the Bubble Tea model for the interactive notification list
type ListModel struct {
	items             []triage.PrioritizedItem
	priorityItems     []triage.PrioritizedItem // Items excluding orphaned
	orphanedItems     []triage.PrioritizedItem // Only orphaned items
	activePane        pane                     // Which pane is focused
	priorityCursor    int                      // Cursor for priority pane
	orphanedCursor    int                      // Cursor for orphaned pane
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

	// Config for persisting preferences
	config *config.Config
}

// ListOption is a functional option for configuring ListModel
type ListOption func(*ListModel)

// WithConfig provides the config for loading and saving UI preferences.
func WithConfig(cfg *config.Config) ListOption {
	return func(m *ListModel) {
		m.config = cfg
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
		activePane:         panePriority,
		priorityCursor:     0,
		orphanedCursor:     0,
		prioritySortColumn: defaultPrioritySortColumn,
		prioritySortDesc:   true, // default: descending (highest priority first)
		orphanedSortColumn: defaultOrphanedSortColumn,
		orphanedSortDesc:   true, // default: descending (most stale first)
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

// splitItems separates items into priority (non-orphaned) and orphaned lists
func (m *ListModel) splitItems() {
	m.priorityItems = nil
	m.orphanedItems = nil
	for _, item := range m.items {
		if item.Item.Reason == model.ReasonOrphaned {
			m.orphanedItems = append(m.orphanedItems, item)
		} else {
			m.priorityItems = append(m.priorityItems, item)
		}
	}
	// Sort both lists based on configured column and direction
	m.sortPriorityItems()
	m.sortOrphanedItems()
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
			less = a.Item.UpdatedAt.Before(b.Item.UpdatedAt)
		case SortRepo:
			// Inverted so that descending (▼) gives A-Z order
			less = a.Item.Repository.FullName > b.Item.Repository.FullName
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
	if item.Item.Details == nil {
		return 0
	}
	d := item.Item.Details
	if d.LastTeamActivityAt != nil {
		return int(time.Since(*d.LastTeamActivityAt).Hours() / 24)
	}
	if !d.CreatedAt.IsZero() {
		return int(time.Since(d.CreatedAt).Hours() / 24)
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
			less = a.Item.UpdatedAt.Before(b.Item.UpdatedAt)
		case SortAuthor:
			// Inverted so that descending (▼) gives A-Z order
			authorA, authorB := "", ""
			if a.Item.Details != nil {
				authorA = a.Item.Details.Author
			}
			if b.Item.Details != nil {
				authorB = b.Item.Details.Author
			}
			less = authorA > authorB
		case SortRepo:
			// Inverted so that descending (▼) gives A-Z order
			less = a.Item.Repository.FullName > b.Item.Repository.FullName
		case SortComments:
			commentsA, commentsB := 0, 0
			if a.Item.Details != nil {
				commentsA = a.Item.Details.CommentCount
			}
			if b.Item.Details != nil {
				commentsB = b.Item.Details.CommentCount
			}
			less = commentsA < commentsB
		case SortStale:
			// Calculate days since last team activity
			staleA := daysSinceTeamActivity(a)
			staleB := daysSinceTeamActivity(b)
			less = staleA < staleB
		case SortSize:
			// For PRs: sort by review size (additions + deletions)
			// For issues: sort by comment count
			sizeA, sizeB := 0, 0
			if a.Item.Details != nil {
				if a.Item.Details.IsPR {
					sizeA = a.Item.Details.Additions + a.Item.Details.Deletions
				} else {
					sizeA = a.Item.Details.CommentCount
				}
			}
			if b.Item.Details != nil {
				if b.Item.Details.IsPR {
					sizeB = b.Item.Details.Additions + b.Item.Details.Deletions
				} else {
					sizeB = b.Item.Details.CommentCount
				}
			}
			less = sizeA < sizeB
		default:
			less = a.Item.UpdatedAt.Before(b.Item.UpdatedAt)
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
	if m.activePane == paneOrphaned {
		return m.orphanedItems
	}
	return m.priorityItems
}

// activeCursor returns the cursor position for the active pane
func (m *ListModel) activeCursor() int {
	if m.activePane == paneOrphaned {
		return m.orphanedCursor
	}
	return m.priorityCursor
}

// setActiveCursor sets the cursor position for the active pane
func (m *ListModel) setActiveCursor(pos int) {
	if m.activePane == paneOrphaned {
		m.orphanedCursor = pos
	} else {
		m.priorityCursor = pos
	}
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
		// Toggle between panes
		if m.activePane == panePriority {
			m.activePane = paneOrphaned
		} else {
			m.activePane = panePriority
		}
		return m, nil

	case "1":
		m.activePane = panePriority
		return m, nil

	case "2":
		m.activePane = paneOrphaned
		return m, nil

	case "j", "down":
		items := m.activeItems()
		cursor := m.activeCursor()
		if cursor < len(items)-1 {
			m.setActiveCursor(cursor + 1)
		}
		return m, nil

	case "k", "up":
		cursor := m.activeCursor()
		if cursor > 0 {
			m.setActiveCursor(cursor - 1)
		}
		return m, nil

	case "g", "home":
		m.setActiveCursor(0)
		return m, nil

	case "G", "end":
		items := m.activeItems()
		if len(items) > 0 {
			m.setActiveCursor(len(items) - 1)
		}
		return m, nil

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
	if m.activePane == paneOrphaned {
		m.orphanedItems = append(m.orphanedItems[:cursor], m.orphanedItems[cursor+1:]...)
		if m.orphanedCursor >= len(m.orphanedItems) && m.orphanedCursor > 0 {
			m.orphanedCursor = len(m.orphanedItems) - 1
		}
	} else {
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

	if item.Item.Details != nil && item.Item.Details.HTMLURL != "" {
		url = item.Item.Details.HTMLURL
	} else if item.Item.Repository.HTMLURL != "" {
		url = item.Item.Repository.HTMLURL
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

	if m.activePane == paneOrphaned {
		columns = orphanedSortColumns
		currentCol = &m.orphanedSortColumn
	} else {
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
	if m.activePane == paneOrphaned {
		m.sortOrphanedItems()
	} else {
		m.sortPriorityItems()
	}

	// Preserve cursor position on the same item
	m.preserveCursorPosition(currentItem)

	// Save preferences
	m.saveSortPreferences()

	// Show status message
	direction := "▼"
	if (m.activePane == paneOrphaned && !m.orphanedSortDesc) ||
		(m.activePane == panePriority && !m.prioritySortDesc) {
		direction = "▲"
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

	if m.activePane == paneOrphaned {
		m.orphanedSortDesc = !m.orphanedSortDesc
		m.sortOrphanedItems()
	} else {
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
	if m.activePane == paneOrphaned {
		col = m.orphanedSortColumn
		desc = m.orphanedSortDesc
	} else {
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

	if m.activePane == paneOrphaned {
		m.orphanedSortColumn = defaultOrphanedSortColumn
		m.orphanedSortDesc = true
		m.sortOrphanedItems()
	} else {
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
	if m.activePane == paneOrphaned {
		col = m.orphanedSortColumn
	} else {
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
		if it.Item.ID == item.Item.ID {
			m.setActiveCursor(i)
			return
		}
	}
	// Item not found, keep cursor in bounds
	if m.activeCursor() >= len(items) && len(items) > 0 {
		m.setActiveCursor(len(items) - 1)
	}
}

// GetPrioritySortColumn returns the current priority sort column for rendering
func (m ListModel) GetPrioritySortColumn() SortColumn {
	return m.prioritySortColumn
}

// GetPrioritySortDesc returns whether priority sort is descending
func (m ListModel) GetPrioritySortDesc() bool {
	return m.prioritySortDesc
}

// GetOrphanedSortColumn returns the current orphaned sort column for rendering
func (m ListModel) GetOrphanedSortColumn() SortColumn {
	return m.orphanedSortColumn
}

// GetOrphanedSortDesc returns whether orphaned sort is descending
func (m ListModel) GetOrphanedSortDesc() bool {
	return m.orphanedSortDesc
}

// View implements tea.Model
func (m ListModel) View() string {
	if m.quitting {
		return ""
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
