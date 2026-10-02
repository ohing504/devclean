# Architecture

Module boundaries and the contracts several modules keep together. User-visible behavior is in [SPEC.md](SPEC.md); package responsibilities are in each package's doc comment (`go doc ./internal/<pkg>`), type definitions in the code.

## Pipeline

```text
Scan → Classify → Filter/Sort → Output/Clean
```

devclean is a monolithic Go CLI binary: every ecosystem scanner is built in, with no external plugins.

1. **Scan**: Registry batches all project (walk-based) scanners into a single filesystem pass, then runs stat scanners (fixed paths) sequentially. Real-time progress via spinner.
2. **Classify**: activity status + gitignore-aware protection ([SPEC](SPEC.md#classification))
3. **Filter/Sort**: CLI flags filter by ecosystem, category, status; sort by size/time/name; top N projects
4. **Output**: colored table or JSON (`--json`)
5. **Clean**: selection → trash/force choice → per-artifact deletion ([SPEC](SPEC.md#clean))

## Scanner Design

Two scanner families share the `Scanner` interface:

- **Walk ecosystems** (Node, Rust, Ruby, Python, Go, Flutter, Android): declarative rule tables (`walkEcosystem` in `internal/scanner/walk.go`) executed by a single-pass walk engine. The registry partitions walk-based scanners out of every scan and batches them into **one** recursive filesystem traversal, dispatching each directory against all active tables — one pass regardless of how many project ecosystems are active. The engine reads each directory **once** (`os.ReadDir`) and reuses those entries for both marker detection and recursion.
- **Stat scanners** (Xcode, Docker, Global Caches, LLM Model Stores): implement `Scanner` directly and check fixed, known paths instead of walking a tree.

Scanners report progress via context-attached callbacks: the walk batch reports under a single "projects" label, stat scanners under their own names.

**Sizing** collects two figures per artifact through an in-process walk (`scanner.Measure`):

- **disk** — allocated blocks (`st_blocks×512`), sparse-accurate and matching `du`. The primary figure: sorting, `--min-size`, and totals all use it. Directories contribute their own blocks (real on ext4, ~0 on APFS).
- **apparent** — sum of logical file sizes. Surfaces only when a file is materially sparse.

Two invariants keep the figures honest:

- **Hard links** (`Nlink>1`) are counted once per artifact, keyed by `(dev, ino)`, so shared blocks net out across artifacts.
- **Shared traversal**: sizing walks with `fstree`, so it counts what finding reports and deletion removes.

Neither scanner family sizes inline. Each collects its artifacts first — the walk during its single pass, stat scanners via `stat` — then sizes them through one shared bounded worker pool (`sizePending`, `min(NumCPU, 8)`) so the tree-walk I/O overlaps.

### Shared traversal (`fstree`)

Finding, sizing and the pre-delete check walk through one package, so they cover the same files:

- **Read each directory once.**
- **Never follow symlinks.** Only the root is read through, and only for `--path` — [decision](decisions/symlink-no-follow.md)
- **Stay on the root's filesystem** (like `du -x`); a directory on another device, of unknown device, or unreadable is returned as skipped — [decision](decisions/one-filesystem.md)

### Walk engine

A `walkEcosystem` table declares how one ecosystem participates in the shared walk: marker file names that identify a project root (`package.json`, `Cargo.toml`, any of six Python markers, …) and artifact rules that apply beneath those roots. Rules come in three match forms, each carrying its own category and safety:

| Form | Example | Semantics |
|------|---------|-----------|
| exact relative path | `node_modules`, `vendor/bundle`, `ios/Pods` | matches exactly that path under the nearest project root |
| any-depth name | `__pycache__`, `.venv` | matches the directory name anywhere under the nearest project root |
| any-depth suffix | `*.egg-info` | matches a directory-name suffix anywhere under the nearest project root |

Per directory, the engine matches artifact rules against the **nearest** enclosing project root of each active ecosystem (table order, first match wins), emits the artifact and skips its subtree on a match, and otherwise reads the directory once to detect new project roots (pushed onto a recursion-scoped context stack) before descending into its child directories. Artifact matching runs **before** the hidden-directory check so compound rules ending in a hidden segment (`android/.gradle`) still match; unmatched hidden directories below the scan root are descended into only when an active ecosystem lists the name as an artifact.

Tables can also:

- contribute **per-project extra rules** on root detection — the Node table adds React Native compound artifacts (`ios/Pods`, `android/.gradle`, …) when the project carries `ios/Podfile` or `metro.config.{js,ts,cjs,mjs}`;
- opt into **`ScanResult.ProjectRoot` attribution** — the Python table sets the matched root on every result because its artifacts sit at arbitrary depth and output grouping needs explicit attribution (nested project roots win over parents);
- declare a **`PruneRoot`** hook that skips the ecosystem's own SDK checkout.

**Pruned trees** are skipped whole — no artifact matching, no project context, no descent — so nothing inside is ever a deletion target: installed-package trees (pnpm store version dirs, macOS `.app` bundles) engine-wide, and an ecosystem's SDK checkout via `PruneRoot` — [decision](decisions/prune-non-project-trees.md).

**Deduplication**: a directory matching rules of several active ecosystems is reported once, attributed to the first table in order (node → rust → ruby → python → go → flutter → android), and no scanner descends into another's matched artifact (`__pycache__` inside `node_modules` is not reported separately). Attribution can therefore differ between a full scan and an `--eco` subset scan.

Results are sorted by (table order, path) before returning, keeping output order stable.

### Metadata Enrichment

`ScanResult` carries two optional display fields populated by scanners after detection:

- `Label` — human-readable display name when the path basename is opaque (e.g. simulator UUID → `iPhone 17 Pro · iOS 26.3`). Every view that lists artifacts builds its row cells from the same `internal/ui` functions.
- `Recommendation` — actionable hint shown as a trailing tag (e.g. `superseded by newer build`). Lets the user decide what to delete without external lookup.

Scanners derive these from peer comparison (DeviceSupport build ages), vendor APIs (`xcrun simctl list devices --json` for simulator names), known-name maps (DerivedData shared subdirectories), or vendor metadata files (DerivedData `info.plist` → source workspace path). Enrichment is best-effort — if the vendor command fails or returns malformed data, the scan still works with raw basenames.

Use this pattern when a single ecosystem produces directories whose names alone don't tell the user what they are.

### Deletion Strategy

Reclaiming is not always `os.RemoveAll` on a path — Docker images or simulator devices are reclaimed through vendor commands or API calls. Each `ScanResult` carries an optional `Delete *model.DeleteMethod` (kind path/command/api + display + `Run`); nil means path removal.

The cleaner applies its policy gates (protected refusal, dry-run) uniformly, then executes: `Delete.Run(ctx)` when a method is attached, otherwise path removal (trash or permanent). A method with a nil `Run` is refused rather than falling back to path removal — a misconfigured item must never delete a path its method didn't intend. Trash/permanent choice only applies to path removal. Path removal (including dry-run) is refused when `fstree` skips any directory in the item: a recursive delete does not stop at mounts. Example: the global scanner's Homebrew item (`brew cleanup`).

In JSON output, non-path items serialize as `"delete": {"kind": "command", "display": "..."}` (`Run` never serializes), so agents can tell strategies apart; absence of the key means path removal.

### Vendor Cleanup

Scanners may implement the optional `VendorCleaner` interface to register ecosystem-native cleanup commands. `VendorCleanup` embeds `model.DeleteMethod`: it is the ecosystem-level **bulk** counterpart of a per-item `ScanResult.Delete`, sharing one execution contract. Bulk actions (e.g. `pnpm store prune`) register here; per-item non-path reclaims (e.g. `docker rmi <id>`) attach a `DeleteMethod` to their `ScanResult`.

`devclean clean --vendor-cleanup` collects cleanups from selected ecosystems and runs them alongside path-based deletion. Vendor commands keep the ecosystem's internal state consistent (e.g. `xcrun simctl delete unavailable` removes simulator devices and updates CoreSimulator's database in one step). Dry-run prints the command without executing.

Implemented commands: [SPEC](SPEC.md#vendor-cleanups). Natural future fits: Docker `system prune` (behind a future `--include-destructive` gate), Gradle `--stop`.

## Classification

Classification forks git per repo (`git rev-parse` to resolve roots, `git status` + `git log` for protection and last-commit time, `git check-ignore` per dirty root). These run across a bounded worker pool (`min(NumCPU, 8)`) — root resolution over distinct project dirs, then git info over distinct roots, then check-ignore over dirty roots — instead of serially. As with sizing, the win is overlapping the forks, not replacing them; results are applied serially afterwards so output stays deterministic regardless of scheduling.

## Adding an ecosystem

Pick the scanner family by where the artifacts live.

### Walk ecosystem — artifacts inside project trees

No `Scanner` implementation needed — add a rule table.

1. **Define the table**: `internal/scanner/<lang>.go` with a `walkEcosystem` value — marker file names plus artifact rules (exact rel-path, any-depth name, or suffix; each with category + safety). References: Rust (`rust.go`) smallest, Ruby (`ruby.go`) compound path, Python (`python.go`) any-depth + suffix rules with `SetProjectRoot`, Node (`node.go`) per-project extra rules, Flutter (`flutter.go`) `PruneRoot`.
2. **Register**: append the table to `walkEcosystemTable` in `internal/scanner/walk.go` (position = dedup attribution priority) and add `reg.Register(newWalkScanner(<lang>WalkEcosystem))` at the same position in `internal/scanner/registry_default.go`.
3. **Tests**: `internal/scanner/<lang>_test.go` calling `scanner.WalkScan(ctx, root, model.EcoYourLang)`. Cover detection, artifact discovery, edge cases (no marker, false-positive siblings, monorepo nesting if applicable). Use `t.TempDir()` for fixtures.

### Stat ecosystem — fixed paths, vendor APIs

1. **Define the scanner**: `internal/scanner/<name>.go` implementing `Scanner`. Xcode (`xcode.go`) is the reference — a fixed path catalog, scope-gated by the scan root.
2. **Register**: add `reg.Register(NewYourScanner())` in `internal/scanner/registry_default.go`.
3. **Tests**: `internal/scanner/<name>_test.go` exercising `Scan` directly.

### Both families

1. **Ecosystem id**: `EcoYourLang` constant in `internal/model/types.go`, appended to `AllEcosystems()`.
2. **SDK checkout check**: scan the tool's real SDK/toolchain checkout and verify nothing inside is flagged; fix any false positive in the same PR — [decision](decisions/prune-non-project-trees.md).
3. **Real-world run**: build and run `./devclean scan --path <real-project> --eco <yourlang>` against an actual project before declaring done. A wrong marker file or path-walk regression passes unit tests but fails in the wild.
4. **Docs**: add a section and status row in [SPEC](SPEC.md#ecosystems).
