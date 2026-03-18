# Architecture

## Overview

devclean is a monolithic Go CLI binary. All ecosystem scanners are built into a single binary with no external plugins.

## Pipeline

```
Scan → Classify → Filter/Sort → Output/Clean
```

1. **Scan**: Registry iterates ecosystem scanners sequentially, each walks the filesystem for artifacts. Real-time progress via spinner.
2. **Classify**: Activity status (time-based, using most recent of artifact mtime / git commit / project dir mtime) + git protection (gitignore-aware)
3. **Filter/Sort**: CLI flags filter by ecosystem, category, status; sort by size/time/name; top N projects
4. **Output**: Colored table with sub-package grouping (default) or JSON (`--json`)
5. **Clean**: Interactive project selection → Trash/Force delete choice → per-artifact deletion with results

## Package Structure

```
cmd/devclean/          → entrypoint, wires CLI
internal/
  model/               → core domain types (Ecosystem, Category, SafetyLevel, ScanResult, ArtifactDef)
  scanner/             → Scanner interface, Registry, per-ecosystem implementations
  classifier/          → activity status, git info (protection + last commit + gitignore-aware)
  cleaner/             → trash (macOS/Linux) + force delete, dry-run, protection enforcement
  output/              → JSON formatter, lipgloss colored table (ecosystem → project → sub-package → artifact)
  cli/                 → cobra commands (scan, clean, list)
  ui/                  → terminal spinner, tree selector (bubbletea), shared styles
```

## Scanner Design

Hybrid pattern:

- **Declarative**: most ecosystems define `ArtifactDef` structs (pattern, category, safety) and use filesystem walking to detect artifacts near marker files (e.g., `package.json`)
- **Custom**: complex ecosystems (Xcode fixed paths, Docker CLI) implement the `Scanner` interface directly

```go
type Scanner interface {
    Name() string
    Ecosystem() Ecosystem
    Scan(ctx context.Context, root string) ([]ScanResult, error)
}
```

Scanners report progress via context-attached callbacks for real-time UI updates. Size calculated using `du -sk` for accurate disk usage.

## Safety Model

Three levels with gitignore-aware protection:

| Level | Meaning | Determined by |
|-------|---------|---------------|
| `safe` | freely deletable, auto-regenerated | `ArtifactDef.AlwaysSafe` |
| `caution` | deletable but may need rebuild | `ArtifactDef.AlwaysSafe = false` |
| `protected` | should not be deleted | tracked by git (not in .gitignore) + uncommitted changes |

Gitignored artifacts (node_modules, .next, etc.) are always deletable even in repos with uncommitted changes. Only tracked artifacts are protected.

## Activity Detection

Uses the most recent of three timestamps:
1. Artifact filesystem mtime
2. Git last commit time (`git log -1 --format=%ct`)
3. Project directory mtime

Thresholds are configurable:

| Status | Default |
|--------|---------|
| active | < 7 days |
| recent | 7–30 days |
| stale | 30–90 days |
| dormant | 90+ days |

## Output Grouping

Table output groups results by: **ecosystem → project → sub-package → artifacts**

- Ecosystems sorted by total size descending
- Projects sorted by total size descending within each ecosystem
- Each project shows: name, status badge, protected badge, total size, relative time
- **Monorepo support**: artifacts grouped by git root. Sub-packages (apps/web, packages/ui) shown with headers and sizes
- Default mode collapses small sub-packages (< 1MB) with "... and N more packages"
- `-v` flag shows all sub-packages and artifacts

## Clean Flow

1. Scan + classify (same pipeline as scan command)
2. Interactive tree selector (bubbletea) — ecosystem → project → artifact hierarchy with checkboxes
   - `[space]` toggles project (all children) or individual artifact
   - `[←→]` jumps between projects, `[↑↓]` moves through items
   - Quick select: `[a]` all, `[n]` none, `[s]` safe only, `[d]` dormant only
   - Protected projects hidden with explanation
3. Choose: Move to Trash / Permanently delete / Cancel
4. Execute with per-artifact status output
5. Summary: items cleaned, space freed
