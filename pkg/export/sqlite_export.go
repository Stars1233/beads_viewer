// Package export provides SQLite-based data export for static viewer deployment.
//
// This file implements the SQLiteExporter which exports bv's issue data to a SQLite
// database optimized for client-side querying with sql.js WASM.
package export

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteExporter exports bv data to a SQLite database for static deployment.
type SQLiteExporter struct {
	Issues  []*model.Issue
	Deps    []*model.Dependency
	Metrics map[string]*model.IssueMetrics
	Stats   *analysis.GraphStats
	Triage  *analysis.TriageResult
	Config  SQLiteExportConfig
	gitHash string
}

// NewSQLiteExporter creates a new exporter with the given data.
// The third parameter may be either:
//   - map[string]*model.IssueMetrics (explicit metrics)
//   - *analysis.GraphStats (for computed metrics)
//
// This keeps backward compatibility with legacy call sites/tests.
func NewSQLiteExporter(issues []*model.Issue, deps []*model.Dependency, metricsOrStats interface{}, triage *analysis.TriageResult) *SQLiteExporter {
	exp := &SQLiteExporter{
		Issues: issues,
		Deps:   deps,
		Triage: triage,
		Config: DefaultSQLiteExportConfig(),
	}
	switch v := metricsOrStats.(type) {
	case map[string]*model.IssueMetrics:
		exp.Metrics = v
	case *analysis.GraphStats:
		exp.Stats = v
	case nil:
		// nothing
	default:
		// ignore unsupported type to avoid panics in callers
	}
	return exp
}

// SetGitHash sets the git commit hash for export metadata.
func (e *SQLiteExporter) SetGitHash(hash string) {
	e.gitHash = hash
}

