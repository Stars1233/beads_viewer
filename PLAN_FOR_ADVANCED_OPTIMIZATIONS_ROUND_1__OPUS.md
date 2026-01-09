# Performance Optimization Plan: Round 1

## Executive Summary

This document captures a rigorous performance analysis of beads_viewer following a data-driven methodology. The analysis identified a single dominant hotspot responsible for 71% of memory allocations and 49% of CPU time, with a clear optimization path that is provably isomorphic (outputs unchanged).

**Key Finding**: Buffer pooling for `singleSourceBetweenness` can eliminate **60-80%** of allocations from the dominant hotspot, with potential for 90%+ when combined with additional caching.

---

## Methodology

The analysis followed these requirements:

- **A) Baseline First**: Run benchmarks with `-benchmem -count=3`
- **B) Profile Before Proposing**: Capture CPU + allocation profiles; identify top 3-5 hotspots
- **C) Equivalence Oracle**: Define golden outputs + invariants
- **D) Isomorphism Proof**: Every proposed change includes proof that outputs cannot change
- **E) Opportunity Matrix**: Rank by (Impact × Confidence) / Effort
- **F) Minimal Diffs**: One performance lever per change
- **G) Regression Guardrails**: Add benchmark thresholds

---

## Phase A: Baseline Metrics

### Environment

```
go version go1.25.5 linux/amd64
cpu: AMD Ryzen Threadripper PRO 5975WX 32-Cores
```

### Complete Benchmark Results

| Benchmark | ns/op | allocs/op | B/op |
|-----------|-------|-----------|------|
| **ApproxBetweenness_500nodes_Exact** | 70,263,842 | **499,557** | 34,110,194 |
| ApproxBetweenness_500nodes_Sample100 | 13,586,671 | 199,548 | 29,574,627 |
| ApproxBetweenness_500nodes_Sample50 | 5,568,698 | 100,756 | 14,841,955 |
| FullAnalysis_Sparse500 | 14,232,742 | 82,316 | 26,428,791 |
| GenerateAllSuggestions_Medium | 6,927,667 | 49,278 | 3,850,163 |
| Cycles_ManyCycles20 | 149,467,208 | 294,826 | 120,980,422 |
| Cycles_ManyCycles30 | 500,355,711 | 3,335,574 | 1,824,125,368 |
| DetectCycleWarnings_Medium | 222,044 | 1,435 | 140,619 |
| FindCyclesSafe_Large | 141,035 | 1,417 | 103,872 |
| SuggestLabels_LargeSet | 1,597,699 | 11,341 | 602,962 |
| Complete_Betweenness15 | 259,190 | 3,867 | 1,621,209 |
| Complete_PageRank20 | 35,544 | 545 | 205,901 |

### Key Observations

1. **Betweenness computation dominates**: 500K allocations for exact mode
2. **Linear scaling with sample size**: 100 samples = 200K allocs, 50 samples = 100K allocs
3. **Pathological cases bounded by timeout**: Cycles_ManyCycles30 hits 500ms timeout

---

## Phase B: Profiling Results

### CPU Profile (138.02s total sample time)

| Hotspot | Time (s) | % | Category |
|---------|----------|---|----------|
| runtime.gcDrain | 44.04 | 31.9% | GC overhead |
| runtime.mapassign_fast64 | 39.38 | 28.5% | Map operations |
| **singleSourceBetweenness** | 68.01 | **49.3%** | Core algorithm |
| runtime.scanobject | 18.24 | 13.2% | GC scanning |
| runtime.memclrNoHeapPointers | 9.78 | 7.1% | Memory clearing |
| internal/runtime/maps.table.grow | 26.31 | 19.1% | Map growth |

**Root Cause**: ~45% GC proper (gcDrain + scanobject) + ~48% map operations. These overlap since map allocations trigger GC.

### Memory Profile (41.34GB total allocations)

| Allocator | Memory (MB) | % |
|-----------|-------------|---|
| **singleSourceBetweenness** (direct) | 29,463 | **71.3%** |
| gonum iterators (inside singleSourceBetweenness) | ~8,000 | ~19% |
| Other | ~4,000 | ~10% |

**Note**: The 71.3% represents DIRECT allocations (4 maps). The gonum iterator allocations (~19%) occur during `g.From(v)` calls INSIDE singleSourceBetweenness but are attributed to gonum. Total during singleSourceBetweenness execution: **~90%**.

**Root Cause**: `singleSourceBetweenness` creates 4 fresh maps per call:

```go
sigma := make(map[int64]float64)  // N entries
dist := make(map[int64]int)       // N entries
delta := make(map[int64]float64)  // N entries
pred := make(map[int64][]int64)   // N entries + dynamic slices
```

For 500 nodes × 100 samples = 400 maps with 200K+ entries allocated per operation.

---

