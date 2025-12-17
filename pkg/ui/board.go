package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// BoardModel represents the Kanban board view with adaptive columns
type BoardModel struct {
	columns      [4][]model.Issue
	activeColIdx []int  // Indices of non-empty columns (for navigation)
	focusedCol   int    // Index into activeColIdx
	selectedRow  [4]int // Store selection for each column
	theme        Theme

	// Reverse dependency index: maps issue ID -> slice of issue IDs it blocks (bv-1daf)
	blocksIndex map[string][]string

	// Detail panel (bv-r6kh)
	showDetail   bool
	detailVP     viewport.Model
	mdRenderer   *glamour.TermRenderer
	lastDetailID string // Track which issue detail is currently rendered
}

// Column indices for the Kanban board
const (
	ColOpen       = 0
	ColInProgress = 1
	ColBlocked    = 2
	ColClosed     = 3
)

// sortIssuesByPriorityAndDate sorts issues by priority (ascending) then by creation date (descending)
func sortIssuesByPriorityAndDate(issues []model.Issue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Priority != issues[j].Priority {
			return issues[i].Priority < issues[j].Priority
		}
		return issues[i].CreatedAt.After(issues[j].CreatedAt)
	})
}

// updateActiveColumns rebuilds the list of non-empty column indices
func (b *BoardModel) updateActiveColumns() {
	b.activeColIdx = nil
	for i := 0; i < 4; i++ {
		if len(b.columns[i]) > 0 {
			b.activeColIdx = append(b.activeColIdx, i)
		}
	}
	// If all columns are empty, include all columns anyway
	if len(b.activeColIdx) == 0 {
		b.activeColIdx = []int{ColOpen, ColInProgress, ColBlocked, ColClosed}
	}
	// Ensure focused column is within valid range
	if b.focusedCol >= len(b.activeColIdx) {
		b.focusedCol = len(b.activeColIdx) - 1
	}
	if b.focusedCol < 0 {
		b.focusedCol = 0
	}
}

// buildBlocksIndex creates a reverse dependency map: for each issue that is depended on,
// it stores the list of issue IDs that depend on it (bv-1daf)
func buildBlocksIndex(issues []model.Issue) map[string][]string {
	index := make(map[string][]string)
	for _, issue := range issues {
		for _, dep := range issue.Dependencies {
			if dep != nil && dep.Type.IsBlocking() {
				// dep.DependsOnID blocks issue.ID
				index[dep.DependsOnID] = append(index[dep.DependsOnID], issue.ID)
			}
		}
	}
	return index
}

// NewBoardModel creates a new Kanban board from the given issues
func NewBoardModel(issues []model.Issue, theme Theme) BoardModel {
	var cols [4][]model.Issue

	// Distribute issues into columns by status
	for _, issue := range issues {
		switch issue.Status {
		case model.StatusOpen:
			cols[ColOpen] = append(cols[ColOpen], issue)
		case model.StatusInProgress:
			cols[ColInProgress] = append(cols[ColInProgress], issue)
		case model.StatusBlocked:
			cols[ColBlocked] = append(cols[ColBlocked], issue)
		case model.StatusClosed:
			cols[ColClosed] = append(cols[ColClosed], issue)
		}
	}

	// Sort each column
	for i := 0; i < 4; i++ {
		sortIssuesByPriorityAndDate(cols[i])
	}

	// Initialize markdown renderer for detail panel (bv-r6kh)
	var mdRenderer *glamour.TermRenderer
	mdRenderer, _ = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(60),
	)

	b := BoardModel{
		columns:     cols,
		focusedCol:  0,
		theme:       theme,
		blocksIndex: buildBlocksIndex(issues),
		detailVP:    viewport.New(40, 20),
		mdRenderer:  mdRenderer,
	}
	b.updateActiveColumns()
	return b
}

