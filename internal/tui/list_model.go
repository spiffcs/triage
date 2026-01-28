package tui

import (
	"os/exec"
	"runtime"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spiffcs/triage/config"
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

// ListModel is the Bubble Tea model for the interactive notification list
type ListModel struct {
	items               []triage.PrioritizedItem
	priorityItems       []triage.PrioritizedItem // Items excluding orphaned
	orphanedItems       []triage.PrioritizedItem // Only orphaned items
	activePane          pane                     // Which pane is focused
	priorityCursor      int                      // Cursor for priority pane
	orphanedCursor      int                      // Cursor for orphaned pane
	resolved            *resolved.Store
	windowWidth         int
	windowHeight        int
	statusMsg           string
	statusTime          time.Time
	quitting            bool
	hotTopicThreshold   int
	prSizeXS            int
	prSizeS             int
	prSizeM             int
	prSizeL             int
	currentUser         string
	hideAssignedCI bool // Hide Assigned and CI columns (for orphaned view)
	hidePriority   bool // Hide Priority column (for orphaned view)
	sortColumn     string // Sort column for orphaned pane
	sortDescending bool   // Sort direction for orphaned pane (true = descending)
}

// ListOption is a functional option for configuring ListModel
type ListOption func(*ListModel)

// WithSortBy configures the sort column and direction for the orphaned pane.
// Column can be: updated, created, author, repo, comments, stale, size.
// Descending=true sorts highest/newest first, false sorts lowest/oldest first.
func WithSortBy(column string, descending bool) ListOption {
	return func(m *ListModel) {
		m.sortColumn = column
		m.sortDescending = descending
	}
}

// NewListModel creates a new list model
func NewListModel(items []triage.PrioritizedItem, store *resolved.Store, weights config.ScoreWeights, currentUser string, opts ...ListOption) ListModel {
	m := ListModel{
		items:             items,
		resolved:          store,
		windowWidth:       80,
		windowHeight:      24,
		hotTopicThreshold: weights.HotTopicThreshold,
		prSizeXS:          weights.PRSizeXS,
		prSizeS:           weights.PRSizeS,
		prSizeM:           weights.PRSizeM,
		prSizeL:           weights.PRSizeL,
		currentUser:       currentUser,
		activePane:        panePriority,
		priorityCursor:    0,
		orphanedCursor:    0,
	}
	for _, opt := range opts {
		opt(&m)
	}
	// Split items into priority and orphaned lists
	m.splitItems()
	return m
}

// splitItems separates items into priority (non-orphaned) and orphaned lists
func (m *ListModel) splitItems() {
	m.priorityItems = nil
	m.orphanedItems = nil
	for _, item := range m.items {
		if item.Priority == triage.PriorityOrphaned {
			m.orphanedItems = append(m.orphanedItems, item)
		} else {
			m.priorityItems = append(m.priorityItems, item)
		}
	}
	// Sort orphaned items based on configured column and direction
	m.sortOrphanedItems()
}

// daysSinceTeamActivity calculates how many days since the last team activity on an item.
func daysSinceTeamActivity(item triage.PrioritizedItem) int {
	if item.Notification.Details == nil || item.Notification.Details.LastTeamActivityAt == nil {
		return 0
	}
	return int(time.Since(*item.Notification.Details.LastTeamActivityAt).Hours() / 24)
}

// sortOrphanedItems sorts the orphaned items by the configured column and direction.
func (m *ListModel) sortOrphanedItems() {
	if len(m.orphanedItems) == 0 {
		return
	}

	// Default to updated descending (newest first) if no sort specified
	column := m.sortColumn
	if column == "" {
		column = "updated"
	}
	desc := m.sortDescending
	if m.sortColumn == "" {
		desc = true // default to descending for dates
	}

	sort.Slice(m.orphanedItems, func(i, j int) bool {
		a, b := m.orphanedItems[i], m.orphanedItems[j]
		var less bool

		switch column {
		case "updated":
			less = a.Notification.UpdatedAt.Before(b.Notification.UpdatedAt)
		case "created":
			if a.Notification.Details != nil && b.Notification.Details != nil {
				less = a.Notification.Details.CreatedAt.Before(b.Notification.Details.CreatedAt)
			} else {
				less = a.Notification.UpdatedAt.Before(b.Notification.UpdatedAt)
			}
		case "author":
			authorA, authorB := "", ""
			if a.Notification.Details != nil {
				authorA = a.Notification.Details.Author
			}
			if b.Notification.Details != nil {
				authorB = b.Notification.Details.Author
			}
			less = authorA < authorB
		case "repo":
			less = a.Notification.Repository.FullName < b.Notification.Repository.FullName
		case "comments":
			commentsA, commentsB := 0, 0
			if a.Notification.Details != nil {
				commentsA = a.Notification.Details.CommentCount
			}
			if b.Notification.Details != nil {
				commentsB = b.Notification.Details.CommentCount
			}
			less = commentsA < commentsB
		case "stale":
			// Calculate days since last team activity
			staleA := daysSinceTeamActivity(a)
			staleB := daysSinceTeamActivity(b)
			less = staleA < staleB
		case "size":
			sizeA, sizeB := 0, 0
			if a.Notification.Details != nil {
				sizeA = a.Notification.Details.Additions + a.Notification.Details.Deletions
			}
			if b.Notification.Details != nil {
				sizeB = b.Notification.Details.Additions + b.Notification.Details.Deletions
			}
			less = sizeA < sizeB
		default:
			less = a.Notification.UpdatedAt.Before(b.Notification.UpdatedAt)
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
	n := item.Notification

	// Resolve using the notification's UpdatedAt time
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

	if item.Notification.Details != nil && item.Notification.Details.HTMLURL != "" {
		url = item.Notification.Details.HTMLURL
	} else if item.Notification.Repository.HTMLURL != "" {
		url = item.Notification.Repository.HTMLURL
	}

	if url == "" {
		m.statusMsg = "No URL available"
		m.statusTime = time.Now()
		return m, clearStatusAfter(2 * time.Second)
	}

	return m, openURL(url)
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
