package analysis

import (
	"math"
	"sync"
	"time"

	"beads_viewer/pkg/model"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

// StartupProfile captures detailed timing information for startup diagnostics.
// Use AnalyzeWithProfile to populate this structure.
type StartupProfile struct {
	// Data characteristics
	NodeCount int `json:"node_count"`
	EdgeCount int `json:"edge_count"`
	Density   float64 `json:"density"`

	// Phase 1 timings
	BuildGraph time.Duration `json:"build_graph"`
	Degree     time.Duration `json:"degree"`
	TopoSort   time.Duration `json:"topo_sort"`
	Phase1     time.Duration `json:"phase1_total"`

	// Phase 2 timings (zero if skipped)
	PageRank      time.Duration `json:"pagerank"`
	PageRankTO    bool          `json:"pagerank_timeout"`
	Betweenness   time.Duration `json:"betweenness"`
	BetweennessTO bool          `json:"betweenness_timeout"`
	Eigenvector   time.Duration `json:"eigenvector"`
	HITS          time.Duration `json:"hits"`
	HITSTO        bool          `json:"hits_timeout"`
	CriticalPath  time.Duration `json:"critical_path"`
	Cycles        time.Duration `json:"cycles"`
	CyclesTO      bool          `json:"cycles_timeout"`
	CycleCount    int           `json:"cycle_count"`
	Phase2        time.Duration `json:"phase2_total"`

	// Configuration used
	Config AnalysisConfig `json:"config"`

	// Totals
	Total time.Duration `json:"total"`
}

// GraphStats holds the results of graph analysis.
// Phase 1 fields (OutDegree, InDegree, TopologicalOrder, Density) are populated
// immediately and can be read without synchronization after AnalyzeAsync returns.
// Phase 2 fields (centrality metrics, cycles) are computed in background and
// must be accessed via thread-safe accessor methods.
type GraphStats struct {
	// Phase 1 - Available immediately after AnalyzeAsync returns (read-only after init)
	OutDegree        map[string]int // Number of dependencies this issue has (edges out)
	InDegree         map[string]int // Number of issues that depend on this issue (edges in)
	TopologicalOrder []string
	Density          float64
	NodeCount        int // Number of nodes in graph
	EdgeCount        int // Number of edges in graph

	// Configuration used for this analysis (read-only after init)
	Config AnalysisConfig

	// Phase 2 - Computed in background, access via thread-safe methods only
	mu                sync.RWMutex
	phase2Ready       bool
	phase2Done        chan struct{} // Closed when Phase 2 completes
	pageRank          map[string]float64
	betweenness       map[string]float64
	eigenvector       map[string]float64
	hubs              map[string]float64
	authorities       map[string]float64
	criticalPathScore map[string]float64
	cycles            [][]string
}

// IsPhase2Ready returns true if Phase 2 metrics have been computed.
func (s *GraphStats) IsPhase2Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.phase2Ready
}

// WaitForPhase2 blocks until Phase 2 computation completes.
func (s *GraphStats) WaitForPhase2() {
	if s.phase2Done != nil {
		<-s.phase2Done
	}
}

// GetPageRankScore returns the PageRank score for a single issue.
// Returns 0 if Phase 2 is not yet complete or if the issue is not found.
func (s *GraphStats) GetPageRankScore(id string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pageRank == nil {
		return 0
	}
	return s.pageRank[id]
}

// GetBetweennessScore returns the betweenness centrality for a single issue.
func (s *GraphStats) GetBetweennessScore(id string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.betweenness == nil {
		return 0
	}
	return s.betweenness[id]
}

// GetEigenvectorScore returns the eigenvector centrality for a single issue.
func (s *GraphStats) GetEigenvectorScore(id string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.eigenvector == nil {
		return 0
	}
	return s.eigenvector[id]
}

// GetHubScore returns the hub score for a single issue.
func (s *GraphStats) GetHubScore(id string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hubs == nil {
		return 0
	}
	return s.hubs[id]
}

// GetAuthorityScore returns the authority score for a single issue.
func (s *GraphStats) GetAuthorityScore(id string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.authorities == nil {
		return 0
	}
	return s.authorities[id]
}

