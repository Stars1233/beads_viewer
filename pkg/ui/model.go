package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"beads_viewer/pkg/analysis"
	"beads_viewer/pkg/model"
	"beads_viewer/pkg/updater"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// View width thresholds for adaptive layout
const (
	SplitViewThreshold     = 100
	WideViewThreshold      = 140
	UltraWideViewThreshold = 180
)

// focus represents which UI element has keyboard focus
type focus int

const (
	focusList focus = iota
	focusDetail
	focusBoard
	focusInsights
	focusHelp
	focusQuitConfirm
)

// UpdateMsg is sent when a new version is available
type UpdateMsg struct {
	TagName string
	URL     string
}

// CheckUpdateCmd returns a command that checks for updates
func CheckUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		tag, url, err := updater.CheckForUpdates()
		if err == nil && tag != "" {
			return UpdateMsg{TagName: tag, URL: url}
		}
		return nil
	}
}

// Model is the main Bubble Tea model for the beads viewer
type Model struct {
	// Data
	issues   []model.Issue
	issueMap map[string]*model.Issue
	analysis analysis.GraphStats

	// UI Components
	list          list.Model
	viewport      viewport.Model
	renderer      *glamour.TermRenderer
	board         BoardModel
	insightsPanel InsightsModel
	theme         Theme

	// Update State
	updateAvailable bool
	updateTag       string
	updateURL       string

	// Focus and View State
	focused         focus
	isSplitView     bool
	isBoardView     bool
	showDetails     bool
	showHelp        bool
	showQuitConfirm bool
	ready           bool
	width           int
	height          int

	// Filter state
	currentFilter string
	searchTerm    string

	// Stats (cached)
	countOpen    int
	countReady   int
	countBlocked int
	countClosed  int
}

// NewModel creates a new Model from the given issues
func NewModel(issues []model.Issue) Model {
	// Build lookup map
	issueMap := make(map[string]*model.Issue, len(issues))

	// Sort issues: Open first, then by Priority (ascending), then by date (newest first)
	sort.Slice(issues, func(i, j int) bool {
		iClosed := issues[i].Status == model.StatusClosed
		jClosed := issues[j].Status == model.StatusClosed
		if iClosed != jClosed {
			return !iClosed // Open issues first
		}
		if issues[i].Priority != issues[j].Priority {
			return issues[i].Priority < issues[j].Priority // Lower priority number = higher priority
		}
		return issues[i].CreatedAt.After(issues[j].CreatedAt) // Newer first
	})

	// Graph Analysis (single pass for performance)
	analyzer := analysis.NewAnalyzer(issues)
	graphStats := analyzer.Analyze()

	// Build list items with pre-computed graph scores
	items := make([]list.Item, len(issues))
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]

		items[i] = IssueItem{
			Issue:      issues[i],
			GraphScore: graphStats.PageRank[issues[i].ID],
			Impact:     graphStats.CriticalPathScore[issues[i].ID],
		}
	}

	// Compute stats
	cOpen, cReady, cBlocked, cClosed := 0, 0, 0, 0
	for i := range issues {
		issue := &issues[i]
		if issue.Status == model.StatusClosed {
			cClosed++
			continue
		}

		cOpen++
		if issue.Status == model.StatusBlocked {
			cBlocked++
			continue
		}

		// Check if blocked by open dependencies
		isBlocked := false
		for _, dep := range issue.Dependencies {
			if dep.Type != model.DepBlocks {
				continue
			}
			if blocker, exists := issueMap[dep.DependsOnID]; exists && blocker.Status != model.StatusClosed {
				isBlocked = true
				break
			}
		}
		if !isBlocked {
			cReady++
		}
	}

	// Theme
	theme := DefaultTheme(lipgloss.NewRenderer(os.Stdout))

	// List setup
	delegate := IssueDelegate{Theme: theme}
	l := list.New(items, delegate, 0, 0)
	l.Title = ""
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	// Clear all default styles that might add extra lines
	l.Styles.Title = lipgloss.NewStyle()
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(theme.Primary)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(theme.Primary)
	l.Styles.StatusBar = lipgloss.NewStyle()
	l.Styles.StatusEmpty = lipgloss.NewStyle()
	l.Styles.StatusBarActiveFilter = lipgloss.NewStyle()
	l.Styles.StatusBarFilterCount = lipgloss.NewStyle()
	l.Styles.NoItems = lipgloss.NewStyle()
	l.Styles.PaginationStyle = lipgloss.NewStyle()
	l.Styles.HelpStyle = lipgloss.NewStyle()

	// Glamour markdown renderer
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	// Initialize sub-components
	board := NewBoardModel(issues, theme)
	ins := graphStats.GenerateInsights(10)
	insightsPanel := NewInsightsModel(ins, issueMap, theme)

	return Model{
		issues:        issues,
		issueMap:      issueMap,
		analysis:      graphStats,
		list:          l,
		renderer:      renderer,
		board:         board,
		insightsPanel: insightsPanel,
		theme:         theme,
		currentFilter: "all",
		focused:       focusList,
		countOpen:     cOpen,
		countReady:    cReady,
		countBlocked:  cBlocked,
		countClosed:   cClosed,
	}
}

