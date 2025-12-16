package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Gradients
	GradientLow  = lipgloss.Color("#44475A")
	GradientMid  = lipgloss.Color("#6272A4")
	GradientHigh = lipgloss.Color("#BD93F9")
	GradientPeak = lipgloss.Color("#FF79C6")
)

// RenderSparkline creates a textual bar chart of value (0.0 - 1.0)
func RenderSparkline(val float64, width int) string {
	if width <= 0 {
		return ""
	}

	chars := []string{" ", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	// If width > 1, we can show history? Or just a bar?
	// Let's render a horizontal bar: `███▌    `

	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}

	// Calculate fullness
	fullChars := int(val * float64(width))
	remainder := (val * float64(width)) - float64(fullChars)

	var sb strings.Builder
	for i := 0; i < fullChars; i++ {
		sb.WriteString("█")
	}

	if fullChars < width {
		idx := int(remainder * float64(len(chars)))
		// Ensure non-zero values are visible
		if idx == 0 && remainder > 0 {
			idx = 1
		}
		if idx >= len(chars) {
			idx = len(chars) - 1
		}
		if idx > 0 {
			sb.WriteString(chars[idx])
		} else {
			sb.WriteString(" ")
		}
	}

	// Pad
	padding := width - fullChars - 1
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}

	return sb.String()
}

// GetHeatmapColor returns a color based on score (0-1)
func GetHeatmapColor(score float64) lipgloss.Color {
	if score > 0.8 {
		return GradientPeak
	} else if score > 0.5 {
		return GradientHigh
	} else if score > 0.2 {
		return GradientMid
	}
	return GradientLow
}

// RepoColors maps repo prefixes to distinctive colors for visual differentiation
var RepoColors = []lipgloss.Color{
	lipgloss.Color("#FF6B6B"), // Coral red
	lipgloss.Color("#4ECDC4"), // Teal
	lipgloss.Color("#45B7D1"), // Sky blue
	lipgloss.Color("#96CEB4"), // Sage green
	lipgloss.Color("#DDA0DD"), // Plum
	lipgloss.Color("#F7DC6F"), // Gold
	lipgloss.Color("#BB8FCE"), // Lavender
	lipgloss.Color("#85C1E9"), // Light blue
}

// GetRepoColor returns a consistent color for a repo prefix based on hash
func GetRepoColor(prefix string) lipgloss.Color {
	if prefix == "" {
		return ColorMuted
	}
	// Simple hash based on prefix characters
	hash := 0
	for _, c := range prefix {
		hash = (hash*31 + int(c)) % len(RepoColors)
	}
	if hash < 0 {
		hash = -hash
	}
	return RepoColors[hash%len(RepoColors)]
}

// RenderRepoBadge creates a compact colored badge for a repository prefix
// Example: "api" -> "[API]" with distinctive color
func RenderRepoBadge(prefix string) string {
	if prefix == "" {
		return ""
	}
	// Uppercase and limit to 4 chars for compactness
	display := strings.ToUpper(prefix)
	if len(display) > 4 {
		display = display[:4]
	}

	color := GetRepoColor(prefix)
	return lipgloss.NewStyle().
		Foreground(color).
		Bold(true).
		Render("[" + display + "]")
}