// GetCriticalPathScore returns the critical path score for a single issue.
func (s *GraphStats) GetCriticalPathScore(id string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.criticalPathScore == nil {
		return 0
	}
	return s.criticalPathScore[id]
}

// PageRank returns a copy of the PageRank map. Safe for concurrent iteration.
// Returns an empty map if Phase 2 is not yet complete.
func (s *GraphStats) PageRank() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pageRank == nil {
		return nil
	}
	cp := make(map[string]float64, len(s.pageRank))
	for k, v := range s.pageRank {
		cp[k] = v
	}
	return cp
}

// Betweenness returns a copy of the Betweenness map. Safe for concurrent iteration.
// Returns an empty map if Phase 2 is not yet complete.
func (s *GraphStats) Betweenness() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.betweenness == nil {
		return nil
	}
	cp := make(map[string]float64, len(s.betweenness))
	for k, v := range s.betweenness {
		cp[k] = v
	}
	return cp
}

// Eigenvector returns a copy of the Eigenvector map. Safe for concurrent iteration.
// Returns an empty map if Phase 2 is not yet complete.
func (s *GraphStats) Eigenvector() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.eigenvector == nil {
		return nil
	}
	cp := make(map[string]float64, len(s.eigenvector))
	for k, v := range s.eigenvector {
		cp[k] = v
	}
	return cp
}

// Hubs returns a copy of the Hubs map. Safe for concurrent iteration.
// Returns an empty map if Phase 2 is not yet complete.
func (s *GraphStats) Hubs() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hubs == nil {
		return nil
	}
	cp := make(map[string]float64, len(s.hubs))
	for k, v := range s.hubs {
		cp[k] = v
	}
	return cp
}

// Authorities returns a copy of the Authorities map. Safe for concurrent iteration.
// Returns an empty map if Phase 2 is not yet complete.
func (s *GraphStats) Authorities() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.authorities == nil {
		return nil
	}
	cp := make(map[string]float64, len(s.authorities))
	for k, v := range s.authorities {
		cp[k] = v
	}
	return cp
}

// CriticalPathScore returns a copy of the CriticalPathScore map. Safe for concurrent iteration.
// Returns an empty map if Phase 2 is not yet complete.
func (s *GraphStats) CriticalPathScore() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.criticalPathScore == nil {
		return nil
	}
	cp := make(map[string]float64, len(s.criticalPathScore))
	for k, v := range s.criticalPathScore {
		cp[k] = v
	}
	return cp
}

// Cycles returns a copy of detected cycles. Safe for concurrent iteration.
// Returns nil if Phase 2 is not yet complete.
func (s *GraphStats) Cycles() [][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cycles == nil {
		return nil
	}
	cp := make([][]string, len(s.cycles))
	for i, c := range s.cycles {
		cp[i] = append([]string(nil), c...)
	}
	return cp
}

// NewGraphStatsForTest creates a GraphStats with the given data for testing.
// This allows tests to create GraphStats with specific values without needing
// to run the full analyzer.
func NewGraphStatsForTest(
	pageRank, betweenness, eigenvector, hubs, authorities, criticalPathScore map[string]float64,
	outDegree, inDegree map[string]int,
	cycles [][]string,
	density float64,
	topologicalOrder []string,
) *GraphStats {
	stats := &GraphStats{
		OutDegree:         outDegree,
		InDegree:          inDegree,
		TopologicalOrder:  topologicalOrder,
		Density:           density,
		phase2Done:        make(chan struct{}),
		pageRank:          pageRank,
		betweenness:       betweenness,
		eigenvector:       eigenvector,
		hubs:              hubs,
		authorities:       authorities,
		criticalPathScore: criticalPathScore,
		cycles:            cycles,
		phase2Ready:       true,
	}
	close(stats.phase2Done)
	return stats
}

// Analyzer encapsulates the graph logic
type Analyzer struct {
	g        *simple.DirectedGraph
	idToNode map[string]int64
	nodeToID map[int64]string
	issueMap map[string]model.Issue
	config   *AnalysisConfig // Optional custom config, nil means use size-based defaults
}

