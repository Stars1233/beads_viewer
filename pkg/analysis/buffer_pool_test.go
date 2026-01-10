package analysis

import (
	"runtime"
	"sync"
	"testing"

	"gonum.org/v1/gonum/graph"
)

// mockNode implements graph.Node for testing
type mockNode struct {
	id int64
}

func (n mockNode) ID() int64 { return n.id }

// createTestBuffer creates a brandesBuffers with test data
func createTestBuffer() *brandesBuffers {
	return &brandesBuffers{
		sigma:     make(map[int64]float64, 256),
		dist:      make(map[int64]int, 256),
		delta:     make(map[int64]float64, 256),
		pred:      make(map[int64][]int64, 256),
		queue:     make([]int64, 0, 256),
		stack:     make([]int64, 0, 256),
		neighbors: make([]int64, 0, 32),
	}
}

// =============================================================================
// brandesBuffers Struct Tests
// =============================================================================

// TestBrandesBuffersInitialization verifies struct creation
func TestBrandesBuffersInitialization(t *testing.T) {
	t.Log("Testing brandesBuffers struct initialization...")

	buf := createTestBuffer()

	t.Logf("Created buffer with queue capacity: %d", cap(buf.queue))

	if buf.sigma == nil {
		t.Fatal("sigma map should be initialized")
	}
	if buf.dist == nil {
		t.Fatal("dist map should be initialized")
	}
	if buf.delta == nil {
		t.Fatal("delta map should be initialized")
	}
	if buf.pred == nil {
		t.Fatal("pred map should be initialized")
	}
	if cap(buf.queue) != 256 {
		t.Errorf("queue capacity: got %d, want 256", cap(buf.queue))
	}
	if cap(buf.stack) != 256 {
		t.Errorf("stack capacity: got %d, want 256", cap(buf.stack))
	}
	if cap(buf.neighbors) != 32 {
		t.Errorf("neighbors capacity: got %d, want 32", cap(buf.neighbors))
	}

	t.Log("PASS: All fields initialized correctly")
}

// =============================================================================
// reset() Method Tests
// =============================================================================

// TestResetClearsAllValues verifies reset produces clean state
func TestResetClearsAllValues(t *testing.T) {
	t.Log("Testing reset() clears all values...")

	// Create buffer with stale data
	buf := createTestBuffer()
	buf.sigma[1] = 999.0
	buf.dist[1] = 999
	buf.delta[1] = 999.0
	buf.pred[1] = []int64{1, 2, 3}
	buf.queue = append(buf.queue, 1, 2, 3)
	buf.stack = append(buf.stack, 4, 5, 6)

	t.Logf("Before reset: sigma[1]=%v, dist[1]=%v, queue len=%d",
		buf.sigma[1], buf.dist[1], len(buf.queue))

	// Create mock nodes
	nodes := []graph.Node{mockNode{id: 1}, mockNode{id: 2}}

	// Reset
	buf.reset(nodes)

	t.Logf("After reset: sigma[1]=%v, dist[1]=%v, queue len=%d",
		buf.sigma[1], buf.dist[1], len(buf.queue))

	// Verify reset state matches fresh allocation
	if buf.sigma[1] != 0.0 {
		t.Errorf("sigma[1] should be 0 after reset, got %v", buf.sigma[1])
	}
	if buf.dist[1] != -1 {
		t.Errorf("dist[1] should be -1 after reset, got %v", buf.dist[1])
	}
	if buf.delta[1] != 0.0 {
		t.Errorf("delta[1] should be 0 after reset, got %v", buf.delta[1])
	}
	if len(buf.pred[1]) != 0 {
		t.Errorf("pred[1] should be empty after reset, got %v", buf.pred[1])
	}
	if len(buf.queue) != 0 {
		t.Errorf("queue should be empty after reset, got len %d", len(buf.queue))
	}
	if len(buf.stack) != 0 {
		t.Errorf("stack should be empty after reset, got len %d", len(buf.stack))
	}

	t.Log("PASS: reset() produces correct initial state")
}