// SetIssues updates the board data, typically after filtering
func (b *BoardModel) SetIssues(issues []model.Issue) {
	var cols [4][]model.Issue

	for _, issue := range issues {
		switch issue.Status {
		case model.StatusOpen:
			cols[ColOpen] = append(cols[ColOpen], issue)
		case model.StatusInProgress:
			cols[ColInProgress] = append(cols[ColInProgress], issue)
		case model.StatusBlocked:
			cols[ColBlocked] = append(cols[ColBlocked], issue)
		case model.StatusClosed:
			cols[ColClosed] = append(cols[ColClosed], issue)
		}
	}

	// Sort each column
	for i := 0; i < 4; i++ {
		sortIssuesByPriorityAndDate(cols[i])
	}

	b.columns = cols
	b.blocksIndex = buildBlocksIndex(issues) // Rebuild reverse dependency index (bv-1daf)

	// Sanitize selection to prevent out-of-bounds
	for i := 0; i < 4; i++ {
		if b.selectedRow[i] >= len(b.columns[i]) {
			if len(b.columns[i]) > 0 {
				b.selectedRow[i] = len(b.columns[i]) - 1
			} else {
				b.selectedRow[i] = 0
			}
		}
	}

	b.updateActiveColumns()
}

// actualFocusedCol returns the actual column index (0-3) being focused
func (b *BoardModel) actualFocusedCol() int {
	if len(b.activeColIdx) == 0 {
		return 0
	}
	return b.activeColIdx[b.focusedCol]
}

// Navigation methods
func (b *BoardModel) MoveDown() {
	col := b.actualFocusedCol()
	count := len(b.columns[col])
	if count == 0 {
		return
	}
	if b.selectedRow[col] < count-1 {
		b.selectedRow[col]++
	}
}

func (b *BoardModel) MoveUp() {
	col := b.actualFocusedCol()
	if b.selectedRow[col] > 0 {
		b.selectedRow[col]--
	}
}

func (b *BoardModel) MoveRight() {
	if b.focusedCol < len(b.activeColIdx)-1 {
		b.focusedCol++
	}
}

func (b *BoardModel) MoveLeft() {
	if b.focusedCol > 0 {
		b.focusedCol--
	}
}

func (b *BoardModel) MoveToTop() {
	col := b.actualFocusedCol()
	b.selectedRow[col] = 0
}

func (b *BoardModel) MoveToBottom() {
	col := b.actualFocusedCol()
	count := len(b.columns[col])
	if count > 0 {
		b.selectedRow[col] = count - 1
	}
}

func (b *BoardModel) PageDown(visibleRows int) {
	col := b.actualFocusedCol()
	count := len(b.columns[col])
	if count == 0 {
		return
	}
	newRow := b.selectedRow[col] + visibleRows/2
	if newRow >= count {
		newRow = count - 1
	}
	b.selectedRow[col] = newRow
}

func (b *BoardModel) PageUp(visibleRows int) {
	col := b.actualFocusedCol()
	newRow := b.selectedRow[col] - visibleRows/2
	if newRow < 0 {
		newRow = 0
	}
	b.selectedRow[col] = newRow
}

// Detail panel methods (bv-r6kh)

// ToggleDetail toggles the detail panel visibility
func (b *BoardModel) ToggleDetail() {
	b.showDetail = !b.showDetail
}

// ShowDetail shows the detail panel
func (b *BoardModel) ShowDetail() {
	b.showDetail = true
}

// HideDetail hides the detail panel
func (b *BoardModel) HideDetail() {
	b.showDetail = false
}

// IsDetailShown returns whether detail panel is visible
func (b *BoardModel) IsDetailShown() bool {
	return b.showDetail
}

// DetailScrollDown scrolls the detail panel down
func (b *BoardModel) DetailScrollDown(lines int) {
	b.detailVP.LineDown(lines)
}

// DetailScrollUp scrolls the detail panel up
func (b *BoardModel) DetailScrollUp(lines int) {
	b.detailVP.LineUp(lines)
}

// SelectedIssue returns the currently selected issue, or nil if none
func (b *BoardModel) SelectedIssue() *model.Issue {
	col := b.actualFocusedCol()
	cols := b.columns[col]
	row := b.selectedRow[col]
	if len(cols) > 0 && row < len(cols) {
		return &cols[row]
	}
	return nil
}

