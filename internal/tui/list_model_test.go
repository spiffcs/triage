package tui

import (
	"os"
	"path/filepath"
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
