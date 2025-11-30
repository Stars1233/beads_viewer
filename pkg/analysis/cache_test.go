package analysis_test

import (
	"testing"
	"time"

	"beads_viewer/pkg/analysis"
	"beads_viewer/pkg/model"
)

func TestComputeDataHash_Empty(t *testing.T) {
	hash := analysis.ComputeDataHash(nil)
	if hash != "empty" {
		t.Errorf("Expected 'empty' for nil issues, got %s", hash)
	}

	hash = analysis.ComputeDataHash([]model.Issue{})
	if hash != "empty" {
		t.Errorf("Expected 'empty' for empty slice, got %s", hash)
	}
}

func TestComputeDataHash_Deterministic(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "One"},
		{ID: "B", Title: "Two"},
	}

	hash1 := analysis.ComputeDataHash(issues)
	hash2 := analysis.ComputeDataHash(issues)

	if hash1 != hash2 {
		t.Errorf("Hash should be deterministic: %s != %s", hash1, hash2)
	}
}

func TestComputeDataHash_OrderIndependent(t *testing.T) {
	issues1 := []model.Issue{
		{ID: "A", Title: "One"},
		{ID: "B", Title: "Two"},
	}
	issues2 := []model.Issue{
		{ID: "B", Title: "Two"},
		{ID: "A", Title: "One"},
	}

	hash1 := analysis.ComputeDataHash(issues1)
	hash2 := analysis.ComputeDataHash(issues2)

	if hash1 != hash2 {
		t.Errorf("Hash should be order-independent: %s != %s", hash1, hash2)
	}
}

func TestComputeDataHash_DifferentData(t *testing.T) {
	issues1 := []model.Issue{{ID: "A", Title: "Alpha"}}
	issues2 := []model.Issue{{ID: "A", Title: "Beta"}}  // title change
	issues3 := []model.Issue{{ID: "B", Title: "Alpha"}} // id change

	hash1 := analysis.ComputeDataHash(issues1)
	hash2 := analysis.ComputeDataHash(issues2)
	hash3 := analysis.ComputeDataHash(issues3)

	if hash1 == hash2 {
		t.Error("Different content hashes should produce different hashes")
	}
	if hash1 == hash3 {
		t.Error("Different IDs should produce different hashes")
	}
}

func TestComputeDataHash_Dependencies(t *testing.T) {
	issues1 := []model.Issue{{
		ID: "A",
		Dependencies: []*model.Dependency{
			{DependsOnID: "B", Type: model.DepBlocks},
		},
	}}
	issues2 := []model.Issue{{
		ID:           "A",
		Dependencies: nil,
	}}

	hash1 := analysis.ComputeDataHash(issues1)
	hash2 := analysis.ComputeDataHash(issues2)

	if hash1 == hash2 {
		t.Error("Different dependencies should produce different hashes")
	}
}

func TestCache_GetSet(t *testing.T) {
	cache := analysis.NewCache(5 * time.Minute)
	issues := []model.Issue{{ID: "A"}}

	// Initially empty
	stats, ok := cache.Get(issues)
	if ok || stats != nil {
		t.Error("Cache should be empty initially")
	}

	// Create and cache stats
	an := analysis.NewAnalyzer(issues)
	graphStats := an.AnalyzeAsync()
	graphStats.WaitForPhase2()

	cache.Set(issues, graphStats)

	// Should hit cache
	cached, ok := cache.Get(issues)
	if !ok {
		t.Error("Cache should hit after Set")
	}
	if cached != graphStats {
		t.Error("Cached stats should match original")
	}
}