// ColumnCount returns the number of issues in a column
func (b *BoardModel) ColumnCount(col int) int {
	if col >= 0 && col < 4 {
		return len(b.columns[col])
	}
	return 0
}

// TotalCount returns the total number of issues across all columns
func (b *BoardModel) TotalCount() int {
	total := 0
	for i := 0; i < 4; i++ {
		total += len(b.columns[i])
	}
	return total
}

// View renders the Kanban board with adaptive columns
func (b BoardModel) View(width, height int) string {
	t := b.theme

	// Calculate how many columns we're showing
	numCols := len(b.activeColIdx)
	if numCols == 0 {
		return t.Renderer.NewStyle().
			Width(width).
			Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(t.Secondary).
			Render("No issues to display")
	}

	// Calculate board width vs detail panel width (bv-r6kh)
	// Detail panel takes ~35% of width when shown, min 40 chars
	boardWidth := width
	detailWidth := 0
	if b.showDetail && width > 120 {
		detailWidth = width * 35 / 100
		if detailWidth < 40 {
			detailWidth = 40
		}
		if detailWidth > 80 {
			detailWidth = 80
		}
		boardWidth = width - detailWidth - 1 // 1 char gap
	}

	// Calculate column widths - distribute space evenly
	// Minimum column width for readability, NO maximum cap (bv-ic17)
	minColWidth := 28

	// Calculate available width (subtract gaps between columns)
	gaps := numCols - 1
	availableWidth := boardWidth - (gaps * 2) // 2 chars gap between columns

	// Distribute width evenly across columns, respecting minimum
	baseWidth := availableWidth / numCols
	if baseWidth < minColWidth {
		baseWidth = minColWidth
	}
	// NO maxColWidth cap - use all available horizontal space

	colHeight := height - 4 // Account for header
	if colHeight < 8 {
		colHeight = 8
	}

	columnTitles := []string{"OPEN", "IN PROGRESS", "BLOCKED", "CLOSED"}
	columnColors := []lipgloss.AdaptiveColor{t.Open, t.InProgress, t.Blocked, t.Closed}
	columnEmoji := []string{"📋", "🔄", "🚫", "✅"}

	var renderedCols []string

	for i, colIdx := range b.activeColIdx {
		isFocused := b.focusedCol == i
		issues := b.columns[colIdx]
		issueCount := len(issues)

		// Header with emoji, title, and count
		headerText := fmt.Sprintf("%s %s (%d)", columnEmoji[colIdx], columnTitles[colIdx], issueCount)
		headerStyle := t.Renderer.NewStyle().
			Width(baseWidth).
			Align(lipgloss.Center).
			Bold(true).
			Padding(0, 1)

		if isFocused {
			headerStyle = headerStyle.
				Background(columnColors[colIdx]).
				Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1a1a1a"})
		} else {
			headerStyle = headerStyle.
				Background(lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#2a2a2a"}).
				Foreground(columnColors[colIdx])
		}

		header := headerStyle.Render(headerText)

		// Calculate visible rows (bv-1daf: now 4 content lines)
		// Cards have 4 content lines + 1 margin, plus borders:
		// - Non-selected: bottom border only (+1) = ~6 lines
		// - Selected: full rounded border (+2) = ~7 lines
		// Use 6 as average to avoid overflow
		cardHeight := 6
		visibleCards := (colHeight - 1) / cardHeight
		if visibleCards < 1 {
			visibleCards = 1
		}

		sel := b.selectedRow[colIdx]
		if sel >= issueCount && issueCount > 0 {
			sel = issueCount - 1
		}

		// Simple scrolling: keep selected card visible
		start := 0
		if sel >= visibleCards {
			start = sel - visibleCards + 1
		}

		end := start + visibleCards
		if end > issueCount {
			end = issueCount
		}

		// Render cards
		var cards []string
		for rowIdx := start; rowIdx < end; rowIdx++ {
			issue := issues[rowIdx]
			isSelected := isFocused && rowIdx == sel

			card := b.renderCard(issue, baseWidth-4, isSelected, colIdx)
			cards = append(cards, card)
		}

		// Empty column placeholder
		if issueCount == 0 {
			emptyStyle := t.Renderer.NewStyle().
				Width(baseWidth-4).
				Height(colHeight-2).
				Align(lipgloss.Center, lipgloss.Center).
				Foreground(t.Secondary).
				Italic(true)
			cards = append(cards, emptyStyle.Render("(empty)"))
		}

		// Scroll indicator
		if issueCount > visibleCards {
			scrollInfo := fmt.Sprintf("↕ %d/%d", sel+1, issueCount)
			scrollStyle := t.Renderer.NewStyle().
				Width(baseWidth - 4).
				Align(lipgloss.Center).
				Foreground(t.Secondary).
				Italic(true)
			cards = append(cards, scrollStyle.Render(scrollInfo))
		}

		// Column content
		content := lipgloss.JoinVertical(lipgloss.Left, cards...)

		// Column container
		colStyle := t.Renderer.NewStyle().
			Width(baseWidth).
			Height(colHeight).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder())

		if isFocused {
			colStyle = colStyle.BorderForeground(columnColors[colIdx])
		} else {
			colStyle = colStyle.BorderForeground(t.Secondary)
		}

		column := lipgloss.JoinVertical(lipgloss.Center, header, colStyle.Render(content))
		renderedCols = append(renderedCols, column)
	}

	// Join columns with gaps
	boardView := lipgloss.JoinHorizontal(lipgloss.Top, renderedCols...)

	// Add detail panel if shown (bv-r6kh)
	if detailWidth > 0 {
		detailPanel := b.renderDetailPanel(detailWidth, height-2)
		return lipgloss.JoinHorizontal(lipgloss.Top, boardView, detailPanel)
	}

	return boardView
}