// SetConfig sets a custom analysis configuration.
// Pass nil to use size-based automatic configuration.
func (a *Analyzer) SetConfig(config *AnalysisConfig) {
	a.config = config
}

func NewAnalyzer(issues []model.Issue) *Analyzer {
	g := simple.NewDirectedGraph()
	// Pre-allocate maps for efficiency
	idToNode := make(map[string]int64, len(issues))
	nodeToID := make(map[int64]string, len(issues))
	issueMap := make(map[string]model.Issue, len(issues))

	// 1. Add Nodes
	for _, issue := range issues {
		issueMap[issue.ID] = issue
		n := g.NewNode()
		g.AddNode(n)
		idToNode[issue.ID] = n.ID()
		nodeToID[n.ID()] = issue.ID
	}

	// 2. Add Edges (Dependency Direction)
	// We only model *blocking* relationships in the analysis graph. Non-blocking
	// links such as "related" should not influence centrality metrics or cycle
	// detection because they do not gate execution order.
	for _, issue := range issues {
		u, ok := idToNode[issue.ID]
		if !ok {
			continue
		}

		for _, dep := range issue.Dependencies {
			if dep == nil {
				continue
			}

			// Only model blocking relationships in the analysis graph
			if !isBlockingDep(dep.Type) {
				continue
			}

			v, exists := idToNode[dep.DependsOnID]
			if exists {
				// Issue (u) depends on v → edge u -> v
				g.SetEdge(g.NewEdge(g.Node(u), g.Node(v)))
			}
		}
	}

	return &Analyzer{
		g:        g,
		idToNode: idToNode,
		nodeToID: nodeToID,
		issueMap: issueMap,
	}
}

// AnalyzeAsync performs graph analysis in two phases for fast startup.
// Phase 1 (instant): Degree centrality, topological order, density
// Phase 2 (background): PageRank, Betweenness, Eigenvector, HITS, Cycles
// Returns immediately with Phase 1 data. Use IsPhase2Ready() or WaitForPhase2() for Phase 2.
//
// If SetConfig was called, uses that config. Otherwise uses ConfigForSize() to
// automatically select appropriate algorithms based on graph size.
func (a *Analyzer) AnalyzeAsync() *GraphStats {
	var config AnalysisConfig
	if a.config != nil {
		config = *a.config
	} else {
		nodeCount := len(a.issueMap)
		edgeCount := a.g.Edges().Len()
		config = ConfigForSize(nodeCount, edgeCount)
	}
	return a.AnalyzeAsyncWithConfig(config)
}

// AnalyzeAsyncWithConfig performs graph analysis with a custom configuration.
// This allows callers to override the default size-based algorithm selection.
func (a *Analyzer) AnalyzeAsyncWithConfig(config AnalysisConfig) *GraphStats {
	nodeCount := len(a.issueMap)
	edgeCount := a.g.Edges().Len()

	stats := &GraphStats{
		OutDegree:         make(map[string]int),
		InDegree:          make(map[string]int),
		NodeCount:         nodeCount,
		EdgeCount:         edgeCount,
		Config:            config,
		phase2Done:        make(chan struct{}),
		pageRank:          make(map[string]float64),
		betweenness:       make(map[string]float64),
		eigenvector:       make(map[string]float64),
		hubs:              make(map[string]float64),
		authorities:       make(map[string]float64),
		criticalPathScore: make(map[string]float64),
	}

	// Handle empty graph - mark phase 2 ready immediately
	if nodeCount == 0 {
		stats.phase2Ready = true
		close(stats.phase2Done)
		return stats
	}

	// Phase 1: Fast metrics (degree centrality, topo sort, density)
	a.computePhase1(stats)

	// Phase 2: Expensive metrics in background goroutine
	go a.computePhase2(stats, config)

	return stats
}

