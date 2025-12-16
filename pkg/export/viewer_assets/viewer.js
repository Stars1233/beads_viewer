/**
 * Beads Viewer - Static SQL.js WASM-based issue viewer
 *
 * Follows mcp_agent_mail's architecture for client-side sql.js querying with:
 * - OPFS caching for offline support
 * - Chunk reassembly for large databases
 * - FTS5 full-text search
 * - Materialized views for fast queries
 */

// ============================================================================
// Error Handling and Diagnostics
// ============================================================================

/**
 * Global error state for the application
 */
const ERROR_STATE = {
  error: null,           // Current error object or null
  errors: [],            // Error history
};

/**
 * Diagnostics state for debugging
 */
const DIAGNOSTICS = {
  wasm: false,           // sql.js WASM loaded
  opfs: null,            // OPFS available (null = not checked, true/false)
  graphWasm: false,      // bv_graph WASM loaded
  dbSource: 'unknown',   // 'network' | 'cache' | 'chunks'
  dbSizeBytes: 0,        // Database size in bytes
  issueCount: 0,         // Number of issues
  loadTimeMs: 0,         // Total load time
  startTime: Date.now(), // When loading started
  queryCount: 0,         // Number of queries executed
  queryErrors: 0,        // Number of query errors
};

/**
 * Show an error to the user with optional actions
 * @param {Object} options - Error display options
 * @param {string} options.title - Error title
 * @param {string} options.message - User-friendly error message
 * @param {string} [options.details] - Technical details (stack trace, etc)
 * @param {Array} [options.actions] - Array of action buttons
 * @param {boolean} [options.dismissible] - Whether the error can be dismissed (default true)
 */
function showError({ title, message, details = null, actions = [], dismissible = true }) {
  const error = {
    id: Date.now(),
    title,
    message,
    details,
    actions,
    dismissible,
    timestamp: new Date().toISOString(),
  };

  ERROR_STATE.error = error;
  ERROR_STATE.errors.push(error);

  // Log to console for debugging
  console.error(`[Error] ${title}: ${message}`, details);

  return error;
}

/**
 * Clear the current error
 */
function clearError() {
  ERROR_STATE.error = null;
}

/**
 * Safe query wrapper with error handling and fallback
 * @param {string} sql - SQL query
 * @param {Array} params - Query parameters
 * @param {*} fallback - Value to return on error
 * @returns {Object} Query result with success flag
 */
function safeQuery(sql, params = [], fallback = []) {
  if (!DB_STATE.db) {
    console.warn('[safeQuery] Database not loaded');
    return { success: false, data: fallback, error: 'Database not loaded' };
  }

  DIAGNOSTICS.queryCount++;

  try {
    const result = DB_STATE.db.exec(sql, params);
    if (!result.length) return { success: true, data: [] };

    const { columns, values } = result[0];
    const data = values.map(row => {
      const obj = {};
      columns.forEach((col, i) => {
        obj[col] = row[i];
      });
      return obj;
    });

    return { success: true, data };
  } catch (err) {
    DIAGNOSTICS.queryErrors++;
    console.error('[safeQuery] Query failed:', sql, err);
    return { success: false, data: fallback, error: err.message };
  }
}

/**
 * Show a toast notification (non-blocking)
 * @param {string} message - Toast message
 * @param {string} type - Toast type: 'info' | 'success' | 'warning' | 'error'
 */
function showToast(message, type = 'info') {
  // This will be picked up by the Alpine toast component
  window.dispatchEvent(new CustomEvent('show-toast', {
    detail: { message, type, id: Date.now() }
  }));
}

// Database state
const DB_STATE = {
  sql: null,          // sql.js library instance
  db: null,           // Database instance
  cacheKey: null,     // OPFS cache key (hash)
  source: 'unknown',  // 'network' | 'cache' | 'chunks'
};

// Graph engine state (WASM)
const GRAPH_STATE = {
  wasm: null,         // WASM module (bv_graph.js)
  graph: null,        // DiGraph instance
  nodeMap: null,      // Map<string, number> - issue ID to node index
  ready: false,       // true when graph is loaded
};

// WASM support detection
const WASM_STATUS = {
  supported: null,    // null = not checked, true/false = checked
  reason: null,       // Reason for failure if not supported
  fallbackMode: false, // true when using pre-computed data only
  features: {
    basic: null,      // Basic WASM support
    simd: null,       // SIMD support
    threads: null,    // Thread support (SharedArrayBuffer)
  },
};

/**
 * Check WASM support in the browser
 * @returns {Promise<{supported: boolean, reason: string|null, features: Object}>}
 */
async function checkWASMSupport() {
  // Already checked
  if (WASM_STATUS.supported !== null) {
    return WASM_STATUS;
  }

  // Check basic WASM support
  if (typeof WebAssembly !== 'object') {
    WASM_STATUS.supported = false;
    WASM_STATUS.reason = 'WebAssembly not available in this browser';
    WASM_STATUS.fallbackMode = true;
    return WASM_STATUS;
  }

  // Check WebAssembly APIs
  try {
    const testModule = new WebAssembly.Module(
      new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00])
    );
    if (!(testModule instanceof WebAssembly.Module)) {
      throw new Error('WebAssembly.Module validation failed');
    }
    WASM_STATUS.features.basic = true;
  } catch (e) {
    WASM_STATUS.supported = false;
    WASM_STATUS.reason = `WebAssembly validation failed: ${e.message}`;
    WASM_STATUS.fallbackMode = true;
    WASM_STATUS.features.basic = false;
    return WASM_STATUS;
  }

  // Check SharedArrayBuffer (needed for some WASM features)
  WASM_STATUS.features.threads = typeof SharedArrayBuffer !== 'undefined';

  // SIMD detection (optional, not required)
  try {
    WASM_STATUS.features.simd = WebAssembly.validate(new Uint8Array([
      0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
      0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7b, 0x03,
      0x02, 0x01, 0x00, 0x0a, 0x0a, 0x01, 0x08, 0x00,
      0xfd, 0x0c, 0x00, 0x00, 0x00, 0x00, 0x0b
    ]));
  } catch {
    WASM_STATUS.features.simd = false;
  }

  WASM_STATUS.supported = true;
  WASM_STATUS.reason = null;
  return WASM_STATUS;
}

/**
 * Enable fallback mode (use pre-computed data only)
 */
function enableFallbackMode(reason) {
  WASM_STATUS.fallbackMode = true;
  WASM_STATUS.reason = reason;
  GRAPH_STATE.ready = false;
  DIAGNOSTICS.graphWasm = false;
  console.warn('[WASM] Fallback mode enabled:', reason);
  showToast('Using pre-computed metrics only', 'warning');
}

/**
 * Initialize sql.js library
 */