// TestResetRetainsPredCapacity verifies pred slices retain capacity
func TestResetRetainsPredCapacity(t *testing.T) {
	t.Log("Testing reset() retains predecessor slice capacity...")

	buf := createTestBuffer()
	nodes := []graph.Node{mockNode{id: 1}}

	// First reset - allocates small slice
	buf.reset(nodes)
	t.Logf("After first reset: pred[1] cap=%d", cap(buf.pred[1]))

	// Add predecessors to grow slice
	buf.pred[1] = append(buf.pred[1], 10, 20, 30, 40, 50)
	oldCap := cap(buf.pred[1])
	t.Logf("After appends: pred[1] cap=%d", oldCap)

	// Reset again - should retain capacity
	buf.reset(nodes)
	newCap := cap(buf.pred[1])
	t.Logf("After second reset: pred[1] cap=%d", newCap)

	if newCap < oldCap {
		t.Errorf("pred capacity should be retained: got %d, want >= %d", newCap, oldCap)
	}
	if len(buf.pred[1]) != 0 {
		t.Errorf("pred length should be 0 after reset, got %d", len(buf.pred[1]))
	}

	t.Log("PASS: reset() retains predecessor slice capacity")
}

// TestResetTriggersClearOnOversizedMaps verifies 2x threshold
func TestResetTriggersClearOnOversizedMaps(t *testing.T) {
	t.Log("Testing reset() triggers clear() on oversized maps...")

	buf := createTestBuffer()

	// Grow maps very large
	for i := int64(0); i < 5000; i++ {
		buf.sigma[i] = float64(i)
		buf.dist[i] = int(i)
		buf.delta[i] = float64(i)
		buf.pred[i] = []int64{i}
	}
	t.Logf("Grew maps to %d entries", len(buf.sigma))

	// Reset with tiny node set (should trigger clear due to 2x threshold)
	nodes := []graph.Node{mockNode{id: 0}, mockNode{id: 1}}
	buf.reset(nodes)

	t.Logf("After reset with 2 nodes: sigma has %d entries", len(buf.sigma))

	// Should have been cleared and only 2 entries remain
	if len(buf.sigma) != 2 {
		t.Errorf("oversized map should be cleared: got %d entries, want 2", len(buf.sigma))
	}
	if len(buf.dist) != 2 {
		t.Errorf("dist map should be cleared: got %d entries, want 2", len(buf.dist))
	}

	t.Log("PASS: clear() triggered for oversized maps")
}

// TestResetHandlesEmptyNodes verifies reset with empty node slice
func TestResetHandlesEmptyNodes(t *testing.T) {
	t.Log("Testing reset() with empty node slice...")

	buf := createTestBuffer()
	buf.sigma[1] = 999.0

	nodes := []graph.Node{}
	buf.reset(nodes)

	if len(buf.queue) != 0 {
		t.Errorf("queue should be empty, got len %d", len(buf.queue))
	}
	if len(buf.stack) != 0 {
		t.Errorf("stack should be empty, got len %d", len(buf.stack))
	}

	t.Log("PASS: reset() handles empty node slice")
}

// =============================================================================
// Pool Behavior Tests
// =============================================================================

// TestPoolReturnsNonNilBuffer verifies pool.Get() works
func TestPoolReturnsNonNilBuffer(t *testing.T) {
	t.Log("Testing brandesPool.Get() returns valid buffer...")

	for i := 0; i < 10; i++ {
		buf := brandesPool.Get().(*brandesBuffers)
		if buf == nil {
			t.Fatal("pool should never return nil")
		}
		t.Logf("Got buffer %d: sigma=%p", i, buf.sigma)
		brandesPool.Put(buf)
	}

	t.Log("PASS: Pool consistently returns valid buffers")
}

// TestPoolPreallocation verifies pool's New() function allocates correctly
func TestPoolPreallocation(t *testing.T) {
	t.Log("Testing pool preallocation capacities...")

	// Force pool to create new buffer by exhausting any cached ones
	buffers := make([]*brandesBuffers, 10)
	for i := range buffers {
		buffers[i] = brandesPool.Get().(*brandesBuffers)
	}

	// Check one of them
	buf := buffers[0]

	if cap(buf.queue) < 256 {
		t.Errorf("queue capacity should be >= 256, got %d", cap(buf.queue))
	}
	if cap(buf.stack) < 256 {
		t.Errorf("stack capacity should be >= 256, got %d", cap(buf.stack))
	}
	if cap(buf.neighbors) < 32 {
		t.Errorf("neighbors capacity should be >= 32, got %d", cap(buf.neighbors))
	}

	// Return all buffers
	for _, b := range buffers {
		brandesPool.Put(b)
	}

	t.Log("PASS: Pool preallocation correct")
}