// Analyze performs synchronous graph analysis (for backward compatibility).
// Blocks until all metrics are computed.
func (a *Analyzer) Analyze() GraphStats {
	stats := a.AnalyzeAsync()
	stats.WaitForPhase2()
	// Return a copy with public fields populated for backward compatibility
	return GraphStats{
		OutDegree:         stats.OutDegree,
		InDegree:          stats.InDegree,
		TopologicalOrder:  stats.TopologicalOrder,
		Density:           stats.Density,
		NodeCount:         stats.NodeCount,
		EdgeCount:         stats.EdgeCount,
		Config:            stats.Config,
		pageRank:          stats.pageRank,
		betweenness:       stats.betweenness,
		eigenvector:       stats.eigenvector,
		hubs:              stats.hubs,
		authorities:       stats.authorities,
		criticalPathScore: stats.criticalPathScore,
		cycles:            stats.cycles,
		phase2Ready:       true,
	}
}

// AnalyzeWithConfig performs synchronous graph analysis with a custom configuration.
func (a *Analyzer) AnalyzeWithConfig(config AnalysisConfig) GraphStats {
	stats := a.AnalyzeAsyncWithConfig(config)
	stats.WaitForPhase2()
	return GraphStats{
		OutDegree:         stats.OutDegree,
		InDegree:          stats.InDegree,
		TopologicalOrder:  stats.TopologicalOrder,
		Density:           stats.Density,
		NodeCount:         stats.NodeCount,
		EdgeCount:         stats.EdgeCount,
		Config:            stats.Config,
		pageRank:          stats.pageRank,
		betweenness:       stats.betweenness,
		eigenvector:       stats.eigenvector,
		hubs:              stats.hubs,
		authorities:       stats.authorities,
		criticalPathScore: stats.criticalPathScore,
		cycles:            stats.cycles,
		phase2Ready:       true,
	}
}

// AnalyzeWithProfile performs synchronous graph analysis and returns detailed timing profile.
// This is intended for diagnostics and the --profile-startup CLI flag.
func (a *Analyzer) AnalyzeWithProfile(config AnalysisConfig) (*GraphStats, *StartupProfile) {
	profile := &StartupProfile{
		Config: config,
	}

	totalStart := time.Now()

	nodeCount := len(a.issueMap)
	edgeCount := a.g.Edges().Len()

	profile.NodeCount = nodeCount
	profile.EdgeCount = edgeCount

	stats := &GraphStats{
		OutDegree:         make(map[string]int),
		InDegree:          make(map[string]int),
		NodeCount:         nodeCount,
		EdgeCount:         edgeCount,
		Config:            config,
		phase2Done:        make(chan struct{}),
		pageRank:          make(map[string]float64),
		betweenness:       make(map[string]float64),
		eigenvector:       make(map[string]float64),
		hubs:              make(map[string]float64),
		authorities:       make(map[string]float64),
		criticalPathScore: make(map[string]float64),
	}

	// Handle empty graph
	if nodeCount == 0 {
		stats.phase2Ready = true
		close(stats.phase2Done)
		profile.Total = time.Since(totalStart)
		return stats, profile
	}

	// Phase 1: Fast metrics with timing
	phase1Start := time.Now()
	a.computePhase1WithProfile(stats, profile)
	profile.Phase1 = time.Since(phase1Start)

	profile.Density = stats.Density

	// Phase 2: Expensive metrics synchronously with timing
	phase2Start := time.Now()
	a.computePhase2WithProfile(stats, config, profile)
	profile.Phase2 = time.Since(phase2Start)

	stats.phase2Ready = true
	close(stats.phase2Done)

	profile.Total = time.Since(totalStart)
	return stats, profile
}

// computePhase1WithProfile calculates fast metrics with timing instrumentation.
func (a *Analyzer) computePhase1WithProfile(stats *GraphStats, profile *StartupProfile) {
	// Degree centrality
	degreeStart := time.Now()
	nodes := a.g.Nodes()
	for nodes.Next() {
		n := nodes.Node()
		id := a.nodeToID[n.ID()]
		to := a.g.To(n.ID())
		stats.InDegree[id] = to.Len()
		from := a.g.From(n.ID())
		stats.OutDegree[id] = from.Len()
	}
	profile.Degree = time.Since(degreeStart)

	// Topological Sort
	topoStart := time.Now()
	sorted, err := topo.Sort(a.g)
	if err == nil {
		for i := len(sorted) - 1; i >= 0; i-- {
			stats.TopologicalOrder = append(stats.TopologicalOrder, a.nodeToID[sorted[i].ID()])
		}
	}
	profile.TopoSort = time.Since(topoStart)

	// Density
	n := float64(len(a.issueMap))
	e := float64(a.g.Edges().Len())
	if n > 1 {
		stats.Density = e / (n * (n - 1))
	}
}