func (m Model) Init() tea.Cmd {
	return CheckUpdateCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case UpdateMsg:
		m.updateAvailable = true
		m.updateTag = msg.TagName
		m.updateURL = msg.URL

	case tea.KeyMsg:
		// Handle quit confirmation first
		if m.showQuitConfirm {
			switch msg.String() {
			case "esc", "y", "Y":
				return m, tea.Quit
			default:
				m.showQuitConfirm = false
				m.focused = focusList
				return m, nil
			}
		}

		// Handle help overlay toggle (? or F1)
		if (msg.String() == "?" || msg.String() == "f1") && m.list.FilterState() != list.Filtering {
			m.showHelp = !m.showHelp
			if m.showHelp {
				m.focused = focusHelp
			} else {
				m.focused = focusList
			}
			return m, nil
		}

		// If help is showing, any key (except ?/F1) dismisses it
		if m.focused == focusHelp {
			m.showHelp = false
			m.focused = focusList
			return m, nil
		}

		// Handle keys when not filtering
		if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit

			case "q":
				// q closes current view or quits if at top level
				if m.showDetails && !m.isSplitView {
					m.showDetails = false
					return m, nil
				}
				if m.focused == focusInsights {
					m.focused = focusList
					return m, nil
				}
				if m.isBoardView {
					m.isBoardView = false
					m.focused = focusList
					return m, nil
				}
				return m, tea.Quit

			case "esc":
				// Escape closes modals and goes back
				if m.showDetails && !m.isSplitView {
					m.showDetails = false
					return m, nil
				}
				if m.focused == focusInsights {
					m.focused = focusList
					return m, nil
				}
				if m.isBoardView {
					m.isBoardView = false
					m.focused = focusList
					return m, nil
				}
				// At main list - show quit confirmation
				m.showQuitConfirm = true
				m.focused = focusQuitConfirm
				return m, nil

			case "tab":
				if m.isSplitView && !m.isBoardView {
					if m.focused == focusList {
						m.focused = focusDetail
					} else {
						m.focused = focusList
					}
				}

			case "b":
				m.isBoardView = !m.isBoardView
				if m.isBoardView {
					m.focused = focusBoard
				} else {
					m.focused = focusList
				}

			case "i":
				if m.focused == focusInsights {
					m.focused = focusList
				} else {
					m.focused = focusInsights
				}
			}

			// Focus-specific key handling
			switch m.focused {
			case focusInsights:
				m = m.handleInsightsKeys(msg)

			case focusBoard:
				m = m.handleBoardKeys(msg)

			case focusList:
				m = m.handleListKeys(msg)

			case focusDetail:
				m.viewport, cmd = m.viewport.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.isSplitView = msg.Width > SplitViewThreshold
		m.ready = true

		// Layout calculations
		headerHeight := 1
		availableHeight := msg.Height - headerHeight

		var listWidth int

		if m.isSplitView {
			listWidth = int(float64(msg.Width) * 0.4)
			detailWidth := msg.Width - listWidth - 4

			m.list.SetSize(listWidth, availableHeight)
			m.viewport = viewport.New(detailWidth, availableHeight-2)

			// Update renderer width for split view (keep existing if this fails)
			if r, err := glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(m.viewport.Width),
			); err == nil {
				m.renderer = r
			}
		} else {
			listWidth = msg.Width
			m.list.SetSize(msg.Width, availableHeight)
			m.viewport = viewport.New(msg.Width, availableHeight-2)
		}

		// Update delegate (it uses width directly now)
		m.list.SetDelegate(IssueDelegate{Theme: m.theme})

		m.insightsPanel.SetSize(m.width, m.height-headerHeight)
		m.updateViewportContent()
	}

	// Always update list (handles filtering input)
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	// Update viewport if list selection changed in split view
	if m.isSplitView && m.focused == focusList {
		m.updateViewportContent()
	}

	return m, tea.Batch(cmds...)
}

