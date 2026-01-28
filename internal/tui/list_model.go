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

// Pane represents which pane is active in the dual-pane TUI
type Pane int

const (
	// PanePriority is the priority list pane (non-orphaned items)
	PanePriority Pane = iota
	// PaneOrphaned is the orphaned list pane
	PaneOrphaned
)

// ListModel is the Bubble Tea model for the interactive notification list
type ListModel struct {
	items               []triage.PrioritizedItem
	priorityItems       []triage.PrioritizedItem // Items excluding orphaned
	orphanedItems       []triage.PrioritizedItem // Only orphaned items
	activePane          Pane                     // Which pane is focused
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
	hideAssignedCI      bool // Hide Assigned and CI columns (for orphaned view)
	hidePriority        bool // Hide Priority column (for orphaned view)
	orphanedOldestFirst bool // Sort orphaned by oldest first instead of newest
}

// ListOption is a functional option for configuring ListModel
type ListOption func(*ListModel)

// WithHideAssignedCI hides the Assigned and CI columns
func WithHideAssignedCI() ListOption {
	return func(m *ListModel) {
		m.hideAssignedCI = true
	}
}

// WithHidePriority hides the Priority column
func WithHidePriority() ListOption {
	return func(m *ListModel) {
		m.hidePriority = true
	}
}

// WithOrphanedOldestFirst sorts orphaned items oldest first instead of newest
func WithOrphanedOldestFirst() ListOption {
	return func(m *ListModel) {
		m.orphanedOldestFirst = true
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
		activePane:        PanePriority,
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
	// Sort orphaned items by age (newest first by default)
	sort.Slice(m.orphanedItems, func(i, j int) bool {
		if m.orphanedOldestFirst {
			return m.orphanedItems[i].Notification.UpdatedAt.Before(m.orphanedItems[j].Notification.UpdatedAt)
		}
		return m.orphanedItems[i].Notification.UpdatedAt.After(m.orphanedItems[j].Notification.UpdatedAt)
	})
}

// activeItems returns the items for the active pane
func (m *ListModel) activeItems() []triage.PrioritizedItem {
	if m.activePane == PaneOrphaned {
		return m.orphanedItems
	}
	return m.priorityItems
}

// activeCursor returns the cursor position for the active pane
func (m *ListModel) activeCursor() int {
	if m.activePane == PaneOrphaned {
		return m.orphanedCursor
	}
	return m.priorityCursor
}

// setActiveCursor sets the cursor position for the active pane
func (m *ListModel) setActiveCursor(pos int) {
	if m.activePane == PaneOrphaned {
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
		if m.activePane == PanePriority {
			m.activePane = PaneOrphaned
		} else {
			m.activePane = PanePriority
		}
		return m, nil

	case "1":
		m.activePane = PanePriority
		return m, nil

	case "2":
		m.activePane = PaneOrphaned
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
	if m.activePane == PaneOrphaned {
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
