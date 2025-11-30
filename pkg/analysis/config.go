package analysis

import "time"

// AnalysisConfig controls which metrics to compute and their timeouts.
// This enables size-based algorithm selection for optimal performance.
type AnalysisConfig struct {
	// Betweenness centrality (expensive: O(V*E))
	ComputeBetweenness     bool
	BetweennessTimeout     time.Duration
	BetweennessSkipReason  string          // Set when skipped, explains why
	BetweennessMode        BetweennessMode // "exact", "approximate", or "skip"
	BetweennessSampleSize  int             // Sample size for approximate mode
	BetweennessIsApproximate bool          // True if approximation was used (set after computation)

	// PageRank
	ComputePageRank    bool
	PageRankTimeout    time.Duration
	PageRankSkipReason string

	// HITS (Hubs and Authorities)
	ComputeHITS    bool
	HITSTimeout    time.Duration
	HITSSkipReason string

	// Cycle detection (potentially exponential)
	ComputeCycles    bool
	CyclesTimeout    time.Duration
	MaxCyclesToStore int
	CyclesSkipReason string

	// Eigenvector centrality (usually fast)
	ComputeEigenvector bool

	// Critical path scoring (fast, O(V+E))
	ComputeCriticalPath bool
}

// DefaultConfig returns the default analysis configuration.
// All metrics enabled with standard timeouts. Uses exact betweenness.
func DefaultConfig() AnalysisConfig {
	return AnalysisConfig{
		ComputeBetweenness: true,
		BetweennessMode:    BetweennessExact,
		BetweennessTimeout: 500 * time.Millisecond,

		ComputePageRank: true,
		PageRankTimeout: 500 * time.Millisecond,

		ComputeHITS: true,
		HITSTimeout: 500 * time.Millisecond,

		ComputeCycles:    true,
		CyclesTimeout:    500 * time.Millisecond,
		MaxCyclesToStore: 100,

		ComputeEigenvector:  true,
		ComputeCriticalPath: true,
	}
}