async function initSqlJs() {
  if (DB_STATE.sql) {
    DIAGNOSTICS.wasm = true;
    return DB_STATE.sql;
  }

  // Load sql.js from CDN (with WASM)
  const sqlPromise = initSqlJs.cached || (initSqlJs.cached = new Promise(async (resolve, reject) => {
    try {
      let usedLocal = false;
      // Try loading from local vendor first
      let sqlJs;
      try {
        const script = document.createElement('script');
        script.src = './vendor/sql-wasm.js';
        document.head.appendChild(script);
        await new Promise((res, rej) => {
          script.onload = res;
          script.onerror = rej;
        });
        sqlJs = window.initSqlJs;
        usedLocal = true;
      } catch {
        // Fallback to CDN
        const script = document.createElement('script');
        script.src = 'https://cdn.jsdelivr.net/npm/sql.js@1.10.3/dist/sql-wasm.js';
        document.head.appendChild(script);
        await new Promise((res, rej) => {
          script.onload = res;
          script.onerror = rej;
        });
        sqlJs = window.initSqlJs;
        usedLocal = false;
      }

      const SQL = await sqlJs({
        locateFile: file => {
          // Prefer local vendored wasm when available for offline use
          if (usedLocal) {
            return `./vendor/${file}`;
          }
          return `https://cdn.jsdelivr.net/npm/sql.js@1.10.3/dist/${file}`;
        }
      });

      DIAGNOSTICS.wasm = true;
      resolve(SQL);
    } catch (err) {
      DIAGNOSTICS.wasm = false;
      showError({
        title: 'Browser Compatibility Issue',
        message: 'This viewer requires WebAssembly support to run SQL queries.',
        details: err.message,
        actions: [
          { label: 'Check Browser Support', url: 'https://caniuse.com/wasm' },
          { label: 'Reload Page', action: () => location.reload() },
        ],
        dismissible: false,
      });
      reject(err);
    }
  }));

  DB_STATE.sql = await sqlPromise;
  return DB_STATE.sql;
}

/**
 * Load database from OPFS cache
 */
async function loadFromOPFS(cacheKey) {
  if (!('storage' in navigator) || !navigator.storage.getDirectory) {
    DIAGNOSTICS.opfs = false;
    console.info('[OPFS] Not available in this browser');
    return null;
  }

  try {
    const root = await navigator.storage.getDirectory();
    const filename = `beads-${cacheKey || 'default'}.sqlite3`;
    const handle = await root.getFileHandle(filename, { create: false });
    const file = await handle.getFile();
    const buffer = await file.arrayBuffer();
    DIAGNOSTICS.opfs = true;
    DIAGNOSTICS.dbSizeBytes = buffer.byteLength;
    console.log(`[OPFS] Loaded ${buffer.byteLength} bytes from cache`);
    return new Uint8Array(buffer);
  } catch (err) {
    if (err.name === 'NotFoundError') {
      DIAGNOSTICS.opfs = true; // OPFS available, just no cache yet
    } else {
      // Private browsing or permission denied
      DIAGNOSTICS.opfs = false;
      console.info('[OPFS] Cache unavailable:', err.message);
    }
    return null;
  }
}

/**
 * Cache database to OPFS
 */
async function cacheToOPFS(data, cacheKey) {
  if (!('storage' in navigator) || !navigator.storage.getDirectory) {
    return false;
  }

  try {
    const root = await navigator.storage.getDirectory();
    const filename = `beads-${cacheKey || 'default'}.sqlite3`;
    const handle = await root.getFileHandle(filename, { create: true });
    const writable = await handle.createWritable();
    await writable.write(data);
    await writable.close();
    console.log(`[OPFS] Cached ${data.byteLength} bytes`);
    return true;
  } catch (err) {
    console.warn('[OPFS] Cache failed:', err);
    return false;
  }
}

/**
 * Fetch JSON file
 */
async function fetchJSON(url) {
  const response = await fetch(url);
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return response.json();
}

/**
 * Load database chunks and reassemble
 */
async function loadChunks(config) {
  const chunks = [];
  const totalChunks = config.chunk_count;

  for (let i = 0; i < totalChunks; i++) {
    const chunkPath = `./chunks/${String(i).padStart(5, '0')}.bin`;
    const response = await fetch(chunkPath);
    if (!response.ok) throw new Error(`Failed to load chunk ${i}`);
    const buffer = await response.arrayBuffer();
    chunks.push(new Uint8Array(buffer));
  }

  // Concatenate all chunks
  const totalSize = chunks.reduce((sum, c) => sum + c.length, 0);
  const combined = new Uint8Array(totalSize);
  let offset = 0;
  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.length;
  }

  console.log(`[Chunks] Reassembled ${totalChunks} chunks, ${totalSize} bytes`);
  return combined;
}

/**
 * Load database with caching strategy
 */
async function loadDatabase(updateStatus) {
  const SQL = await initSqlJs();

  updateStatus?.('Checking cache...');

  // Load config to get cache key
  let config = null;
  try {
    config = await fetchJSON('./beads.sqlite3.config.json');
    DB_STATE.cacheKey = config.hash || null;
  } catch {
    // Config file may not exist for small DBs
  }

  // Try OPFS cache first
  if (DB_STATE.cacheKey) {
    const cached = await loadFromOPFS(DB_STATE.cacheKey);
    if (cached) {
      DB_STATE.db = new SQL.Database(cached);
      DB_STATE.source = 'cache';
      DIAGNOSTICS.dbSource = 'cache';
      return DB_STATE.db;
    }
  }

  updateStatus?.('Loading database...');

  // Check if database is chunked
  let dbData;
  try {
    if (config?.chunked) {
      updateStatus?.(`Loading ${config.chunk_count} chunks...`);
      dbData = await loadChunks(config);
      DB_STATE.source = 'chunks';
      DIAGNOSTICS.dbSource = 'chunks';
    } else {
      // Load single file - try multiple paths
      const paths = ['./beads.sqlite3', './data/beads.sqlite3'];
      let loaded = false;

      for (const path of paths) {
        try {
          const response = await fetch(path);
          if (response.ok) {
            const buffer = await response.arrayBuffer();
            dbData = new Uint8Array(buffer);
            DIAGNOSTICS.dbSizeBytes = buffer.byteLength;
            DB_STATE.source = 'network';
            DIAGNOSTICS.dbSource = 'network';
            loaded = true;
            break;
          }
        } catch {
          // Try next path
        }
      }

      if (!loaded) {
        throw new Error('Database not found at any known path');
      }
    }
  } catch (err) {
    showError({
      title: 'Database Not Found',
      message: 'Could not load the issues database.',
      details: `${err.message}\n\nThe beads.sqlite3 file may be missing or corrupted.`,
      actions: [
        { label: 'Reload Page', action: () => location.reload() },
      ],
      dismissible: false,
    });
    throw err;
  }

  try {
    DB_STATE.db = new SQL.Database(dbData);
  } catch (err) {
    showError({
      title: 'Database Corrupted',
      message: 'The database file could not be opened.',
      details: err.message,
      actions: [
        { label: 'Reload Page', action: () => location.reload() },
      ],
      dismissible: false,
    });
    throw err;
  }

  // Cache for next time
  if (DB_STATE.cacheKey) {
    updateStatus?.('Caching for offline...');
    await cacheToOPFS(DB_STATE.db.export(), DB_STATE.cacheKey);
  }

  return DB_STATE.db;
}

/**
 * Execute a SQL query and return results as array of objects
 */
function execQuery(sql, params = []) {
  if (!DB_STATE.db) throw new Error('Database not loaded');

  try {
    const result = DB_STATE.db.exec(sql, params);
    if (!result.length) return [];

    const { columns, values } = result[0];
    return values.map(row => {
      const obj = {};
      columns.forEach((col, i) => {
        obj[col] = row[i];
      });
      return obj;
    });
  } catch (err) {
    console.error('Query error:', err, sql);
    throw err;
  }
}

