package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// Cache holds cached analysis results keyed by data hash.
// Thread-safe for concurrent access.
type Cache struct {
	mu         sync.RWMutex
	dataHash   string
	stats      *GraphStats
	computedAt time.Time
	ttl        time.Duration
}

// DefaultCacheTTL is the default time-to-live for cached results.
const DefaultCacheTTL = 5 * time.Minute

// globalCache is the package-level cache instance.
var globalCache = &Cache{
	ttl: DefaultCacheTTL,
}

// GetGlobalCache returns the global cache instance.
func GetGlobalCache() *Cache {
	return globalCache
}

// NewCache creates a new cache with the specified TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		ttl: ttl,
	}
}

// Get retrieves cached stats if the data hash matches and TTL hasn't expired.
// Returns (stats, true) on cache hit, (nil, false) on cache miss.
func (c *Cache) Get(issues []model.Issue) (*GraphStats, bool) {
	// Compute hash outside the lock (expensive operation)
	hash := ComputeDataHash(issues)
	return c.GetByHash(hash)
}

// GetByHash retrieves cached stats if the hash matches and TTL hasn't expired.
// This is more efficient when the hash has already been computed.
func (c *Cache) GetByHash(hash string) (*GraphStats, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.stats == nil {
		return nil, false
	}

	if hash == c.dataHash && time.Since(c.computedAt) < c.ttl {
		return c.stats, true
	}
	return nil, false
}

// Set stores analysis results in the cache.
func (c *Cache) Set(issues []model.Issue, stats *GraphStats) {
	// Compute hash outside the lock (expensive operation)
	hash := ComputeDataHash(issues)
	c.SetByHash(hash, stats)
}

// SetByHash stores analysis results with a pre-computed hash.
func (c *Cache) SetByHash(hash string, stats *GraphStats) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dataHash = hash
	c.stats = stats
	c.computedAt = time.Now()
}

// Invalidate clears the cache.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dataHash = ""
	c.stats = nil
	c.computedAt = time.Time{}
}

// SetTTL updates the cache TTL.
func (c *Cache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
}

// Hash returns the current data hash, or empty string if no cached data.
func (c *Cache) Hash() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dataHash
}

// Stats returns cache statistics for debugging.
func (c *Cache) Stats() (hash string, age time.Duration, hasData bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.stats == nil {
		return "", 0, false
	}
	return c.dataHash, time.Since(c.computedAt), true
}

// ComputeDataHash generates a deterministic hash of issue data.
// The hash includes issue IDs, content hashes, and dependency relationships.
// Issues are sorted by ID to ensure consistent hashing regardless of input order.
func ComputeDataHash(issues []model.Issue) string {
	if len(issues) == 0 {
		return "empty"
	}

	// Sort issues by ID for deterministic ordering
	sorted := make([]model.Issue, len(issues))
	copy(sorted, issues)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	h := sha256.New()
	for _, issue := range sorted {
		// Core identity
		h.Write([]byte(issue.ID))
		h.Write([]byte{0})

		// Important scalar fields
		h.Write([]byte(issue.Title))
		h.Write([]byte{0})
		h.Write([]byte(issue.Description))
		h.Write([]byte{0})
		h.Write([]byte(issue.Notes))
		h.Write([]byte{0})
		h.Write([]byte(issue.Design))
		h.Write([]byte{0})
		h.Write([]byte(issue.AcceptanceCriteria))
		h.Write([]byte{0})
		h.Write([]byte(issue.Assignee))
		h.Write([]byte{0})
		h.Write([]byte(issue.SourceRepo))
		h.Write([]byte{0})
		if issue.ExternalRef != nil {
			h.Write([]byte(*issue.ExternalRef))
		}
		h.Write([]byte{0})

		h.Write([]byte(issue.Status))
		h.Write([]byte{0})
		h.Write([]byte(issue.IssueType))
		h.Write([]byte{0})

		// Numeric fields
		h.Write([]byte(strconv.Itoa(issue.Priority)))
		h.Write([]byte{0})
		if issue.EstimatedMinutes != nil {
			h.Write([]byte(strconv.Itoa(*issue.EstimatedMinutes)))
		}
		h.Write([]byte{0})
		h.Write([]byte(issue.CreatedAt.UTC().Format(time.RFC3339Nano)))
		h.Write([]byte{0})
		h.Write([]byte(issue.UpdatedAt.UTC().Format(time.RFC3339Nano)))
		h.Write([]byte{0})
		if issue.ClosedAt != nil {
			h.Write([]byte(issue.ClosedAt.UTC().Format(time.RFC3339Nano)))
		}
		h.Write([]byte{0})

		// Labels (sorted for determinism)
		if len(issue.Labels) > 0 {
			labels := append([]string(nil), issue.Labels...)
			sort.Strings(labels)
			for _, lbl := range labels {
				h.Write([]byte(lbl))
				h.Write([]byte{0})
			}
		}
		h.Write([]byte{0})

		// Dependencies (sorted)
		if len(issue.Dependencies) > 0 {
			deps := make([]string, 0, len(issue.Dependencies))
			for _, dep := range issue.Dependencies {
				if dep == nil {
					continue
				}
				deps = append(deps, dep.DependsOnID+":"+string(dep.Type))
			}
			sort.Strings(deps)
			for _, dep := range deps {
				h.Write([]byte(dep))
				h.Write([]byte{0})
			}
		}

		h.Write([]byte{1}) // issue separator
	}

	return hex.EncodeToString(h.Sum(nil))[:16] // Use first 16 chars for brevity
}

