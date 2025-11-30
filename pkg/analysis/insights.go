package analysis

import (
	"sort"
)

// InsightItem represents a single item in an insight list with its metric value
type InsightItem struct {
	ID    string
	Value float64
}

// Insights is a high-level summary of graph analysis
type Insights struct {
	Bottlenecks    []InsightItem // Top betweenness nodes
	Keystones      []InsightItem // Top impact nodes
	Influencers    []InsightItem // Top eigenvector centrality
	Hubs           []InsightItem // Strong dependency aggregators
	Authorities    []InsightItem // Strong prerequisite providers
	Orphans        []string      // No dependencies (and not blocked?) - Leaf nodes
	Cycles         [][]string
	ClusterDensity float64

	// Full stats for calculation explanations
	Stats *GraphStats
}

// GenerateInsights translates raw stats into actionable data
func (s *GraphStats) GenerateInsights(limit int) Insights {
	// Get thread-safe copies of all Phase 2 data
	pageRank := s.PageRank()
	betweenness := s.Betweenness()
	criticalPath := s.CriticalPathScore()
	eigenvector := s.Eigenvector()
	hubs := s.Hubs()
	authorities := s.Authorities()
	cycles := s.Cycles()

	if limit <= 0 {
		limit = len(pageRank) // use full set; maps all share same key set
	}

	return Insights{
		Bottlenecks:    getTopItems(betweenness, limit),
		Keystones:      getTopItems(criticalPath, limit),
		Influencers:    getTopItems(eigenvector, limit),
		Hubs:           getTopItems(hubs, limit),
		Authorities:    getTopItems(authorities, limit),
		Cycles:         cycles,
		ClusterDensity: s.Density,
		Stats:          s,
	}
}

func getTopItems(m map[string]float64, limit int) []InsightItem {
	type kv struct {
		Key   string
		Value float64
	}
	var ss []kv
	for k, v := range m {
		ss = append(ss, kv{k, v})
	}

	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})

	result := make([]InsightItem, 0)
	for i := 0; i < len(ss) && i < limit; i++ {
		result = append(result, InsightItem{ID: ss[i].Key, Value: ss[i].Value})
	}
	return result
}