// computePhase2WithProfile calculates expensive metrics with timing instrumentation.
func (a *Analyzer) computePhase2WithProfile(stats *GraphStats, config AnalysisConfig, profile *StartupProfile) {
	localPageRank := make(map[string]float64)
	localBetweenness := make(map[string]float64)
	localEigenvector := make(map[string]float64)
	localHubs := make(map[string]float64)
	localAuthorities := make(map[string]float64)
	localCriticalPath := make(map[string]float64)
	var localCycles [][]string

	// PageRank
	if config.ComputePageRank {
		prStart := time.Now()
		prDone := make(chan map[int64]float64, 1)
		go func() {
			prDone <- network.PageRank(a.g, 0.85, 1e-6)
		}()

		select {
		case pr := <-prDone:
			for id, score := range pr {
				localPageRank[a.nodeToID[id]] = score
			}
		case <-time.After(config.PageRankTimeout):
			profile.PageRankTO = true
			uniform := 1.0 / float64(len(a.issueMap))
			for id := range a.issueMap {
				localPageRank[id] = uniform
			}
		}
		profile.PageRank = time.Since(prStart)
	}

	// Betweenness
	if config.ComputeBetweenness {
		bwStart := time.Now()
		bwDone := make(chan BetweennessResult, 1)
		go func() {
			// Choose algorithm based on mode
			if config.BetweennessMode == BetweennessApproximate && config.BetweennessSampleSize > 0 {
				bwDone <- ApproxBetweenness(a.g, config.BetweennessSampleSize)
			} else {
				// Exact mode or mode not set (default to exact)
				exact := network.Betweenness(a.g)
				bwDone <- BetweennessResult{
					Scores:     exact,
					Mode:       BetweennessExact,
					TotalNodes: a.g.Nodes().Len(),
				}
			}
		}()

		select {
		case result := <-bwDone:
			for id, score := range result.Scores {
				localBetweenness[a.nodeToID[id]] = score
			}
			// Track if approximation was used (update stats.Config, not local copy)
			if result.Mode == BetweennessApproximate {
				stats.Config.BetweennessIsApproximate = true
			}
		case <-time.After(config.BetweennessTimeout):
			profile.BetweennessTO = true
		}
		profile.Betweenness = time.Since(bwStart)
	}

	// Eigenvector
	if config.ComputeEigenvector {
		evStart := time.Now()
		for id, score := range computeEigenvector(a.g) {
			localEigenvector[a.nodeToID[id]] = score
		}
		profile.Eigenvector = time.Since(evStart)
	}

	// HITS
	if config.ComputeHITS && a.g.Edges().Len() > 0 {
		hitsStart := time.Now()
		hitsDone := make(chan map[int64]network.HubAuthority, 1)
		go func() {
			hitsDone <- network.HITS(a.g, 1e-3)
		}()

		select {
		case hubAuth := <-hitsDone:
			for id, ha := range hubAuth {
				localHubs[a.nodeToID[id]] = ha.Hub
				localAuthorities[a.nodeToID[id]] = ha.Authority
			}
		case <-time.After(config.HITSTimeout):
			profile.HITSTO = true
		}
		profile.HITS = time.Since(hitsStart)
	}

	// Critical Path
	if config.ComputeCriticalPath {
		cpStart := time.Now()
		sorted, err := topo.Sort(a.g)
		if err == nil {
			localCriticalPath = a.computeHeights(sorted)
		}
		profile.CriticalPath = time.Since(cpStart)
	}

	// Cycles
	if config.ComputeCycles {
		cyclesStart := time.Now()
		maxCycles := config.MaxCyclesToStore
		if maxCycles == 0 {
			maxCycles = 100
		}

		sccs := topo.TarjanSCC(a.g)
		hasCycles := false
		for _, scc := range sccs {
			if len(scc) > 1 {
				hasCycles = true
				break
			}
		}

		if hasCycles {
			cyclesDone := make(chan [][]graph.Node, 1)
			go func() {
				cyclesDone <- topo.DirectedCyclesIn(a.g)
			}()

			select {
			case cycles := <-cyclesDone:
				profile.CycleCount = len(cycles)
				cyclesToProcess := cycles
				if len(cyclesToProcess) > maxCycles {
					cyclesToProcess = cyclesToProcess[:maxCycles]
				}

				for _, cycle := range cyclesToProcess {
					var cycleIDs []string
					for _, n := range cycle {
						cycleIDs = append(cycleIDs, a.nodeToID[n.ID()])
					}
					localCycles = append(localCycles, cycleIDs)
				}
			case <-time.After(config.CyclesTimeout):
				profile.CyclesTO = true
			}
		}
		profile.Cycles = time.Since(cyclesStart)
	}

	// Atomic assignment
	stats.mu.Lock()
	stats.pageRank = localPageRank
	stats.betweenness = localBetweenness
	stats.eigenvector = localEigenvector
	stats.hubs = localHubs
	stats.authorities = localAuthorities
	stats.criticalPathScore = localCriticalPath
	stats.cycles = localCycles
	stats.phase2Ready = true
	stats.mu.Unlock()
}