/**
 * Get a single value from a query
 */
function execScalar(sql, params = []) {
  const result = execQuery(sql, params);
  if (!result.length) return null;
  return Object.values(result[0])[0];
}

// ============================================================================
// WASM Graph Engine - Live graph calculations
// ============================================================================

/**
 * Initialize the WASM graph engine
 */
async function initGraphEngine() {
  if (GRAPH_STATE.ready) return true;

  // Check if we're already in fallback mode
  if (WASM_STATUS.fallbackMode) {
    console.log('[Graph] Fallback mode active, skipping WASM init');
    return false;
  }

  // Check WASM support first
  await checkWASMSupport();
  if (!WASM_STATUS.supported) {
    enableFallbackMode(WASM_STATUS.reason);
    return false;
  }

  try {
    // Dynamic import of the WASM module
    const wasmModule = await import('./vendor/bv_graph.js');
    await wasmModule.default(); // Initialize WASM

    GRAPH_STATE.wasm = wasmModule;
    GRAPH_STATE.graph = new wasmModule.DiGraph();
    GRAPH_STATE.nodeMap = new Map();

    // Load graph data from SQLite
    if (!DB_STATE.db) {
      console.warn('[Graph] Database not loaded yet');
      return false;
    }

    const deps = execQuery(`
      SELECT issue_id, depends_on_id
      FROM dependencies
      WHERE type = 'blocks'
    `);

    for (const row of deps) {
      const from = row.issue_id;
      const to = row.depends_on_id;

      if (!GRAPH_STATE.nodeMap.has(from)) {
        const idx = GRAPH_STATE.graph.addNode(from);
        GRAPH_STATE.nodeMap.set(from, idx);
      }
      if (!GRAPH_STATE.nodeMap.has(to)) {
        const idx = GRAPH_STATE.graph.addNode(to);
        GRAPH_STATE.nodeMap.set(to, idx);
      }

      GRAPH_STATE.graph.addEdge(
        GRAPH_STATE.nodeMap.get(from),
        GRAPH_STATE.nodeMap.get(to)
      );
    }

    GRAPH_STATE.ready = true;
    WASM_STATUS.fallbackMode = false;
    console.log(`[Graph] Loaded: ${GRAPH_STATE.graph.nodeCount()} nodes, ${GRAPH_STATE.graph.edgeCount()} edges`);
    return true;
  } catch (err) {
    console.warn('[Graph] WASM init failed:', err.message);
    enableFallbackMode(`WASM load failed: ${err.message}`);
    return false;
  }
}

/**
 * Build closed set array from database
 * Returns Uint8Array where 1 = closed, 0 = open
 */
function buildClosedSet() {
  if (!GRAPH_STATE.ready) return null;

  const n = GRAPH_STATE.graph.nodeCount();
  const closed = new Uint8Array(n);

  const closedIssues = execQuery(`
    SELECT id FROM issues WHERE status = 'closed'
  `);

  for (const row of closedIssues) {
    const idx = GRAPH_STATE.nodeMap.get(row.id);
    if (idx !== undefined) {
      closed[idx] = 1;
    }
  }

  return closed;
}

/**
 * Recalculate graph metrics for a filtered set of issues
 */
function recalculateMetrics(issueIds) {
  if (!GRAPH_STATE.ready) return null;

  const start = performance.now();
  const indices = issueIds
    .map(id => GRAPH_STATE.nodeMap.get(id))
    .filter(idx => idx !== undefined);

  if (indices.length === 0) return null;

  // Extract subgraph for filtered issues
  const subgraph = GRAPH_STATE.graph.subgraph(new Uint32Array(indices));

  const result = {
    nodeCount: subgraph.nodeCount(),
    edgeCount: subgraph.edgeCount(),
    pagerank: subgraph.pagerankDefault(),
    betweenness: subgraph.betweenness(),
    hasCycles: subgraph.hasCycles(),
    criticalPath: subgraph.criticalPathHeights(),
  };

  const elapsed = performance.now() - start;
  console.log(`[Graph] Recalculated metrics in ${elapsed.toFixed(1)}ms`);

  return result;
}

/**
 * What-if analysis: compute cascade impact of closing an issue
 */
function whatIfClose(issueId) {
  if (!GRAPH_STATE.ready) return null;

  const idx = GRAPH_STATE.nodeMap.get(issueId);
  if (idx === undefined) return null;

  const closedSet = buildClosedSet();
  const result = GRAPH_STATE.graph.whatIfClose(idx, closedSet);

  // Convert node indices back to issue IDs
  if (result && result.cascade_ids) {
    result.cascade_issue_ids = result.cascade_ids
      .map(i => GRAPH_STATE.graph.nodeId(i))
      .filter(Boolean);
  }

  return result;
}

/**
 * Get top issues by cascade impact
 */
function topWhatIf(limit = 10) {
  if (!GRAPH_STATE.ready) return [];

  const closedSet = buildClosedSet();
  const results = GRAPH_STATE.graph.topWhatIf(closedSet, limit);

  // Enrich with issue IDs
  return (results || []).map(item => ({
    ...item,
    issueId: GRAPH_STATE.graph.nodeId(item.node),
    result: item.result,
  }));
}

/**
 * Get actionable issues (all blockers closed)
 */
function getActionableIssues() {
  if (!GRAPH_STATE.ready) return [];

  const closedSet = buildClosedSet();
  const indices = GRAPH_STATE.graph.actionableNodes(closedSet);

  return (indices || [])
    .map(idx => GRAPH_STATE.graph.nodeId(idx))
    .filter(Boolean);
}

/**
 * Find cycle break suggestions
 */
function getCycleBreakSuggestions(limit = 5) {
  if (!GRAPH_STATE.ready) return null;

  const result = GRAPH_STATE.graph.cycleBreakSuggestions(limit, 100);
  return result;
}

/**
 * Get greedy top-k issues to maximize unblocks
 */
function getTopKSet(k = 5) {
  if (!GRAPH_STATE.ready) return null;

  const closedSet = buildClosedSet();
  const result = GRAPH_STATE.graph.topkSet(closedSet, k);

  // Enrich with issue IDs
  if (result && result.items) {
    result.items = result.items.map(item => ({
      ...item,
      issueId: GRAPH_STATE.graph.nodeId(item.node),
      unblocked_issue_ids: (item.unblocked_ids || [])
        .map(i => GRAPH_STATE.graph.nodeId(i))
        .filter(Boolean),
    }));
  }

  return result;
}

// ============================================================================
// Query Layer - Using materialized views for performance
// ============================================================================

/**
 * Build WHERE clauses from filters (shared between query and count)
 */