// TestPoolEvictionRecovery verifies behavior after GC
func TestPoolEvictionRecovery(t *testing.T) {
	t.Log("Testing pool recovery after GC eviction...")

	// Get and return a buffer
	buf1 := brandesPool.Get().(*brandesBuffers)
	buf1.sigma[42] = 3.14
	brandesPool.Put(buf1)

	t.Log("Forcing GC to potentially evict pool entries...")
	runtime.GC()
	runtime.GC()

	// Get buffer again - might be new or recycled
	buf2 := brandesPool.Get().(*brandesBuffers)
	if buf2 == nil {
		t.Fatal("pool must return buffer even after GC")
	}

	// Key point: behavior is correct regardless of whether buf1 == buf2
	t.Logf("Got buffer after GC: sigma=%p (may or may not be same)", buf2.sigma)

	brandesPool.Put(buf2)
	t.Log("PASS: Pool handles GC eviction gracefully")
}

// =============================================================================
// Equivalence to Fresh Allocation Tests
// =============================================================================

// TestResetEquivalentToFreshAllocation is the KEY isomorphism test
func TestResetEquivalentToFreshAllocation(t *testing.T) {
	t.Log("Testing that reset() produces state equivalent to fresh allocation...")

	nodes := []graph.Node{mockNode{id: 1}, mockNode{id: 2}, mockNode{id: 3}}

	// Fresh allocation (baseline)
	fresh := &brandesBuffers{
		sigma: make(map[int64]float64),
		dist:  make(map[int64]int),
		delta: make(map[int64]float64),
		pred:  make(map[int64][]int64),
	}
	for _, n := range nodes {
		nid := n.ID()
		fresh.sigma[nid] = 0
		fresh.dist[nid] = -1
		fresh.delta[nid] = 0
		fresh.pred[nid] = make([]int64, 0)
	}

	// Pooled + reset (optimized)
	pooled := brandesPool.Get().(*brandesBuffers)
	pooled.sigma[999] = 999.0 // Add stale data
	pooled.dist[999] = 999
	pooled.delta[999] = 999.0
	pooled.reset(nodes)

	// Compare
	for _, n := range nodes {
		nid := n.ID()
		t.Logf("Node %d: fresh sigma=%v, pooled sigma=%v", nid, fresh.sigma[nid], pooled.sigma[nid])

		if fresh.sigma[nid] != pooled.sigma[nid] {
			t.Errorf("sigma mismatch for node %d: fresh=%v, pooled=%v", nid, fresh.sigma[nid], pooled.sigma[nid])
		}
		if fresh.dist[nid] != pooled.dist[nid] {
			t.Errorf("dist mismatch for node %d: fresh=%v, pooled=%v", nid, fresh.dist[nid], pooled.dist[nid])
		}
		if fresh.delta[nid] != pooled.delta[nid] {
			t.Errorf("delta mismatch for node %d: fresh=%v, pooled=%v", nid, fresh.delta[nid], pooled.delta[nid])
		}
		if len(fresh.pred[nid]) != len(pooled.pred[nid]) {
			t.Errorf("pred len mismatch for node %d: fresh=%d, pooled=%d", nid, len(fresh.pred[nid]), len(pooled.pred[nid]))
		}
	}

	brandesPool.Put(pooled)
	t.Log("PASS: reset() produces state equivalent to fresh allocation")
}

// TestStaleEntriesNotAccessible verifies stale entries don't affect correctness
func TestStaleEntriesNotAccessible(t *testing.T) {
	t.Log("Testing that stale entries from previous usage don't affect results...")

	buf := createTestBuffer()

	// Simulate first usage with many nodes
	oldNodes := make([]graph.Node, 100)
	for i := range oldNodes {
		oldNodes[i] = mockNode{id: int64(i)}
	}
	buf.reset(oldNodes)

	// Add some values
	for i := int64(0); i < 100; i++ {
		buf.sigma[i] = float64(i * 10)
		buf.dist[i] = int(i)
	}
	t.Logf("Set values for 100 nodes")

	// Now reset with smaller set
	newNodes := []graph.Node{mockNode{id: 5}, mockNode{id: 10}}
	buf.reset(newNodes)

	// Only nodes 5 and 10 should have fresh values
	if buf.sigma[5] != 0.0 {
		t.Errorf("sigma[5] should be 0, got %v", buf.sigma[5])
	}
	if buf.dist[5] != -1 {
		t.Errorf("dist[5] should be -1, got %v", buf.dist[5])
	}
	if buf.sigma[10] != 0.0 {
		t.Errorf("sigma[10] should be 0, got %v", buf.sigma[10])
	}

	// Note: stale entries for other nodes may still exist but won't be accessed
	// by the algorithm since only nodes in the current graph are traversed
	t.Log("PASS: Stale entries don't affect active node values")
}

