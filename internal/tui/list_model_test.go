package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/triage"
)

// newTestStore creates a resolved.Store backed by a temp directory.
func newTestStore(t *testing.T) *resolved.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "resolved.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := resolved.NewStoreFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// makeItem creates a PrioritizedItem with the given id, type, and updatedAt.
func makeItem(id string, itemType model.ItemType, updatedAt time.Time) triage.PrioritizedItem {
	return triage.PrioritizedItem{
		Item: model.Item{
			ID:        id,
			Type:      itemType,
			Reason:    model.ReasonSubscribed,
			UpdatedAt: updatedAt,
			Assignees: []string{"testuser"},
			Subject:   model.Subject{Title: id},
		},
	}
}

func TestMarkDoneWithTypeFilter(t *testing.T) {
	store := newTestStore(t)

	// Use distinct times so sort order (UpdatedAt descending) is deterministic.
	// Sorted order will be: pr-1 (newest), issue-1, pr-2, issue-2 (oldest).
	now := time.Now()
	items := []triage.PrioritizedItem{
		makeItem("pr-1", model.ItemTypePullRequest, now),
		makeItem("issue-1", model.ItemTypeIssue, now.Add(-1*time.Hour)),
		makeItem("pr-2", model.ItemTypePullRequest, now.Add(-2*time.Hour)),
		makeItem("issue-2", model.ItemTypeIssue, now.Add(-3*time.Hour)),
	}

	m := NewListModel(items, store, config.ScoreWeights{}, "testuser")
	// All items land in the assigned pane for "testuser"
	if len(m.assignedItems) != 4 {
		t.Fatalf("expected 4 assigned items, got %d", len(m.assignedItems))
	}

	// Filter to PRs only — filtered view should be [pr-1, pr-2]
	m.typeFilter = typeFilterPR
	filtered := m.activeItems()
	if len(filtered) != 2 {
		t.Fatalf("expected 2 PRs in filtered view, got %d", len(filtered))
	}

	// Move cursor to the second filtered PR (pr-2) and mark it done
	m.assignedCursor = 1
	result, _ := m.markDone()
	m = result.(ListModel)

	// The underlying list should have 3 items, with pr-2 removed
	if len(m.assignedItems) != 3 {
		t.Fatalf("expected 3 assigned items after mark done, got %d", len(m.assignedItems))
	}
	for _, item := range m.assignedItems {
		if item.Item.ID == "pr-2" {
			t.Fatal("pr-2 should have been removed but is still present")
		}
	}

	// Both issues should still be present
	var issueCount int
	for _, item := range m.assignedItems {
		if item.Item.Type == model.ItemTypeIssue {
			issueCount++
		}
	}
	if issueCount != 2 {
		t.Errorf("expected 2 issues to remain, got %d", issueCount)
	}

	// The filtered view should now show only 1 PR (pr-1)
	filtered = m.activeItems()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 PR in filtered view after mark done, got %d", len(filtered))
	}
	if filtered[0].Item.ID != "pr-1" {
		t.Errorf("expected remaining PR to be pr-1, got %s", filtered[0].Item.ID)
	}
}

func TestHelpVisibleAfterSort(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	items := []triage.PrioritizedItem{
		makeItem("pr-1", model.ItemTypePullRequest, now),
		makeItem("pr-2", model.ItemTypePullRequest, now.Add(-1*time.Hour)),
		makeItem("pr-3", model.ItemTypePullRequest, now.Add(-2*time.Hour)),
	}

	m := NewListModel(items, store, config.ScoreWeights{}, "testuser")
	m.windowWidth = 160
	m.windowHeight = 30

	// Verify help appears before sort
	view := m.View()
	if !strings.Contains(view, "j/k: nav") {
		t.Fatal("help text should be visible before sort")
	}

	// Cycle sort column (simulates pressing 's')
	result, _ := m.cycleSortColumn()
	m = result.(ListModel)

	view = m.View()
	if !strings.Contains(view, "j/k: nav") {
		t.Fatal("help text should be visible after cycleSortColumn")
	}

	// Toggle sort direction (simulates pressing 'S')
	result, _ = m.toggleSortDirection()
	m = result.(ListModel)

	view = m.View()
	if !strings.Contains(view, "j/k: nav") {
		t.Fatal("help text should be visible after toggleSortDirection")
	}

	// Reset sort (simulates pressing 'r')
	result, _ = m.resetSort()
	m = result.(ListModel)

	view = m.View()
	if !strings.Contains(view, "j/k: nav") {
		t.Fatal("help text should be visible after resetSort")
	}
}