// getAgeColor returns a color based on issue age (bv-1daf)
// green (<7d), yellow (7-30d), red (>30d stale)
func getAgeColor(t time.Time) lipgloss.TerminalColor {
	if t.IsZero() {
		return ColorMuted
	}
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days < 7:
		return lipgloss.AdaptiveColor{Light: "#2e7d32", Dark: "#81c784"} // green
	case days < 30:
		return lipgloss.AdaptiveColor{Light: "#f57c00", Dark: "#ffb74d"} // yellow/orange
	default:
		return lipgloss.AdaptiveColor{Light: "#c62828", Dark: "#e57373"} // red
	}
}

// formatPriority returns priority as P0/P1/P2/P3/P4 (bv-1daf)
func formatPriority(p int) string {
	if p < 0 {
		p = 0
	}
	if p > 4 {
		p = 4
	}
	return fmt.Sprintf("P%d", p)
}

// renderCard creates a visually rich card for an issue (bv-1daf: 4-line format)
func (b BoardModel) renderCard(issue model.Issue, width int, selected bool, colIdx int) string {
	t := b.theme

	// ══════════════════════════════════════════════════════════════════════════
	// CARD STYLING - Fixed 4-line height for consistency (bv-1daf)
	// ══════════════════════════════════════════════════════════════════════════
	cardStyle := t.Renderer.NewStyle().
		Width(width).
		Padding(0, 1).
		MarginBottom(1)

	if selected {
		// Selected: elevated with accent border
		cardStyle = cardStyle.
			Background(t.Highlight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.Primary)
	} else {
		// Unselected: subtle card with bottom border
		cardStyle = cardStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.Border)
	}

	// ══════════════════════════════════════════════════════════════════════════
	// LINE 1: Type icon + Priority (P0/P1/P2) + ID + Age with color (bv-1daf)
	// ══════════════════════════════════════════════════════════════════════════
	icon, iconColor := t.GetTypeIcon(string(issue.IssueType))

	// Priority as P0/P1/P2 text (clearer than emoji flame levels)
	prioText := formatPriority(issue.Priority)
	prioStyle := t.Renderer.NewStyle().Bold(true)
	if issue.Priority <= 1 {
		prioStyle = prioStyle.Foreground(lipgloss.AdaptiveColor{Light: "#c62828", Dark: "#ef5350"})
	} else {
		prioStyle = prioStyle.Foreground(t.Secondary)
	}

	// Truncate ID for narrow cards - reserve space for age indicator
	maxIDLen := width - 14 // Icon(2) + space + P#(2) + space + age(6) + spacing
	if maxIDLen < 6 {
		maxIDLen = 6
	}
	displayID := truncateRunesHelper(issue.ID, maxIDLen, "…")

	// Age indicator with color coding: green(<7d), yellow(7-30d), red(>30d)
	ageText := FormatTimeRel(issue.UpdatedAt)
	if len(ageText) > 6 {
		ageText = truncateRunesHelper(ageText, 6, "")
	}
	ageColor := getAgeColor(issue.UpdatedAt)
	ageStyled := t.Renderer.NewStyle().Foreground(ageColor).Render(ageText)

	line1 := fmt.Sprintf("%s %s %s %s",
		t.Renderer.NewStyle().Foreground(iconColor).Render(icon),
		prioStyle.Render(prioText),
		t.Renderer.NewStyle().Bold(true).Foreground(t.Secondary).Render(displayID),
		ageStyled,
	)

	// ══════════════════════════════════════════════════════════════════════════
	// LINE 2: Title with full available width (bv-1daf)
	// ══════════════════════════════════════════════════════════════════════════
	titleWidth := width - 2
	if titleWidth < 10 {
		titleWidth = 10
	}
	truncatedTitle := truncateRunesHelper(issue.Title, titleWidth, "…")

	titleStyle := t.Renderer.NewStyle()
	if selected {
		titleStyle = titleStyle.Foreground(t.Primary).Bold(true)
	} else {
		titleStyle = titleStyle.Foreground(t.Base.GetForeground())
	}
	line2 := titleStyle.Render(truncatedTitle)

	// ══════════════════════════════════════════════════════════════════════════
	// LINE 3: Blocked-by + Blocks count + Labels (bv-1daf)
	// No @assignee - not useful for agent workflows per spec
	// ══════════════════════════════════════════════════════════════════════════
	var meta []string

	// Blocked-by indicator: 🚫←bv-456 (compact) - show first blocking dep
	for _, dep := range issue.Dependencies {
		if dep != nil && dep.Type.IsBlocking() {
			blockerID := truncateRunesHelper(dep.DependsOnID, 10, "…")
			blockedStyle := t.Renderer.NewStyle().Foreground(t.Blocked)
			meta = append(meta, blockedStyle.Render("🚫←"+blockerID))
			break // Only show first blocker
		}
	}

	// Blocks count: ⚡→N (this card blocks N others) - from reverse index
	if blockedIDs, ok := b.blocksIndex[issue.ID]; ok && len(blockedIDs) > 0 {
		blocksStyle := t.Renderer.NewStyle().Foreground(t.Feature)
		meta = append(meta, blocksStyle.Render(fmt.Sprintf("⚡→%d", len(blockedIDs))))
	}

	// Labels: show 2-3 label names (no "+N" count per spec)
	if len(issue.Labels) > 0 {
		maxLabels := 3
		if len(issue.Labels) < maxLabels {
			maxLabels = len(issue.Labels)
		}
		var labelParts []string
		for i := 0; i < maxLabels; i++ {
			labelParts = append(labelParts, truncateRunesHelper(issue.Labels[i], 8, ""))
		}
		labelText := strings.Join(labelParts, ",")
		labelStyle := t.Renderer.NewStyle().Foreground(t.InProgress)
		meta = append(meta, labelStyle.Render(labelText))
	}

	line3 := ""
	if len(meta) > 0 {
		line3 = strings.Join(meta, " ")
	}

	// ══════════════════════════════════════════════════════════════════════════
	// LINE 4: Empty line for consistent 4-line height (bv-1daf)
	// Could be used for activity indicator in future
	// ══════════════════════════════════════════════════════════════════════════
	line4 := "" // Placeholder for optional activity bar

	return cardStyle.Render(lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3, line4))
}

