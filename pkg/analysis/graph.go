package analysis

import (
	"beads_viewer/pkg/model"
	
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

// GraphStats holds the results of graph analysis
type GraphStats struct {
	PageRank          map[string]float64
	Betweenness       map[string]float64
	OutDegree         map[string]int // Number of issues blocked by this issue
	InDegree          map[string]int // Number of dependencies this issue has
	CriticalPathScore map[string]float64 // Heuristic for critical path
	Cycles            [][]string
	Density           float64
	TopologicalOrder  []string
}

// Analyzer encapsulates the graph logic
type Analyzer struct {
	g          *simple.DirectedGraph
	idToNode   map[string]int64
	nodeToID   map[int64]string
	issueMap   map[string]model.Issue
}

func NewAnalyzer(issues []model.Issue) *Analyzer {
	g := simple.NewDirectedGraph()
	idToNode := make(map[string]int64)
	nodeToID := make(map[int64]string)
	issueMap := make(map[string]model.Issue)

	// 1. Add Nodes
	for _, issue := range issues {
		issueMap[issue.ID] = issue
		n := g.NewNode()
		g.AddNode(n)
		idToNode[issue.ID] = n.ID()
		nodeToID[n.ID()] = issue.ID
	}

	// 2. Add Edges (Dependency Direction)
	// If A depends on B, B blocks A.
	// Edge: B -> A (Blocker -> Blocked)
	// This way, PageRank flows from Blockers to Blocked.
	// High PageRank = "Highly Blocked" (Fragile).
	// REVERSE Logic for "Criticality":
	// If we want "Critical" tasks to have high scores, we should flow FROM Blocked TO Blocker?
	// Or just use OutDegree (Blocks count).
	// Let's stick to natural flow: B -> A means B "causes" A.
	// Wait, usually dependency graph is A -> B (A depends on B).
	// Let's use: Edge A -> B means A DEPENDS ON B.
	// Then High In-Degree = Many things depend on me (I am a blocker).
	// High Out-Degree = I depend on many things (I am blocked).
	
	for _, issue := range issues {
		u, ok := idToNode[issue.ID]
		if !ok { continue } // Should not happen
		
		for _, dep := range issue.Dependencies {
			v, exists := idToNode[dep.DependsOnID]
			if exists {
				// Issue (u) DependsOn (v)
				// Edge: u -> v
				// Meaning: "Control flows from u to v" (u needs v)
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

func (a *Analyzer) Analyze() GraphStats {
	stats := GraphStats{
		PageRank:          make(map[string]float64),
		Betweenness:       make(map[string]float64),
		OutDegree:         make(map[string]int),
		InDegree:          make(map[string]int),
		CriticalPathScore: make(map[string]float64),
	}

	nodes := a.g.Nodes()
	
	// 1. Basic Degree Centrality
	// In our graph A->B (A depends on B):
	// In-Degree: Nodes pointing TO me. (Who depends on me?) -> Wait, edges are A->B.
	// A->B. A has out-degree 1 (to B). B has in-degree 1 (from A).
	// So:
	// In-Degree of B = Count of nodes that depend on B (Importance/Blocker score).
	// Out-Degree of A = Count of nodes A depends on (Fragility/Blocked score).
	for nodes.Next() {
		n := nodes.Node()
		id := a.nodeToID[n.ID()]
		
		to := a.g.To(n.ID())
		stats.InDegree[id] = to.Len() // Issues depending on me
		
		from := a.g.From(n.ID())
		stats.OutDegree[id] = from.Len() // Issues I depend on
	}
	nodes.Reset()

	// 2. PageRank
	// PageRank on A->B (Dependency) means "authority" flows to B.
	// High PageRank = Fundamental Dependencies (Deep Blockers).
	pr := network.PageRank(a.g, 0.85, 1e-6)
	for id, score := range pr {
		stats.PageRank[a.nodeToID[id]] = score
	}

	// 3. Betweenness Centrality
	// Nodes that bridge clusters.
	bw := network.Betweenness(a.g)
	for id, score := range bw {
		stats.Betweenness[a.nodeToID[id]] = score
	}

	// 4. Cycles
	cycles := topo.DirectedCyclesIn(a.g)
	for _, cycle := range cycles {
		var cycleIDs []string
		for _, n := range cycle {
			cycleIDs = append(cycleIDs, a.nodeToID[n.ID()])
		}
		stats.Cycles = append(stats.Cycles, cycleIDs)
	}

	// 5. Topological Sort (Linear Order)
	sorted, err := topo.Sort(a.g)
	if err == nil {
		// Sort returns roughly "execution order".
		// Prereqs (B) come after Dependents (A) in standard Sort?
		// topo.Sort returns nodes such that for every edge u->v, u comes before v.
		// If A->B (A depends on B), A comes before B?
		// No, usually Topo sort is for task scheduling: if B must be done before A, edge is B->A.
		// We defined A->B (A depends on B).
		// So if we want execution order, we need to reverse edges or interpret the sort.
		// In A->B graph, A appears before B.
		// So `sorted` list is "Start with Dependent -> End with Root Prereq".
		// Reverse it for "Start with Prereq -> End with Final Product".
		for i := len(sorted)-1; i >= 0; i-- {
			stats.TopologicalOrder = append(stats.TopologicalOrder, a.nodeToID[sorted[i].ID()])
		}
	}

	// 6. Critical Path Heuristic
	// Longest path to a root.
	// We can compute "Height" of each node in DAG.
	// Height(u) = 1 + max(Height(v)) for all u->v.
	// Since graph might have cycles, we operate on the condensation or just handle iteratively if DAG.
	// If err != nil (cycles), skip DAG-only stats.
	if err == nil {
		stats.CriticalPathScore = a.computeHeights()
	}

	// 7. Density
	n := float64(len(stats.PageRank))
	e := float64(a.g.Edges().Len())
	if n > 1 {
		stats.Density = e / (n * (n - 1))
	}

	return stats
}

func (a *Analyzer) computeHeights() map[string]float64 {
	heights := make(map[int64]float64)
	sorted, _ := topo.Sort(a.g)
	
	// Process in reverse topological order (B then A, if A->B)
	// If A->B, A depends on B.
	// Height of B (Prereq) = 0 (if leaf).
	// Height of A = Height(B) + 1.
	// This measures "Depth of Dependency Chain" below the item.
	// High Height = Lots of upstream work required? No.
	// Let's define:
	// Depth: Length of chain *upwards* (prereqs).
	// Impact: Length of chain *downwards* (dependents).
	
	// Let's compute "Impact Depth": How many layers of tasks depend on this?
	// This requires reversing the graph edges or traversing differently.
	// If A->B (A depends on B), edges point to Prereq.
	// To find Impact, we want dist from B to A.
	
	// Let's assume we want to know "How critical is this?". 
	// Critical = many dependents, long chains of dependents.
	// We need to walk backwards from edges A->B.
	
	impactScores := make(map[string]float64)
	
	// Simple iterative approach for DAG:
	// For every node, score = 1 + sum(score of incoming).
	// Incoming to B is A (since A->B).
	// gonum To(B) returns nodes pointing to B (A).
	
	// We can use the reversed topo sort (Prereqs last).
	// Iterate Prereq -> Dependent.
	// But A->B means A is dependent. 
	// Topo sort gives [A, B].
	// Iterate reverse: B, A.
	// B: score = 1 + sum(score of incoming). Incoming to B is A.
	// A has not been processed yet?
	// Wait. A->B. A comes BEFORE B in Topo sort.
	// So iterating Topo sort forward:
	// Process A. Has outgoing B. A adds its score to B?
	// Let's stick to simple "Level" logic.
	
	// Let's assign "Level" based on dependencies.
	// Level(u) = 1 + max(Level(v)) where u depends on v (u->v).
	// This is "Dependency Depth". 
	// High Depth = "I am the tip of a deep iceberg". (End product).
	
	// Let's also compute "Reverse Depth" (Impact).
	// How many steps can I walk backwards?
	
	// Let's just do a simple BFS/DFS max depth for now to keep it fast.
	// Actually, with Topo sort:
	
	// 1. Dependency Depth (Effort to reach me)
	// Order: Leafs (no deps) first.
	// But our edges are A->B (A depends on B).
	// So Leafs are nodes with OutDegree 0.
	// If we reverse sort: B (leaf), A.
	// We can compute depth easily.
	
	for i := len(sorted)-1; i >= 0; i-- {
		u := sorted[i]
		uid := u.ID()
		maxChildHeight := 0.0
		
		// Edges are u->v.
		to := a.g.From(uid)
		for to.Next() {
			v := to.Node()
			if h, ok := heights[v.ID()]; ok {
				if h > maxChildHeight {
					maxChildHeight = h
				}
			}
		}
		heights[uid] = 1.0 + maxChildHeight
		impactScores[a.nodeToID[uid]] = heights[uid]
	}
	
	return impactScores
}