// handleBoardKeys handles keyboard input when the board is focused
func (m Model) handleBoardKeys(msg tea.KeyMsg) Model {
	switch msg.String() {
	case "h", "left":
		m.board.MoveLeft()
	case "l", "right":
		m.board.MoveRight()
	case "j", "down":
		m.board.MoveDown()
	case "k", "up":
		m.board.MoveUp()
	case "g":
		m.board.MoveToTop()
	case "G":
		m.board.MoveToBottom()
	case "ctrl+d":
		m.board.PageDown(m.height / 3)
	case "ctrl+u":
		m.board.PageUp(m.height / 3)
	case "enter":
		if selected := m.board.SelectedIssue(); selected != nil {
			// Find and select in list
			for i, item := range m.list.Items() {
				if issueItem, ok := item.(IssueItem); ok && issueItem.Issue.ID == selected.ID {
					m.list.Select(i)
					break
				}
			}
			m.isBoardView = false
			m.focused = focusList
			if m.isSplitView {
				m.focused = focusDetail
			} else {
				m.showDetails = true
			}
			m.updateViewportContent()
		}
	}
	return m
}

// handleInsightsKeys handles keyboard input when insights panel is focused
func (m Model) handleInsightsKeys(msg tea.KeyMsg) Model {
	switch msg.String() {
	case "esc":
		m.focused = focusList
	case "j", "down":
		m.insightsPanel.MoveDown()
	case "k", "up":
		m.insightsPanel.MoveUp()
	case "h", "left":
		m.insightsPanel.PrevPanel()
	case "l", "right", "tab":
		m.insightsPanel.NextPanel()
	case "e":
		// Toggle explanations
		m.insightsPanel.ToggleExplanations()
	case "x":
		// Toggle calculation details
		m.insightsPanel.ToggleCalculation()
	case "enter":
		// Jump to selected issue in list view
		selectedID := m.insightsPanel.SelectedIssueID()
		if selectedID != "" {
			for i, item := range m.list.Items() {
				if issueItem, ok := item.(IssueItem); ok && issueItem.Issue.ID == selectedID {
					m.list.Select(i)
					break
				}
			}
			m.focused = focusList
			if m.isSplitView {
				m.focused = focusDetail
			} else {
				m.showDetails = true
			}
			m.updateViewportContent()
		}
	}
	return m
}

// handleListKeys handles keyboard input when the list is focused
func (m Model) handleListKeys(msg tea.KeyMsg) Model {
	switch msg.String() {
	case "enter":
		if !m.isSplitView {
			m.showDetails = true
			m.updateViewportContent()
		}
	case "g":
		m.list.Select(0)
	case "G":
		if len(m.list.Items()) > 0 {
			m.list.Select(len(m.list.Items()) - 1)
		}
	case "ctrl+d":
		// Page down
		itemCount := len(m.list.Items())
		if itemCount > 0 {
			currentIdx := m.list.Index()
			newIdx := currentIdx + m.height/3
			if newIdx >= itemCount {
				newIdx = itemCount - 1
			}
			m.list.Select(newIdx)
		}
	case "ctrl+u":
		// Page up
		if len(m.list.Items()) > 0 {
			currentIdx := m.list.Index()
			newIdx := currentIdx - m.height/3
			if newIdx < 0 {
				newIdx = 0
			}
			m.list.Select(newIdx)
		}
	case "o":
		m.currentFilter = "open"
		m.applyFilter()
	case "c":
		m.currentFilter = "closed"
		m.applyFilter()
	case "r":
		m.currentFilter = "ready"
		m.applyFilter()
	case "a":
		m.currentFilter = "all"
		m.applyFilter()
	}
	return m
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var body string

	// Quit confirmation overlay takes highest priority
	if m.showQuitConfirm {
		body = m.renderQuitConfirm()
	} else if m.showHelp {
		body = m.renderHelpOverlay()
	} else if m.focused == focusInsights {
		body = m.insightsPanel.View()
	} else if m.isBoardView {
		body = m.board.View(m.width, m.height-1)
	} else if m.isSplitView {
		body = m.renderSplitView()
	} else {
		// Mobile view
		if m.showDetails {
			body = m.viewport.View()
		} else {
			body = m.renderListWithHeader()
		}
	}

	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (m Model) renderQuitConfirm() string {
	t := m.theme

	boxStyle := t.Renderer.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Blocked).
		Padding(1, 3).
		Align(lipgloss.Center)

	titleStyle := t.Renderer.NewStyle().
		Foreground(t.Blocked).
		Bold(true)

	textStyle := t.Renderer.NewStyle().
		Foreground(t.Base.GetForeground())

	keyStyle := t.Renderer.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	content := titleStyle.Render("Quit bv?") + "\n\n" +
		textStyle.Render("Press ") + keyStyle.Render("Esc") + textStyle.Render(" or ") + keyStyle.Render("Y") + textStyle.Render(" to quit\n") +
		textStyle.Render("Press any other key to cancel")

	box := boxStyle.Render(content)

	return lipgloss.Place(
		m.width,
		m.height-1,
		lipgloss.Center,
		lipgloss.Center,
		box,
	)
}