// ComputeConfigHash generates a deterministic hash of the analysis configuration.
func ComputeConfigHash(config *AnalysisConfig) string {
	if config == nil {
		return "dynamic"
	}
	h := sha256.New()
	// Using %#v is stable enough for configuration struct
	h.Write([]byte(fmt.Sprintf("%#v", *config)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// CachedAnalyzer wraps an Analyzer with caching support.
type CachedAnalyzer struct {
	*Analyzer
	cache      *Cache
	issues     []model.Issue
	dataHash   string // Hash of the issue data
	configHash string // Hash of the configuration
	cacheHit   bool   // Set by AnalyzeAsync to track if it was a cache hit
}

// NewCachedAnalyzer creates an analyzer that checks the cache before computing.
// The Analyzer is always created because it may be needed for GenerateRecommendations
// even on cache hit. Creating the Analyzer (graph building) is O(V+E) which is fast;
// the expensive part is the analysis itself, which we skip on cache hit.
func NewCachedAnalyzer(issues []model.Issue, cache *Cache) *CachedAnalyzer {
	if cache == nil {
		cache = globalCache
	}
	return &CachedAnalyzer{
		Analyzer:   NewAnalyzer(issues),
		cache:      cache,
		issues:     issues,
		dataHash:   ComputeDataHash(issues),
		configHash: "dynamic",
	}
}

// SetConfig updates the analyzer configuration and the configuration hash.
func (ca *CachedAnalyzer) SetConfig(config *AnalysisConfig) {
	ca.Analyzer.SetConfig(config)
	ca.configHash = ComputeConfigHash(config)
}

// AnalyzeAsync returns cached stats if available, otherwise computes and caches.
func (ca *CachedAnalyzer) AnalyzeAsync() *GraphStats {
	// Combined key: dataHash|configHash
	fullHash := ca.dataHash + "|" + ca.configHash

	// Check cache first
	if stats, ok := ca.cache.GetByHash(fullHash); ok {
		ca.cacheHit = true
		return stats
	}

	// Cache miss - compute fresh
	ca.cacheHit = false
	stats := ca.Analyzer.AnalyzeAsync()

	// Store in cache when Phase 2 completes
	go func() {
		stats.WaitForPhase2()
		ca.cache.SetByHash(fullHash, stats)
	}()

	return stats
}

// Analyze returns cached stats if available, otherwise computes synchronously.
// Note: This returns a value copy that shares map references with the original.
// This is safe because the maps are immutable after Phase 2 completion.
func (ca *CachedAnalyzer) Analyze() GraphStats {
	stats := ca.AnalyzeAsync()
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

// DataHash returns the computed hash for the analyzer's issue data.
func (ca *CachedAnalyzer) DataHash() string {
	return ca.dataHash
}

// WasCacheHit returns true if the last AnalyzeAsync call was a cache hit.
func (ca *CachedAnalyzer) WasCacheHit() bool {
	return ca.cacheHit
}
