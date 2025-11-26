# Beads Viewer (bv)

A polished, high-performance TUI for managing and exploring [Beads](https://github.com/steveyegge/beads) issue trackers.

## Features

### 🧠 Graph Theory Analytics
*   **Critical Path Analysis**: Automatically identifies "Deep" tasks that block long chains of work (Impact Score).
*   **Centrality Metrics**: Computes PageRank and Betweenness to highlight structural bottlenecks.
*   **Cycle Detection**: Warns about circular dependencies.

### 🖥️ Visual Dashboard
*   **Kanban Board**: Press `b` to toggle a 4-column Kanban board.
*   **Adaptive Split-View**: Master-detail dashboard on wide screens.
*   **Ultra-Wide Layouts**: Shows Impact Scores (🌋/🏔️) on large displays.

### ⚡ Workflow
*   **Instant Filtering**: `o` (Open), `r` (Ready), `c` (Closed), `a` (All).
*   **Mermaid Export**: `bv --export-md report.md` generates a report with a visual dependency graph.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/Dicklesworthstone/beads_viewer/main/install.sh | bash
```

## Usage

```bash
bv
```

### Keybindings

| Key | Context | Action |
| :--- | :--- | :--- |
| `b` | Global | Toggle **Kanban Board** |
| `Tab` | Split View | Switch focus |
| `h`/`j`/`k`/`l`| Board | Navigate |
| `o` / `r` / `c` | Global | Filter status |
| `q` | Global | Quit |

## CI/CD

*   **CI**: Runs tests on every push.
*   **Release**: Builds binaries for all platforms.

## License

MIT