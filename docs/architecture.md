# Architecture

## Overview

devclean is a monolithic Go CLI binary. All ecosystem scanners are built into a single binary with no external plugins.

## Pipeline

```
Scan → Classify → Filter/Sort → Output/Clean
```

1. **Scan**: Registry batches all project (walk-based) scanners into a single filesystem pass, then runs stat scanners (fixed paths) sequentially. Real-time progress via spinner.
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
  ui/                  → terminal spinner, tree selector (bubbletea), shared styles + display formatters (StatusBadge, SafetyIcon, RelativeTime) reused by output/
```

## Scanner Design

Two scanner families share the `Scanner` interface:

- **Walk ecosystems** (Node, Rust, Ruby, Python, Go): declarative rule tables (`walkEcosystem` in `internal/scanner/walk.go`) executed by a single-pass walk engine. The registry partitions walk-based scanners out of every scan and batches them into **one** recursive filesystem traversal, dispatching each directory against all active tables — one pass regardless of how many project ecosystems are active. The engine reads each directory **once** (`os.ReadDir`) and reuses those entries for both marker detection and recursion.
- **Stat scanners** (Xcode, Global Caches, LLM Model Stores): implement `Scanner` directly and check fixed, known paths instead of walking a tree.

```go
type Scanner interface {
    Name() string
    Ecosystem() Ecosystem
    Scan(ctx context.Context, root string) ([]ScanResult, error)
}
```

Scanners report progress via context-attached callbacks: the walk batch reports under a single "projects" label, stat scanners under their own names.

**Sizing** collects two figures per artifact through an in-process walk (`scanner.Measure`):

- **disk** — allocated blocks (`st_blocks×512`), sparse-accurate and matching `du`. The primary figure: sorting, `--min-size`, and totals all use it. Directories contribute their own blocks (real on ext4, ~0 on APFS).
- **apparent** — sum of logical file sizes. Surfaces only when a file is materially sparse.

Two invariants keep the figures honest:

- **Hard links** (`Nlink>1`) are counted once per artifact, keyed by `(dev, ino)`, so shared blocks net out across artifacts.
- **Symlinks** are never followed.

Neither scanner family sizes inline. Each collects its artifacts first — the walk during its single pass, stat scanners via `stat` — then sizes them through one shared bounded worker pool (`sizePending`, `min(NumCPU, 8)`) so the tree-walk I/O overlaps.

### Walk engine

A `walkEcosystem` table declares how one ecosystem participates in the shared walk: marker file names that identify a project root (`package.json`, `Cargo.toml`, any of six Python markers, …) and artifact rules that apply beneath those roots. Rules come in three match forms, each carrying its own category and safety:

| Form | Example | Semantics |
|------|---------|-----------|
| exact relative path | `node_modules`, `vendor/bundle`, `ios/Pods` | matches exactly that path under the nearest project root |
| any-depth name | `__pycache__`, `.venv` | matches the directory name anywhere under the nearest project root |
| any-depth suffix | `*.egg-info` | matches a directory-name suffix anywhere under the nearest project root |

Per directory, the engine matches artifact rules against the **nearest** enclosing project root of each active ecosystem (table order, first match wins), emits the artifact and skips its subtree on a match, and otherwise reads the directory once to detect new project roots (pushed onto a recursion-scoped context stack) before descending into its child directories. That single `os.ReadDir` serves both marker detection and recursion — no directory is read twice. Artifact matching runs **before** the hidden-directory check so compound rules ending in a hidden segment (`android/.gradle`) still match; unmatched hidden directories are descended into only when an active ecosystem lists the name as an artifact.

Tables can also:

- contribute **per-project extra rules** on root detection — the Node table adds React Native compound artifacts (`ios/Pods`, `android/.gradle`, …) when the project carries `ios/Podfile` or `metro.config.{js,ts,cjs,mjs}`;
- opt into **`ScanResult.ProjectRoot` attribution** — the Python table sets the matched root on every result because its artifacts sit at arbitrary depth and output grouping needs explicit attribution (nested project roots win over parents).

**Deduplication**: a directory matching rules of several active ecosystems is reported once, attributed to the first table in order (node → rust → ruby → python → go), and no scanner descends into another's matched artifact (`__pycache__` inside `node_modules` is not reported separately). Attribution can therefore differ between a full scan and an `--eco` subset scan — a shared `coverage/` goes to node in a full scan, to ruby under `--eco ruby`.

**Symlink policy — never follow**: the walk skips any entry that is a symlink (explicit `os.ModeSymlink` check), so a symlink is never descended into and never matched as an artifact, even when it is named like one (a symlinked `node_modules`, as pnpm and some monorepos produce). This is deliberate: a symlink's target is real content that lives on disk elsewhere, so following it would double-count that space and inflate the reported reclaimable total, and deleting a symlinked artifact reclaims only the link (bytes) while risking a target shared by other projects. No-follow also means symlink cycles can never be walked, so no separate cycle guard is needed. The reclaimable content behind these links is surfaced instead by the Global Caches scanner (e.g. the pnpm store) and hardlink-aware sizing, not by following per-project links.

Results are sorted by (table order, path) before returning, keeping output order stable.

### Display Units

`model.HumanSize` formats sizes with **decimal SI units** (1 KB = 1000 B). The CLI's `--min-size` flag uses the same convention by default (humanize.ParseBytes), so the threshold a user types and the size they see in output agree on the same arithmetic. Internally sizes come from `st_blocks×512` (binary 512-byte units), but the formatting layer is decimal — so a 1 GiB directory renders as `1.1 GB` and `--min-size 1GB` will include it. Binary suffixes (`KiB`, `MiB`, `GiB`) are still accepted by `--min-size` for users who want explicit binary thresholds.

**Sparse-aware display**: the table shows an artifact's real on-disk size, annotating it with the larger size the file nominally reports when that apparent size exceeds double the disk figure by more than 1 GiB — e.g. a `Docker.raw` image renders `24.0 GB (appears as 460.0 GB)`, making clear it only uses 24 GB on disk though it presents as 460 GB. The JSON output always carries `apparent_size` (`omitzero`, so dropped when zero) so agents can detect sparse files. Ordinary directories, where block-rounding leaves apparent ≤ disk, are never annotated.

### Metadata Enrichment

`ScanResult` carries two optional display fields populated by scanners after detection:

- `Label` — human-readable display name when the path basename is opaque (e.g. simulator UUID `027AEA9C-...` → `iPhone 17 Pro · iOS 26.3`). Output renders Label in place of basename when present.
- `Recommendation` — actionable hint shown as a trailing tag (e.g. `superseded by newer build`, `runtime unavailable — safe to remove`). Lets the user decide what to delete without external lookup.

Scanners derive these from peer comparison (DeviceSupport build ages), vendor APIs (`xcrun simctl list devices --json` for simulator names), or known-name maps (DerivedData shared subdirectories). Enrichment is best-effort — if the vendor command fails or returns malformed data, the scan still works with raw basenames.

Use this pattern when a single ecosystem produces directories whose names alone don't tell the user what they are.

### Vendor Cleanup

Scanners may opt into an additional interface to register ecosystem-native cleanup commands:

```go
type VendorCleaner interface {
    VendorCleanups() []VendorCleanup
}

