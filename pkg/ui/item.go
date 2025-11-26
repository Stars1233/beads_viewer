package ui

import (
	"fmt"
	"strings"
	
	"beads_viewer/pkg/model"
)

// IssueItem wraps model.Issue to implement list.Item
type IssueItem struct {
	Issue      model.Issue
	GraphScore float64
	Impact     float64
}

func (i IssueItem) Title() string {
	return i.Issue.Title
}

func (i IssueItem) Description() string {
	return fmt.Sprintf("%s %s • %s", i.Issue.ID, i.Issue.Status, i.Issue.Assignee)
}

func (i IssueItem) FilterValue() string {
	// Enhanced filter value including labels and assignee
	var sb strings.Builder
	sb.WriteString(i.Issue.Title)
	sb.WriteString(" ")
	sb.WriteString(i.Issue.ID)
	sb.WriteString(" ")
	sb.WriteString(string(i.Issue.Status))
	sb.WriteString(" ")
	sb.WriteString(string(i.Issue.IssueType))
	
	if i.Issue.Assignee != "" {
		sb.WriteString(" ")
		sb.WriteString(i.Issue.Assignee)
	}
	
	if len(i.Issue.Labels) > 0 {
		sb.WriteString(" ")
		sb.WriteString(strings.Join(i.Issue.Labels, " "))
	}
	
	return sb.String()
}