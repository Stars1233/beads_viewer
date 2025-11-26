package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tier represents the width tier of the display
type Tier int

const (
	TierCompact Tier = iota
	TierNormal
	TierWide
	TierUltraWide
)

type IssueDelegate struct {
	Tier Tier
}

func (d IssueDelegate) Height() int {
	return 1
}

func (d IssueDelegate) Spacing() int {
	return 0
}

func (d IssueDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d IssueDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(IssueItem)
	if !ok {
		return
	}

	// Styles
	var baseStyle lipgloss.Style
	if index == m.Index() {
		baseStyle = SelectedItemStyle
	} else {
		baseStyle = ItemStyle
	}

	// Base Columns (Compact)
	id := ColIDStyle.Render(i.Issue.ID)
	
	iconStr, iconColor := GetTypeIcon(string(i.Issue.IssueType))
	typeIcon := ColTypeStyle.Foreground(iconColor).Render(iconStr)
	
	prio := ColPrioStyle.Render(GetPriorityIcon(i.Issue.Priority))
	
	statusColor := GetStatusColor(string(i.Issue.Status))
	status := ColStatusStyle.Foreground(statusColor).Render(strings.ToUpper(string(i.Issue.Status)))

	// Optional Columns
	age := ""
	comments := ""
	updated := ""
	assignee := ""
	
	extraWidth := 0

	// Assignee (Normal+)
	if d.Tier >= TierNormal {
		if i.Issue.Assignee != "" {
			assignee = ColAssigneeStyle.Render("@" + i.Issue.Assignee)
			extraWidth += 12
		} else {
			// Even empty, we might want to reserve space or just let it collapse?
			// Let's collapse for cleaner look, but that makes alignment jagged. 
			// Better to fix width.
			assignee = ColAssigneeStyle.Render("")
			extraWidth += 12
		}
	}

	// Age & Comments (Wide+)
	if d.Tier >= TierWide {
		ageStr := FormatTimeRel(i.Issue.CreatedAt)
		age = ColAgeStyle.Render(ageStr)
		
		commentCount := len(i.Issue.Comments)
		if commentCount > 0 {
			comments = ColCommentsStyle.Render(fmt.Sprintf("💬%d", commentCount))
		} else {
			comments = ColCommentsStyle.Render("")
		}
		extraWidth += 8 + 4 // Age + Comments
	}

	// Updated (UltraWide)
	if d.Tier >= TierUltraWide {
		updatedStr := FormatTimeRel(i.Issue.UpdatedAt)
		updated = ColAgeStyle.Copy().Width(10).Render(updatedStr)
		
		// Impact Score
		impactVal := int(i.Impact)
		impactStr := ""
		if impactVal > 5 {
			impactStr = fmt.Sprintf("🌋%d", impactVal)
		} else if impactVal > 2 {
			impactStr = fmt.Sprintf("🏔️%d", impactVal)
		} else if impactVal > 0 {
			impactStr = fmt.Sprintf("⛰️%d", impactVal)
		}
		updated = lipgloss.JoinHorizontal(lipgloss.Left, updated, lipgloss.NewStyle().Width(6).Align(lipgloss.Right).Render(impactStr))
		
		extraWidth += 16 // 10 (Updated) + 6 (Impact)
	}

	// Calculate Title Width
	// Fixed widths: ID(8) + Type(2) + Prio(3) + Status(12) + Extra + Spacing
	// Spacing depends on number of active columns. 
	// Base gaps: ID-Type(1) Type-Prio(0) Prio-Status(0) Status-Title(1) = 2
	// Assignee(1) Comments(1) Age(1) Updated(1)
	
	gaps := 4 
	if d.Tier >= TierNormal { gaps += 1 }
	if d.Tier >= TierWide { gaps += 2 }
	if d.Tier >= TierUltraWide { gaps += 1 }

	fixedWidth := 8 + 2 + 3 + 12 + extraWidth + gaps
	availableWidth := m.Width() - fixedWidth - 4 // -4 for padding
	
	if availableWidth < 10 {
		availableWidth = 10
	}

	titleStyle := ColTitleStyle.Copy().Width(availableWidth).MaxWidth(availableWidth)
	if index == m.Index() {
		titleStyle = titleStyle.Foreground(ColorPrimary).Bold(true)
	}
	
	titleStr := i.Issue.Title
	title := titleStyle.Render(titleStr)

	// Compose Row based on Tier
	var row string
	
	// Base: ID | Type | Prio | Status | Title
	parts := []string{id, typeIcon, prio, status, title}
	
	if d.Tier >= TierWide {
		parts = append(parts, comments, age)
	}
	
	if d.Tier >= TierNormal {
		parts = append(parts, assignee)
	}
	
	if d.Tier >= TierUltraWide {
		parts = append(parts, updated)
	}

	row = lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	fmt.Fprint(w, baseStyle.Render(row))
}