function buildFilterClauses(filters = {}) {
  const clauses = [];
  const params = [];

  // Status filter (supports array for multi-select)
  if (filters.status?.length) {
    const statuses = Array.isArray(filters.status) ? filters.status : [filters.status];
    if (statuses.length === 1) {
      clauses.push(`status = ?`);
      params.push(statuses[0]);
    } else {
      clauses.push(`status IN (${statuses.map(() => '?').join(',')})`);
      params.push(...statuses);
    }
  }

  // Type filter (supports array for multi-select)
  if (filters.type?.length) {
    const types = Array.isArray(filters.type) ? filters.type : [filters.type];
    if (types.length === 1) {
      clauses.push(`issue_type = ?`);
      params.push(types[0]);
    } else {
      clauses.push(`issue_type IN (${types.map(() => '?').join(',')})`);
      params.push(...types);
    }
  }

  // Priority filter (supports array for multi-select)
  if (filters.priority?.length) {
    const priorities = (Array.isArray(filters.priority) ? filters.priority : [filters.priority])
      .map(p => parseInt(p))
      .filter(p => !isNaN(p));
    if (priorities.length === 1) {
      clauses.push(`priority = ?`);
      params.push(priorities[0]);
    } else if (priorities.length > 1) {
      clauses.push(`priority IN (${priorities.map(() => '?').join(',')})`);
      params.push(...priorities);
    }
  }

  // Assignee filter
  if (filters.assignee) {
    clauses.push(`assignee = ?`);
    params.push(filters.assignee);
  }

  // Blocked filter
  if (filters.hasBlockers === true || filters.hasBlockers === 'true') {
    clauses.push(`(blocked_by_ids IS NOT NULL AND blocked_by_ids != '')`);
  } else if (filters.hasBlockers === false || filters.hasBlockers === 'false') {
    clauses.push(`(blocked_by_ids IS NULL OR blocked_by_ids = '')`);
  }

  // Blocking filter (has items depending on it)
  if (filters.isBlocking === true || filters.isBlocking === 'true') {
    clauses.push(`blocks_count > 0`);
  }

  // Label filter (JSON array contains)
  if (filters.labels?.length) {
    const labels = Array.isArray(filters.labels) ? filters.labels : [filters.labels];
    const labelClauses = labels.map(() => `labels LIKE ?`);
    clauses.push(`(${labelClauses.join(' OR ')})`);
    params.push(...labels.map(l => `%"${l}"%`));
  }

  // Search filter (LIKE-based, FTS5 handled separately)
  if (filters.search) {
    clauses.push(`(title LIKE ? OR description LIKE ? OR id LIKE ?)`);
    const searchTerm = `%${filters.search}%`;
    params.push(searchTerm, searchTerm, searchTerm);
  }

  return { clauses, params };
}

/**
 * Query issues with filters, sorting, and pagination
 */
function queryIssues(filters = {}, sort = 'priority', limit = 50, offset = 0) {
  const { clauses, params } = buildFilterClauses(filters);

  let sql = `SELECT * FROM issue_overview_mv`;
  if (clauses.length > 0) {
    sql += ` WHERE ${clauses.join(' AND ')}`;
  }

  // Sorting
  const sortMap = {
    'priority': 'priority ASC, triage_score DESC',
    'updated': 'updated_at DESC',
    'score': 'triage_score DESC',
    'blocks': 'blocks_count DESC',
    'created': 'created_at DESC',
    'title': 'title ASC',
    'id': 'id ASC',
  };
  sql += ` ORDER BY ${sortMap[sort] || sortMap.priority}`;
  sql += ` LIMIT ? OFFSET ?`;
  params.push(limit, offset);

  return execQuery(sql, params);
}

/**
 * Count issues matching filters
 */
function countIssues(filters = {}) {
  const { clauses, params } = buildFilterClauses(filters);

  let sql = `SELECT COUNT(*) as count FROM issue_overview_mv`;
  if (clauses.length > 0) {
    sql += ` WHERE ${clauses.join(' AND ')}`;
  }

  return execScalar(sql, params) || 0;
}

/**
 * Get unique values for filter dropdowns
 */
function getFilterOptions() {
  return {
    statuses: execQuery(`SELECT DISTINCT status FROM issue_overview_mv ORDER BY status`).map(r => r.status),
    types: execQuery(`SELECT DISTINCT issue_type FROM issue_overview_mv ORDER BY issue_type`).map(r => r.issue_type),
    priorities: execQuery(`SELECT DISTINCT priority FROM issue_overview_mv ORDER BY priority`).map(r => r.priority),
    assignees: execQuery(`SELECT DISTINCT assignee FROM issue_overview_mv WHERE assignee IS NOT NULL AND assignee != '' ORDER BY assignee`).map(r => r.assignee),
    labels: getUniqueLabels(),
  };
}

/**
 * Get unique labels from all issues
 */
function getUniqueLabels() {
  const results = execQuery(`SELECT labels FROM issue_overview_mv WHERE labels IS NOT NULL AND labels != ''`);
  const labelSet = new Set();
  for (const row of results) {
    try {
      const labels = JSON.parse(row.labels);
      if (Array.isArray(labels)) {
        labels.forEach(l => labelSet.add(l));
      }
    } catch { /* ignore parse errors */ }
  }
  return Array.from(labelSet).sort();
}

/**
 * Get a single issue by ID
 */
function getIssue(id) {
  const results = execQuery(`SELECT * FROM issue_overview_mv WHERE id = ?`, [id]);
  return results[0] || null;
}

/**
 * Full-text search using FTS5 (if available)
 */
function searchIssues(term, limit = 50) {
  // Try FTS5 first
  try {
    const sql = `
      SELECT id, title,
             snippet(issues_fts, 2, '<mark>', '</mark>', '...', 32) as snippet,
             bm25(issues_fts) as rank
      FROM issues_fts
      WHERE issues_fts MATCH ?
      ORDER BY rank
      LIMIT ?
    `;
    return execQuery(sql, [term + '*', limit]);
  } catch {
    // Fallback to LIKE search
    return queryIssues({ search: term }, 'score', limit, 0);
  }
}

/**
 * Get project statistics
 */
function getStats() {
  const stats = {};

  // Count by status
  const statusCounts = execQuery(`
    SELECT status, COUNT(*) as count
    FROM issue_overview_mv
    GROUP BY status
  `);
  statusCounts.forEach(row => {
    stats[row.status] = row.count;
  });

  // Count blocked (has blocked_by_ids and status is open/in_progress)
  stats.blocked = execScalar(`
    SELECT COUNT(*) FROM issue_overview_mv
    WHERE blocked_by_ids IS NOT NULL
    AND blocked_by_ids != ''
    AND status IN ('open', 'in_progress')
  `) || 0;

  // Count actionable (open/in_progress with NO open blockers)
  stats.actionable = execScalar(`
    SELECT COUNT(*) FROM issue_overview_mv
    WHERE status IN ('open', 'in_progress')
    AND (blocked_by_ids IS NULL OR blocked_by_ids = '')
  `) || 0;

  // Total
  stats.total = execScalar(`SELECT COUNT(*) FROM issue_overview_mv`) || 0;

  return stats;
}

/**
 * Get quick wins - actionable issues that unblock the most items
 */
function getQuickWins(limit = 5) {
  return execQuery(`
    SELECT * FROM issue_overview_mv
    WHERE status IN ('open', 'in_progress')
    AND (blocked_by_ids IS NULL OR blocked_by_ids = '')
    ORDER BY blocks_count DESC, triage_score DESC
    LIMIT ?
  `, [limit]);
}

/**
 * Get blockers to clear - issues blocking the most other issues
 */
function getBlockersToClose(limit = 5) {
  return execQuery(`
    SELECT * FROM issue_overview_mv
    WHERE status IN ('open', 'in_progress')
    AND blocks_count > 0
    ORDER BY blocks_count DESC, triage_score DESC
    LIMIT ?
  `, [limit]);
}