// renderDetailPanel renders the detail panel for the selected issue (bv-r6kh)
func (b *BoardModel) renderDetailPanel(width, height int) string {
	t := b.theme

	// Get the selected issue
	issue := b.SelectedIssue()

	// Update viewport dimensions
	vpWidth := width - 4 // Account for border
	vpHeight := height - 6
	if vpWidth < 20 {
		vpWidth = 20
	}
	if vpHeight < 5 {
		vpHeight = 5
	}
	b.detailVP.Width = vpWidth
	b.detailVP.Height = vpHeight

	// Build content based on selection state
	if issue == nil {
		// No issue selected - show help text (use special marker to detect "no selection" state)
		if b.lastDetailID != "_none_" {
			b.lastDetailID = "_none_"
			helpText := "## No Selection\n\nNavigate to a card with **h/l** and **j/k** to see details here.\n\nPress **Tab** to hide this panel."
			rendered := helpText
			if b.mdRenderer != nil {
				if md, err := b.mdRenderer.Render(helpText); err == nil {
					rendered = md
				}
			}
			b.detailVP.SetContent(rendered)
			b.detailVP.GotoTop()
		}
	} else {
		// Issue selected - only update content if the issue changed
		if b.lastDetailID != issue.ID {
			b.lastDetailID = issue.ID

			var content strings.Builder

			// Header with ID and type
			icon, _ := t.GetTypeIcon(string(issue.IssueType))
			content.WriteString(fmt.Sprintf("## %s %s\n\n", icon, issue.ID))

			// Title
			content.WriteString(fmt.Sprintf("**%s**\n\n", issue.Title))

			// Status and Priority
			statusIcon := GetStatusIcon(string(issue.Status))
			prioIcon := GetPriorityIcon(issue.Priority)
			content.WriteString(fmt.Sprintf("%s %s  %s P%d\n\n",
				statusIcon, issue.Status, prioIcon, issue.Priority))

			// Metadata section
			if issue.Assignee != "" {
				content.WriteString(fmt.Sprintf("**Assignee:** @%s\n\n", issue.Assignee))
			}

			if len(issue.Labels) > 0 {
				content.WriteString(fmt.Sprintf("**Labels:** %s\n\n", strings.Join(issue.Labels, ", ")))
			}

			// Dependencies
			if len(issue.Dependencies) > 0 {
				content.WriteString("**Blocked by:**\n")
				for _, dep := range issue.Dependencies {
					content.WriteString(fmt.Sprintf("- %s\n", dep))
				}
				content.WriteString("\n")
			}

			// Description
			if issue.Description != "" {
				content.WriteString("---\n\n")
				content.WriteString(issue.Description)
				content.WriteString("\n")
			}

			// Timestamps
			content.WriteString("\n---\n\n")
			content.WriteString(fmt.Sprintf("*Created: %s*\n", FormatTimeRel(issue.CreatedAt)))
			content.WriteString(fmt.Sprintf("*Updated: %s*\n", FormatTimeRel(issue.UpdatedAt)))

			// Render with markdown
			rendered := content.String()
			if b.mdRenderer != nil {
				if md, err := b.mdRenderer.Render(rendered); err == nil {
					rendered = md
				}
			}
			b.detailVP.SetContent(rendered)
			b.detailVP.GotoTop()
		}
	}

	// Build scroll indicator
	var sb strings.Builder
	sb.WriteString(b.detailVP.View())

	scrollPercent := b.detailVP.ScrollPercent()
	if scrollPercent < 1.0 || b.detailVP.YOffset > 0 {
		scrollHint := t.Renderer.NewStyle().
			Foreground(t.Secondary).
			Italic(true).
			Render(fmt.Sprintf("─ %d%% ─ ctrl+j/k", int(scrollPercent*100)))
		sb.WriteString("\n")
		sb.WriteString(scrollHint)
	}

	// Panel border style
	panelStyle := t.Renderer.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary).
		Width(width).
		Height(height).
		Padding(0, 1)

	// Title bar
	titleBar := t.Renderer.NewStyle().
		Bold(true).
		Foreground(t.Primary).
		Width(width - 4).
		Align(lipgloss.Center).
		Render("DETAILS")

	return panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleBar, sb.String()))
}