func (m Model) renderListWithHeader() string {
	t := m.theme

	// Calculate dimensions
	availableHeight := m.height - 3 // footer + header + border

	// Render column header
	headerStyle := t.Renderer.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#282A36"}).
		Bold(true).
		Width(m.width - 2)

	header := headerStyle.Render("  TYPE PRI STATUS      ID                                   TITLE")

	// Page info
	totalItems := len(m.list.Items())
	currentIdx := m.list.Index()
	itemsPerPage := availableHeight - 1
	if itemsPerPage < 1 {
		itemsPerPage = 1
	}
	currentPage := (currentIdx / itemsPerPage) + 1
	totalPages := (totalItems + itemsPerPage - 1) / itemsPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	startItem := (currentPage-1)*itemsPerPage + 1
	endItem := startItem + itemsPerPage - 1
	if endItem > totalItems {
		endItem = totalItems
	}

	pageInfo := fmt.Sprintf(" Page %d of %d (items %d-%d of %d) ", currentPage, totalPages, startItem, endItem, totalItems)
	pageStyle := t.Renderer.NewStyle().
		Foreground(t.Secondary).
		Align(lipgloss.Right).
		Width(m.width - 2)

	// Combine header with page info on the right
	headerLine := lipgloss.JoinHorizontal(lipgloss.Top,
		header,
	)

	// List view - just render it normally since bubbles handles scrolling
	listView := m.list.View()

	// Page indicator line
	pageLine := pageStyle.Render(pageInfo)

	return lipgloss.JoinVertical(lipgloss.Left, headerLine, listView, pageLine)
}

func (m Model) renderSplitView() string {
	t := m.theme

	var listStyle, detailStyle lipgloss.Style

	if m.focused == focusList {
		listStyle = FocusedPanelStyle
		detailStyle = PanelStyle
	} else {
		listStyle = PanelStyle
		detailStyle = FocusedPanelStyle
	}

	listWidth := m.list.Width()
	panelHeight := m.height - 2

	// Create header row for list
	headerStyle := t.Renderer.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#282A36"}).
		Bold(true).
		Width(listWidth - 2)

	header := headerStyle.Render("  TYPE PRI STATUS      ID                     TITLE")

	// Page info for list
	totalItems := len(m.list.Items())
	currentIdx := m.list.Index()
	listHeight := panelHeight - 3 // account for header and page indicator
	if listHeight < 1 {
		listHeight = 1
	}
	currentPage := (currentIdx / listHeight) + 1
	totalPages := (totalItems + listHeight - 1) / listHeight
	if totalPages < 1 {
		totalPages = 1
	}
	startItem := (currentPage-1)*listHeight + 1
	endItem := startItem + listHeight - 1
	if endItem > totalItems {
		endItem = totalItems
	}
	if totalItems == 0 {
		startItem = 0
		endItem = 0
	}

	pageInfo := fmt.Sprintf("Page %d/%d (%d-%d of %d)", currentPage, totalPages, startItem, endItem, totalItems)
	pageStyle := t.Renderer.NewStyle().
		Foreground(t.Secondary).
		Width(listWidth - 2).
		Align(lipgloss.Center)

	pageLine := pageStyle.Render(pageInfo)

	// Combine header + list + page indicator
	listContent := lipgloss.JoinVertical(lipgloss.Left, header, m.list.View(), pageLine)
	listView := listStyle.Width(listWidth).Height(panelHeight).Render(listContent)

	detailView := detailStyle.Width(m.viewport.Width + 2).Height(panelHeight).Render(m.viewport.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, listView, detailView)
}