// computePhase1 calculates fast metrics synchronously.
func (a *Analyzer) computePhase1(stats *GraphStats) {
	nodes := a.g.Nodes()

	// Basic Degree Centrality
	for nodes.Next() {
		n := nodes.Node()
		id := a.nodeToID[n.ID()]

		to := a.g.To(n.ID())
		stats.InDegree[id] = to.Len() // Issues depending on me

		from := a.g.From(n.ID())
		stats.OutDegree[id] = from.Len() // Issues I depend on
	}

	// Topological Sort (fast for DAGs)
	sorted, err := topo.Sort(a.g)
	if err == nil {
		for i := len(sorted) - 1; i >= 0; i-- {
			stats.TopologicalOrder = append(stats.TopologicalOrder, a.nodeToID[sorted[i].ID()])
		}
	}

	// Density
	n := float64(len(a.issueMap))
	e := float64(a.g.Edges().Len())
	if n > 1 {
		stats.Density = e / (n * (n - 1))
	}
}

// computePhase2 calculates expensive metrics in background.
// Computes to local variables first, then atomically assigns under lock.
// Respects the config to skip expensive algorithms for large graphs.
func (a *Analyzer) computePhase2(stats *GraphStats, config AnalysisConfig) {
	defer close(stats.phase2Done)

	// Compute all metrics to LOCAL variables first (no lock needed)
	localPageRank := make(map[string]float64)
	localBetweenness := make(map[string]float64)
	localEigenvector := make(map[string]float64)
	localHubs := make(map[string]float64)
	localAuthorities := make(map[string]float64)
	localCriticalPath := make(map[string]float64)
	var localCycles [][]string

	// PageRank with timeout (if enabled)
	if config.ComputePageRank {
		prDone := make(chan map[int64]float64, 1)
		go func() {
			prDone <- network.PageRank(a.g, 0.85, 1e-6)
		}()

		select {
		case pr := <-prDone:
			for id, score := range pr {
				localPageRank[a.nodeToID[id]] = score
			}
		case <-time.After(config.PageRankTimeout):
			// Timeout - use uniform distribution
			uniform := 1.0 / float64(len(a.issueMap))
			for id := range a.issueMap {
				localPageRank[id] = uniform
			}
		}
	}

	// Betweenness with timeout (if enabled)
	if config.ComputeBetweenness {
		bwDone := make(chan BetweennessResult, 1)
		go func() {
			// Choose algorithm based on mode
			if config.BetweennessMode == BetweennessApproximate && config.BetweennessSampleSize > 0 {
				bwDone <- ApproxBetweenness(a.g, config.BetweennessSampleSize)
			} else {
				// Exact mode or mode not set (default to exact)
				exact := network.Betweenness(a.g)
				bwDone <- BetweennessResult{
					Scores:     exact,
					Mode:       BetweennessExact,
					TotalNodes: a.g.Nodes().Len(),
				}
			}
		}()

		select {
		case result := <-bwDone:
			for id, score := range result.Scores {
				localBetweenness[a.nodeToID[id]] = score
			}
			// Track if approximation was used (update stats.Config, not local copy)
			if result.Mode == BetweennessApproximate {
				stats.Config.BetweennessIsApproximate = true
			}
		case <-time.After(config.BetweennessTimeout):
			// Timeout - skip (leave empty)
		}
	}

	// Eigenvector (if enabled - usually fast, no timeout needed)
	if config.ComputeEigenvector {
		for id, score := range computeEigenvector(a.g) {
			localEigenvector[a.nodeToID[id]] = score
		}
	}

	// HITS with timeout (if enabled and graph has edges)
	if config.ComputeHITS && a.g.Edges().Len() > 0 {
		hitsDone := make(chan map[int64]network.HubAuthority, 1)
		go func() {
			hitsDone <- network.HITS(a.g, 1e-3)
		}()

		select {
		case hubAuth := <-hitsDone:
			for id, ha := range hubAuth {
				localHubs[a.nodeToID[id]] = ha.Hub
				localAuthorities[a.nodeToID[id]] = ha.Authority
			}
		case <-time.After(config.HITSTimeout):
			// Timeout - skip
		}
	}

	// Critical Path (if enabled - requires topological sort)
	if config.ComputeCriticalPath {
		sorted, err := topo.Sort(a.g)
		if err == nil {
			localCriticalPath = a.computeHeights(sorted)
		}
	}

	// Cycles with SCC pre-check and timeout (if enabled)
	if config.ComputeCycles {
		maxCycles := config.MaxCyclesToStore
		if maxCycles == 0 {
			maxCycles = 100 // Default
		}

		sccs := topo.TarjanSCC(a.g)
		hasCycles := false
		for _, scc := range sccs {
			if len(scc) > 1 {
				hasCycles = true
				break
			}
		}

		if hasCycles {
			cyclesDone := make(chan [][]graph.Node, 1)
			go func() {
				cyclesDone <- topo.DirectedCyclesIn(a.g)
			}()

			select {
			case cycles := <-cyclesDone:
				cyclesToProcess := cycles
				truncated := false
				if len(cyclesToProcess) > maxCycles {
					cyclesToProcess = cyclesToProcess[:maxCycles]
					truncated = true
				}

				for _, cycle := range cyclesToProcess {
					var cycleIDs []string
					for _, n := range cycle {
						cycleIDs = append(cycleIDs, a.nodeToID[n.ID()])
					}
					localCycles = append(localCycles, cycleIDs)
				}

				if truncated {
					localCycles = append(localCycles, []string{"...", "CYCLES_TRUNCATED"})
				}
			case <-time.After(config.CyclesTimeout):
				localCycles = [][]string{{"CYCLE_DETECTION_TIMEOUT"}}
			}
		}
	}

	// ATOMIC ASSIGNMENT: Lock once and assign all computed values
	stats.mu.Lock()
	stats.pageRank = localPageRank
	stats.betweenness = localBetweenness
	stats.eigenvector = localEigenvector
	stats.hubs = localHubs
	stats.authorities = localAuthorities
	stats.criticalPathScore = localCriticalPath
	stats.cycles = localCycles
	stats.phase2Ready = true
	stats.mu.Unlock()
}