## Phase C: Equivalence Oracles

### Existing Golden Tests

Location: `pkg/analysis/golden_test.go`

The project has robust golden file validation:

```go
// Tolerances defined in validateMapFloat64()
PageRank:          1e-5  // Iterative convergence variance
Betweenness:       1e-6  // Exact algorithm
Eigenvector:       1e-6
Hubs/Authorities:  1e-6
CriticalPathScore: 1e-6
```

### Invariants

1. **Betweenness bounds**: `0 ≤ BC(v) ≤ (n-1)(n-2)` for directed graphs (note: `/2` for undirected)
2. **PageRank sum**: `Σ PR(v) = 1.0`
3. **Deterministic ordering**: Sorted by value descending, then by ID ascending
4. **Map completeness**: All node IDs present in output maps

### Property-Based Tests (Recommended Addition)

```go
// Requires: go get pgregory.net/rapid
func TestBetweennessScoreRange(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := rapid.IntRange(5, 100).Draw(t, "nodes")
        issues := generateSparseGraph(n)  // Helper to generate test issues
        analyzer := NewAnalyzer(issues)
        stats := analyzer.Analyze()

        // For directed graphs: max BC is (n-1)(n-2), not /2
        maxBC := float64((n - 1) * (n - 2))
        for id, score := range stats.Betweenness() {
            if score < 0 || score > maxBC {
                t.Errorf("BC[%s] = %f out of bounds [0, %f]", id, score, maxBC)
            }
        }
    })
}
```

---

## Phase D: Opportunity Matrix

| # | Candidate | Impact | Confidence | Effort | Score (I×C/E) | Risk |
|---|-----------|--------|------------|--------|---------------|------|
| **1** | **Buffer pooling for Brandes** | **0.70** | **0.95** | **0.40** | **1.66** | LOW |
| 2 | Array-based indexing (no maps) | 0.50 | 0.90 | 0.50 | 0.90 | LOW |
| 3 | Cached adjacency lists | 0.30 | 0.70 | 0.60 | 0.35 | MED |
| 4 | Avoid gonum iterators | 0.22 | 0.60 | 0.70 | 0.19 | MED |

### Scoring Methodology

- **Impact**: Fraction of total allocation/CPU that would be eliminated
- **Confidence**: Certainty that the optimization will achieve stated impact
- **Effort**: Relative implementation complexity (0.1 = trivial, 1.0 = major refactor)
- **Score**: Higher is better; prioritize items with Score > 0.5

---

## Phase E: Proposed Change - Priority #1

### Buffer Pooling for Brandes' Algorithm

**Location**: `pkg/analysis/betweenness_approx.go:162-241`

**Current Implementation**:
```go
func singleSourceBetweenness(g *simple.DirectedGraph, source graph.Node, bc map[int64]float64) {
    sourceID := source.ID()
    nodes := graph.NodesOf(g.Nodes())  // Still allocates (not pooled)

    // ALLOCATES 4 FRESH MAPS PER CALL - THIS IS THE PROBLEM
    sigma := make(map[int64]float64)
    dist := make(map[int64]int)
    delta := make(map[int64]float64)
    pred := make(map[int64][]int64)

    // ... rest of algorithm
}
```

**Proposed Implementation**:
```go
// brandesBuffers holds reusable data structures for Brandes' algorithm.
type brandesBuffers struct {
    sigma     map[int64]float64
    dist      map[int64]int
    delta     map[int64]float64
    pred      map[int64][]int64
    queue     []int64
    stack     []int64
    neighbors []int64
}

var brandesPool = sync.Pool{
    New: func() interface{} {
        return &brandesBuffers{
            sigma:     make(map[int64]float64, 256),
            dist:      make(map[int64]int, 256),
            delta:     make(map[int64]float64, 256),
            pred:      make(map[int64][]int64, 256),
            queue:     make([]int64, 0, 256),
            stack:     make([]int64, 0, 256),
            neighbors: make([]int64, 0, 32),
        }
    },
}

// reset clears all values but retains map/slice capacity.
func (b *brandesBuffers) reset(nodes []graph.Node) {
    // Clear maps if they've grown too large (prevents unbounded memory growth)
    if len(b.sigma) > len(nodes)*2 {
        clear(b.sigma)
        clear(b.dist)
        clear(b.delta)
        clear(b.pred)
    }

    for _, n := range nodes {
        nid := n.ID()
        b.sigma[nid] = 0
        b.dist[nid] = -1
        b.delta[nid] = 0
        // Reuse slice backing array, clear length
        if existing, ok := b.pred[nid]; ok {
            b.pred[nid] = existing[:0]
        } else {
            b.pred[nid] = make([]int64, 0, 4)  // First-call allocation unavoidable
        }
    }
    b.queue = b.queue[:0]
    b.stack = b.stack[:0]
    b.neighbors = b.neighbors[:0]
}

func singleSourceBetweenness(g *simple.DirectedGraph, source graph.Node, bc map[int64]float64) {
    sourceID := source.ID()
    nodes := graph.NodesOf(g.Nodes())  // Still allocates

    // Get buffer from pool
    buf := brandesPool.Get().(*brandesBuffers)
    defer brandesPool.Put(buf)

    buf.reset(nodes)

    // Use buf.sigma, buf.dist, buf.delta, buf.pred instead of local maps

    // BFS phase - use pooled neighbors slice:
    // buf.neighbors = buf.neighbors[:0]
    // for to.Next() {
    //     buf.neighbors = append(buf.neighbors, to.Node().ID())
    // }

    // ... rest of algorithm unchanged
}
```