func TestCache_HashMismatch(t *testing.T) {
	cache := analysis.NewCache(5 * time.Minute)
	issues1 := []model.Issue{{ID: "A"}}
	issues2 := []model.Issue{{ID: "B"}}

	an := analysis.NewAnalyzer(issues1)
	graphStats := an.AnalyzeAsync()
	graphStats.WaitForPhase2()

	cache.Set(issues1, graphStats)

	// Different issues should miss
	cached, ok := cache.Get(issues2)
	if ok || cached != nil {
		t.Error("Cache should miss for different data")
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	cache := analysis.NewCache(50 * time.Millisecond)
	issues := []model.Issue{{ID: "A"}}

	an := analysis.NewAnalyzer(issues)
	graphStats := an.AnalyzeAsync()
	graphStats.WaitForPhase2()

	cache.Set(issues, graphStats)

	// Should hit immediately
	_, ok := cache.Get(issues)
	if !ok {
		t.Error("Cache should hit immediately after Set")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should miss after TTL
	_, ok = cache.Get(issues)
	if ok {
		t.Error("Cache should miss after TTL expires")
	}
}

func TestCache_Invalidate(t *testing.T) {
	cache := analysis.NewCache(5 * time.Minute)
	issues := []model.Issue{{ID: "A"}}

	an := analysis.NewAnalyzer(issues)
	graphStats := an.AnalyzeAsync()
	graphStats.WaitForPhase2()

	cache.Set(issues, graphStats)

	// Should hit
	_, ok := cache.Get(issues)
	if !ok {
		t.Error("Cache should hit after Set")
	}

	// Invalidate
	cache.Invalidate()

	// Should miss after invalidate
	_, ok = cache.Get(issues)
	if ok {
		t.Error("Cache should miss after Invalidate")
	}
}

func TestCache_Stats(t *testing.T) {
	cache := analysis.NewCache(5 * time.Minute)
	issues := []model.Issue{{ID: "A"}}

	// Initially no data
	_, _, hasData := cache.Stats()
	if hasData {
		t.Error("Should have no data initially")
	}

	an := analysis.NewAnalyzer(issues)
	graphStats := an.AnalyzeAsync()
	graphStats.WaitForPhase2()

	cache.Set(issues, graphStats)

	hash, age, hasData := cache.Stats()
	if !hasData {
		t.Error("Should have data after Set")
	}
	if hash == "" {
		t.Error("Hash should not be empty")
	}
	if age < 0 || age > time.Second {
		t.Errorf("Age should be reasonable: %v", age)
	}
}

func TestCachedAnalyzer_CacheHit(t *testing.T) {
	cache := analysis.NewCache(5 * time.Minute)
	issues := []model.Issue{
		{ID: "A", Status: model.StatusOpen},
		{ID: "B", Status: model.StatusOpen, Dependencies: []*model.Dependency{
			{DependsOnID: "A", Type: model.DepBlocks},
		}},
	}

	// First analysis - cache miss
	ca1 := analysis.NewCachedAnalyzer(issues, cache)
	stats1 := ca1.AnalyzeAsync()
	stats1.WaitForPhase2()

	if ca1.WasCacheHit() {
		t.Error("First analysis should be a cache miss")
	}

	// Wait a bit for cache to be populated
	time.Sleep(10 * time.Millisecond)

	// Second analysis - should hit cache
	ca2 := analysis.NewCachedAnalyzer(issues, cache)
	stats2 := ca2.AnalyzeAsync()

	if !ca2.WasCacheHit() {
		t.Error("Second analysis should be a cache hit")
	}

	// Should return same stats pointer
	if stats1 != stats2 {
		t.Error("Cache hit should return same stats pointer")
	}
}

func TestCachedAnalyzer_CacheMiss_DifferentData(t *testing.T) {
	cache := analysis.NewCache(5 * time.Minute)
	issues1 := []model.Issue{{ID: "A"}}
	issues2 := []model.Issue{{ID: "B"}}

	// First analysis
	ca1 := analysis.NewCachedAnalyzer(issues1, cache)
	stats1 := ca1.AnalyzeAsync()
	stats1.WaitForPhase2()

	// Wait for cache
	time.Sleep(10 * time.Millisecond)

	// Different data - should miss
	ca2 := analysis.NewCachedAnalyzer(issues2, cache)
	stats2 := ca2.AnalyzeAsync()

	if ca2.WasCacheHit() {
		t.Error("Different data should be a cache miss")
	}

	// Should return different stats
	if stats1 == stats2 {
		t.Error("Cache miss should compute new stats")
	}
}

func TestCachedAnalyzer_DataHash(t *testing.T) {
	issues := []model.Issue{{ID: "A", ContentHash: "test"}}
	ca := analysis.NewCachedAnalyzer(issues, nil)

	hash := ca.DataHash()
	expected := analysis.ComputeDataHash(issues)

	if hash != expected {
		t.Errorf("DataHash() = %s, want %s", hash, expected)
	}
}

func TestGlobalCache(t *testing.T) {
	cache := analysis.GetGlobalCache()
	if cache == nil {
		t.Error("Global cache should not be nil")
	}

	// Clear any existing state
	cache.Invalidate()

	issues := []model.Issue{{ID: "test-global"}}
	an := analysis.NewAnalyzer(issues)
	stats := an.AnalyzeAsync()
	stats.WaitForPhase2()

	cache.Set(issues, stats)

	// Should be accessible
	cached, ok := cache.Get(issues)
	if !ok {
		t.Error("Global cache should return cached stats")
	}
	if cached != stats {
		t.Error("Global cache should return same stats")
	}

	// Clean up
	cache.Invalidate()
}
