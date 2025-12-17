package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ══════════════════════════════════════════════════════════════════════════════
// DESIGN TOKENS - Consistent spacing, colors, and visual language
// ══════════════════════════════════════════════════════════════════════════════

// Spacing constants for consistent layout (in characters)
const (
	SpaceXS = 1
	SpaceSM = 2
	SpaceMD = 3
	SpaceLG = 4
	SpaceXL = 6
)

// ══════════════════════════════════════════════════════════════════════════════
// COLOR PALETTE - Dracula-inspired with extended semantic colors
// ══════════════════════════════════════════════════════════════════════════════

var (
	// Base colors
	ColorBg          = lipgloss.Color("#282A36")
	ColorBgDark      = lipgloss.Color("#1E1F29")
	ColorBgSubtle    = lipgloss.Color("#363949")
	ColorBgHighlight = lipgloss.Color("#44475A")
	ColorText        = lipgloss.Color("#F8F8F2")
	ColorSubtext     = lipgloss.Color("#BFBFBF")
	ColorMuted       = lipgloss.Color("#6272A4")

	// Primary accent colors
	ColorPrimary   = lipgloss.Color("#BD93F9")
	ColorSecondary = lipgloss.Color("#6272A4")
	ColorInfo      = lipgloss.Color("#8BE9FD")
	ColorSuccess   = lipgloss.Color("#50FA7B")
	ColorWarning   = lipgloss.Color("#FFB86C")
	ColorDanger    = lipgloss.Color("#FF5555")

	// Status colors
	ColorStatusOpen       = lipgloss.Color("#50FA7B")
	ColorStatusInProgress = lipgloss.Color("#8BE9FD")
	ColorStatusBlocked    = lipgloss.Color("#FF5555")
	ColorStatusClosed     = lipgloss.Color("#6272A4")

	// Status background colors (for badges)
	ColorStatusOpenBg       = lipgloss.Color("#1A3D2A")
	ColorStatusInProgressBg = lipgloss.Color("#1A3344")
	ColorStatusBlockedBg    = lipgloss.Color("#3D1A1A")
	ColorStatusClosedBg     = lipgloss.Color("#2A2A3D")

	// Priority colors
	ColorPrioCritical = lipgloss.Color("#FF5555")
	ColorPrioHigh     = lipgloss.Color("#FFB86C")
	ColorPrioMedium   = lipgloss.Color("#F1FA8C")
	ColorPrioLow      = lipgloss.Color("#50FA7B")

	// Priority background colors
	ColorPrioCriticalBg = lipgloss.Color("#3D1A1A")
	ColorPrioHighBg     = lipgloss.Color("#3D2A1A")
	ColorPrioMediumBg   = lipgloss.Color("#3D3D1A")
	ColorPrioLowBg      = lipgloss.Color("#1A3D2A")

	// Type colors
	ColorTypeBug     = lipgloss.Color("#FF5555")
	ColorTypeFeature = lipgloss.Color("#FFB86C")
	ColorTypeTask    = lipgloss.Color("#F1FA8C")
	ColorTypeEpic    = lipgloss.Color("#BD93F9")
	ColorTypeChore   = lipgloss.Color("#8BE9FD")
)

// ══════════════════════════════════════════════════════════════════════════════
// PANEL STYLES - For split view layouts
// ══════════════════════════════════════════════════════════════════════════════

var (
	// PanelStyle is the default style for unfocused panels
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#44475A"))

	// FocusedPanelStyle is the style for focused panels
	FocusedPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#BD93F9"))
)

// ══════════════════════════════════════════════════════════════════════════════
// BADGE RENDERING - Polished, consistent badge styles
// ══════════════════════════════════════════════════════════════════════════════

// RenderPriorityBadge returns a styled priority badge
// Priority values: 0=Critical, 1=High, 2=Medium, 3=Low, 4=Backlog
func RenderPriorityBadge(priority int) string {
	var fg, bg lipgloss.Color
	var label string

	switch priority {
	case 0:
		fg, bg, label = ColorPrioCritical, ColorPrioCriticalBg, "P0"
	case 1:
		fg, bg, label = ColorPrioHigh, ColorPrioHighBg, "P1"
	case 2:
		fg, bg, label = ColorPrioMedium, ColorPrioMediumBg, "P2"
	case 3:
		fg, bg, label = ColorPrioLow, ColorPrioLowBg, "P3"
	case 4:
		fg, bg, label = ColorMuted, ColorBgSubtle, "P4"
	default:
		fg, bg, label = ColorMuted, ColorBgSubtle, "P?"
	}

	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Bold(true).
		Padding(0, 0).
		Render(label)
}

// RenderStatusBadge returns a styled status badge
func RenderStatusBadge(status string) string {
	var fg, bg lipgloss.Color
	var label string

	switch status {
	case "open":
		fg, bg, label = ColorStatusOpen, ColorStatusOpenBg, "OPEN"
	case "in_progress":
		fg, bg, label = ColorStatusInProgress, ColorStatusInProgressBg, "PROG"
	case "blocked":
		fg, bg, label = ColorStatusBlocked, ColorStatusBlockedBg, "BLKD"
	case "closed":
		fg, bg, label = ColorStatusClosed, ColorStatusClosedBg, "DONE"
	default:
		fg, bg, label = ColorMuted, ColorBgSubtle, "????"
	}

	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Padding(0, 0).
		Render(label)
}

// ══════════════════════════════════════════════════════════════════════════════
// METRIC VISUALIZATION - Mini-bars and rank badges
// ══════════════════════════════════════════════════════════════════════════════

// RenderMiniBar renders a mini horizontal bar for a value between 0 and 1
func RenderMiniBar(value float64, width int) string {
	if width <= 0 {
		return ""
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}

	filled := int(value * float64(width))
	if filled > width {
		filled = width
	}

	// Choose color based on value
	var barColor lipgloss.Color
	if value >= 0.75 {
		barColor = ColorSuccess
	} else if value >= 0.5 {
		barColor = ColorWarning
	} else if value >= 0.25 {
		barColor = ColorInfo
	} else {
		barColor = ColorMuted
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return lipgloss.NewStyle().Foreground(barColor).Render(bar)
}

// RenderRankBadge renders a rank badge like "#1" with color based on percentile
func RenderRankBadge(rank, total int) string {
	if total == 0 {
		return lipgloss.NewStyle().Foreground(ColorMuted).Render("#?")
	}

	percentile := float64(rank) / float64(total)

	var color lipgloss.Color
	if percentile <= 0.1 {
		color = ColorSuccess // Top 10%
	} else if percentile <= 0.25 {
		color = ColorInfo // Top 25%
	} else if percentile <= 0.5 {
		color = ColorWarning // Top 50%
	} else {
		color = ColorMuted // Bottom 50%
	}

	return lipgloss.NewStyle().
		Foreground(color).
		Render(fmt.Sprintf("#%d", rank))
}

// ══════════════════════════════════════════════════════════════════════════════
// DIVIDERS AND SEPARATORS
// ══════════════════════════════════════════════════════════════════════════════

// RenderDivider renders a horizontal divider line
func RenderDivider(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(ColorBgHighlight).
		Render(strings.Repeat("─", width))
}

// RenderSubtleDivider renders a more subtle divider using dots
func RenderSubtleDivider(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(strings.Repeat("·", width))
}