func (a *Analyzer) computeHeights(sorted []graph.Node) map[string]float64 {
	heights := make(map[int64]float64)
	impactScores := make(map[string]float64)

	for _, n := range sorted {
		nid := n.ID()
		maxParentHeight := 0.0

		to := a.g.To(nid)
		for to.Next() {
			p := to.Node()
			if h, ok := heights[p.ID()]; ok {
				if h > maxParentHeight {
					maxParentHeight = h
				}
			}
		}
		heights[nid] = 1.0 + maxParentHeight
		impactScores[a.nodeToID[nid]] = heights[nid]
	}

	return impactScores
}

// isBlockingDep returns true if the dependency type represents a blocking relationship.
// Empty type defaults to blocks for legacy compatibility.
func isBlockingDep(depType model.DependencyType) bool {
	if depType == "" {
		return true // Legacy deps default to blocking
	}
	return depType == model.DepBlocks
}

// GetActionableIssues returns issues that can be worked on immediately.
// An issue is actionable if:
// 1. It is not closed
// 2. All its blocking dependencies (type "blocks") are closed
// Missing blockers don't block (graceful degradation).
func (a *Analyzer) GetActionableIssues() []model.Issue {
	var actionable []model.Issue

	for _, issue := range a.issueMap {
		if issue.Status == model.StatusClosed {
			continue
		}

		isBlocked := false
		for _, dep := range issue.Dependencies {
			if !isBlockingDep(dep.Type) {
				continue
			}

			blocker, exists := a.issueMap[dep.DependsOnID]
			if !exists {
				continue
			}

			if blocker.Status != model.StatusClosed {
				isBlocked = true
				break
			}
		}

		if !isBlocked {
			actionable = append(actionable, issue)
		}
	}

	return actionable
}

