package export

import (
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// sanitizeMermaidID ensures an ID is valid for Mermaid diagrams.
// Mermaid node IDs must be alphanumeric with hyphens/underscores.
func sanitizeMermaidID(id string) string {
	var sb strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	result := sb.String()
	if result == "" {
		return "node"
	}
	return result
}

// sanitizeMermaidText prepares text for use in Mermaid node labels.
// Removes/escapes characters that break Mermaid syntax.
func sanitizeMermaidText(text string) string {
	// Remove or replace problematic characters
	replacer := strings.NewReplacer(
		"\"", "'",
		"[", "(",
		"]", ")",
		"{", "(",
		"}", ")",
		"<", "&lt;",
		">", "&gt;",
		"|", "/",
		"`", "'",
		"\n", " ",
		"\r", "",
	)
	result := replacer.Replace(text)

	// Remove any remaining control characters
	result = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, result)

	result = strings.TrimSpace(result)

	// Truncate if too long (UTF-8 safe using runes)
	runes := []rune(result)
	if len(runes) > 40 {
		result = string(runes[:37]) + "..."
	}

	return result
}

// GenerateMarkdown creates a comprehensive markdown report of all issues
func GenerateMarkdown(issues []model.Issue, title string) (string, error) {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n", time.Now().Format(time.RFC1123)))

	// Summary Statistics
	sb.WriteString("## Summary\n\n")

	open, inProgress, blocked, closed := 0, 0, 0, 0
	for _, i := range issues {
		switch i.Status {
		case model.StatusOpen:
			open++
		case model.StatusInProgress:
			inProgress++
		case model.StatusBlocked:
			blocked++
		case model.StatusClosed:
			closed++
		}
	}

	sb.WriteString("| Metric | Count |\n|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| **Total** | %d |\n", len(issues)))
	sb.WriteString(fmt.Sprintf("| Open | %d |\n", open))
	sb.WriteString(fmt.Sprintf("| In Progress | %d |\n", inProgress))
	sb.WriteString(fmt.Sprintf("| Blocked | %d |\n", blocked))
	sb.WriteString(fmt.Sprintf("| Closed | %d |\n\n", closed))

	// Quick Actions Section
	sb.WriteString(generateQuickActions(issues))

	// Table of Contents
	sb.WriteString("## Table of Contents\n\n")
	for _, i := range issues {
		// Create a slug for the anchor (lowercase, hyphens for spaces)
		slug := createSlug(i.ID)
		statusIcon := getStatusEmoji(string(i.Status))
		sb.WriteString(fmt.Sprintf("- [%s %s %s](#%s)\n", statusIcon, i.ID, i.Title, slug))
	}
	sb.WriteString("\n---\n\n")

	// Dependency Graph (Mermaid)
	sb.WriteString("## Dependency Graph\n\n")
	sb.WriteString("```mermaid\ngraph TD\n")

	// Style definitions
	sb.WriteString("    classDef open fill:#50FA7B,stroke:#333,color:#000\n")
	sb.WriteString("    classDef inprogress fill:#8BE9FD,stroke:#333,color:#000\n")
	sb.WriteString("    classDef blocked fill:#FF5555,stroke:#333,color:#000\n")
	sb.WriteString("    classDef closed fill:#6272A4,stroke:#333,color:#fff\n")
	sb.WriteString("\n")

	hasLinks := false
	issueIDs := make(map[string]bool)
	for _, i := range issues {
		issueIDs[i.ID] = true
	}

	// Build deterministic, collision-free Mermaid IDs
	safeIDMap := make(map[string]string)
	usedSafe := make(map[string]bool)
	getSafeID := func(orig string) string {
		if safe, ok := safeIDMap[orig]; ok {
			return safe
		}
		base := sanitizeMermaidID(orig)
		if base == "" {
			base = "node"
		}
		safe := base
		if usedSafe[safe] && safeIDMap[orig] == "" {
			// Collision: derive stable hash-based suffix
			h := fnv.New32a()
			_, _ = h.Write([]byte(orig))
			safe = fmt.Sprintf("%s_%x", base, h.Sum32())
		}
		usedSafe[safe] = true
		safeIDMap[orig] = safe
		return safe
	}

	for _, i := range issues {
		safeID := getSafeID(i.ID)
		safeTitle := sanitizeMermaidText(i.Title)
		// Also sanitize the ID for the label in case it contains quotes or special chars
		safeLabelID := sanitizeMermaidText(i.ID)

		// Node definition with status-based styling
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s\"]\n", safeID, safeLabelID, safeTitle))

		// Apply class based on status
		var class string
		switch i.Status {
		case model.StatusOpen:
			class = "open"
		case model.StatusInProgress:
			class = "inprogress"
		case model.StatusBlocked:
			class = "blocked"
		case model.StatusClosed:
			class = "closed"
		}
		sb.WriteString(fmt.Sprintf("    class %s %s\n", safeID, class))

		// Add edges for dependencies
		for _, dep := range i.Dependencies {
			if dep == nil {
				continue
			}
			// Only add edges to issues that exist in our set
			if !issueIDs[dep.DependsOnID] {
				continue
			}

			safeDepID := getSafeID(dep.DependsOnID)
			linkStyle := "-.->" // Dashed for related
			if dep.Type == model.DepBlocks {
				linkStyle = "==>" // Bold for blockers
			}
			sb.WriteString(fmt.Sprintf("    %s %s %s\n", safeID, linkStyle, safeDepID))
			hasLinks = true
		}
	}

	if !hasLinks && len(issues) > 0 {
		sb.WriteString("    NoLinks[\"No Dependencies\"]\n")
	}
	sb.WriteString("```\n\n")
	sb.WriteString("---\n\n")

	// Individual Issues
	for _, i := range issues {
		typeIcon := getTypeEmoji(string(i.IssueType))
		sb.WriteString(fmt.Sprintf("## %s %s %s\n\n", typeIcon, i.ID, i.Title))

		// Metadata Table
		sb.WriteString("| Property | Value |\n|----------|-------|\n")
		sb.WriteString(fmt.Sprintf("| **Type** | %s %s |\n", typeIcon, i.IssueType))
		sb.WriteString(fmt.Sprintf("| **Priority** | %s |\n", getPriorityLabel(i.Priority)))
		sb.WriteString(fmt.Sprintf("| **Status** | %s %s |\n", getStatusEmoji(string(i.Status)), i.Status))
		if i.Assignee != "" {
			escapedAssignee := strings.ReplaceAll(i.Assignee, "|", "\\|")
			sb.WriteString(fmt.Sprintf("| **Assignee** | @%s |\n", escapedAssignee))
		}
		sb.WriteString(fmt.Sprintf("| **Created** | %s |\n", i.CreatedAt.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("| **Updated** | %s |\n", i.UpdatedAt.Format("2006-01-02 15:04")))
		if i.ClosedAt != nil {
			sb.WriteString(fmt.Sprintf("| **Closed** | %s |\n", i.ClosedAt.Format("2006-01-02 15:04")))
		}
		if len(i.Labels) > 0 {
			// Escape pipe characters in labels to avoid breaking markdown table
			escapedLabels := make([]string, len(i.Labels))
			for idx, label := range i.Labels {
				escapedLabels[idx] = strings.ReplaceAll(label, "|", "\\|")
			}
			sb.WriteString(fmt.Sprintf("| **Labels** | %s |\n", strings.Join(escapedLabels, ", ")))
		}
		sb.WriteString("\n")

		if i.Description != "" {
			sb.WriteString("### Description\n\n")
			sb.WriteString(i.Description + "\n\n")
		}

		if i.AcceptanceCriteria != "" {
			sb.WriteString("### Acceptance Criteria\n\n")
			sb.WriteString(i.AcceptanceCriteria + "\n\n")
		}

		if i.Design != "" {
			sb.WriteString("### Design\n\n")
			sb.WriteString(i.Design + "\n\n")
		}

		if i.Notes != "" {
			sb.WriteString("### Notes\n\n")
			sb.WriteString(i.Notes + "\n\n")
		}

		if len(i.Dependencies) > 0 {
			sb.WriteString("### Dependencies\n\n")
			for _, dep := range i.Dependencies {
				if dep == nil {
					continue
				}
				icon := "🔗"
				if dep.Type == model.DepBlocks {
					icon = "⛔"
				}
				sb.WriteString(fmt.Sprintf("- %s **%s**: `%s`\n", icon, dep.Type, dep.DependsOnID))
			}
			sb.WriteString("\n")
		}

		if len(i.Comments) > 0 {
			sb.WriteString("### Comments\n\n")
			for _, c := range i.Comments {
				if c == nil {
					continue
				}
				escapedText := strings.ReplaceAll(c.Text, "\n", "\n> ")
				sb.WriteString(fmt.Sprintf("> **%s** (%s)\n>\n> %s\n\n",
					c.Author, c.CreatedAt.Format("2006-01-02"), escapedText))
			}
		}

		// Per-issue command snippets
		sb.WriteString(generateIssueCommands(i))

		sb.WriteString("---\n\n")
	}

	return sb.String(), nil
}