// =============================================================================
// Slice Behavior Tests
// =============================================================================

// TestSliceCapacityRetention verifies queue/stack retain capacity
func TestSliceCapacityRetention(t *testing.T) {
	t.Log("Testing slice capacity retention across resets...")

	buf := createTestBuffer()
	nodes := []graph.Node{mockNode{id: 1}}
	buf.reset(nodes)

	// Grow queue and stack
	for i := int64(0); i < 500; i++ {
		buf.queue = append(buf.queue, i)
		buf.stack = append(buf.stack, i)
	}
	queueCap := cap(buf.queue)
	stackCap := cap(buf.stack)
	t.Logf("Grew slices: queue cap=%d, stack cap=%d", queueCap, stackCap)

	// Reset
	buf.reset(nodes)

	// Capacity should be retained
	if cap(buf.queue) < queueCap {
		t.Errorf("queue capacity decreased: got %d, want >= %d", cap(buf.queue), queueCap)
	}
	if cap(buf.stack) < stackCap {
		t.Errorf("stack capacity decreased: got %d, want >= %d", cap(buf.stack), stackCap)
	}
	if len(buf.queue) != 0 {
		t.Errorf("queue length should be 0, got %d", len(buf.queue))
	}
	if len(buf.stack) != 0 {
		t.Errorf("stack length should be 0, got %d", len(buf.stack))
	}

	t.Log("PASS: Slice capacity retained, length reset")
}

// =============================================================================
// Concurrent Access / Race Condition Tests
// =============================================================================

// TestBufferPoolConcurrentAccess verifies no races under heavy concurrent load.
// Run with: go test -race -run TestBufferPoolConcurrentAccess -count=10
func TestBufferPoolConcurrentAccess(t *testing.T) {
	t.Log("Testing buffer pool concurrent access...")

	const numGoroutines = 50
	const iterationsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				// Get buffer, use it, return it
				buf := brandesPool.Get().(*brandesBuffers)
				if buf == nil {
					t.Error("Got nil buffer in concurrent access")
					return
				}

				// Simulate work
				nodes := []graph.Node{mockNode{id: int64(workerID*1000 + j)}}
				buf.reset(nodes)
				buf.sigma[int64(workerID*1000+j)] = float64(j)
				buf.queue = append(buf.queue, int64(j))

				brandesPool.Put(buf)
			}
		}(i)
	}

	wg.Wait()
	t.Logf("PASS: Completed %d concurrent operations without race",
		numGoroutines*iterationsPerGoroutine)
}

// TestBufferPoolLifecycle verifies correct Get/Put semantics
func TestBufferPoolLifecycle(t *testing.T) {
	t.Log("Testing buffer pool lifecycle...")

	// Get a buffer
	buf1 := brandesPool.Get().(*brandesBuffers)
	if buf1 == nil {
		t.Fatal("First Get returned nil")
	}

	// Modify it
	buf1.sigma[42] = 1.5
	buf1.queue = append(buf1.queue, 100)
	t.Logf("Modified buffer: sigma[42]=%v, queue=%v", buf1.sigma[42], buf1.queue)

	// Return it
	brandesPool.Put(buf1)

	// Get again - might be same buffer or new one
	buf2 := brandesPool.Get().(*brandesBuffers)
	if buf2 == nil {
		t.Fatal("Second Get returned nil")
	}

	// Key invariant: no panic, no race
	t.Logf("Got second buffer: sigma=%p", buf2.sigma)
	brandesPool.Put(buf2)

	t.Log("PASS: Pool lifecycle works correctly")
}

// TestConcurrentPoolGetPut tests rapid Get/Put cycles
func TestConcurrentPoolGetPut(t *testing.T) {
	t.Log("Testing rapid concurrent Get/Put cycles...")

	const cycles = 1000
	const workers = 10

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < cycles; i++ {
				buf := brandesPool.Get().(*brandesBuffers)
				if buf == nil {
					t.Error("Got nil buffer")
					return
				}
				// Immediately return
				brandesPool.Put(buf)
			}
		}()
	}

	wg.Wait()
	t.Logf("PASS: Completed %d rapid Get/Put cycles without race", cycles*workers)
}