func (m Model) renderHelpOverlay() string {
	t := m.theme

	titleStyle := t.Renderer.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		MarginBottom(1)

	sectionStyle := t.Renderer.NewStyle().
		Foreground(t.Secondary).
		Bold(true).
		MarginTop(1)

	keyStyle := t.Renderer.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#BD93F9"}).
		Bold(true).
		Width(12)

	descStyle := t.Renderer.NewStyle().
		Foreground(t.Base.GetForeground())

	var sb strings.Builder

	sb.WriteString(titleStyle.Render("⌨️  Keyboard Shortcuts"))
	sb.WriteString("\n\n")

	// Navigation
	sb.WriteString(sectionStyle.Render("Navigation"))
	sb.WriteString("\n")
	shortcuts := []struct{ key, desc string }{
		{"j / ↓", "Move down"},
		{"k / ↑", "Move up"},
		{"g", "Go to first item"},
		{"G", "Go to last item"},
		{"Ctrl+d", "Page down"},
		{"Ctrl+u", "Page up"},
		{"Tab", "Switch focus (split view)"},
		{"Enter", "View details"},
		{"Esc", "Back / close"},
	}
	for _, s := range shortcuts {
		sb.WriteString(keyStyle.Render(s.key) + descStyle.Render(s.desc) + "\n")
	}

	// Views
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("Views"))
	sb.WriteString("\n")
	views := []struct{ key, desc string }{
		{"b", "Toggle Kanban board"},
		{"i", "Toggle Insights dashboard"},
		{"?", "Toggle this help"},
	}
	for _, s := range views {
		sb.WriteString(keyStyle.Render(s.key) + descStyle.Render(s.desc) + "\n")
	}

	// Insights (when in insights view)
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("Insights Panel"))
	sb.WriteString("\n")
	insightsKeys := []struct{ key, desc string }{
		{"h/l/Tab", "Switch metric panels"},
		{"j/k", "Navigate items"},
		{"e", "Toggle explanations"},
		{"x", "Toggle calculation details"},
		{"Enter", "Jump to issue"},
	}
	for _, s := range insightsKeys {
		sb.WriteString(keyStyle.Render(s.key) + descStyle.Render(s.desc) + "\n")
	}

	// Filters
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("Filters"))
	sb.WriteString("\n")
	filters := []struct{ key, desc string }{
		{"o", "Show Open issues"},
		{"c", "Show Closed issues"},
		{"r", "Show Ready (unblocked)"},
		{"a", "Show All issues"},
		{"/", "Fuzzy search"},
	}
	for _, s := range filters {
		sb.WriteString(keyStyle.Render(s.key) + descStyle.Render(s.desc) + "\n")
	}

	// General
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("General"))
	sb.WriteString("\n")
	general := []struct{ key, desc string }{
		{"q", "Back / Quit"},
		{"Ctrl+c", "Force quit"},
	}
	for _, s := range general {
		sb.WriteString(keyStyle.Render(s.key) + descStyle.Render(s.desc) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(t.Renderer.NewStyle().Foreground(t.Secondary).Italic(true).Render("Press any key to close"))

	// Center the help content
	helpContent := sb.String()
	helpBox := t.Renderer.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary).
		Padding(1, 3).
		Render(helpContent)

	// Center in viewport
	return lipgloss.Place(
		m.width,
		m.height-1,
		lipgloss.Center,
		lipgloss.Center,
		helpBox,
	)
}