### What This Does NOT Address

The proposed optimization addresses the 4 internal maps but does NOT address:

```go
// Line 169 - STILL ALLOCATES (not pooled)
nodes := graph.NodesOf(g.Nodes())

// Line 116 in ApproxBetweenness - STILL ALLOCATES per goroutine
localBC := make(map[int64]float64)

// gonum iterator overhead from g.From(v) calls
```

To achieve 90%+ reduction, would also need to pool the `nodes` slice and `localBC` maps (requires signature changes or Analyzer-level caching).

### Isomorphism Proof

**Theorem**: Buffer reuse produces identical outputs to fresh allocation.

**Proof**:

1. **Initialization Equivalence**:
   - Current: `sigma[nid] = 0`, `dist[nid] = -1`, `delta[nid] = 0`
   - Proposed: Same assignments in `reset()`
   - Since we iterate over ALL nodes and explicitly set values, prior state is irrelevant.

2. **Graph Change Safety**:
   - If node IDs differ between calls, stale entries don't affect output
   - BFS only visits reachable nodes from source
   - All visited nodes are explicitly initialized before use

3. **Predecessor Slice Safety**:
   - Current: `pred[nid] = make([]int64, 0)`
   - Proposed: `pred[nid] = pred[nid][:0]` (reuses backing array)
   - Equivalence: Empty slice has `len=0`. Both produce identical behavior.

4. **Floating-Point Determinism**:
   - All arithmetic operations `(σ_v/σ_w) × (1+δ_w)` are identical
   - IEEE-754 floating-point is deterministic for same inputs
   - Ordering guaranteed by sorted node IDs in BFS/accumulation

5. **Concurrency Safety**:
   - Each goroutine in `ApproxBetweenness` gets its own buffer from pool
   - Buffers are returned AFTER results are merged to `partialBC`
   - `sync.Pool` guarantees no concurrent access to same buffer

6. **Pool Eviction Safety**:
   - If buffer is evicted during GC and recreated, behavior is identical to fresh allocation

**QED** ∎

### Caveats