/**
 * Get distribution by type
 */
function getDistributionByType() {
  return execQuery(`
    SELECT issue_type as type, COUNT(*) as count
    FROM issue_overview_mv
    WHERE status != 'closed'
    GROUP BY issue_type
    ORDER BY count DESC
  `);
}

/**
 * Get distribution by priority
 */
function getDistributionByPriority() {
  return execQuery(`
    SELECT priority, COUNT(*) as count
    FROM issue_overview_mv
    WHERE status != 'closed'
    GROUP BY priority
    ORDER BY priority ASC
  `);
}

/**
 * Get top issues by triage score
 */
function getTopPicks(limit = 5) {
  return execQuery(`
    SELECT * FROM issue_overview_mv
    WHERE status IN ('open', 'in_progress')
    ORDER BY triage_score DESC
    LIMIT ?
  `, [limit]);
}

/**
 * Get recent issues by update time
 */
function getRecentIssues(limit = 10) {
  return execQuery(`
    SELECT * FROM issue_overview_mv
    ORDER BY updated_at DESC
    LIMIT ?
  `, [limit]);
}

/**
 * Get top issues by PageRank
 */
function getTopByPageRank(limit = 10) {
  return execQuery(`
    SELECT * FROM issue_overview_mv
    WHERE pagerank > 0
    ORDER BY pagerank DESC
    LIMIT ?
  `, [limit]);
}

/**
 * Get top issues by triage score
 */
function getTopByTriageScore(limit = 10) {
  return execQuery(`
    SELECT * FROM issue_overview_mv
    WHERE triage_score > 0
    ORDER BY triage_score DESC
    LIMIT ?
  `, [limit]);
}

/**
 * Get top blocking issues
 */
function getTopBlockers(limit = 10) {
  return execQuery(`
    SELECT * FROM issue_overview_mv
    WHERE blocks_count > 0
    ORDER BY blocks_count DESC
    LIMIT ?
  `, [limit]);
}

/**
 * Get top issues by betweenness centrality (bottlenecks)
 */
function getTopByBetweenness(limit = 10) {
  // Try betweenness column if it exists
  try {
    const results = execQuery(`
      SELECT * FROM issue_overview_mv
      WHERE betweenness > 0
      ORDER BY betweenness DESC
      LIMIT ?
    `, [limit]);
    if (results.length > 0) return results;
  } catch {
    // Column may not exist
  }

  // Fallback: use WASM if available
  if (GRAPH_STATE.ready) {
    const betweenness = GRAPH_STATE.graph.betweenness();
    if (betweenness && betweenness.length > 0) {
      // Get top N by betweenness value
      const indexed = Array.from(betweenness).map((val, idx) => ({ idx, val }));
      indexed.sort((a, b) => b.val - a.val);
      const topNodes = indexed.slice(0, limit);

      return topNodes.map(node => {
        const id = GRAPH_STATE.graph.nodeId(node.idx);
        const issue = getIssue(id);
        if (issue) {
          issue.betweenness = node.val;
        }
        return issue;
      }).filter(Boolean);
    }
  }

  return [];
}

/**
 * Get top issues by critical path height (keystones)
 */
function getTopByCriticalPath(limit = 10) {
  // Try critical_path column if it exists
  try {
    const results = execQuery(`
      SELECT * FROM issue_overview_mv
      WHERE critical_path_height > 0
      ORDER BY critical_path_height DESC
      LIMIT ?
    `, [limit]);
    if (results.length > 0) return results;
  } catch {
    // Column may not exist
  }

  // Fallback: use WASM if available
  if (GRAPH_STATE.ready) {
    const heights = GRAPH_STATE.graph.criticalPathHeights();
    if (heights && heights.length > 0) {
      const indexed = Array.from(heights).map((val, idx) => ({ idx, val }));
      indexed.sort((a, b) => b.val - a.val);
      const topNodes = indexed.slice(0, limit);

      return topNodes.map(node => {
        const id = GRAPH_STATE.graph.nodeId(node.idx);
        const issue = getIssue(id);
        if (issue) {
          issue.critical_path_height = node.val;
        }
        return issue;
      }).filter(Boolean);
    }
  }

  return [];
}

/**
 * Get cycle information from graph
 */
function getCycleInfo() {
  if (!GRAPH_STATE.ready) {
    return { hasCycles: false, cycleCount: 0, suggestions: null };
  }

  const hasCycles = GRAPH_STATE.graph.hasCycles();
  const suggestions = hasCycles ? getCycleBreakSuggestions(5) : null;

  return {
    hasCycles,
    cycleCount: suggestions?.cycles?.length || 0,
    suggestions,
  };
}

/**
 * Get export metadata
 */
function getMeta() {
  const meta = {};
  const rows = execQuery(`SELECT key, value FROM export_meta`);
  rows.forEach(row => {
    meta[row.key] = row.value;
  });
  return meta;
}

/**
 * Get dependencies for an issue
 */
function getIssueDependencies(id) {
  const blocks = execQuery(`
    SELECT i.* FROM issue_overview_mv i
    JOIN dependencies d ON i.id = d.depends_on_id
    WHERE d.issue_id = ? AND d.type = 'blocks'
  `, [id]);

  const blockedBy = execQuery(`
    SELECT i.* FROM issue_overview_mv i
    JOIN dependencies d ON i.id = d.issue_id
    WHERE d.depends_on_id = ? AND d.type = 'blocks'
  `, [id]);

  return { blocks, blockedBy };
}

// ============================================================================
// URL State Sync - Shareable filtered views
// ============================================================================

/**
 * Serialize filters to URL search params
 */
function filtersToURL(filters, sort, searchQuery) {
  const params = new URLSearchParams();

  if (filters.status?.length) {
    const statuses = Array.isArray(filters.status) ? filters.status : [filters.status];
    if (statuses.length > 0 && statuses[0]) {
      params.set('status', statuses.join(','));
    }
  }

  if (filters.type?.length) {
    const types = Array.isArray(filters.type) ? filters.type : [filters.type];
    if (types.length > 0 && types[0]) {
      params.set('type', types.join(','));
    }
  }

  if (filters.priority?.length) {
    const priorities = Array.isArray(filters.priority) ? filters.priority : [filters.priority];
    const validPriorities = priorities.filter(p => p !== '' && p !== null && p !== undefined);
    if (validPriorities.length > 0) {
      params.set('priority', validPriorities.join(','));
    }
  }

  if (filters.labels?.length) {
    params.set('labels', filters.labels.join(','));
  }

  if (filters.assignee) {
    params.set('assignee', filters.assignee);
  }

  if (filters.hasBlockers === true || filters.hasBlockers === 'true') {
    params.set('blocked', 'true');
  } else if (filters.hasBlockers === false || filters.hasBlockers === 'false') {
    params.set('blocked', 'false');
  }

  if (filters.isBlocking === true || filters.isBlocking === 'true') {
    params.set('blocking', 'true');
  }

  if (searchQuery) {
    params.set('q', searchQuery);
  }

  if (sort && sort !== 'priority') {
    params.set('sort', sort);
  }

  return params.toString();
}

/**
 * Parse URL search params to filters
 */