func (m *Model) renderFooter() string {
	filterStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	helpStyle := lipgloss.NewStyle().Foreground(ColorSubtext)
	countStyle := lipgloss.NewStyle().Foreground(ColorSecondary).Padding(0, 1)

	var filterTxt string
	switch m.currentFilter {
	case "all":
		filterTxt = "ALL"
	case "open":
		filterTxt = "OPEN"
	case "closed":
		filterTxt = "CLOSED"
	case "ready":
		filterTxt = "READY"
	}

	status := fmt.Sprintf(" Filter: %s ", filterTxt)
	count := fmt.Sprintf("%d issues", len(m.list.Items()))

	// Stats block
	stats := fmt.Sprintf(" Open:%d Ready:%d Blocked:%d Closed:%d ", m.countOpen, m.countReady, m.countBlocked, m.countClosed)

	// Update block
	updateTxt := ""
	if m.updateAvailable {
		updateTxt = fmt.Sprintf(" ⭐ %s available ", m.updateTag)
	}

	var keys string
	if m.showHelp {
		keys = "Press any key to close help"
	} else if m.focused == focusInsights {
		keys = "h/l: panels • j/k: items • e: explain • x: calc • enter: jump • ?: help"
	} else if m.isBoardView {
		keys = "h/j/k/l: nav • g/G: top/bottom • enter: view • b: list • ?: help"
	} else if m.list.FilterState() == list.Filtering {
		keys = "esc: cancel • enter: select"
	} else {
		if m.isSplitView {
			keys = "tab: focus • b: board • i: insights • o/r/c/a: filter • /: search • ?: help"
		} else {
			if m.showDetails {
				keys = "esc: back • j/k: scroll • ?: help • q: back"
			} else {
				keys = "enter: details • b: board • i: insights • /: search • ?: help • q: quit"
			}
		}
	}

	statusSection := filterStyle.Background(ColorPrimary).Padding(0, 1).Render(status)
	updateSection := lipgloss.NewStyle().Background(ColorTypeFeature).Foreground(ColorBg).Bold(true).Render(updateTxt)
	statsSection := lipgloss.NewStyle().Background(ColorBgHighlight).Foreground(ColorText).Render(stats)
	countSection := countStyle.Render(count)
	keysSection := helpStyle.Padding(0, 1).Render(keys)

	// Calculate filler width
	leftWidth := lipgloss.Width(statusSection) + lipgloss.Width(updateSection) + lipgloss.Width(statsSection)
	rightWidth := lipgloss.Width(countSection) + lipgloss.Width(keysSection)

	remaining := m.width - leftWidth - rightWidth
	if remaining < 0 {
		remaining = 0
	}
	filler := lipgloss.NewStyle().Background(ColorBgDark).Width(remaining).Render("")

	return lipgloss.JoinHorizontal(lipgloss.Bottom, statusSection, updateSection, statsSection, filler, countSection, keysSection)
}

func (m *Model) applyFilter() {
	var filteredItems []list.Item
	var filteredIssues []model.Issue

	for _, issue := range m.issues {
		include := false
		switch m.currentFilter {
		case "all":
			include = true
		case "open":
			include = issue.Status != model.StatusClosed
		case "closed":
			include = issue.Status == model.StatusClosed
		case "ready":
			// Ready = Open/InProgress AND No Open Blockers
			if issue.Status != model.StatusClosed && issue.Status != model.StatusBlocked {
				isBlocked := false
				for _, dep := range issue.Dependencies {
					if dep.Type == model.DepBlocks {
						if blocker, exists := m.issueMap[dep.DependsOnID]; exists && blocker.Status != model.StatusClosed {
							isBlocked = true
							break
						}
					}
				}
				include = !isBlocked
			}
		}

		if include {
			// Use pre-computed graph scores (avoid redundant calculation)
			filteredItems = append(filteredItems, IssueItem{
				Issue:      issue,
				GraphScore: m.analysis.PageRank[issue.ID],
				Impact:     m.analysis.CriticalPathScore[issue.ID],
			})
			filteredIssues = append(filteredIssues, issue)
		}
	}

	m.list.SetItems(filteredItems)
	m.board.SetIssues(filteredIssues)

	// Keep selection in bounds
	if len(filteredItems) > 0 && m.list.Index() >= len(filteredItems) {
		m.list.Select(0)
	}
	m.updateViewportContent()
}

