package tui

import (
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/triage"
)

// ListModel is the Bubble Tea model for the interactive notification list
type ListModel struct {
	items             []triage.PrioritizedItem
	cursor            int
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
}

// NewListModel creates a new list model
func NewListModel(items []triage.PrioritizedItem, store *resolved.Store, weights config.ScoreWeights, currentUser string) ListModel {
	return ListModel{
		items:             items,
		cursor:            0,
		resolved:          store,
		windowWidth:       80,
		windowHeight:      24,
		hotTopicThreshold: weights.HotTopicThreshold,
		prSizeXS:          weights.PRSizeXS,
		prSizeS:           weights.PRSizeS,
		prSizeM:           weights.PRSizeM,
		prSizeL:           weights.PRSizeL,
		currentUser:       currentUser,
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

	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "g", "home":
		m.cursor = 0
		return m, nil

	case "G", "end":
		if len(m.items) > 0 {
			m.cursor = len(m.items) - 1
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
	if len(m.items) == 0 {
		return m, nil
	}

	item := m.items[m.cursor]
	n := item.Notification

	// Resolve using the notification's UpdatedAt time
	if err := m.resolved.Resolve(n.ID, n.UpdatedAt); err != nil {
		m.statusMsg = "Error: " + err.Error()
		m.statusTime = time.Now()
		return m, clearStatusAfter(2 * time.Second)
	}

	// Remove from current list
	m.items = append(m.items[:m.cursor], m.items[m.cursor+1:]...)

	// Adjust cursor if needed
	if m.cursor >= len(m.items) && m.cursor > 0 {
		m.cursor = len(m.items) - 1
	}

	m.statusMsg = "Marked as done"
	m.statusTime = time.Now()

	return m, clearStatusAfter(2 * time.Second)
}

// openInBrowser opens the current item in the default browser
func (m ListModel) openInBrowser() (tea.Model, tea.Cmd) {
	if len(m.items) == 0 {
		return m, nil
	}

	item := m.items[m.cursor]
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
