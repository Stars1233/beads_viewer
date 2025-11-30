package analysis

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
)

// BetweennessMode specifies how betweenness centrality should be computed.
type BetweennessMode string

const (
	// BetweennessExact computes exact betweenness centrality using Brandes' algorithm.
	// Complexity: O(V*E) - fast for small graphs, slow for large graphs.
	BetweennessExact BetweennessMode = "exact"

	// BetweennessApproximate uses sampling-based approximation.
	// Complexity: O(k*E) where k is the sample size - much faster for large graphs.
	// Error: O(1/sqrt(k)) - with k=100, ~10% error in ranking.
	BetweennessApproximate BetweennessMode = "approximate"

	// BetweennessSkip disables betweenness computation entirely.
	BetweennessSkip BetweennessMode = "skip"
)

// BetweennessResult contains the result of betweenness computation.
type BetweennessResult struct {
	// Scores maps node IDs to their betweenness centrality scores
	Scores map[int64]float64

	// Mode indicates how the result was computed
	Mode BetweennessMode

	// SampleSize is the number of pivot nodes used (only for approximate mode)
	SampleSize int

	// TotalNodes is the total number of nodes in the graph
	TotalNodes int

	// Elapsed is the time taken to compute
	Elapsed time.Duration

	// TimedOut indicates if computation was interrupted by timeout
	TimedOut bool
}

// ApproxBetweenness computes approximate betweenness centrality using sampling.
//
// Instead of computing shortest paths from ALL nodes (O(V*E)), we sample k pivot
// nodes and extrapolate. This is Brandes' approximation algorithm.
//
// Error bounds: With k samples, approximation error is O(1/sqrt(k)):
//   - k=50: ~14% error
//   - k=100: ~10% error
//   - k=200: ~7% error
//
// For ranking purposes (which node is most central), this is usually sufficient.
//
// References:
//   - "A Faster Algorithm for Betweenness Centrality" (Brandes, 2001)
//   - "Approximating Betweenness Centrality" (Bader et al., 2007)
func ApproxBetweenness(g *simple.DirectedGraph, sampleSize int) BetweennessResult {
	start := time.Now()
	nodes := graph.NodesOf(g.Nodes())
	n := len(nodes)

	result := BetweennessResult{
		Scores:     make(map[int64]float64),
		Mode:       BetweennessApproximate,
		SampleSize: sampleSize,
		TotalNodes: n,
	}

	if n == 0 {
		result.Elapsed = time.Since(start)
		return result
	}

	// For small graphs or when sample size >= node count, use exact algorithm
	if sampleSize >= n {
		exact := network.Betweenness(g)
		result.Scores = exact
		result.Mode = BetweennessExact
		result.SampleSize = n
		result.Elapsed = time.Since(start)
		return result
	}

	// Sample k random pivot nodes
	pivots := sampleNodes(nodes, sampleSize)

	// Compute partial betweenness from sampled pivots only
	partialBC := make(map[int64]float64)
	for _, pivot := range pivots {
		singleSourceBetweenness(g, pivot, partialBC)
	}

	// Scale up: BC_approx = BC_partial * (n / k)
	// This extrapolates from the sample to the full graph
	scale := float64(n) / float64(sampleSize)
	for id := range partialBC {
		partialBC[id] *= scale
	}

	result.Scores = partialBC
	result.Elapsed = time.Since(start)
	return result
}

// sampleNodes returns a random sample of k nodes from the input slice.
// Uses Fisher-Yates shuffle for unbiased sampling.
func sampleNodes(nodes []graph.Node, k int) []graph.Node {
	if k >= len(nodes) {
		return nodes
	}

	// Create a copy to avoid modifying the original
	shuffled := make([]graph.Node, len(nodes))
	copy(shuffled, nodes)

	// Fisher-Yates shuffle for first k elements
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < k; i++ {
		j := i + rng.Intn(len(shuffled)-i)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:k]
}