// createSlug creates a URL-friendly slug from an ID
func createSlug(id string) string {
	// Convert to lowercase and replace non-alphanumeric with hyphens
	slug := strings.ToLower(id)
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func getStatusEmoji(status string) string {
	switch status {
	case "open":
		return "🟢"
	case "in_progress":
		return "🔵"
	case "blocked":
		return "🔴"
	case "closed":
		return "⚫"
	default:
		return "⚪"
	}
}

func getTypeEmoji(issueType string) string {
	switch issueType {
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

func getPriorityLabel(priority int) string {
	switch priority {
	case 0:
		return "🔥 Critical (P0)"
	case 1:
		return "⚡ High (P1)"
	case 2:
		return "🔹 Medium (P2)"
	case 3:
		return "☕ Low (P3)"
	case 4:
		return "💤 Backlog (P4)"
	default:
		return fmt.Sprintf("P%d", priority)
	}
}

// SaveMarkdownToFile writes the generated markdown to a file
func SaveMarkdownToFile(issues []model.Issue, filename string) error {
	// Make a copy to avoid mutating the caller's slice
	issuesCopy := make([]model.Issue, len(issues))
	copy(issuesCopy, issues)

	// Sort issues for the report: Open first, then priority, then date
	sort.Slice(issuesCopy, func(i, j int) bool {
		iClosed := issuesCopy[i].Status == model.StatusClosed
		jClosed := issuesCopy[j].Status == model.StatusClosed
		if iClosed != jClosed {
			return !iClosed
		}
		if issuesCopy[i].Priority != issuesCopy[j].Priority {
			return issuesCopy[i].Priority < issuesCopy[j].Priority
		}
		return issuesCopy[i].CreatedAt.After(issuesCopy[j].CreatedAt)
	})

	content, err := GenerateMarkdown(issuesCopy, "Beads Export")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, []byte(content), 0644)
}

// generateQuickActions creates a Quick Actions section with bulk commands
func generateQuickActions(issues []model.Issue) string {
	var sb strings.Builder

	// Collect non-closed issues for bulk operations
	var openIDs, inProgressIDs, blockedIDs []string
	var highPriorityIDs []string // P0 and P1

	for _, i := range issues {
		escapedID := shellEscape(i.ID)
		switch i.Status {
		case model.StatusOpen:
			openIDs = append(openIDs, escapedID)
		case model.StatusInProgress:
			inProgressIDs = append(inProgressIDs, escapedID)
		case model.StatusBlocked:
			blockedIDs = append(blockedIDs, escapedID)
		}
		if i.Status != model.StatusClosed && i.Priority <= 1 {
			highPriorityIDs = append(highPriorityIDs, escapedID)
		}
	}

	// Only generate section if there are actionable items
	if len(openIDs)+len(inProgressIDs)+len(blockedIDs) == 0 {
		return ""
	}

	sb.WriteString("## Quick Actions\n\n")
	sb.WriteString("Ready-to-run commands for bulk operations:\n\n")
	sb.WriteString("```bash\n")

	// Close in-progress items (most common action)
	if len(inProgressIDs) > 0 {
		sb.WriteString("# Close all in-progress items\n")
		sb.WriteString(fmt.Sprintf("bd close %s\n\n", strings.Join(inProgressIDs, " ")))
	}

	// Close open items
	if len(openIDs) > 0 && len(openIDs) <= 10 {
		sb.WriteString("# Close all open items\n")
		sb.WriteString(fmt.Sprintf("bd close %s\n\n", strings.Join(openIDs, " ")))
	} else if len(openIDs) > 10 {
		sb.WriteString(fmt.Sprintf("# Close open items (%d total, showing first 10)\n", len(openIDs)))
		sb.WriteString(fmt.Sprintf("bd close %s\n\n", strings.Join(openIDs[:10], " ")))
	}

	// Bulk priority update for high-priority items
	if len(highPriorityIDs) > 0 {
		sb.WriteString("# View high-priority items (P0/P1)\n")
		sb.WriteString(fmt.Sprintf("bd show %s\n\n", strings.Join(highPriorityIDs, " ")))
	}

	// Unblock blocked items
	if len(blockedIDs) > 0 {
		sb.WriteString("# Update blocked items to in_progress when unblocked\n")
		sb.WriteString(fmt.Sprintf("bd update %s -s in_progress\n", strings.Join(blockedIDs, " ")))
	}

	sb.WriteString("```\n\n")

	return sb.String()
}

// generateIssueCommands creates command snippets for a single issue
func generateIssueCommands(issue model.Issue) string {
	var sb strings.Builder

	// Skip command snippets for closed issues
	if issue.Status == model.StatusClosed {
		return ""
	}

	escapedID := shellEscape(issue.ID)

	sb.WriteString("<details>\n<summary>📋 Commands</summary>\n\n")
	sb.WriteString("```bash\n")

	// Status transitions based on current state
	switch issue.Status {
	case model.StatusOpen:
		sb.WriteString("# Start working on this issue\n")
		sb.WriteString(fmt.Sprintf("bd update %s -s in_progress\n\n", escapedID))
	case model.StatusInProgress:
		sb.WriteString("# Mark as complete\n")
		sb.WriteString(fmt.Sprintf("bd close %s\n\n", escapedID))
	case model.StatusBlocked:
		sb.WriteString("# Unblock and start working\n")
		sb.WriteString(fmt.Sprintf("bd update %s -s in_progress\n\n", escapedID))
	}

	// Common actions
	sb.WriteString("# Add a comment\n")
	sb.WriteString(fmt.Sprintf("bd comment %s 'Your comment here'\n\n", escapedID))

	sb.WriteString("# Change priority (0=Critical, 1=High, 2=Medium, 3=Low)\n")
	sb.WriteString(fmt.Sprintf("bd update %s -p 1\n\n", escapedID))

	sb.WriteString("# View full details\n")
	sb.WriteString(fmt.Sprintf("bd show %s\n", escapedID))

	sb.WriteString("```\n\n")
	sb.WriteString("</details>\n\n")

	return sb.String()
}

// shellEscape escapes a string for safe use in shell commands.
// Uses single quotes and escapes any single quotes within the string.
func shellEscape(s string) string {
	// If the string contains no special characters, return as-is
	if isShellSafe(s) {
		return s
	}
	// Otherwise, wrap in single quotes and escape any single quotes
	escaped := strings.ReplaceAll(s, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// isShellSafe returns true if the string is safe to use unquoted in shell
func isShellSafe(s string) bool {
	for _, r := range s {
		if !isShellSafeChar(r) {
			return false
		}
	}
	return len(s) > 0
}

// isShellSafeChar returns true if the character is safe in unquoted shell strings
func isShellSafeChar(r rune) bool {
	// Allow alphanumeric, hyphen, underscore, period, and some punctuation
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.' || r == ':'
}