type VendorCleanup struct {
    ID          string
    Description string
    Command     string                          // for dry-run display
    Run         func(ctx context.Context) error // executes the command
}
```

`devclean clean --vendor-cleanup` collects cleanups from selected ecosystems and runs them alongside path-based deletion. Vendor commands keep the ecosystem's internal state consistent (e.g. `xcrun simctl delete unavailable` removes simulator devices and updates CoreSimulator's database in one step). Dry-run prints the command without executing.

Current implementations:
- `xcode`: `xcrun simctl delete unavailable`

Natural future fits: Docker `system prune`, Homebrew `cleanup`, Gradle `--stop`, pip cache purge.

## Safety Model

Three levels with gitignore-aware protection:

| Level | Meaning | Determined by |
|-------|---------|---------------|
| `safe` | freely deletable, auto-regenerated | declared per artifact rule / catalog entry |
| `caution` | deletable but may need rebuild | declared per artifact rule / catalog entry |
| `protected` | should not be deleted | tracked by git (not in .gitignore) + uncommitted changes |

Gitignored artifacts (node_modules, .next, etc.) are always deletable even in repos with uncommitted changes. Only tracked artifacts are protected.

## Activity Detection

Uses the most recent of three timestamps:
1. Artifact filesystem mtime
2. Git last commit time (`git log -1 --format=%ct`)
3. Project directory mtime

Classification forks git per repo (`git rev-parse` to resolve roots, `git status` + `git log` for protection and last-commit time, `git check-ignore` per dirty root). These run across a bounded worker pool (`min(NumCPU, 8)`) — root resolution over distinct project dirs, then git info over distinct roots, then check-ignore over dirty roots — instead of serially. As with the walk engine's sizing stage, the win is overlapping the forks, not replacing them; results are applied serially afterwards so output stays deterministic regardless of scheduling.

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

Trash moves use `os.Rename` into the Trash dir on the home volume. When the artifact lives on a different filesystem (external drive, separate partition), `os.Rename` fails with `EXDEV`; the cleaner falls back to a recursive copy followed by removing the original. The copy recreates directories, regular files (contents + permission bits), and symlinks (as links, never followed). The original is removed only after the copy fully succeeds — a mid-copy failure leaves the original intact and discards the partial copy, so a cross-device move can never lose data.
