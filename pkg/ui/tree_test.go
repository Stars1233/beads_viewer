package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/charmbracelet/lipgloss"
)

func newTreeTestTheme() Theme {
	return DefaultTheme(lipgloss.NewRenderer(nil))
}

// TestTreeBuildEmpty verifies Build() handles empty issues slice
func TestTreeBuildEmpty(t *testing.T) {
	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(nil)

	if !tree.IsBuilt() {
		t.Error("expected tree to be marked as built")
	}
	if tree.RootCount() != 0 {
		t.Errorf("expected 0 roots, got %d", tree.RootCount())
	}
	if tree.NodeCount() != 0 {
		t.Errorf("expected 0 nodes, got %d", tree.NodeCount())
	}
}

// TestTreeBuildNoHierarchy verifies all issues become roots when no parent-child deps
func TestTreeBuildNoHierarchy(t *testing.T) {
	issues := []model.Issue{
		{ID: "bv-1", Title: "Task 1", Priority: 1, IssueType: model.TypeTask},
		{ID: "bv-2", Title: "Task 2", Priority: 2, IssueType: model.TypeTask},
		{ID: "bv-3", Title: "Task 3", Priority: 0, IssueType: model.TypeBug},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	if tree.RootCount() != 3 {
		t.Errorf("expected 3 roots (no hierarchy), got %d", tree.RootCount())
	}
	if tree.NodeCount() != 3 {
		t.Errorf("expected 3 visible nodes, got %d", tree.NodeCount())
	}
}

// TestTreeBuildParentChild verifies proper nesting with parent-child deps
func TestTreeBuildParentChild(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "epic-1", Title: "Epic", Priority: 1, IssueType: model.TypeEpic, CreatedAt: now},
		{
			ID: "task-1", Title: "Task under Epic", Priority: 2, IssueType: model.TypeTask, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{
				{IssueID: "task-1", DependsOnID: "epic-1", Type: model.DepParentChild},
			},
		},
		{
			ID: "subtask-1", Title: "Subtask", Priority: 3, IssueType: model.TypeTask, CreatedAt: now.Add(2 * time.Hour),
			Dependencies: []*model.Dependency{
				{IssueID: "subtask-1", DependsOnID: "task-1", Type: model.DepParentChild},
			},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Should have 1 root (epic-1)
	if tree.RootCount() != 1 {
		t.Errorf("expected 1 root, got %d", tree.RootCount())
	}

	// With depth < 2 auto-expand, all 3 should be visible
	if tree.NodeCount() != 3 {
		t.Errorf("expected 3 visible nodes (auto-expanded), got %d", tree.NodeCount())
	}

	// Verify hierarchy structure
	root := tree.roots[0]
	if root.Issue.ID != "epic-1" {
		t.Errorf("expected root to be epic-1, got %s", root.Issue.ID)
	}
	if len(root.Children) != 1 {
		t.Errorf("expected epic to have 1 child, got %d", len(root.Children))
	}
	if root.Children[0].Issue.ID != "task-1" {
		t.Errorf("expected child to be task-1, got %s", root.Children[0].Issue.ID)
	}
	if len(root.Children[0].Children) != 1 {
		t.Errorf("expected task to have 1 child, got %d", len(root.Children[0].Children))
	}
	if root.Children[0].Children[0].Issue.ID != "subtask-1" {
		t.Errorf("expected grandchild to be subtask-1, got %s", root.Children[0].Children[0].Issue.ID)
	}
}

// TestTreeBuildOrphanParent verifies issues with non-existent parent become roots
func TestTreeBuildOrphanParent(t *testing.T) {
	issues := []model.Issue{
		{ID: "root-1", Title: "Root", Priority: 1, IssueType: model.TypeTask},
		{
			ID: "orphan-1", Title: "Orphan with missing parent", Priority: 2, IssueType: model.TypeTask,
			Dependencies: []*model.Dependency{
				{IssueID: "orphan-1", DependsOnID: "nonexistent-parent", Type: model.DepParentChild},
			},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// orphan-1 declares a parent that doesn't exist in the issue set.
	// Rather than disappearing from the tree entirely (bad UX), orphan-1
	// should be treated as a root - its parent reference is dangling.

	if tree.RootCount() != 2 {
		t.Errorf("expected 2 roots (orphan with missing parent becomes root), got %d", tree.RootCount())
	}
	// Both issues should be visible as roots
	if tree.NodeCount() != 2 {
		t.Errorf("expected 2 visible nodes, got %d", tree.NodeCount())
	}
}

// TestTreeBuildCycleDetection verifies cycles are handled gracefully
func TestTreeBuildCycleDetection(t *testing.T) {
	// Create a cycle: A -> B -> A (A is parent of B, B is parent of A)
	// This shouldn't cause infinite recursion
	issues := []model.Issue{
		{
			ID: "cycle-a", Title: "Cycle A", Priority: 1, IssueType: model.TypeTask,
			Dependencies: []*model.Dependency{
				{IssueID: "cycle-a", DependsOnID: "cycle-b", Type: model.DepParentChild},
			},
		},
		{
			ID: "cycle-b", Title: "Cycle B", Priority: 1, IssueType: model.TypeTask,
			Dependencies: []*model.Dependency{
				{IssueID: "cycle-b", DependsOnID: "cycle-a", Type: model.DepParentChild},
			},
		},
	}

	// This should not hang or panic
	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Both issues have parents, so neither is a root in the normal sense
	// But they form a cycle, which the algorithm handles
	if !tree.IsBuilt() {
		t.Error("expected tree to be built despite cycle")
	}
	// With the cycle, both have parents, so there are no roots
	// This is correct behavior - a pure cycle has no entry point
}

// TestTreeBuildChildSorting verifies children are sorted by priority, type, date
func TestTreeBuildChildSorting(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "parent", Title: "Parent", Priority: 1, IssueType: model.TypeEpic, CreatedAt: now},
		{
			ID: "child-p2-task", Title: "P2 Task", Priority: 2, IssueType: model.TypeTask, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child-p2-task", DependsOnID: "parent", Type: model.DepParentChild}},
		},
		{
			ID: "child-p1-bug", Title: "P1 Bug", Priority: 1, IssueType: model.TypeBug, CreatedAt: now.Add(2 * time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child-p1-bug", DependsOnID: "parent", Type: model.DepParentChild}},
		},
		{
			ID: "child-p1-task", Title: "P1 Task", Priority: 1, IssueType: model.TypeTask, CreatedAt: now.Add(3 * time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child-p1-task", DependsOnID: "parent", Type: model.DepParentChild}},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	if tree.RootCount() != 1 {
		t.Fatalf("expected 1 root, got %d", tree.RootCount())
	}

	children := tree.roots[0].Children
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}

	// Expected order: P1 Task (priority 1, task before bug), P1 Bug, P2 Task
	expectedOrder := []string{"child-p1-task", "child-p1-bug", "child-p2-task"}
	for i, expected := range expectedOrder {
		if children[i].Issue.ID != expected {
			t.Errorf("child[%d]: expected %s, got %s", i, expected, children[i].Issue.ID)
		}
	}
}

// TestTreeBuildBlockingDepsIgnored verifies blocking deps don't create hierarchy
func TestTreeBuildBlockingDepsIgnored(t *testing.T) {
	issues := []model.Issue{
		{ID: "blocker", Title: "Blocker", Priority: 1, IssueType: model.TypeTask},
		{
			ID: "blocked", Title: "Blocked task", Priority: 2, IssueType: model.TypeTask,
			Dependencies: []*model.Dependency{
				{IssueID: "blocked", DependsOnID: "blocker", Type: model.DepBlocks},
			},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Blocking deps shouldn't create hierarchy - both should be roots
	if tree.RootCount() != 2 {
		t.Errorf("expected 2 roots (blocking deps ignored), got %d", tree.RootCount())
	}
}

// TestTreeBuildRelatedDepsIgnored verifies related deps don't create hierarchy
func TestTreeBuildRelatedDepsIgnored(t *testing.T) {
	issues := []model.Issue{
		{ID: "main", Title: "Main task", Priority: 1, IssueType: model.TypeTask},
		{
			ID: "related", Title: "Related task", Priority: 2, IssueType: model.TypeTask,
			Dependencies: []*model.Dependency{
				{IssueID: "related", DependsOnID: "main", Type: model.DepRelated},
			},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Related deps shouldn't create hierarchy - both should be roots
	if tree.RootCount() != 2 {
		t.Errorf("expected 2 roots (related deps ignored), got %d", tree.RootCount())
	}
}

// TestTreeNavigation verifies cursor movement through the tree
func TestTreeNavigation(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "root-1", Title: "Root 1", Priority: 1, IssueType: model.TypeEpic, CreatedAt: now},
		{
			ID: "child-1", Title: "Child 1", Priority: 1, IssueType: model.TypeTask, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child-1", DependsOnID: "root-1", Type: model.DepParentChild}},
		},
		{ID: "root-2", Title: "Root 2", Priority: 2, IssueType: model.TypeTask, CreatedAt: now.Add(2 * time.Hour)},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Initial selection should be first node (root-1)
	if sel := tree.SelectedIssue(); sel == nil || sel.ID != "root-1" {
		t.Errorf("expected initial selection root-1, got %v", sel)
	}

	// Move down to child-1 (auto-expanded)
	tree.MoveDown()
	if sel := tree.SelectedIssue(); sel == nil || sel.ID != "child-1" {
		t.Errorf("expected selection child-1 after MoveDown, got %v", sel)
	}

	// Move down to root-2
	tree.MoveDown()
	if sel := tree.SelectedIssue(); sel == nil || sel.ID != "root-2" {
		t.Errorf("expected selection root-2 after second MoveDown, got %v", sel)
	}

	// Move up back to child-1
	tree.MoveUp()
	if sel := tree.SelectedIssue(); sel == nil || sel.ID != "child-1" {
		t.Errorf("expected selection child-1 after MoveUp, got %v", sel)
	}

	// Jump to bottom
	tree.JumpToBottom()
	if sel := tree.SelectedIssue(); sel == nil || sel.ID != "root-2" {
		t.Errorf("expected selection root-2 after JumpToBottom, got %v", sel)
	}

	// Jump to top
	tree.JumpToTop()
	if sel := tree.SelectedIssue(); sel == nil || sel.ID != "root-1" {
		t.Errorf("expected selection root-1 after JumpToTop, got %v", sel)
	}
}

// TestTreeExpandCollapse verifies expand/collapse functionality
func TestTreeExpandCollapse(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "root", Title: "Root", Priority: 1, IssueType: model.TypeEpic, CreatedAt: now},
		{
			ID: "child", Title: "Child", Priority: 1, IssueType: model.TypeTask, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child", DependsOnID: "root", Type: model.DepParentChild}},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Initially auto-expanded (depth < 2)
	if tree.NodeCount() != 2 {
		t.Errorf("expected 2 visible nodes (auto-expanded), got %d", tree.NodeCount())
	}

	// Collapse root
	tree.ToggleExpand() // cursor is on root
	if tree.NodeCount() != 1 {
		t.Errorf("expected 1 visible node after collapse, got %d", tree.NodeCount())
	}

	// Expand root
	tree.ToggleExpand()
	if tree.NodeCount() != 2 {
		t.Errorf("expected 2 visible nodes after expand, got %d", tree.NodeCount())
	}

	// Collapse all
	tree.CollapseAll()
	if tree.NodeCount() != 1 {
		t.Errorf("expected 1 visible node after CollapseAll, got %d", tree.NodeCount())
	}

	// Expand all
	tree.ExpandAll()
	if tree.NodeCount() != 2 {
		t.Errorf("expected 2 visible nodes after ExpandAll, got %d", tree.NodeCount())
	}
}

// TestTreeIssueMap verifies the issueMap lookup is populated
func TestTreeIssueMap(t *testing.T) {
	issues := []model.Issue{
		{ID: "test-1", Title: "Test 1", Priority: 1, IssueType: model.TypeTask},
		{ID: "test-2", Title: "Test 2", Priority: 2, IssueType: model.TypeTask},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Verify issueMap contains all nodes
	if len(tree.issueMap) != 2 {
		t.Errorf("expected issueMap to have 2 entries, got %d", len(tree.issueMap))
	}

	if _, ok := tree.issueMap["test-1"]; !ok {
		t.Error("expected test-1 in issueMap")
	}
	if _, ok := tree.issueMap["test-2"]; !ok {
		t.Error("expected test-2 in issueMap")
	}
}

// TestIssueTypeOrder verifies the ordering of issue types
func TestIssueTypeOrder(t *testing.T) {
	tests := []struct {
		issueType model.IssueType
		expected  int
	}{
		{model.TypeEpic, 0},
		{model.TypeFeature, 1},
		{model.TypeTask, 2},
		{model.TypeBug, 3},
		{model.TypeChore, 4},
		{"unknown", 5},
	}

	for _, tt := range tests {
		got := issueTypeOrder(tt.issueType)
		if got != tt.expected {
			t.Errorf("issueTypeOrder(%s) = %d, want %d", tt.issueType, got, tt.expected)
		}
	}
}

// TestTreeViewEmpty verifies View() output for empty tree
func TestTreeViewEmpty(t *testing.T) {
	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(nil)
	tree.SetSize(80, 20)

	view := tree.View()
	if !strings.Contains(view, "No issues to display") {
		t.Errorf("expected empty state message, got:\n%s", view)
	}
	if !strings.Contains(view, "Press E to return") {
		t.Errorf("expected return hint in empty state, got:\n%s", view)
	}
}

// TestTreeViewRendering verifies View() renders tree structure correctly
func TestTreeViewRendering(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "epic-1", Title: "Epic Issue", Priority: 1, IssueType: model.TypeEpic, Status: model.StatusOpen, CreatedAt: now},
		{
			ID: "task-1", Title: "Task under Epic", Priority: 2, IssueType: model.TypeTask, Status: model.StatusInProgress, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "task-1", DependsOnID: "epic-1", Type: model.DepParentChild}},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)
	tree.SetSize(100, 30)

	view := tree.View()

	// Should contain both issue IDs
	if !strings.Contains(view, "epic-1") {
		t.Errorf("expected epic-1 in view, got:\n%s", view)
	}
	if !strings.Contains(view, "task-1") {
		t.Errorf("expected task-1 in view, got:\n%s", view)
	}

	// Should contain titles
	if !strings.Contains(view, "Epic Issue") {
		t.Errorf("expected 'Epic Issue' in view, got:\n%s", view)
	}

	// Should contain tree characters (for child node)
	if !strings.Contains(view, "└") && !strings.Contains(view, "├") {
		t.Errorf("expected tree branch characters in view, got:\n%s", view)
	}

	// Should contain expand/collapse indicators
	if !strings.Contains(view, "▾") && !strings.Contains(view, "▸") && !strings.Contains(view, "•") {
		t.Errorf("expected expand/collapse indicators in view, got:\n%s", view)
	}
}

// TestTreeViewIndicators verifies expand/collapse indicators
func TestTreeViewIndicators(t *testing.T) {
	tree := NewTreeModel(newTreeTestTheme())

	// Test leaf node indicator
	leafNode := &IssueTreeNode{
		Issue:    &model.Issue{ID: "leaf"},
		Children: nil,
	}
	if got := tree.getExpandIndicator(leafNode); got != "•" {
		t.Errorf("leaf indicator = %q, want %q", got, "•")
	}

	// Test expanded node indicator
	expandedNode := &IssueTreeNode{
		Issue:    &model.Issue{ID: "expanded"},
		Children: []*IssueTreeNode{{Issue: &model.Issue{ID: "child"}}},
		Expanded: true,
	}
	if got := tree.getExpandIndicator(expandedNode); got != "▾" {
		t.Errorf("expanded indicator = %q, want %q", got, "▾")
	}

	// Test collapsed node indicator
	collapsedNode := &IssueTreeNode{
		Issue:    &model.Issue{ID: "collapsed"},
		Children: []*IssueTreeNode{{Issue: &model.Issue{ID: "child"}}},
		Expanded: false,
	}
	if got := tree.getExpandIndicator(collapsedNode); got != "▸" {
		t.Errorf("collapsed indicator = %q, want %q", got, "▸")
	}
}

// TestTreeTruncateTitle verifies title truncation
func TestTreeTruncateTitle(t *testing.T) {
	tree := NewTreeModel(newTreeTestTheme())

	tests := []struct {
		title  string
		maxLen int
		want   string
	}{
		{"Short", 20, "Short"},
		{"This is a very long title that should be truncated", 20, "This is a very long…"},
		{"ABC", 3, "..."},
		{"A", 10, "A"},
	}

	for _, tt := range tests {
		got := tree.truncateTitle(tt.title, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateTitle(%q, %d) = %q, want %q", tt.title, tt.maxLen, got, tt.want)
		}
	}
}

// TestTreeJumpToParent verifies JumpToParent navigation
func TestTreeJumpToParent(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "root", Title: "Root", Priority: 1, IssueType: model.TypeEpic, CreatedAt: now},
		{
			ID: "child", Title: "Child", Priority: 2, IssueType: model.TypeTask, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child", DependsOnID: "root", Type: model.DepParentChild}},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Move to child
	tree.MoveDown()
	if tree.GetSelectedID() != "child" {
		t.Fatalf("expected child selected, got %s", tree.GetSelectedID())
	}

	// Jump to parent
	tree.JumpToParent()
	if tree.GetSelectedID() != "root" {
		t.Errorf("expected root after JumpToParent, got %s", tree.GetSelectedID())
	}

	// Jump to parent at root should do nothing
	tree.JumpToParent()
	if tree.GetSelectedID() != "root" {
		t.Errorf("expected root to stay selected, got %s", tree.GetSelectedID())
	}
}

// TestTreeExpandOrMoveToChild verifies → key behavior
func TestTreeExpandOrMoveToChild(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "root", Title: "Root", Priority: 1, IssueType: model.TypeEpic, CreatedAt: now},
		{
			ID: "child", Title: "Child", Priority: 2, IssueType: model.TypeTask, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child", DependsOnID: "root", Type: model.DepParentChild}},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Root is initially expanded (auto-expand depth < 2)
	// ExpandOrMoveToChild should move to first child
	tree.ExpandOrMoveToChild()
	if tree.GetSelectedID() != "child" {
		t.Errorf("expected child after ExpandOrMoveToChild on expanded node, got %s", tree.GetSelectedID())
	}

	// Go back to root
	tree.JumpToTop()

	// Collapse root first
	tree.ToggleExpand()
	if tree.NodeCount() != 1 {
		t.Fatalf("expected 1 node after collapse, got %d", tree.NodeCount())
	}

	// Now ExpandOrMoveToChild should expand
	tree.ExpandOrMoveToChild()
	if tree.NodeCount() != 2 {
		t.Errorf("expected 2 nodes after expand, got %d", tree.NodeCount())
	}
	// Cursor should still be on root
	if tree.GetSelectedID() != "root" {
		t.Errorf("expected cursor on root after expand, got %s", tree.GetSelectedID())
	}
}

// TestTreeCollapseOrJumpToParent verifies ← key behavior
func TestTreeCollapseOrJumpToParent(t *testing.T) {
	now := time.Now()
	issues := []model.Issue{
		{ID: "root", Title: "Root", Priority: 1, IssueType: model.TypeEpic, CreatedAt: now},
		{
			ID: "child", Title: "Child", Priority: 2, IssueType: model.TypeTask, CreatedAt: now.Add(time.Hour),
			Dependencies: []*model.Dependency{{IssueID: "child", DependsOnID: "root", Type: model.DepParentChild}},
		},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Root is expanded - CollapseOrJumpToParent should collapse
	tree.CollapseOrJumpToParent()
	if tree.NodeCount() != 1 {
		t.Errorf("expected 1 node after collapse, got %d", tree.NodeCount())
	}

	// Now root is collapsed - CollapseOrJumpToParent should do nothing (already at root)
	tree.CollapseOrJumpToParent()
	if tree.GetSelectedID() != "root" {
		t.Errorf("expected cursor on root, got %s", tree.GetSelectedID())
	}

	// Expand and move to child
	tree.ExpandOrMoveToChild() // expand
	tree.ExpandOrMoveToChild() // move to child
	if tree.GetSelectedID() != "child" {
		t.Fatalf("expected child selected, got %s", tree.GetSelectedID())
	}

	// CollapseOrJumpToParent on leaf should jump to parent
	tree.CollapseOrJumpToParent()
	if tree.GetSelectedID() != "root" {
		t.Errorf("expected root after jump to parent from leaf, got %s", tree.GetSelectedID())
	}
}

// TestTreePageNavigation verifies PageUp/PageDown
func TestTreePageNavigation(t *testing.T) {
	// Create many issues for pagination testing
	var issues []model.Issue
	for i := 0; i < 20; i++ {
		issues = append(issues, model.Issue{
			ID:        fmt.Sprintf("issue-%d", i),
			Title:     fmt.Sprintf("Issue %d", i),
			Priority:  2,
			IssueType: model.TypeTask,
		})
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)
	tree.SetSize(80, 10) // Height of 10 -> page size of 5

	// PageDown
	tree.PageDown()
	if tree.cursor != 5 {
		t.Errorf("expected cursor at 5 after PageDown, got %d", tree.cursor)
	}

	// PageDown again
	tree.PageDown()
	if tree.cursor != 10 {
		t.Errorf("expected cursor at 10 after 2nd PageDown, got %d", tree.cursor)
	}

	// PageUp
	tree.PageUp()
	if tree.cursor != 5 {
		t.Errorf("expected cursor at 5 after PageUp, got %d", tree.cursor)
	}

	// Jump to bottom and PageDown should stay at end
	tree.JumpToBottom()
	tree.PageDown()
	if tree.cursor != 19 {
		t.Errorf("expected cursor at 19 (end), got %d", tree.cursor)
	}
}

// TestTreeSelectByID verifies cursor preservation by ID
func TestTreeSelectByID(t *testing.T) {
	issues := []model.Issue{
		{ID: "first", Title: "First", Priority: 1, IssueType: model.TypeTask},
		{ID: "second", Title: "Second", Priority: 2, IssueType: model.TypeTask},
		{ID: "third", Title: "Third", Priority: 3, IssueType: model.TypeTask},
	}

	tree := NewTreeModel(newTreeTestTheme())
	tree.Build(issues)

	// Select middle issue
	if !tree.SelectByID("second") {
		t.Fatal("SelectByID failed to find 'second'")
	}
	if tree.GetSelectedID() != "second" {
		t.Errorf("expected 'second' selected, got %s", tree.GetSelectedID())
	}

	// Try to select non-existent
	if tree.SelectByID("nonexistent") {
		t.Error("SelectByID should return false for non-existent ID")
	}
	// Cursor should remain unchanged
	if tree.GetSelectedID() != "second" {
		t.Errorf("cursor should not change after failed SelectByID, got %s", tree.GetSelectedID())
	}
}

// =============================================================================
// TreeState persistence tests (bv-zv7p)
// =============================================================================

func TestDefaultTreeState(t *testing.T) {
	state := DefaultTreeState()

	if state.Version != TreeStateVersion {
		t.Errorf("expected version %d, got %d", TreeStateVersion, state.Version)
	}
	if state.Expanded == nil {
		t.Error("Expanded map should not be nil")
	}
	if len(state.Expanded) != 0 {
		t.Errorf("expected empty Expanded map, got %d entries", len(state.Expanded))
	}
}

func TestTreeStatePath(t *testing.T) {
	tests := []struct {
		name     string
		beadsDir string
		want     string
	}{
		{
			name:     "default empty beads dir",
			beadsDir: "",
			want:     ".beads/tree-state.json",
		},
		{
			name:     "custom beads dir",
			beadsDir: "/path/to/beads",
			want:     "/path/to/beads/tree-state.json",
		},
		{
			name:     "relative beads dir",
			beadsDir: "custom/.beads",
			want:     "custom/.beads/tree-state.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TreeStatePath(tt.beadsDir)
			if got != tt.want {
				t.Errorf("TreeStatePath(%q) = %q, want %q", tt.beadsDir, got, tt.want)
			}
		})
	}
}

func TestTreeStateVersion(t *testing.T) {
	// Ensure version constant is reasonable
	if TreeStateVersion < 1 {
		t.Errorf("TreeStateVersion should be >= 1, got %d", TreeStateVersion)
	}
}
