// tree.go - Hierarchical tree view for epic/task/subtask relationships (bv-gllx)
package ui

import (
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/charmbracelet/bubbles/viewport"
)

// TreeViewMode determines what relationships are displayed
type TreeViewMode int

const (
	TreeModeHierarchy TreeViewMode = iota // parent-child deps (default)
	TreeModeBlocking                      // blocking deps (future)
)

// IssueTreeNode represents a node in the hierarchical issue tree
type IssueTreeNode struct {
	Issue    *model.Issue     // Reference to the actual issue
	Children []*IssueTreeNode // Child nodes
	Expanded bool             // Is this node expanded?
	Depth    int              // Nesting level (0 = root)
	Parent   *IssueTreeNode   // Back-reference for navigation
}

// TreeModel manages the hierarchical tree view state
type TreeModel struct {
	roots    []*IssueTreeNode           // Root nodes (issues with no parent)
	flatList []*IssueTreeNode           // Flattened visible nodes for navigation
	cursor   int                        // Current selection index in flatList
	viewport viewport.Model             // For scrolling
	theme    Theme                      // Visual styling
	mode     TreeViewMode               // Hierarchy vs blocking
	issueMap map[string]*IssueTreeNode  // Quick lookup by issue ID
	width    int                        // Available width
	height   int                        // Available height

	// Build state
	built    bool   // Has tree been built?
	lastHash string // Hash of issues for cache invalidation
}

// NewTreeModel creates an empty tree model
func NewTreeModel(theme Theme) TreeModel {
	return TreeModel{
		theme:    theme,
		mode:     TreeModeHierarchy,
		issueMap: make(map[string]*IssueTreeNode),
	}
}

// SetSize updates the available dimensions for the tree view
func (t *TreeModel) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.viewport.Width = width
	t.viewport.Height = height
}

// Build constructs the tree from issues using parent-child dependencies.
// This is a placeholder - actual implementation in bv-j3ck.
func (t *TreeModel) Build(issues []model.Issue) {
	// Placeholder - will be implemented in bv-j3ck
	t.roots = nil
	t.flatList = nil
	t.cursor = 0
	t.built = true
}

// View renders the tree view.
// This is a placeholder - actual implementation in bv-1371.
func (t *TreeModel) View() string {
	if !t.built || len(t.flatList) == 0 {
		return "Tree view: No issues to display.\nPress E to return to list view."
	}
	return "Tree view placeholder"
}

// SelectedIssue returns the currently selected issue, or nil if none.
func (t *TreeModel) SelectedIssue() *model.Issue {
	if t.cursor >= 0 && t.cursor < len(t.flatList) {
		return t.flatList[t.cursor].Issue
	}
	return nil
}

// SelectedNode returns the currently selected tree node, or nil if none.
func (t *TreeModel) SelectedNode() *IssueTreeNode {
	if t.cursor >= 0 && t.cursor < len(t.flatList) {
		return t.flatList[t.cursor]
	}
	return nil
}

// MoveDown moves the cursor down in the flat list.
// Placeholder - actual implementation in bv-2fpk.
func (t *TreeModel) MoveDown() {
	if t.cursor < len(t.flatList)-1 {
		t.cursor++
	}
}

// MoveUp moves the cursor up in the flat list.
// Placeholder - actual implementation in bv-2fpk.
func (t *TreeModel) MoveUp() {
	if t.cursor > 0 {
		t.cursor--
	}
}

// ToggleExpand expands or collapses the currently selected node.
// Placeholder - actual implementation in bv-2fpk.
func (t *TreeModel) ToggleExpand() {
	node := t.SelectedNode()
	if node != nil && len(node.Children) > 0 {
		node.Expanded = !node.Expanded
		t.rebuildFlatList()
	}
}

// ExpandAll expands all nodes in the tree.
// Placeholder - actual implementation in bv-2fpk.
func (t *TreeModel) ExpandAll() {
	for _, root := range t.roots {
		t.setExpandedRecursive(root, true)
	}
	t.rebuildFlatList()
}

// CollapseAll collapses all nodes in the tree.
// Placeholder - actual implementation in bv-2fpk.
func (t *TreeModel) CollapseAll() {
	for _, root := range t.roots {
		t.setExpandedRecursive(root, false)
	}
	t.rebuildFlatList()
}

// JumpToTop moves cursor to the first node.
func (t *TreeModel) JumpToTop() {
	t.cursor = 0
}

// JumpToBottom moves cursor to the last node.
func (t *TreeModel) JumpToBottom() {
	if len(t.flatList) > 0 {
		t.cursor = len(t.flatList) - 1
	}
}

// setExpandedRecursive sets the expanded state for a node and all descendants.
func (t *TreeModel) setExpandedRecursive(node *IssueTreeNode, expanded bool) {
	if node == nil {
		return
	}
	node.Expanded = expanded
	for _, child := range node.Children {
		t.setExpandedRecursive(child, expanded)
	}
}

// rebuildFlatList rebuilds the flattened list of visible nodes.
// Placeholder - actual implementation in bv-2fpk.
func (t *TreeModel) rebuildFlatList() {
	t.flatList = t.flatList[:0]
	for _, root := range t.roots {
		t.appendVisible(root)
	}
	// Ensure cursor stays in bounds
	if t.cursor >= len(t.flatList) {
		t.cursor = len(t.flatList) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

// appendVisible adds a node and its visible descendants to flatList.
func (t *TreeModel) appendVisible(node *IssueTreeNode) {
	if node == nil {
		return
	}
	t.flatList = append(t.flatList, node)
	if node.Expanded {
		for _, child := range node.Children {
			t.appendVisible(child)
		}
	}
}

// IsBuilt returns whether the tree has been built.
func (t *TreeModel) IsBuilt() bool {
	return t.built
}

// NodeCount returns the total number of visible nodes.
func (t *TreeModel) NodeCount() int {
	return len(t.flatList)
}

// RootCount returns the number of root nodes.
func (t *TreeModel) RootCount() int {
	return len(t.roots)
}