// ConfigForSize returns an appropriate configuration based on graph size.
// Larger graphs get more aggressive timeouts and may use approximate algorithms.
//
// Size tiers:
//   - Small (<100 nodes): Full analysis with exact algorithms, generous timeouts
//   - Medium (100-500 nodes): Exact algorithms with standard timeouts
//   - Large (500-2000 nodes): Approximate betweenness for sparse graphs, skip for dense
//   - XL (>2000 nodes): Approximate betweenness, skip cycles and HITS for dense graphs
func ConfigForSize(nodeCount, edgeCount int) AnalysisConfig {
	density := 0.0
	if nodeCount > 1 {
		density = float64(edgeCount) / float64(nodeCount*(nodeCount-1))
	}

	switch {
	case nodeCount < 100:
		// Small graph: run everything with generous timeouts, exact betweenness
		return AnalysisConfig{
			ComputeBetweenness: true,
			BetweennessMode:    BetweennessExact,
			BetweennessTimeout: 2 * time.Second,

			ComputePageRank: true,
			PageRankTimeout: 2 * time.Second,

			ComputeHITS: true,
			HITSTimeout: 2 * time.Second,

			ComputeCycles:    true,
			CyclesTimeout:    2 * time.Second,
			MaxCyclesToStore: 1000,

			ComputeEigenvector:  true,
			ComputeCriticalPath: true,
		}

	case nodeCount < 500:
		// Medium graph: standard timeouts, exact betweenness
		return AnalysisConfig{
			ComputeBetweenness: true,
			BetweennessMode:    BetweennessExact,
			BetweennessTimeout: 500 * time.Millisecond,

			ComputePageRank: true,
			PageRankTimeout: 500 * time.Millisecond,

			ComputeHITS: true,
			HITSTimeout: 500 * time.Millisecond,

			ComputeCycles:    true,
			CyclesTimeout:    500 * time.Millisecond,
			MaxCyclesToStore: 100,

			ComputeEigenvector:  true,
			ComputeCriticalPath: true,
		}

	case nodeCount < 2000:
		// Large graph: use approximate betweenness, shorter timeouts
		cfg := AnalysisConfig{
			ComputePageRank: true,
			PageRankTimeout: 300 * time.Millisecond,

			ComputeHITS: true,
			HITSTimeout: 300 * time.Millisecond,

			ComputeCycles:    true,
			CyclesTimeout:    300 * time.Millisecond,
			MaxCyclesToStore: 50,

			ComputeEigenvector:  true,
			ComputeCriticalPath: true,
		}

		// Use approximate betweenness for large sparse graphs, skip for dense
		if density < 0.01 {
			cfg.ComputeBetweenness = true
			cfg.BetweennessMode = BetweennessApproximate
			cfg.BetweennessSampleSize = RecommendSampleSize(nodeCount, edgeCount)
			cfg.BetweennessTimeout = 500 * time.Millisecond // More time for sampling
		} else {
			cfg.ComputeBetweenness = false
			cfg.BetweennessMode = BetweennessSkip
			cfg.BetweennessSkipReason = "graph too dense (density > 0.01)"
		}

		return cfg

	default:
		// XL graph (>2000 nodes): use approximate betweenness with larger sample
		cfg := AnalysisConfig{
			// Use approximate betweenness for XL graphs
			ComputeBetweenness:    true,
			BetweennessMode:       BetweennessApproximate,
			BetweennessSampleSize: RecommendSampleSize(nodeCount, edgeCount),
			BetweennessTimeout:    500 * time.Millisecond,

			ComputePageRank: true,
			PageRankTimeout: 200 * time.Millisecond,

			ComputeCycles:       false,
			CyclesSkipReason:    "graph too large (>2000 nodes)",
			MaxCyclesToStore:    10,

			ComputeEigenvector:  true,
			ComputeCriticalPath: true,
		}

		// Only compute HITS for very sparse XL graphs
		if density < 0.001 {
			cfg.ComputeHITS = true
			cfg.HITSTimeout = 200 * time.Millisecond
		} else {
			cfg.ComputeHITS = false
			cfg.HITSSkipReason = "graph too large and dense"
		}

		return cfg
	}
}

// FullAnalysisConfig returns a config that computes all metrics regardless of size.
// Useful when --force-full-analysis is specified. Uses exact betweenness.
func FullAnalysisConfig() AnalysisConfig {
	return AnalysisConfig{
		ComputeBetweenness: true,
		BetweennessMode:    BetweennessExact, // Force exact for full analysis
		BetweennessTimeout: 30 * time.Second, // Very generous for forced full analysis

		ComputePageRank: true,
		PageRankTimeout: 30 * time.Second,

		ComputeHITS: true,
		HITSTimeout: 30 * time.Second,

		ComputeCycles:    true,
		CyclesTimeout:    30 * time.Second,
		MaxCyclesToStore: 10000,

		ComputeEigenvector:  true,
		ComputeCriticalPath: true,
	}
}

// SkippedMetrics returns a list of metrics that are configured to be skipped.
func (c AnalysisConfig) SkippedMetrics() []SkippedMetric {
	var skipped []SkippedMetric

	if !c.ComputeBetweenness {
		skipped = append(skipped, SkippedMetric{
			Name:   "Betweenness",
			Reason: c.BetweennessSkipReason,
		})
	}
	if !c.ComputePageRank {
		skipped = append(skipped, SkippedMetric{
			Name:   "PageRank",
			Reason: c.PageRankSkipReason,
		})
	}
	if !c.ComputeHITS {
		skipped = append(skipped, SkippedMetric{
			Name:   "HITS",
			Reason: c.HITSSkipReason,
		})
	}
	if !c.ComputeCycles {
		skipped = append(skipped, SkippedMetric{
			Name:   "Cycles",
			Reason: c.CyclesSkipReason,
		})
	}

	return skipped
}

// SkippedMetric describes a metric that was skipped and why.
type SkippedMetric struct {
	Name   string
	Reason string
}