1. **sync.Pool eviction**: Pool entries can be evicted during GC. Under memory pressure, gains diminish as buffers are recreated.
2. **First-call cost**: First call with a fresh buffer allocates N slices for pred entries. Steady-state performance is better.
3. **Exact path not optimized**: When `sampleSize >= n`, code calls `network.Betweenness(g)` (gonum's implementation). Small graphs (<100 nodes) see no benefit.

---

## Phase F: Minimal Diff

The change is isolated to a single file:

```
pkg/analysis/betweenness_approx.go
  - Add brandesBuffers struct (~25 lines)
  - Add brandesPool with sync.Pool (~10 lines)
  - Add reset() method (~20 lines)
  - Modify singleSourceBetweenness to use pool (~10 line changes)
```

**Total**: ~65 lines added/modified

**No API changes**: Function signature unchanged, output unchanged.

### Rollback Guidance

If issues arise:
1. Remove `brandesPool` and `brandesBuffers`
2. Restore original map allocations in `singleSourceBetweenness`
3. No external code changes required

---

## Phase G: Regression Guardrails

### 1. Allocation Threshold Benchmark

```go
func BenchmarkBrandesAllocationThreshold(b *testing.B) {
    issues := generateSparseGraph(500)
    g := buildGraph(issues)
    nodes := graph.NodesOf(g.Nodes())

    b.ReportAllocs()
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        bc := make(map[int64]float64)
        singleSourceBetweenness(g, nodes[0], bc)
    }

    // After optimization: expect < 100 allocs/op (down from ~1000)
}
```

### 2. Golden Test Coverage

Existing `TestValidateGoldenFiles` in `pkg/analysis/golden_test.go` covers:
- Betweenness scores within 1e-6 tolerance
- All node IDs present in output

### 3. Property Test (New)

```go
func TestBetweennessOutputEquivalence(t *testing.T) {
    // Compare pooled vs fresh allocation outputs
    issues := generateSparseGraph(100)

    // Run with fresh allocation (baseline)
    baseline := computeBetweennessBaseline(issues)

    // Run with pooling (optimized)
    optimized := computeBetweennessOptimized(issues)

    for id, baseVal := range baseline {
        optVal := optimized[id]
        if math.Abs(baseVal-optVal) > 1e-10 {
            t.Errorf("Mismatch for %s: baseline=%v, optimized=%v", id, baseVal, optVal)
        }
    }
}
```

### 4. CI Integration

Add to benchmark CI job:

```yaml
- name: Check allocation regression
  run: |
    go test -bench=BenchmarkApproxBetweenness_500nodes_Sample100 \
      -benchmem ./pkg/analysis/... | tee bench.txt

    # Field positions: 1=name 2=iterations 3=ns/op 4="ns/op" 5=B/op 6="B/op" 7=allocs/op
    ALLOCS=$(grep 'Sample100' bench.txt | awk '{print $7}')
    if [ "$ALLOCS" -gt 50000 ]; then
      echo "Allocation regression: $ALLOCS > 50000"
      exit 1
    fi
```

---

## Expected Gains

### Before Optimization

| Metric | Value |
|--------|-------|
| Allocations/op (Sample100) | 199,550 |
| Bytes/op (Sample100) | 29.5 MB |
| GC CPU overhead | ~45% |

### After Optimization - Conservative (Buffer Pooling Only)

| Metric | Value | Reduction |
|--------|-------|-----------|
| Allocations/op (Sample100) | ~40,000 | **80%** |
| Bytes/op (Sample100) | ~8 MB | **73%** |
| GC CPU overhead | ~15% | **67%** |
| Throughput | 1.5-2x | - |

### After Optimization - With Additional Caching

If combined with node slice caching and localBC pooling:

| Metric | Value | Reduction |
|--------|-------|-----------|
| Allocations/op (Sample100) | ~5,000 | **97%** |
| Bytes/op (Sample100) | ~3 MB | **90%** |
| GC CPU overhead | ~5% | **89%** |
| Throughput | 3-5x | - |

---

## Additional Opportunities (Lower Priority)

### Priority #2: Array-Based Indexing

Replace `map[int64]float64` with `[]float64` indexed by node position:

```go
// Current: O(1) average, but hash overhead + GC pressure
sigma := make(map[int64]float64)
sigma[nodeID] = value

// Proposed: O(1) guaranteed, cache-friendly, no GC pressure
indexOf := buildNodeIndex(nodes)  // map[int64]int, built once
sigma := make([]float64, len(nodes))
sigma[indexOf[nodeID]] = value
```

**Trade-off**: Requires one-time index construction per graph.

### Priority #3: Cached Adjacency Lists

Currently, `g.From(v)` creates a new iterator each call (~19% of allocations). Cache adjacency:

```go
type cachedGraph struct {
    *simple.DirectedGraph
    adj map[int64][]int64  // Pre-built adjacency lists
}

func (c *cachedGraph) From(id int64) []int64 {
    return c.adj[id]  // No allocation
}
```

### Priority #4: Semiring Generalization

PageRank, betweenness, and transitive closure can all be expressed as matrix operations over a semiring. This enables:
- SIMD/vectorization via gonum's BLAS
- Unified caching of matrix structure
- Potential GPU offload for very large graphs

---

## Conclusion

This analysis identified that **~90% of memory allocations** during betweenness computation come from `singleSourceBetweenness` (71% direct + 19% iterator overhead).

The proposed buffer pooling optimization:
- ✅ Has a proven isomorphism (outputs cannot change)
- ✅ Is a minimal diff (~65 lines, single file)
- ✅ Has existing test coverage via golden files
- ✅ Has clear rollback path
- ✅ Projects **80% allocation reduction** (conservative), **97%** with additional caching

**Recommendation**: Implement Priority #1 (buffer pooling), re-profile, then evaluate Priorities #2-3 based on updated baseline.

---

## Appendix: Raw Profiling Commands

```bash
# Run benchmarks with memory stats
go test -bench=. -benchmem -count=3 ./pkg/analysis/... 2>&1 | tee bench_baseline.txt

# Generate CPU profile
go test -run=NONE -bench="BenchmarkApproxBetweenness" \
  -cpuprofile=cpu.prof -benchtime=3s ./pkg/analysis/...

# Generate memory profile
go test -run=NONE -bench="BenchmarkApproxBetweenness" \
  -memprofile=mem.prof -benchtime=3s ./pkg/analysis/...

# Analyze CPU profile
go tool pprof -top cpu.prof | head -40

# Analyze memory profile
go tool pprof -top mem.prof | head -40

# Interactive profile exploration
go tool pprof -http=:8080 cpu.prof
```

---

*Generated: 2026-01-09*
*Author: Claude Code (performance analysis session)*