// singleSourceBetweenness computes the betweenness contribution from a single source node.
// This is the core of Brandes' algorithm, run once per pivot.
//
// The algorithm performs BFS from the source and accumulates dependency scores
// in a reverse topological order traversal.
func singleSourceBetweenness(g *simple.DirectedGraph, source graph.Node, bc map[int64]float64) {
	sourceID := source.ID()
	nodes := graph.NodesOf(g.Nodes())

	// Use shortest paths from gonum
	pt := path.DijkstraFrom(source, g)

	// Build the DAG of shortest paths and compute sigma (number of shortest paths)
	// and delta (dependency accumulation)
	sigma := make(map[int64]float64)  // Number of shortest paths through node
	dist := make(map[int64]float64)   // Distance from source
	delta := make(map[int64]float64)  // Dependency of source on node
	pred := make(map[int64][]int64)   // Predecessors on shortest paths

	for _, n := range nodes {
		nid := n.ID()
		sigma[nid] = 0
		dist[nid] = -1 // -1 means unreachable
		delta[nid] = 0
		pred[nid] = nil
	}

	sigma[sourceID] = 1
	dist[sourceID] = 0

	// BFS from source, building shortest path DAG
	type nodeWithDist struct {
		id   int64
		dist float64
	}
	var stack []nodeWithDist

	// First, compute distances using Dijkstra result
	for _, n := range nodes {
		nid := n.ID()
		_, d := pt.To(nid)
		// math.Inf(1) indicates unreachable
		if d == math.Inf(1) || nid == sourceID {
			continue
		}
		dist[nid] = d
		stack = append(stack, nodeWithDist{id: nid, dist: d})
	}

	// Sort by distance (BFS order)
	sort.Slice(stack, func(i, j int) bool {
		return stack[i].dist < stack[j].dist
	})

	// Build sigma and predecessors
	// For each node in BFS order, find predecessors on shortest paths
	for _, nd := range stack {
		nid := nd.id
		nodeDist := nd.dist

		// Find all predecessors that are on a shortest path
		to := g.To(nid)
		for to.Next() {
			predNode := to.Node()
			predID := predNode.ID()
			predDist := dist[predID]

			if predDist < 0 {
				continue // Unreachable from source
			}

			// Check if this edge is on a shortest path
			// Edge (pred -> n) is on shortest path if dist[pred] + weight(pred,n) == dist[n]
			// For unweighted graphs, weight = 1
			if predDist+1 == nodeDist {
				pred[nid] = append(pred[nid], predID)
			}
		}

		// Compute sigma[nid] = sum of sigma[p] for all predecessors p
		if len(pred[nid]) > 0 {
			for _, p := range pred[nid] {
				sigma[nid] += sigma[p]
			}
		} else if nid != sourceID {
			// No predecessors means direct path from source (distance 1)
			fromSource := g.To(nid)
			for fromSource.Next() {
				if fromSource.Node().ID() == sourceID {
					sigma[nid] = 1
					pred[nid] = []int64{sourceID}
					break
				}
			}
		}
	}

	// Accumulation: traverse in reverse BFS order (farthest first)
	for i := len(stack) - 1; i >= 0; i-- {
		nid := stack[i].id
		if sigma[nid] == 0 {
			continue
		}

		// For each predecessor p of nid
		for _, pid := range pred[nid] {
			// delta[p] += (sigma[p] / sigma[nid]) * (1 + delta[nid])
			if sigma[nid] > 0 {
				delta[pid] += (sigma[pid] / sigma[nid]) * (1 + delta[nid])
			}
		}

		// Add to betweenness (not for source node)
		if nid != sourceID {
			bc[nid] += delta[nid]
		}
	}
}

// RecommendSampleSize returns a recommended sample size based on graph characteristics.
// The goal is to balance accuracy vs. speed.
func RecommendSampleSize(nodeCount, edgeCount int) int {
	switch {
	case nodeCount < 100:
		// Small graph: use exact algorithm
		return nodeCount
	case nodeCount < 500:
		// Medium graph: 20% sample for ~22% error
		minSample := 50
		sample := nodeCount / 5
		if sample > minSample {
			return sample
		}
		return minSample
	case nodeCount < 2000:
		// Large graph: fixed sample for ~10% error
		return 100
	default:
		// XL graph: larger fixed sample
		return 200
	}
}
