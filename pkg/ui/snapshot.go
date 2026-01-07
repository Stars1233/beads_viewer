// Package ui provides the terminal user interface for beads_viewer.
// This file implements the DataSnapshot type for thread-safe UI rendering.
package ui

import (
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// DataSnapshot is an immutable, self-contained representation of all data
// the UI needs to render. Once created, it never changes - this is critical
// for thread safety when the background worker is building the next snapshot.
//
// The UI thread reads exclusively from its current snapshot pointer.
// When a new snapshot is ready, the UI swaps the pointer atomically.
type DataSnapshot struct {
	// Core data
	Issues   []model.Issue          // All issues (sorted)
	IssueMap map[string]*model.Issue // Lookup by ID

	// Graph analysis
	Analyzer  *analysis.Analyzer
	Analysis  *analysis.GraphStats

	// Computed statistics
	CountOpen    int
	CountReady   int
	CountBlocked int
	CountClosed  int

	// Pre-computed UI data (Phase 3 will populate these)
	// For now, they're nil and the UI computes on demand
	ListItems     []IssueItem // Pre-built list items with scores
	TriageScores  map[string]float64
	TriageReasons map[string]analysis.TriageReasons
	QuickWinSet   map[string]bool
	BlockerSet    map[string]bool
	UnblocksMap   map[string][]string

	// Metadata
	CreatedAt time.Time // When this snapshot was built
	DataHash  string    // Hash of source data for cache validation

	// Error state (for graceful degradation)
	LoadError    error     // Non-nil if last load had recoverable errors
	ErrorTime    time.Time // When error occurred
	StaleWarning bool      // True if data is from previous successful load
}

// SnapshotBuilder constructs DataSnapshots from raw data.
// This is used by the BackgroundWorker to build new snapshots.
type SnapshotBuilder struct {
	issues   []model.Issue
	analyzer *analysis.Analyzer
	analysis *analysis.GraphStats
}

// NewSnapshotBuilder creates a builder for constructing a DataSnapshot.
func NewSnapshotBuilder(issues []model.Issue) *SnapshotBuilder {
	return &SnapshotBuilder{
		issues:   issues,
		analyzer: analysis.NewAnalyzer(issues),
	}
}

// WithAnalysis sets the pre-computed analysis (for when we have cached results).
func (b *SnapshotBuilder) WithAnalysis(a *analysis.GraphStats) *SnapshotBuilder {
	b.analysis = a
	return b
}

// Build constructs the final immutable DataSnapshot.
// This performs all necessary computations that should happen in the background.
func (b *SnapshotBuilder) Build() *DataSnapshot {
	issues := b.issues

	// Compute analysis if not provided
	graphStats := b.analysis
	if graphStats == nil {
		stats := b.analyzer.Analyze()
		graphStats = &stats
	}

	// Build lookup map
	issueMap := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	// Compute statistics
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
			if dep == nil || !dep.Type.IsBlocking() {
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

	// Build list items with graph scores
	listItems := make([]IssueItem, len(issues))
	for i := range issues {
		listItems[i] = IssueItem{
			Issue:      issues[i],
			GraphScore: graphStats.GetPageRankScore(issues[i].ID),
			Impact:     graphStats.GetCriticalPathScore(issues[i].ID),
			RepoPrefix: ExtractRepoPrefix(issues[i].ID),
		}
	}

	// Compute triage insights
	triageResult := analysis.ComputeTriageFromAnalyzer(b.analyzer, graphStats, issues, analysis.TriageOptions{}, time.Now())
	triageScores := make(map[string]float64, len(triageResult.Recommendations))
	triageReasons := make(map[string]analysis.TriageReasons, len(triageResult.Recommendations))
	quickWinSet := make(map[string]bool, len(triageResult.QuickWins))
	blockerSet := make(map[string]bool, len(triageResult.BlockersToClear))
	unblocksMap := make(map[string][]string, len(triageResult.Recommendations))

	for _, rec := range triageResult.Recommendations {
		triageScores[rec.ID] = rec.Score
		if len(rec.Reasons) > 0 {
			triageReasons[rec.ID] = analysis.TriageReasons{
				Primary:    rec.Reasons[0],
				All:        rec.Reasons,
				ActionHint: rec.Action,
			}
		}
		unblocksMap[rec.ID] = rec.UnblocksIDs
	}
	for _, qw := range triageResult.QuickWins {
		quickWinSet[qw.ID] = true
	}
	for _, bl := range triageResult.BlockersToClear {
		blockerSet[bl.ID] = true
	}

	// Update list items with triage data
	for i := range listItems {
		id := listItems[i].Issue.ID
		listItems[i].TriageScore = triageScores[id]
		if reasons, exists := triageReasons[id]; exists {
			listItems[i].TriageReason = reasons.Primary
			listItems[i].TriageReasons = reasons.All
		}
		listItems[i].IsQuickWin = quickWinSet[id]
		listItems[i].IsBlocker = blockerSet[id]
		listItems[i].UnblocksCount = len(unblocksMap[id])
	}

	return &DataSnapshot{
		Issues:        issues,
		IssueMap:      issueMap,
		Analyzer:      b.analyzer,
		Analysis:      graphStats,
		CountOpen:     cOpen,
		CountReady:    cReady,
		CountBlocked:  cBlocked,
		CountClosed:   cClosed,
		ListItems:     listItems,
		TriageScores:  triageScores,
		TriageReasons: triageReasons,
		QuickWinSet:   quickWinSet,
		BlockerSet:    blockerSet,
		UnblocksMap:   unblocksMap,
		CreatedAt:     time.Now(),
	}
}

// IsEmpty returns true if the snapshot has no issues.
func (s *DataSnapshot) IsEmpty() bool {
	return s == nil || len(s.Issues) == 0
}

// GetIssue returns an issue by ID, or nil if not found.
func (s *DataSnapshot) GetIssue(id string) *model.Issue {
	if s == nil || s.IssueMap == nil {
		return nil
	}
	return s.IssueMap[id]
}

// Age returns how long ago this snapshot was created.
func (s *DataSnapshot) Age() time.Duration {
	if s == nil {
		return 0
	}
	return time.Since(s.CreatedAt)
}