function filtersFromURL() {
  const hash = window.location.hash;
  const queryIndex = hash.indexOf('?');
  if (queryIndex === -1) return { filters: {}, sort: 'priority', searchQuery: '' };

  const params = new URLSearchParams(hash.slice(queryIndex + 1));

  const filters = {};

  const statusParam = params.get('status');
  if (statusParam) {
    filters.status = statusParam.split(',').filter(Boolean);
  }

  const typeParam = params.get('type');
  if (typeParam) {
    filters.type = typeParam.split(',').filter(Boolean);
  }

  const priorityParam = params.get('priority');
  if (priorityParam) {
    filters.priority = priorityParam.split(',').map(Number).filter(n => !isNaN(n));
  }

  const labelsParam = params.get('labels');
  if (labelsParam) {
    filters.labels = labelsParam.split(',').filter(Boolean);
  }

  const assigneeParam = params.get('assignee');
  if (assigneeParam) {
    filters.assignee = assigneeParam;
  }

  const blockedParam = params.get('blocked');
  if (blockedParam === 'true') {
    filters.hasBlockers = true;
  } else if (blockedParam === 'false') {
    filters.hasBlockers = false;
  }

  const blockingParam = params.get('blocking');
  if (blockingParam === 'true') {
    filters.isBlocking = true;
  }

  return {
    filters,
    sort: params.get('sort') || 'priority',
    searchQuery: params.get('q') || '',
  };
}

/**
 * Update URL with current filter state (without page reload)
 */
function syncFiltersToURL(view, filters, sort, searchQuery) {
  const paramString = filtersToURL(filters, sort, searchQuery);
  const baseHash = `#/${view}`;
  const newHash = paramString ? `${baseHash}?${paramString}` : baseHash;

  if (window.location.hash !== newHash) {
    history.replaceState(null, '', newHash);
  }
}

// ============================================================================
// Router - Hash-based SPA navigation
// ============================================================================

/**
 * Route definitions with pattern matching
 * :param syntax for dynamic segments
 */
const ROUTES = [
  { pattern: '/', view: 'dashboard' },
  { pattern: '/issues', view: 'issues' },
  { pattern: '/issue/:id', view: 'issue' },
  { pattern: '/insights', view: 'insights' },
  { pattern: '/graph', view: 'graph' },
];

/**
 * Parse hash into view and params
 */
function parseRoute(hash) {
  // Remove leading # and extract path vs query
  const hashContent = hash.slice(1) || '/';
  const [path, query] = hashContent.split('?');
  const normalizedPath = path.startsWith('/') ? path : '/' + path;

  // Try to match each route pattern
  for (const route of ROUTES) {
    const match = matchPattern(route.pattern, normalizedPath);
    if (match) {
      return {
        view: route.view,
        params: match.params,
        query: query ? new URLSearchParams(query) : new URLSearchParams(),
      };
    }
  }

  // Default to dashboard
  return { view: 'dashboard', params: {}, query: new URLSearchParams() };
}

/**
 * Match a URL path against a pattern with :param placeholders
 */
function matchPattern(pattern, path) {
  const patternParts = pattern.split('/').filter(Boolean);
  const pathParts = path.split('/').filter(Boolean);

  // Handle root route
  if (patternParts.length === 0 && pathParts.length === 0) {
    return { params: {} };
  }

  if (patternParts.length !== pathParts.length) {
    return null;
  }

  const params = {};
  for (let i = 0; i < patternParts.length; i++) {
    const patternPart = patternParts[i];
    const pathPart = pathParts[i];

    if (patternPart.startsWith(':')) {
      // Dynamic segment - capture as param
      params[patternPart.slice(1)] = decodeURIComponent(pathPart);
    } else if (patternPart !== pathPart) {
      // Static segment mismatch
      return null;
    }
  }

  return { params };
}

/**
 * Navigate to a route (pushes to history)
 */
function navigate(path) {
  const newHash = path.startsWith('#') ? path : '#' + path;
  if (window.location.hash !== newHash) {
    window.location.hash = newHash;
  }
}

/**
 * Navigate to issue detail
 */
function navigateToIssue(id) {
  navigate(`/issue/${encodeURIComponent(id)}`);
}

/**
 * Navigate to issues list with filters
 */
function navigateToIssues(filters = {}, sort = 'priority', search = '') {
  const params = filtersToURL(filters, sort, search);
  navigate(`/issues${params ? '?' + params : ''}`);
}

/**
 * Navigate to dashboard
 */
function navigateToDashboard() {
  navigate('/');
}

/**
 * Go back in history, or to a fallback
 */
function goBack(fallback = '/') {
  if (window.history.length > 1) {
    window.history.back();
  } else {
    navigate(fallback);
  }
}

// ============================================================================
// Alpine.js Application
// ============================================================================

/**
 * Format ISO date to readable string
 */
function formatDate(isoString) {
  if (!isoString) return '';
  try {
    const date = new Date(isoString);
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return isoString;
  }
}

/**
 * Render markdown safely
 */
function renderMarkdown(text) {
  if (!text) return '';
  try {
    const html = marked.parse(text);
    return DOMPurify.sanitize(html);
  } catch {
    return DOMPurify.sanitize(text);
  }
}

/**
 * Main Alpine.js application component
 */