// Export writes the SQLite database and supporting files to the output directory.
func (e *SQLiteExporter) Export(outputDir string) error {
	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Create data subdirectory for JSON outputs
	dataDir := filepath.Join(outputDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(outputDir, "beads.sqlite3")

	// Remove existing database if present
	_ = os.Remove(dbPath)

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Create schema
	if err := CreateSchema(db); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// Insert issues
	if err := e.insertIssues(db); err != nil {
		return fmt.Errorf("insert issues: %w", err)
	}

	// Insert dependencies
	if err := e.insertDependencies(db); err != nil {
		return fmt.Errorf("insert dependencies: %w", err)
	}

	// Insert metrics
	if err := e.insertMetrics(db); err != nil {
		return fmt.Errorf("insert metrics: %w", err)
	}

	// Insert triage recommendations
	if err := e.insertTriageRecommendations(db); err != nil {
		return fmt.Errorf("insert triage: %w", err)
	}

	// Create FTS index
	if err := CreateFTSIndex(db); err != nil {
		// FTS5 may not be available in all SQLite builds - log but continue
		fmt.Printf("Warning: FTS5 not available: %v\n", err)
	}

	// Create materialized views
	if err := CreateMaterializedViews(db); err != nil {
		return fmt.Errorf("create materialized views: %w", err)
	}

	// Insert metadata
	if err := e.insertMeta(db); err != nil {
		return fmt.Errorf("insert meta: %w", err)
	}

	// Optimize database
	if err := OptimizeDatabase(db, e.Config.PageSize); err != nil {
		return fmt.Errorf("optimize database: %w", err)
	}

	// Close database before chunking
	if err := db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	// Write robot JSON outputs
	if e.Config.IncludeRobotOutputs {
		if err := e.writeRobotOutputs(dataDir); err != nil {
			return fmt.Errorf("write robot outputs: %w", err)
		}
	}

	// Chunk if needed
	if err := e.chunkIfNeeded(outputDir, dbPath); err != nil {
		return fmt.Errorf("chunk database: %w", err)
	}

	return nil
}

// insertIssues inserts all issues into the database.
func (e *SQLiteExporter) insertIssues(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO issues (id, title, description, status, priority, issue_type, assignee, labels, created_at, updated_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, issue := range e.Issues {
		labels := ""
		if len(issue.Labels) > 0 {
			labelsJSON, _ := json.Marshal(issue.Labels)
			labels = string(labelsJSON)
		}

		var closedAt *string
		if issue.ClosedAt != nil {
			s := issue.ClosedAt.Format(time.RFC3339)
			closedAt = &s
		}

		_, err := stmt.Exec(
			issue.ID,
			issue.Title,
			issue.Description,
			string(issue.Status),
			issue.Priority,
			string(issue.IssueType),
			issue.Assignee,
			labels,
			issue.CreatedAt.Format(time.RFC3339),
			issue.UpdatedAt.Format(time.RFC3339),
			closedAt,
		)
		if err != nil {
			return fmt.Errorf("insert issue %s: %w", issue.ID, err)
		}
	}

	return tx.Commit()
}

// insertDependencies inserts all dependencies into the database.
func (e *SQLiteExporter) insertDependencies(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO dependencies (issue_id, depends_on_id, type)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, dep := range e.Deps {
		_, err := stmt.Exec(dep.IssueID, dep.DependsOnID, string(dep.Type))
		if err != nil {
			return fmt.Errorf("insert dependency %s->%s: %w", dep.IssueID, dep.DependsOnID, err)
		}
	}

	return tx.Commit()
}

// insertMetrics inserts computed graph metrics for all issues.
func (e *SQLiteExporter) insertMetrics(db *sql.DB) error {
	if e.Stats == nil {
		return nil // No stats available
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO issue_metrics (issue_id, pagerank, betweenness, critical_path_depth, triage_score, blocks_count, blocked_by_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Build dependency lookup maps
	blocksCount := make(map[string]int)
	blockedByCount := make(map[string]int)
	for _, dep := range e.Deps {
		if dep.Type.IsBlocking() {
			blocksCount[dep.IssueID]++
			blockedByCount[dep.DependsOnID]++
		}
	}

	// Get triage scores if available
	triageScores := make(map[string]float64)
	if e.Triage != nil {
		for _, rec := range e.Triage.Recommendations {
			triageScores[rec.ID] = rec.Score
		}
	}

	// Get metrics maps from stats
	pageRankMap := e.Stats.PageRank()
	betweennessMap := e.Stats.Betweenness()
	criticalPathMap := e.Stats.CriticalPathScore()

	for _, issue := range e.Issues {
		id := issue.ID
		pr := pageRankMap[id]
		bw := betweennessMap[id]
		cp := int(criticalPathMap[id])
		score := triageScores[id]
		blocks := blocksCount[id]
		blockedBy := blockedByCount[id]

		_, err := stmt.Exec(id, pr, bw, cp, score, blocks, blockedBy)
		if err != nil {
			return fmt.Errorf("insert metrics for %s: %w", id, err)
		}
	}

	return tx.Commit()
}

// insertTriageRecommendations inserts triage recommendations.
func (e *SQLiteExporter) insertTriageRecommendations(db *sql.DB) error {
	if e.Triage == nil || len(e.Triage.Recommendations) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO triage_recommendations (issue_id, score, action, reasons, unblocks_ids, blocked_by_ids)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, rec := range e.Triage.Recommendations {
		reasonsJSON, _ := json.Marshal(rec.Reasons)
		unblocksJSON, _ := json.Marshal(rec.UnblocksIDs)
		blockedByJSON, _ := json.Marshal(rec.BlockedBy)

		_, err := stmt.Exec(
			rec.ID,
			rec.Score,
			rec.Action,
			string(reasonsJSON),
			string(unblocksJSON),
			string(blockedByJSON),
		)
		if err != nil {
			return fmt.Errorf("insert triage for %s: %w", rec.ID, err)
		}
	}

	return tx.Commit()
}

// insertMeta inserts export metadata.
func (e *SQLiteExporter) insertMeta(db *sql.DB) error {
	meta := map[string]string{
		"version":          "1.0.0",
		"generated_at":     time.Now().UTC().Format(time.RFC3339),
		"issue_count":      fmt.Sprintf("%d", len(e.Issues)),
		"dependency_count": fmt.Sprintf("%d", len(e.Deps)),
		"schema_version":   fmt.Sprintf("%d", SchemaVersion),
	}

	if e.gitHash != "" {
		meta["git_commit"] = e.gitHash
	}
	if e.Config.Title != "" {
		meta["title"] = e.Config.Title
	}

	for key, value := range meta {
		if err := InsertMetaValue(db, key, value); err != nil {
			return fmt.Errorf("insert meta %s: %w", key, err)
		}
	}

	return nil
}

// writeRobotOutputs writes JSON files for robot outputs.
func (e *SQLiteExporter) writeRobotOutputs(dataDir string) error {
	// Write triage output
	if e.Triage != nil {
		if err := writeJSON(filepath.Join(dataDir, "triage.json"), e.Triage); err != nil {
			return fmt.Errorf("write triage.json: %w", err)
		}

		// Also emit a compact project_health.json for fast robot consumption
		if err := writeJSON(filepath.Join(dataDir, "project_health.json"), e.Triage.ProjectHealth); err != nil {
			return fmt.Errorf("write project_health.json: %w", err)
		}
	}

	// Write export metadata
	meta := ExportMeta{
		Version:     "1.0.0",
		GeneratedAt: time.Now().UTC(),
		GitCommit:   e.gitHash,
		IssueCount:  len(e.Issues),
		DepCount:    len(e.Deps),
		Title:       e.Config.Title,
	}
	if err := writeJSON(filepath.Join(dataDir, "meta.json"), meta); err != nil {
		return fmt.Errorf("write meta.json: %w", err)
	}

	return nil
}

// chunkIfNeeded splits the database into chunks if it exceeds the threshold.
func (e *SQLiteExporter) chunkIfNeeded(outputDir, dbPath string) error {
	info, err := os.Stat(dbPath)
	if err != nil {
		return err
	}

	// Write chunk config regardless of whether we chunk
	config := ChunkConfig{
		TotalSize: info.Size(),
	}

	if info.Size() < e.Config.ChunkThreshold {
		config.Chunked = false
		return writeJSON(filepath.Join(outputDir, "beads.sqlite3.config.json"), config)
	}

	// Chunk the database
	chunksDir := filepath.Join(outputDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return fmt.Errorf("create chunks dir: %w", err)
	}

	f, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Calculate file hash
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("hash database: %w", err)
	}
	config.Hash = hex.EncodeToString(hasher.Sum(nil))

	// Reset file position
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	// Split into chunks
	chunkNum := 0
	buf := make([]byte, e.Config.ChunkSize)

	for {
		n, err := f.Read(buf)
		if n > 0 {
			chunkPath := filepath.Join(chunksDir, fmt.Sprintf("%05d.bin", chunkNum))
			if err := os.WriteFile(chunkPath, buf[:n], 0644); err != nil {
				return fmt.Errorf("write chunk %d: %w", chunkNum, err)
			}
			chunkNum++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read for chunk: %w", err)
		}
	}

	// Populate chunk metadata
	config.Chunked = true
	config.ChunkCount = chunkNum
	config.ChunkSize = e.Config.ChunkSize
	config.Chunks = make([]ChunkInfo, 0, chunkNum)

	// Re-read chunks to record paths and hashes
	for i := 0; i < chunkNum; i++ {
		name := fmt.Sprintf("%05d.bin", i)
		path := filepath.Join("chunks", name)
		fullPath := filepath.Join(outputDir, path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("hash chunk %d: %w", i, err)
		}
		h := sha256.Sum256(data)
		config.Chunks = append(config.Chunks, ChunkInfo{
			Path: path,
			Hash: hex.EncodeToString(h[:]),
			Size: int64(len(data)),
		})
	}

	return writeJSON(filepath.Join(outputDir, "beads.sqlite3.config.json"), config)
}

// writeJSON writes data as JSON to a file.
func writeJSON(path string, data interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// GetExportedIssues converts issues to ExportIssue format.
func (e *SQLiteExporter) GetExportedIssues() []ExportIssue {
	// Build dependency lookup maps
	blocksIDs := make(map[string][]string)
	blockedByIDs := make(map[string][]string)
	for _, dep := range e.Deps {
		if dep.Type.IsBlocking() {
			blocksIDs[dep.IssueID] = append(blocksIDs[dep.IssueID], dep.DependsOnID)
			blockedByIDs[dep.DependsOnID] = append(blockedByIDs[dep.DependsOnID], dep.IssueID)
		}
	}

	// Get triage scores
	triageScores := make(map[string]float64)
	if e.Triage != nil {
		for _, rec := range e.Triage.Recommendations {
			triageScores[rec.ID] = rec.Score
		}
	}

	result := make([]ExportIssue, len(e.Issues))
	for i, issue := range e.Issues {
		exp := ExportIssue{
			ID:          issue.ID,
			Title:       issue.Title,
			Description: issue.Description,
			Status:      issue.Status,
			Priority:    issue.Priority,
			IssueType:   issue.IssueType,
			Assignee:    issue.Assignee,
			Labels:      issue.Labels,
			CreatedAt:   issue.CreatedAt,
			UpdatedAt:   issue.UpdatedAt,
			ClosedAt:    issue.ClosedAt,
		}

		if m := e.Metrics; m != nil {
			if mm, ok := m[issue.ID]; ok && mm != nil {
				exp.PageRank = mm.PageRank
				exp.Betweenness = mm.Betweenness
				exp.CriticalPath = mm.CriticalPathDepth
				exp.TriageScore = mm.TriageScore
				exp.BlocksCount = mm.BlocksCount
				exp.BlockedByCount = mm.BlockedByCount
			}
		} else if e.Stats != nil {
			exp.PageRank = e.Stats.GetPageRankScore(issue.ID)
			exp.Betweenness = e.Stats.Betweenness()[issue.ID]
			exp.CriticalPath = int(e.Stats.GetCriticalPathScore(issue.ID))
		}

		// Fallback triage score from recommendations map
		if exp.TriageScore == 0 {
			exp.TriageScore = triageScores[issue.ID]
		}
		// Fallback blocker counts
		if exp.BlocksCount == 0 {
			exp.BlocksIDs = blocksIDs[issue.ID]
			exp.BlocksCount = len(exp.BlocksIDs)
		}
		if exp.BlockedByCount == 0 {
			exp.BlockedByIDs = blockedByIDs[issue.ID]
			exp.BlockedByCount = len(exp.BlockedByIDs)
		}
		// Always set IDs for downstream UI
		if exp.BlocksIDs == nil {
			exp.BlocksIDs = blocksIDs[issue.ID]
		}
		if exp.BlockedByIDs == nil {
			exp.BlockedByIDs = blockedByIDs[issue.ID]
		}

		result[i] = exp
	}

	return result
}

// ExportToJSON exports issues to a JSON file (alternative to SQLite).
func (e *SQLiteExporter) ExportToJSON(path string) error {
	issues := e.GetExportedIssues()

	// Use Config.Title or fallback to default
	title := e.Config.Title
	if title == "" {
		title = "Beads Export"
	}

	output := struct {
		Meta   ExportMeta    `json:"meta"`
		Issues []ExportIssue `json:"issues"`
	}{
		Meta: ExportMeta{
			Version:     "1.0.0",
			GeneratedAt: time.Now().UTC(),
			GitCommit:   e.gitHash,
			IssueCount:  len(issues),
			DepCount:    len(e.Deps),
			Title:       title,
		},
		Issues: issues,
	}

	return writeJSON(path, output)
}

// stringSliceContains checks if a string slice contains a value.
func stringSliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}