func TestViewHeightConstantAcrossSortAndClear(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	items := []triage.PrioritizedItem{
		makeItem("pr-1", model.ItemTypePullRequest, now),
		makeItem("pr-2", model.ItemTypePullRequest, now.Add(-1*time.Hour)),
		makeItem("pr-3", model.ItemTypePullRequest, now.Add(-2*time.Hour)),
	}

	m := NewListModel(items, store, config.ScoreWeights{}, "testuser")
	m.windowWidth = 160
	m.windowHeight = 30

	// Measure height before sort (no status message)
	beforeLines := strings.Count(m.View(), "\n")

	// Sort: status message appears
	result, _ := m.cycleSortColumn()
	m = result.(ListModel)
	afterSortLines := strings.Count(m.View(), "\n")

	// Clear status: simulates the 2-second timer firing
	m.statusMsg = ""
	afterClearLines := strings.Count(m.View(), "\n")

	t.Logf("lines - before: %d, after sort: %d, after clear: %d", beforeLines, afterSortLines, afterClearLines)

	if beforeLines != afterSortLines || afterSortLines != afterClearLines {
		t.Errorf("view height should stay constant: before=%d, afterSort=%d, afterClear=%d",
			beforeLines, afterSortLines, afterClearLines)
	}
}

func TestToggleDoneView(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	items := []triage.PrioritizedItem{
		makeItem("pr-1", model.ItemTypePullRequest, now),
		makeItem("pr-2", model.ItemTypePullRequest, now.Add(-1*time.Hour)),
	}

	m := NewListModel(items, store, config.ScoreWeights{}, "testuser")

	if len(m.assignedItems) != 2 {
		t.Fatalf("expected 2 assigned items, got %d", len(m.assignedItems))
	}

	// Mark pr-2 as done
	m.assignedCursor = 1
	result, _ := m.markDone()
	m = result.(ListModel)

	if len(m.assignedItems) != 1 {
		t.Fatalf("expected 1 active item after mark done, got %d", len(m.assignedItems))
	}
	if len(m.assignedDoneItems) != 1 {
		t.Fatalf("expected 1 done item after mark done, got %d", len(m.assignedDoneItems))
	}

	// Toggle to done view
	result, _ = m.toggleDoneView()
	m = result.(ListModel)

	if !m.showDone {
		t.Fatal("expected showDone to be true after toggle")
	}

	// activeItems should return done items
	doneItems := m.activeItems()
	if len(doneItems) != 1 {
		t.Fatalf("expected 1 done item in done view, got %d", len(doneItems))
	}
	if doneItems[0].Item.ID != "pr-2" {
		t.Errorf("expected done item to be pr-2, got %s", doneItems[0].Item.ID)
	}

	// Toggle back
	result, _ = m.toggleDoneView()
	m = result.(ListModel)

	if m.showDone {
		t.Fatal("expected showDone to be false after second toggle")
	}
	activeItems := m.activeItems()
	if len(activeItems) != 1 {
		t.Fatalf("expected 1 active item after toggle back, got %d", len(activeItems))
	}
}

func TestUndoDone(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	items := []triage.PrioritizedItem{
		makeItem("pr-1", model.ItemTypePullRequest, now),
		makeItem("pr-2", model.ItemTypePullRequest, now.Add(-1*time.Hour)),
	}

	m := NewListModel(items, store, config.ScoreWeights{}, "testuser")

	// Mark pr-1 as done
	m.assignedCursor = 0
	result, _ := m.markDone()
	m = result.(ListModel)

	if len(m.assignedItems) != 1 {
		t.Fatalf("expected 1 active item, got %d", len(m.assignedItems))
	}
	if len(m.assignedDoneItems) != 1 {
		t.Fatalf("expected 1 done item, got %d", len(m.assignedDoneItems))
	}

	// Toggle to done view
	result, _ = m.toggleDoneView()
	m = result.(ListModel)

	// Undo the done (restore pr-1)
	m.assignedDoneCursor = 0
	result, _ = m.undoDone()
	m = result.(ListModel)

	if len(m.assignedDoneItems) != 0 {
		t.Fatalf("expected 0 done items after undo, got %d", len(m.assignedDoneItems))
	}
	if len(m.assignedItems) != 2 {
		t.Fatalf("expected 2 active items after undo, got %d", len(m.assignedItems))
	}

	// Verify the item is no longer resolved in the store
	if store.IsResolved("pr-1") {
		t.Error("pr-1 should no longer be resolved after undo")
	}
}

func TestSplitItemsWithResolvedItems(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	// Pre-resolve pr-2
	if err := store.Resolve("pr-2", now.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}

	items := []triage.PrioritizedItem{
		makeItem("pr-1", model.ItemTypePullRequest, now),
		makeItem("pr-2", model.ItemTypePullRequest, now.Add(-1*time.Hour)),
		makeItem("pr-3", model.ItemTypePullRequest, now.Add(-2*time.Hour)),
	}

	m := NewListModel(items, store, config.ScoreWeights{}, "testuser")

	// pr-2 was resolved, so it should be in the done list
	if len(m.assignedItems) != 2 {
		t.Fatalf("expected 2 active items, got %d", len(m.assignedItems))
	}
	if len(m.assignedDoneItems) != 1 {
		t.Fatalf("expected 1 done item, got %d", len(m.assignedDoneItems))
	}
	if m.assignedDoneItems[0].Item.ID != "pr-2" {
		t.Errorf("expected done item to be pr-2, got %s", m.assignedDoneItems[0].Item.ID)
	}
}