func (m *Model) updateViewportContent() {
	selectedItem := m.list.SelectedItem()
	if selectedItem == nil {
		m.viewport.SetContent("No issues selected")
		return
	}

	// Safe type assertion
	issueItem, ok := selectedItem.(IssueItem)
	if !ok {
		m.viewport.SetContent("Error: invalid item type")
		return
	}
	item := issueItem.Issue

	var sb strings.Builder

	if m.updateAvailable {
		sb.WriteString(fmt.Sprintf("⭐ **Update Available:** [%s](%s)\n\n", m.updateTag, m.updateURL))
	}

	// Title Block
	sb.WriteString(fmt.Sprintf("# %s %s\n", GetTypeIconMD(string(item.IssueType)), item.Title))

	// Meta Table
	sb.WriteString("| ID | Status | Priority | Assignee | Created |\n|---|---|---|---|---|\n")
	sb.WriteString(fmt.Sprintf("| **%s** | **%s** | %s | @%s | %s |\n\n",
		item.ID,
		strings.ToUpper(string(item.Status)),
		GetPriorityIcon(item.Priority),
		item.Assignee,
		item.CreatedAt.Format("2006-01-02"),
	))

	// Graph Analysis
	pr := m.analysis.PageRank[item.ID]
	bt := m.analysis.Betweenness[item.ID]
	imp := m.analysis.CriticalPathScore[item.ID]
	ev := m.analysis.Eigenvector[item.ID]
	hub := m.analysis.Hubs[item.ID]
	auth := m.analysis.Authorities[item.ID]

	sb.WriteString("### Graph Analysis\n")
	sb.WriteString(fmt.Sprintf("- **Impact Depth**: %.0f (downstream chain length)\n", imp))
	sb.WriteString(fmt.Sprintf("- **Centrality**: PR %.4f • BW %.4f • EV %.4f\n", pr, bt, ev))
	sb.WriteString(fmt.Sprintf("- **Flow Role**: Hub %.4f • Authority %.4f\n\n", hub, auth))

	// Description
	if item.Description != "" {
		sb.WriteString("### Description\n")
		sb.WriteString(item.Description + "\n\n")
	}

	// Acceptance Criteria
	if item.AcceptanceCriteria != "" {
		sb.WriteString("### Acceptance Criteria\n")
		sb.WriteString(item.AcceptanceCriteria + "\n\n")
	}

	// Notes
	if item.Notes != "" {
		sb.WriteString("### Notes\n")
		sb.WriteString(item.Notes + "\n\n")
	}

	// Dependency Graph (Tree)
	if len(item.Dependencies) > 0 {
		rootNode := BuildDependencyTree(item.ID, m.issueMap, 3) // Max depth 3
		treeStr := RenderDependencyTree(rootNode)
		sb.WriteString("```\n" + treeStr + "```\n\n")
	}

	// Comments
	if len(item.Comments) > 0 {
		sb.WriteString(fmt.Sprintf("### Comments (%d)\n", len(item.Comments)))
		for _, comment := range item.Comments {
			sb.WriteString(fmt.Sprintf("> **%s** (%s)\n> \n> %s\n\n",
				comment.Author,
				FormatTimeRel(comment.CreatedAt),
				strings.ReplaceAll(comment.Text, "\n", "\n> ")))
		}
	}

	rendered, err := m.renderer.Render(sb.String())
	if err != nil {
		m.viewport.SetContent(fmt.Sprintf("Error rendering markdown: %v", err))
	} else {
		m.viewport.SetContent(rendered)
	}
}

// GetTypeIconMD returns the emoji icon for an issue type (for markdown)
func GetTypeIconMD(t string) string {
	switch t {
	case "bug":
		return "🐛"
	case "feature":
		return "✨"
	case "task":
		return "📋"
	case "epic":
		return "🏔️"
	case "chore":
		return "🧹"
	default:
		return "•"
	}
}

// SetFilter sets the current filter and applies it (exposed for testing)
func (m *Model) SetFilter(f string) {
	m.currentFilter = f
	m.applyFilter()
}

// FilteredIssues returns the currently visible issues (exposed for testing)
func (m Model) FilteredIssues() []model.Issue {
	items := m.list.Items()
	issues := make([]model.Issue, 0, len(items))
	for _, item := range items {
		if issueItem, ok := item.(IssueItem); ok {
			issues = append(issues, issueItem.Issue)
		}
	}
	return issues
}