// GetIssue returns a single issue by ID, or nil if not found
func (a *Analyzer) GetIssue(id string) *model.Issue {
	if issue, ok := a.issueMap[id]; ok {
		return &issue
	}
	return nil
}

// GetBlockers returns the IDs of issues that block the given issue
func (a *Analyzer) GetBlockers(issueID string) []string {
	issue, ok := a.issueMap[issueID]
	if !ok {
		return nil
	}

	var blockers []string
	for _, dep := range issue.Dependencies {
		if isBlockingDep(dep.Type) {
			if _, exists := a.issueMap[dep.DependsOnID]; exists {
				blockers = append(blockers, dep.DependsOnID)
			}
		}
	}
	return blockers
}

// GetOpenBlockers returns the IDs of non-closed issues that block the given issue
func (a *Analyzer) GetOpenBlockers(issueID string) []string {
	issue, ok := a.issueMap[issueID]
	if !ok {
		return nil
	}

	var openBlockers []string
	for _, dep := range issue.Dependencies {
		if isBlockingDep(dep.Type) {
			if blocker, exists := a.issueMap[dep.DependsOnID]; exists {
				if blocker.Status != model.StatusClosed {
					openBlockers = append(openBlockers, dep.DependsOnID)
				}
			}
		}
	}
	return openBlockers
}

// computeEigenvector runs a simple power-iteration to estimate eigenvector centrality.
func computeEigenvector(g graph.Directed) map[int64]float64 {
	nodes := g.Nodes()
	var nodeList []graph.Node
	for nodes.Next() {
		nodeList = append(nodeList, nodes.Node())
	}
	n := len(nodeList)
	if n == 0 {
		return nil
	}

	vec := make([]float64, n)
	for i := range vec {
		vec[i] = 1.0 / float64(n)
	}
	work := make([]float64, n)

	index := make(map[int64]int, n)
	for i, node := range nodeList {
		index[node.ID()] = i
	}

	const iterations = 50
	for iter := 0; iter < iterations; iter++ {
		for i := range work {
			work[i] = 0
		}
		for _, node := range nodeList {
			i := index[node.ID()]
			incoming := g.To(node.ID())
			for incoming.Next() {
				j := index[incoming.Node().ID()]
				work[i] += vec[j]
			}
		}
		sum := 0.0
		for _, v := range work {
			sum += v * v
		}
		if sum == 0 {
			break
		}
		norm := 1 / math.Sqrt(sum)
		for i := range work {
			vec[i] = work[i] * norm
		}
	}

	res := make(map[int64]float64, n)
	for i, node := range nodeList {
		res[node.ID()] = vec[i]
	}
	return res
}