function beadsApp() {
  return {
    // State
    loading: true,
    loadingMessage: 'Initializing...',
    error: null,
    globalError: null,       // Modal error from ERROR_STATE
    showDiagnostics: false,  // Toggle for diagnostics panel (press 'd')
    diagnostics: DIAGNOSTICS, // Reference to global diagnostics
    wasmStatus: WASM_STATUS,  // WASM support status
    toasts: [],              // Toast notifications
    view: 'dashboard',
    mobileMenuOpen: false,   // Mobile hamburger menu state
    mobileSearchOpen: false, // Mobile search bar state
    filtersExpanded: false,  // Collapsible filters on mobile
    darkMode: localStorage.getItem('darkMode') !== null
      ? localStorage.getItem('darkMode') === 'true'
      : window.matchMedia('(prefers-color-scheme: dark)').matches,

    // Data
    stats: {},
    meta: {},
    dbSource: 'loading',

    // Issues list
    issues: [],
    totalIssues: 0,
    page: 1,
    pageSize: 20,

    // Filter options (populated from database)
    filterOptions: {
      statuses: [],
      types: [],
      priorities: [],
      assignees: [],
      labels: [],
    },

    // Filters (supports multi-select arrays)
    filters: {
      status: [],      // Array for multi-select
      type: [],        // Array for multi-select
      priority: [],    // Array for multi-select
      labels: [],      // Array for multi-select
      assignee: '',    // Single select
      hasBlockers: null, // true/false/null
      isBlocking: null,  // true/false/null
    },
    sort: 'priority',
    searchQuery: '',

    // Dashboard data
    topPicks: [],
    recentIssues: [],
    topByPageRank: [],
    topByTriageScore: [],
    topBlockers: [],
    quickWins: [],
    blockersToClose: [],
    distributionByType: [],
    distributionByPriority: [],

    // Selected issue
    selectedIssue: null,
    showDepGraph: false,
    issueNavList: [], // List of issue IDs for j/k navigation

    // Graph engine state
    graphReady: false,
    graphMetrics: null,
    whatIfResult: null,
    topKSet: null,

    // Insights view data
    topByBetweenness: [],
    topByCriticalPath: [],
    cycleInfo: null,
    topImpactIssues: [],

    /**
     * Initialize the application
     */
    async init() {
      // Apply dark mode
      if (this.darkMode) {
        document.documentElement.classList.add('dark');
      }

      // Listen for system preference changes (only if no stored preference)
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
        if (localStorage.getItem('darkMode') === null) {
          this.darkMode = e.matches;
          document.documentElement.classList.toggle('dark', this.darkMode);
          // Re-render Mermaid graphs if visible
          if (this.showDepGraph && this.selectedIssue) {
            this.renderDepGraph();
          }
        }
      });

      // Listen for toast events
      window.addEventListener('show-toast', (e) => {
        this.toasts.push(e.detail);
        setTimeout(() => {
          this.toasts = this.toasts.filter(t => t.id !== e.detail.id);
        }, 5000);
      });

      // Listen for keyboard shortcuts
      window.addEventListener('keydown', (e) => {
        // 'd' key toggles diagnostics panel (when not in input)
        if (e.key === 'd' && !['INPUT', 'TEXTAREA'].includes(e.target.tagName)) {
          this.showDiagnostics = !this.showDiagnostics;
        }
      });

      try {
        this.loadingMessage = 'Loading sql.js WebAssembly...';
        await loadDatabase((msg) => {
          this.loadingMessage = msg;
        });

        // Check for global errors that may have occurred during loading
        if (ERROR_STATE.error) {
          this.globalError = ERROR_STATE.error;
          this.loading = false;
          return;
        }

        this.dbSource = DB_STATE.source;
        this.loadingMessage = 'Loading data...';

        // Load initial data
        this.meta = getMeta();
        this.stats = getStats();
        DIAGNOSTICS.issueCount = this.stats.total || 0;

        this.topPicks = getTopPicks(5);
        this.recentIssues = getRecentIssues(10);
        this.topByPageRank = getTopByPageRank(10);
        this.topByTriageScore = getTopByTriageScore(10);
        this.topBlockers = getTopBlockers(10);

        // Dashboard data
        this.quickWins = getQuickWins(5);
        this.blockersToClose = getBlockersToClose(5);
        this.distributionByType = getDistributionByType();
        this.distributionByPriority = getDistributionByPriority();

        // Load filter options for dropdowns
        this.filterOptions = getFilterOptions();

        // Load issues for list view (initial data)
        this.loadIssues();

        // Handle initial route from URL hash
        if (window.location.hash) {
          this.handleHashChange();
        }

        // Initialize WASM graph engine (non-blocking)
        this.loadingMessage = 'Loading graph engine...';
        this.graphReady = await initGraphEngine();
        DIAGNOSTICS.graphWasm = this.graphReady;
        if (this.graphReady) {
          this.topKSet = getTopKSet(5);
          this.topByBetweenness = getTopByBetweenness(10);
          this.topByCriticalPath = getTopByCriticalPath(10);
          this.cycleInfo = getCycleInfo();
          this.topImpactIssues = topWhatIf(10);
        }

        // Listen for hash changes (browser back/forward)
        window.addEventListener('hashchange', () => this.handleHashChange());

        // Record load time
        DIAGNOSTICS.loadTimeMs = Date.now() - DIAGNOSTICS.startTime;

        this.loading = false;
      } catch (err) {
        console.error('Init failed:', err);
        this.error = err.message || 'Failed to load database';
        // Sync global error state
        if (ERROR_STATE.error) {
          this.globalError = ERROR_STATE.error;
        }
        this.loading = false;
      }
    },

    /**
     * Handle hash change (browser back/forward navigation)
     */
    handleHashChange() {
      const urlState = filtersFromURL();
      const hash = window.location.hash;

      // Parse route
      const route = parseRoute(hash);

      // Handle route
      switch (route.view) {
        case 'issue':
          // Issue detail view
          this.view = 'issues'; // Keep issues as backdrop
          if (route.params.id) {
            // Reset state when switching issues
            this.showDepGraph = false;
            this.whatIfResult = null;
            this.selectedIssue = getIssue(route.params.id);
            // Update nav list from current issues
            if (this.issues.length) {
              this.issueNavList = this.issues.map(i => i.id);
            }
          }
          break;

        case 'issues':
          this.view = 'issues';
          this.selectedIssue = null;
          this.showDepGraph = false;
          this.whatIfResult = null;
          this.filters = { ...this.filters, ...urlState.filters };
          this.sort = urlState.sort;
          this.searchQuery = urlState.searchQuery;
          this.page = 1;
          this.loadIssues();
          break;

        case 'insights':
          this.view = 'insights';
          this.selectedIssue = null;
          break;

        case 'graph':
          this.view = 'graph';
          this.selectedIssue = null;
          break;

        default:
          this.view = 'dashboard';
          this.selectedIssue = null;
      }
    },

    /**
     * Load issues based on current filters
     */
    loadIssues() {
      const offset = (this.page - 1) * this.pageSize;
      const filters = {
        ...this.filters,
        search: this.searchQuery,
      };

      this.issues = queryIssues(filters, this.sort, this.pageSize, offset);
      this.totalIssues = countIssues(filters);

      // Sync URL state (only on issues view)
      if (this.view === 'issues') {
        syncFiltersToURL('issues', this.filters, this.sort, this.searchQuery);
      }
    },

    /**
     * Apply filter and reload (resets to page 1)
     */
    applyFilter() {
      this.page = 1;
      this.loadIssues();
    },

    /**
     * Clear all filters
     */
    clearFilters() {
      this.filters = {
        status: [],
        type: [],
        priority: [],
        labels: [],
        assignee: '',
        hasBlockers: null,
        isBlocking: null,
      };
      this.searchQuery = '';
      this.sort = 'priority';
      this.page = 1;
      this.loadIssues();
    },

    /**
     * Check if any filters are active
     */
    get hasActiveFilters() {
      return this.filters.status?.length > 0 ||
             this.filters.type?.length > 0 ||
             this.filters.priority?.length > 0 ||
             this.filters.labels?.length > 0 ||
             this.filters.assignee ||
             this.filters.hasBlockers !== null ||
             this.filters.isBlocking !== null ||
             this.searchQuery;
    },

    /**
     * Toggle a value in a multi-select filter array
     */
    toggleFilter(filterName, value) {
      if (!Array.isArray(this.filters[filterName])) {
        this.filters[filterName] = [];
      }
      const index = this.filters[filterName].indexOf(value);
      if (index === -1) {
        this.filters[filterName].push(value);
      } else {
        this.filters[filterName].splice(index, 1);
      }
      this.applyFilter();
    },

    /**
     * Check if a value is selected in a multi-select filter
     */
    isFilterSelected(filterName, value) {
      if (!Array.isArray(this.filters[filterName])) return false;
      return this.filters[filterName].includes(value);
    },

    /**
     * Search issues
     */
    search() {
      this.page = 1;
      this.loadIssues();
    },

    /**
     * Pagination
     */
    nextPage() {
      if (this.page * this.pageSize < this.totalIssues) {
        this.page++;
        this.loadIssues();
      }
    },

    prevPage() {
      if (this.page > 1) {
        this.page--;
        this.loadIssues();
      }
    },

    /**
     * Show issue detail (navigates to issue route)
     */
    showIssue(id) {
      navigateToIssue(id);
    },

    /**
     * Close issue detail (navigates back)
     */
    closeIssue() {
      // Reset issue detail state
      this.selectedIssue = null;
      this.showDepGraph = false;
      this.whatIfResult = null;

      // Navigate back
      const currentView = this.view;
      if (currentView === 'issues') {
        navigateToIssues(this.filters, this.sort, this.searchQuery);
      } else {
        navigate('/' + currentView);
      }
    },

    /**
     * Navigate to next/previous issue in the list
     * @param {number} direction - 1 for next, -1 for previous
     */
    navigateIssue(direction) {
      if (!this.selectedIssue) return;

      // Build navigation list from current issues if not set
      if (!this.issueNavList.length && this.issues.length) {
        this.issueNavList = this.issues.map(i => i.id);
      }

      // Find current position
      const currentId = this.selectedIssue.id;
      const currentIndex = this.issueNavList.indexOf(currentId);

      if (currentIndex === -1) {
        // Current issue not in nav list, just stay
        return;
      }

      // Calculate new index with wrapping
      const newIndex = (currentIndex + direction + this.issueNavList.length) % this.issueNavList.length;
      const newId = this.issueNavList[newIndex];

      // Reset state and navigate
      this.showDepGraph = false;
      this.whatIfResult = null;
      navigateToIssue(newId);
    },

    /**
     * Parse JSON labels string to array
     */
    parseLabels(labelsStr) {
      if (!labelsStr) return [];
      try {
        const labels = JSON.parse(labelsStr);
        return Array.isArray(labels) ? labels : [];
      } catch {
        return [];
      }
    },

    /**
     * Render Mermaid dependency graph for the selected issue
     */
    async renderDepGraph() {
      if (!this.selectedIssue || !this.$refs.depGraph) return;

      const issue = this.selectedIssue;
      const blockedBy = (issue.blocked_by_ids || '').split(',').filter(Boolean).map(s => s.trim());
      const blocks = (issue.blocks_ids || '').split(',').filter(Boolean).map(s => s.trim());

      if (blockedBy.length === 0 && blocks.length === 0) {
        this.$refs.depGraph.innerHTML = '<p class="text-gray-500 dark:text-gray-400 text-center">No dependencies</p>';
        return;
      }

      // Build Mermaid flowchart
      let diagram = 'flowchart TB\n';

      // Sanitize ID for mermaid (replace special chars)
      const sanitizeId = (id) => id.replace(/[^a-zA-Z0-9]/g, '_');
      const currentId = sanitizeId(issue.id);

      // Style for current node
      diagram += `  ${currentId}["${issue.id}"]\n`;
      diagram += `  style ${currentId} fill:#0ea5e9,stroke:#0284c7,color:#fff\n`;

      // Add blocked-by nodes (upstream)
      for (const depId of blockedBy) {
        const nodeId = sanitizeId(depId);
        diagram += `  ${nodeId}["${depId}"]\n`;
        diagram += `  ${nodeId} --> ${currentId}\n`;
        diagram += `  style ${nodeId} fill:#fef3c7,stroke:#f59e0b\n`;
      }

      // Add blocks nodes (downstream)
      for (const depId of blocks) {
        const nodeId = sanitizeId(depId);
        diagram += `  ${nodeId}["${depId}"]\n`;
        diagram += `  ${currentId} --> ${nodeId}\n`;
        diagram += `  style ${nodeId} fill:#fee2e2,stroke:#ef4444\n`;
      }

      // Add click handlers
      const allIds = [issue.id, ...blockedBy, ...blocks];
      for (const id of allIds) {
        const nodeId = sanitizeId(id);
        diagram += `  click ${nodeId} call window.beadsViewer.navigateToIssue("${id}")\n`;
      }

      try {
        // Render the diagram
        const { svg } = await mermaid.render('dep-graph-' + Date.now(), diagram);
        this.$refs.depGraph.innerHTML = svg;
      } catch (err) {
        console.warn('Mermaid render failed:', err);
        this.$refs.depGraph.innerHTML = '<p class="text-red-500 text-center text-sm">Failed to render graph</p>';
      }
    },

    /**
     * Toggle dark mode
     */
    toggleDarkMode() {
      this.darkMode = !this.darkMode;
      localStorage.setItem('darkMode', this.darkMode);
      document.documentElement.classList.toggle('dark', this.darkMode);

      // Re-initialize Mermaid with new theme
      if (window.reinitializeMermaid) {
        window.reinitializeMermaid();
      }

      // Re-render visible Mermaid graph if any
      if (this.showDepGraph && this.selectedIssue) {
        this.renderDepGraph();
      }
    },

    // ========================================================================
    // Graph Engine Methods
    // ========================================================================

    /**
     * Recalculate graph metrics for currently filtered issues
     */
    recalculateForFilter() {
      if (!this.graphReady) return;
      const ids = this.issues.map(i => i.id);
      this.graphMetrics = recalculateMetrics(ids);
    },

    /**
     * Compute what-if cascade impact for an issue
     */
    computeWhatIf(issueId) {
      if (!this.graphReady) return;
      this.whatIfResult = whatIfClose(issueId);
    },

    /**
     * Clear what-if result
     */
    clearWhatIf() {
      this.whatIfResult = null;
    },

    /**
     * Refresh top-k set
     */
    refreshTopKSet() {
      if (!this.graphReady) return;
      this.topKSet = getTopKSet(5);
    },

    /**
     * Get top issues by cascade impact
     */
    getTopImpact(limit = 10) {
      if (!this.graphReady) return [];
      return topWhatIf(limit);
    },

    /**
     * Get cycle break suggestions
     */
    getCycleBreaks(limit = 5) {
      if (!this.graphReady) return null;
      return getCycleBreakSuggestions(limit);
    },

    /**
     * Format date helper
     */
    formatDate,

    /**
     * Render markdown helper
     */
    renderMarkdown,

    /**
     * Get issue by ID (wrapper for templates)
     */
    getIssue,

    /**
     * Dismiss the current error modal
     */
    dismissError() {
      if (this.globalError && this.globalError.dismissible) {
        this.globalError = null;
        clearError();
      }
    },

    /**
     * Remove a toast notification
     */
    removeToast(id) {
      this.toasts = this.toasts.filter(t => t.id !== id);
    },
  };
}

// Export for use in graph integration
window.beadsViewer = {
  // Database
  DB_STATE,
  loadDatabase,
  execQuery,
  queryIssues,
  countIssues,
  getIssue,
  getIssueDependencies,
  getStats,
  getMeta,
  getFilterOptions,
  getUniqueLabels,
  searchIssues,

  // URL State & Router
  filtersToURL,
  filtersFromURL,
  syncFiltersToURL,
  parseRoute,
  matchPattern,
  navigate,
  navigateToIssue,
  navigateToIssues,
  navigateToDashboard,
  goBack,

  // Graph Engine
  GRAPH_STATE,
  initGraphEngine,
  buildClosedSet,
  recalculateMetrics,
  whatIfClose,
  topWhatIf,
  getActionableIssues,
  getCycleBreakSuggestions,
  getTopKSet,

  // WASM Fallback
  WASM_STATUS,
  checkWASMSupport,
  enableFallbackMode,

  // Insights helpers
  getTopByBetweenness,
  getTopByCriticalPath,
  getCycleInfo,

  // Error handling
  ERROR_STATE,
  DIAGNOSTICS,
  showError,
  clearError,
  safeQuery,
  showToast,
};
