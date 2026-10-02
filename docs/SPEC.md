# Spec

The behavior the user approved. Code and tests must match this file; a PR that changes behavior changes this file in the same diff. One rule per line or table row so a diff shows exactly what changed.

## Commands

Every command runs to completion via flags alone (`--yes`, `--json`); interactive prompts are optional, so scripts and AI agents never need a TTY.

Run `devclean --help` or `devclean <command> --help` for the most up-to-date flag reference.

`--path` (scan and clean) accepts a relative path, resolved against the current directory; output always carries absolute paths. The given directory is scanned even when its name starts with `.`.

### scan

Scan for reclaimable disk space. See `devclean scan --help` for all flags.

```bash
# Scan workspace for Node.js projects
devclean scan --path ~/workspace --eco node

# Top 10 largest dormant projects
devclean scan --status dormant -n 10

# Sort by last activity
devclean scan --sort time

# Skip noise — only artifacts ≥ 100 MB
devclean scan --min-size 100MB

# Verbose: show all sub-packages and artifacts
devclean scan --path ~/workspace --eco node -v

# JSON output for scripting / AI agents
devclean scan --eco node --json
```

#### Output

Colored table grouped by ecosystem → project → sub-package → artifacts:

```text
● node 3 projects · 5.2 GB
  my-app Active 2.1 GB · 3 days ago
  ~/workspace/my-app
    . (root) (1.8 GB)
      ✔ node_modules (deps)      1.7 GB
      ✔ .turbo (cache)          100.0 MB
    apps/web (300.0 MB)
      ✔ .next (build)           300.0 MB

Total: 5.2 GB (5 items)
Safe to clean: 5.2 GB

Legend: ✔ safe  ⚠ caution  ✖ protected   ● Active  ● Recent  ● Stale  ● Dormant
        Run 'devclean list' for details
```

A sparse artifact is shown as `8.6 GB (appears as 494.4 GB)` — real disk size, then the size it reports. When hard-linked blocks are shared across artifacts, the total counts them once and says so.

#### JSON (`--json`)

```json
{
  "total_size": 5583457484,
  "total_count": 5,
  "results": [
    {
      "path": "/Users/you/workspace/my-app/node_modules",
      "ecosystem": "node",
      "category": "deps",
      "size": 1782579200,
      "apparent_size": 1690123456,
      "last_modified": "2026-07-14T09:12:00Z",
      "activity": "active",
      "safety": "safe",
      "protected": false
    }
  ]
}
```

- `size` — disk usage (allocated blocks); sparse-aware, used for sorting and `--min-size`.
- `apparent_size` — logical size; omitted when zero. Much larger than `size` for sparse files.
- `total_size` — sum of `size` with hard-linked blocks counted once.

Also present when known: `reason`, `project_root`, `label`, `recommendation`, `last_used_at`.

### clean

Clean reclaimable disk space. See `devclean clean --help` for all flags.

```bash
# Interactive: select projects, choose trash vs delete
devclean clean --eco node

# Filter dormant only, then select interactively
devclean clean --eco node --status dormant

# Non-interactive: clean all safe dormant items
devclean clean --eco node --status dormant --safe --yes

# Preview without deleting (items a real run would refuse are marked "would fail")
devclean clean --eco node --dry-run --yes

# Force permanent delete (skip Trash)
devclean clean --eco node --force --yes

# Skip artifacts smaller than 50 MB
devclean clean --eco node --min-size 50MB

# Also run ecosystem-native cleanup commands (e.g. xcrun simctl delete unavailable)
devclean clean --eco xcode --vendor-cleanup --yes
```

#### `--min-size`

Both `scan` and `clean` accept `--min-size <size>` to drop artifacts below the
threshold. Useful when scanning a noisy workspace where lots of sub-MB
artifacts crowd out the real targets. Sizes use SI/IEC suffixes:

- Decimal: `KB`, `MB`, `GB` (1 KB = 1000 bytes)
- Binary: `KiB`, `MiB`, `GiB` (1 KiB = 1024 bytes)
- Plain integer = bytes

`devclean scan --min-size 100MB` keeps only artifacts ≥ 100 MB.

#### Non-interactive safety (`--yes` / `--include-caution`)

`--yes` skips the interactive selector for scripting and AI-agent use. To keep a
mis-classification from becoming silent data loss, `--yes` deletes only `safe`
(auto-regenerated) items by default. `caution` items — shared impact, or state
that is slow or impossible to regenerate — are skipped and reported:

```text
Skipped 3 caution item(s) — pass --include-caution to remove them with --yes.
```

Add `--include-caution` to also delete `caution` items non-interactively. `protected`
items are never deleted either way. The interactive selector (no `--yes`) is
unaffected — you still see and choose caution items yourself.

#### Vendor Cleanups

Some ecosystems ship official cleanup commands that are stricter or safer than
deleting paths directly (they keep the vendor's internal state consistent).
The `--vendor-cleanup` flag runs them in addition to the path-based cleanup.
They are scoped to the ecosystems you target: the `--eco` selection, or — when
`--eco` is omitted — the ecosystems of the artifacts actually being cleaned.

| Ecosystem | Command | What it does |
|-----------|---------|--------------|
| xcode | `xcrun simctl delete unavailable` | Removes simulator devices whose iOS/watchOS/tvOS runtime was uninstalled. |
| global | `npm cache clean --force` | Clears the npm package cache. |
| global | `yarn cache clean` | Clears the Yarn cache. |
| global | `pnpm store prune` | Removes unreferenced packages from the pnpm store. |
| global | `pip cache purge` | Removes all wheels from the pip cache. |
| global | `uv cache prune` | Removes outdated entries from the uv cache. |

Commands for tools not installed on the machine are skipped (detected via PATH
lookup). The `global` ecosystem runs every installed manager's prune together;
individual tools can't be targeted separately since they share one ecosystem.

`--dry-run` prints the commands without executing. `--vendor-cleanup` is
additive — combine with `--safe`, `--status`, `--yes` as usual.

#### Interactive Tree Selector

The clean command uses a tree selector matching scan's output style:

- `[↑↓]` move, `[←→]` jump between projects, `[space]` toggle (a project toggles all its children)
- `[a]` select all, `[n]` none, `[s]` safe only, `[d]` dormant only
- `[enter]` confirm, `[esc]` cancel
- Projects show `[✔]` selected, `[-]` partial, `[✖]` protected
- Protected projects are hidden with explanation

#### Flow

1. Scan and classify (with progress spinner)
2. Tree selector: select projects/artifacts interactively
3. Choose: Move to Trash / Permanently delete / Cancel
4. Execute with per-artifact results — items with an attached `DeleteMethod` run it instead of path removal (see [Deletion Strategy](ARCHITECTURE.md#deletion-strategy))
5. Summary: items cleaned and their total size, hard-linked blocks counted once (as in the selector footer and confirm prompt) — reported as `moved to Trash` for trash moves and `freed` for permanent deletes and `DeleteMethod` items (which run regardless of the trash/force choice)

#### Trash across filesystems

Trash moves use `os.Rename` into the Trash dir on the home volume. When the artifact lives on a different filesystem (external drive, separate partition), `os.Rename` fails with `EXDEV`; the cleaner falls back to a recursive copy followed by removing the original. The copy recreates directories, regular files (contents + permission bits), and symlinks (as links, never followed). The original is removed only after the copy fully succeeds — a mid-copy failure leaves the original intact and discards the partial copy, so a cross-device move can never lose data.

### list

List supported ecosystems, categories, activity statuses, and safety levels.

```bash
devclean list
```

## Classification

### Categories

| Category | Description |
|----------|-------------|
| `cache` | Global or local caches (npm, Gradle, pip, etc.) |
| `build` | Build artifacts (DerivedData, .next, dist, etc.) |
| `runtime` | Runtimes and simulators (iOS runtimes, Docker images) |
| `deps` | Dependencies (node_modules, venv, etc.) |

### Safety Levels

| Level | Icon | Description |
|-------|------|-------------|
| `safe` | ✔ (green) | Freely deletable, auto-regenerated on next build/install |
| `caution` | ⚠ (yellow) | Deletable but may require rebuild or has shared impact |
| `protected` | ✖ (gray) | Never deleted by devclean — set by the classifier (below) or declared by a scanner (Docker's VM image) |

`safe` and `caution` are declared per artifact rule / catalog entry. Choosing a level for a new artifact:

- **`safe`** — auto-regenerated by the ecosystem's own tooling on next build/install. Default for caches and build outputs.
- **`caution`** — deletable but loses something the user might value (hand-curated venvs, vendored deps, build cache that takes long to rebuild, distribution archives). Caution items are hidden from `--safe` runs.
- When in doubt, prefer `caution` — users can opt into deletion explicitly. Better to under-clean than to delete something irrecoverable.

**Gitignore-aware protection**: when a git repo has uncommitted changes anywhere, every artifact in it that is **not** gitignored becomes `protected` — tracked files and untracked-but-not-ignored files alike. Gitignored artifacts (`node_modules`, `.next`, …) stay deletable even in such repos. Clean repos skip this check.

### Activity Status

Uses the most recent of three timestamps:

1. Artifact filesystem mtime
2. Git last commit time (`git log -1 --format=%ct`)
3. Project directory mtime

Thresholds are fixed (making them configurable is planned — see [Configuration](#configuration)):

| Status | Default |
|--------|---------|
| active | < 7 days |
| recent | 7–30 days |
| stale | 30–90 days |
| dormant | 90+ days |

### Output Grouping

Table output groups results by: **ecosystem → project → sub-package → artifacts**

- Ecosystems sorted by total size descending (also after `-n` drops projects)
- Projects sorted by total size descending within each ecosystem
- Equal sizes (and equal `--sort` keys) are ordered by name/path ascending, so repeated runs print the same order
- Each project shows: name, status badge, protected badge, total size, relative time
- **Monorepo support**: artifacts grouped by git root. Sub-packages (apps/web, packages/ui) shown with headers and sizes
- Default mode collapses small sub-packages (< 1MB) with "... and N more packages"
- `-v` flag shows all sub-packages and artifacts
- Every view that lists artifacts (scan table, clean selector) renders one artifact the same way: name (`Label`, else path relative to the project), size, last-used and recommendation tags

### Display Units

Sizes use **decimal SI units** (1 KB = 1000 B). `--min-size` uses the same convention by default, so the threshold a user types and the size they see in output agree on the same arithmetic. Internally sizes come from `st_blocks×512` (binary 512-byte units), but the formatting layer is decimal — so a 1 GiB directory renders as `1.1 GB` and `--min-size 1GB` will include it. Binary suffixes (`KiB`, `MiB`, `GiB`) are still accepted by `--min-size` for users who want explicit binary thresholds.

**Sparse-aware display**: the table shows an artifact's real on-disk size, annotating it with the larger size the file nominally reports when that apparent size exceeds double the disk figure by more than 1 GiB — e.g. a `Docker.raw` image renders `24.0 GB (appears as 460.0 GB)`, making clear it only uses 24 GB on disk though it presents as 460 GB. The JSON output always carries `apparent_size` (`omitzero`, so dropped when zero) so agents can detect sparse files. Ordinary directories, where block-rounding leaves apparent ≤ disk, are never annotated.

## Ecosystems

### Supported Ecosystems

| ID | Name | Status |
|----|------|--------|
| `node` | Node.js | implemented |
| `rust` | Rust | implemented |
| `ruby` | Ruby | implemented |
| `xcode` | iOS/Xcode (macOS only) | implemented |
| `python` | Python | implemented |
| `go` | Go | implemented (per-project only) |
| `flutter` | Flutter/Dart | implemented |
| `android` | Android | implemented |
| `global` | Global Caches | implemented |
| `llm` | LLM Model Stores | implemented |
| `docker` | Docker | implemented (scan only; prune deferred) |

**Dedup attribution**: project ecosystems (node, rust, ruby, python, go, flutter, android) share a single-pass scan — a directory matching artifact rules of several active ecosystems is reported once, attributed to the first in scanner order (node → rust → ruby → python → go → flutter → android), so `--eco` subsets can shift attribution (a shared `coverage/` goes to node in a full scan, to ruby under `--eco ruby`).

### Node.js

**Detection**: `package.json` in parent directory

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `node_modules` | deps | safe | NPM dependencies |
| `.next` | build | safe | Next.js build cache |
| `.nuxt` | build | safe | Nuxt.js build cache |
| `.output` | build | safe | Nuxt 3 output |
| `dist` | build | safe | Build output |
| `.turbo` | cache | safe | Turborepo cache |
| `.parcel-cache` | cache | safe | Parcel cache |
| `coverage` | build | safe | Test coverage reports |
| `.svelte-kit` | build | safe | SvelteKit cache |

**pnpm-installed `node_modules`**: pnpm clones (macOS) or hard-links (Linux) pnpm store files into `node_modules` by default, so deleting it alone frees little; running `pnpm store prune` afterwards removes packages no project references. The result carries this note when `node_modules/.modules.yaml` (written by pnpm on install; `pnpm-lock.yaml` alone does not prove pnpm populated the folder) names a `storeDir` that exists on the same volume. A store on another volume forces pnpm to copy, so no note is given. `packageImportMethod: copy` is not recorded in `.modules.yaml`, so such installs still get the note.

**Installed-package trees are excluded**: `node_modules`/`dist` inside a pnpm store or a macOS `.app` bundle are package content, not project output — see [decision](decisions/prune-non-project-trees.md).

**Monorepo support**: artifacts in sub-packages (apps/, packages/) are grouped under the git root project. Sub-packages are displayed with headers showing their path and total size.

**React Native / Expo**: when a Node project also has `ios/Podfile` or `metro.config.{js,ts,cjs,mjs}`, the scanner additionally collects RN-specific artifacts at multi-segment paths under the project root.

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `ios/Pods` | deps | safe | CocoaPods dependencies (restored by `pod install`) |
| `ios/build` | build | safe | iOS build output |
| `ios/DerivedData` | build | safe | Workspace-local DerivedData (rare) |
| `android/build` | build | safe | Android build output |
| `android/.gradle` | cache | safe | Project-local Gradle cache |
| `.expo` | cache | safe | Expo cache |
| `.metro` | cache | safe | Metro bundler cache |

### Rust

**Detection**: `Cargo.toml` in parent directory

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `target` | build | safe | Rust build artifacts (debug/release binaries, deps) |

**Note**: Tauri projects (Node + Rust hybrid) are detected by both Node.js and Rust scanners, catching both `node_modules` and `target/`.

### Ruby

**Detection**: `Gemfile` in parent directory

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `vendor/bundle` | deps | safe | Bundled gems (`bundle install` to restore) |
| `.bundle` | cache | safe | Bundler configuration and cache |
| `tmp` | cache | safe | Temporary files (Bootsnap compile cache, pids, sockets) |
| `log` | build | safe | Development and test logs |
| `coverage` | build | safe | Test coverage reports (SimpleCov) |
| `.ruby-lsp` | cache | safe | Ruby LSP editor cache |

**Note**: `node_modules` in Rails projects using jsbundling/cssbundling is detected by the Node.js scanner via `package.json`.

### Python

**Detection**: any of `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt`, `Pipfile`, `uv.lock` in a directory marks it as a Python project root. Artifacts are matched anywhere under that root (Python's `__pycache__` lives at every package depth, unlike `node_modules`).

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `__pycache__` | build | safe | Python bytecode cache (recursive — every package level) |
| `.pytest_cache` | cache | safe | pytest cache |
| `.mypy_cache` | cache | safe | mypy type-checker cache |
| `.ruff_cache` | cache | safe | ruff linter cache |
| `.tox` | build | safe | tox testing environments |
| `.nox` | build | safe | nox testing environments |
| `.ipynb_checkpoints` | cache | safe | Jupyter checkpoint files |
| `__pypackages__` | deps | safe | PEP 582 local dependencies |
| `*.egg-info` | build | safe | Packaging metadata (suffix match) |
| `.venv`, `venv` | deps | **caution** | Virtual environments — often hand-curated; not auto-deletable. See kondo PR #182 |

**Notes**:

- `dist`/`build` are intentionally **not** Python artifacts: those names collide with Node and would double-count for mixed projects. Users who need them deleted can do it manually or rely on the Node scanner.
- Nested projects (e.g. monorepo with sub-packages each having `pyproject.toml`) attribute artifacts to the **deepest** matching project root.

### Go

**Detection**: `go.mod` in parent directory.

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `vendor` | deps | **caution** | Vendored modules (regenerate with `go mod vendor`) |

**Notes**:

- `vendor/` is `caution` because `go mod vendor` is an opt-in choice — devs who vendor often do so for offline builds, reproducibility, or supply-chain pinning. Regeneration requires network access plus the original `go.sum`.
- Go's two big disk hogs — `~/.cache/go-build` (build cache) and `~/go/pkg/mod` (module cache) — are global, not per-project. They are reported by the Global Caches scanner so they aren't double-attributed to every Go project on the machine.

### Flutter/Dart

**Detection**: `pubspec.yaml` in parent directory.

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `build` | build | safe | Compiled output (what `flutter clean` removes) |
| `.dart_tool` | build | safe | Build-runner / tooling state (regenerates on next build) |

**Notes**:

- `build/` and `.dart_tool/` are exactly the two directories `flutter clean` deletes; both regenerate on the next build.
- **The Flutter SDK checkout is excluded.** The SDK is itself a git repo full of `pubspec.yaml` roots, and its `engine/src/build` / `engine/src/flutter/build` are committed GN build-system *source* trees — not build output. Matching `build` by name would offer real SDK source for deletion, and gitignore-aware protection misses it (committed-clean files are not `protected`). The scanner detects the SDK root by its invariant bootstrap layout (`bin/flutter` + `bin/internal/engine.version`, location-independent — never a hardcoded path) and skips the whole subtree. See [decision](decisions/prune-non-project-trees.md).
- The global pub package cache `~/.pub-cache` (macOS/Linux default) is home-rooted and handled by the Global Caches scanner as `caution` — it is shared by every Flutter project and re-downloads on the next `flutter pub get`. The `PUB_CACHE` env override is not tracked; only the default location is scanned.

### Android

**Detection**: `build.gradle` or `build.gradle.kts` in parent directory. Every Gradle module carries its own build script, so the root project and each subproject (`app/`, `feature/`, ...) is detected as its own project root.

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `build` | build | safe | Compiled output (what `gradle clean` removes) |
| `.gradle` | cache | safe | Per-project Gradle cache (regenerates on next build) |

**Notes**:

- Because each module has its own marker, the single `build` rule reclaims every module's output (`build`, `app/build`, `feature/build`, ...) without enumerating module names.
- **Scope is per-project only.** The shared Gradle user home (`~/.gradle/caches`), AVD images, and NDK / system-images are home-rooted and handled by the Global Caches scanner, not here — no duplicate registration.
- **No SDK exclusion needed** (unlike Flutter): the Android SDK ships no `build.gradle`, so it never establishes a project context and nothing inside it is offered for deletion.
- **React Native overlap**: an RN project nests an `android/` Gradle tree. `android/build` and `android/.gradle` are matched by node's RN rules first in scanner order, so they attribute to node; the android scanner still covers the deeper module builds (`android/app/build`) the RN rules do not list.

### Xcode (macOS only)

**Detection**: fixed paths under `~/Library/Developer/...` and `~/Library/Logs/...`. The scanner does not walk a project tree — it checks a known set of Xcode/CoreSimulator directories and reports the ones that exist. On non-darwin platforms, the scanner is a no-op.

**Scope rule**: each path is reported only when it is the same as, or a descendant of, the user-supplied scan root. Default root is `~`, so all paths are picked up. Narrowing the root (e.g. `--path ~/workspace`) excludes them.

**Artifacts**:

| Path (relative to home) | Category | Safety | Description |
|-------------------------|----------|--------|-------------|
| `Library/Developer/Xcode/DerivedData` | build | safe | Xcode build cache, regenerated on next build |
| `Library/Developer/Xcode/Archives` | build | caution | Distribution archives (TestFlight/App Store uploads) |
| `Library/Developer/Xcode/iOS DeviceSupport` | runtime | safe | iOS device debug symbols, refetched on device connect |
| `Library/Developer/Xcode/watchOS DeviceSupport` | runtime | safe | watchOS device debug symbols |
| `Library/Developer/Xcode/tvOS DeviceSupport` | runtime | safe | tvOS device debug symbols |
| `Library/Developer/CoreSimulator/Devices` | runtime | caution | Simulator devices and installed app data |
| `Library/Developer/CoreSimulator/Caches` | cache | safe | Simulator runtime caches |
| `Library/Logs/CoreSimulator` | cache | safe | Simulator logs |

**Expansion**: `DerivedData`, `iOS / watchOS / tvOS DeviceSupport`, and `CoreSimulator/Devices` are reported as one result *per child directory* (per project, per iOS version, per simulator device) instead of one big lump. Children share the parent path as `ProjectRoot` so they group together in output.

**Metadata enrichment** (populates `Label` and `Recommendation` on `ScanResult`):

| Source | Label | Recommendation |
|--------|-------|----------------|
| `CoreSimulator/Devices` (`xcrun simctl list devices --json`) | `iPhone 17 Pro · iOS 26.3` | `runtime unavailable — safe to remove` when Apple removed the runtime |
| `iOS DeviceSupport` (peer comparison by `mtime`) | (none) | `superseded by newer build` for older builds when the same `<model> <version>` group has multiple build IDs |
| `DerivedData` (well-known children) | `ModuleCache.noindex — Swift module cache (shared)` etc. | (none) |
| `DerivedData` (per-project children, `WorkspacePath` in `info.plist`) | source workspace path, e.g. `~/workspace/app/ios/Runner.xcworkspace` | `source project no longer exists — safe to remove` when that workspace is gone (deleted clone, removed worktree); not flagged while its external volume is unmounted |

`xcrun simctl` is best-effort — if Xcode CLI tools are not installed, simulator devices fall back to UUID display with no label/recommendation, but the rest of the scan still works.

**Notes**:

- `Archives` is `caution` because losing an archive means losing the ability to symbolicate crash reports for that release.
- `CoreSimulator/Devices` is `caution` because it contains app installs, settings, and user data inside simulators currently in use.

### Docker

**Detection**: a fixed path — the default Docker Desktop VM disk image `~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw`. A stat-based scanner (like Xcode / Global Caches), not a project-tree walk. A non-default Docker data root is not tracked; only the default location is scanned.

**Scope rule**: reported only when the path is the same as, or a descendant of, the scan root (default `~`). Narrowing the root excludes it.

**Artifacts**:

| Path (relative to home) | Category | Safety | Description |
|-------------------------|----------|--------|-------------|
| `Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw` | runtime | protected | Docker Desktop VM disk image — holds every image, container and volume |

**Notes**:

- **Protected, never path-deleted.** The image is a single sparse file holding all Docker state; deleting it destroys every image, container and volume. It is reported as `protected` so the cleaner refuses to remove it (`Protected: true`).
- **Sparse-aware sizing.** `Size` is the real on-disk usage (allocated blocks, `st_blocks×512`); `ApparentSize` is the image's declared size. A 460G-declared image reads as its real ~8G on disk — the shared sparse-aware `Measure` handles this with no Docker-specific code.
- **Reclaiming space is deferred.** Space inside the image is reclaimed by Docker's own prune (`docker system prune`), which is destructive (removes images/containers/volumes/build cache). That vendor cleanup is gated behind a future `--include-destructive` flag and is **not** wired up yet — this scanner reports the image and its real footprint only.

### Global Caches

Shared, home-rooted developer caches that are not tied to any single project. Unlike the per-project scanners, paths are fixed (home-relative) and span package managers and dev tools across ecosystems. Paths owned by a dedicated scanner (Xcode's DerivedData, DeviceSupport, Archives, CoreSimulator) are intentionally excluded to avoid double-counting.

Each entry that is `caution` carries a consequence-of-deletion note in `recommendation` (e.g. "every project re-downloads dependencies on next install") so a user — or an AI agent reading `--json` — can decide without external knowledge. Missing paths are skipped, so macOS (`~/Library/Caches/*`, `~/Library/pnpm/store`) and Linux (`~/.cache/*`) variants coexist in the catalog.

**Installed tool → `caution`.** A `safe` cache whose tool (`~/.npm` → `npm`, `~/.cache/uv` → `uv`, …) is installed is reported as `caution`, since deleting it only makes the tool download it again. "Installed" means found in PATH or in a user-level install directory (Homebrew prefix, `~/.local/bin`, `~/.bun/bin`, nvm, pyenv/rbenv/asdf/mise shims, …).

| Path | Category | Safety |
|------|----------|--------|
| `~/.npm` | cache | safe |
| `~/.bun/install/cache` | cache | safe |
| `~/.cocoapods` | cache | safe |
| `~/.cache/uv`, `~/.cache/puppeteer` | cache | safe (XDG paths, used on macOS too) |
| `~/Library/Caches/{Yarn,pnpm,pip,CocoaPods,go-build,electron,node-gyp,typescript,uv,Cypress,deno,pypoetry}` | cache | safe |
| `~/Library/Caches/Homebrew` | cache | safe (see Homebrew below) |
| `~/.cache/{go-build,pip,node-gyp,yarn,pnpm,electron,Cypress,deno,pypoetry}` | cache | safe |
| `~/Library/pnpm/store` | cache | caution (hard-linked store) |
| `~/.gradle/caches`, `~/.gradle/wrapper/dists` | cache | caution |
| `~/.cargo/registry`, `~/.cargo/git` | cache | caution |
| `~/go/pkg/mod` | deps | caution (read-only files) |
| `~/.pub-cache` | deps | caution (shared by all Flutter projects; re-downloads on next `flutter pub get`) |
| `~/Library/Caches/ms-playwright`, `~/.cache/ms-playwright` | cache | caution |
| `~/.rustup/toolchains` | runtime | caution (toolchains must be reinstalled) |
| `~/.nvm/versions`, `~/.pyenv/versions`, `~/.rbenv/versions` | runtime | caution (installed runtimes deleted, not caches) |
| `~/.local/pipx` | deps | caution (installed CLI tools deleted) |
| `~/.m2/repository` | deps | caution (shared by all Maven projects) |
| `~/Library/Android/sdk/system-images` | runtime | caution |
| `~/Library/Android/sdk/ndk` | deps | caution |
| `~/Library/Android/sdk/build-tools` | runtime | caution |
| `~/Library/Application Support/Cursor/{Cache,CachedData,Code Cache}` | cache | safe (cache subdirs only — settings live alongside) |
| `/private/var/folders/*/*/X/*.code_sign_clone` | cache | safe / caution (macOS only — Browser Temp, see below) |

**Config roots and user state are deliberately excluded.** A home dotfile is treated as config unless it is unambiguously a package/build cache (like `~/.cache/uv`). Deleting an excluded tree is unrecoverable or credential loss, and the "caches" inside only reappear as install-time scaffolding — worthless against that risk, so devclean never offers them for deletion.

| Excluded path | Why |
|---|---|
| `~/.claude` (whole tree) | session transcripts, project memory, agents, skills, plugins, todos |
| `~/.codex`, `~/.gemini` | agent CLI state |
| `~/Library/Caches/claude-cli-nodejs` | Claude Code state |
| `~/.cursor` | extensions & settings |
| `~/.gem` | RubyGems credential + installed gems |
| `~/.android/avd` | emulator user data |

Only genuine caches under those trees (e.g. `~/.cargo/registry`, `~/Library/Application Support/Cursor/Cache`) or dedicated cache dirs remain eligible.

**Homebrew**: with brew installed, reported as one item reclaimed by `HOMEBREW_NO_AUTOREMOVE=1 brew cleanup` (the cleanup brew runs itself after upgrades: old formula versions, stale downloads, logs; casks excluded). Size is brew's dry-run estimate. The item replaces the `~/Library/Caches/Homebrew` entry when `brew --cache` is that path; with nothing to free, only the directory is reported.

**Browser Temp (macOS)**: Chromium-family browsers (Chrome, Brave, Edge, Arc, Vivaldi, …) copy their own bundle to `/private/var/folders/<xx>/<yyy>/X/<bundle-id>.code_sign_clone/` on launch to verify their code signature, removing it on normal exit. Force-killed processes — typically headless automation like lighthouse or puppeteer — leave zombie copies that accumulate (observed: 92 copies / 156 GB).

- **Matching**: a single `*.code_sign_clone` glob, not a per-browser catalog; the label carries the browser name (from the bundle ID) and the copy count.
- **Safety follows run state**: `safe` only when `pgrep` reports no matching process (true zombies); `caution` while it runs (checked via `pgrep`, once per browser — the newest copy may be in use), when the `pgrep` check fails (missing or erroring), or when the bundle ID is unrecognized (run state unknowable).
- **Scope**: the path lies outside home, so it is reported only when the scan root covers the home directory — a `--path` scan of a home subdirectory never surfaces system temp.
- **Size caveat**: reported size may overstate real usage when the copies are APFS clones of the installed app.

### LLM Model Stores

Local LLM model stores at fixed home paths. Model weights dominate these directories (often tens of GB per model) and are always re-downloadable, so every entry is `safe` with a re-download note in `recommendation`. Missing paths are skipped.

| Path | Unit | Label |
|------|------|-------|
| `~/.lmstudio/models/<org>/<model>/` | one result per model directory | `<org>/<model>` |
| `~/.cache/huggingface/hub/models--<org>--<name>/` | one result per model directory | decoded to `<org>/<name>` |
| `~/.ollama/models` | store as a whole | `Ollama model store` |
| `~/.llamafile` | store as a whole | `llamafile store` |

LM Studio and Hugging Face hub lay each model out as its own directory, so they are reported per model — delete only the models you no longer use. Ollama stores weights as content-addressed blobs shared across models, so it is reported as one entry; to remove individual models, prefer `ollama rm <model>` (noted in `recommendation`).

**Last used**: each result carries `last_used_at` (JSON) / a dim "last used …" hint (table), derived from the model directory's mtime (store mtime for Ollama/llamafile) — a rough signal for spotting models that haven't been touched in months. Log-based usage analysis is a possible future refinement.

Like the global caches, stores are home-rooted and reported only when the scan root contains them — a `--path` scan of a home subdirectory excludes them.

## Configuration

> Configuration is not yet implemented. This documents the planned design.

### Location

```text
~/.devclean/settings.json
```

### Planned Settings

```json
{
  "scan_paths": ["~"],
  "exclusions": [],
  "ecosystems": {
    "disabled": []
  },
  "thresholds": {
    "active": 7,
    "recent": 30,
    "stale": 90,
    "dormant": 90
  }
}
```

#### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `scan_paths` | string[] | `["~"]` | Directories to scan |
| `exclusions` | string[] | `[]` | Paths to exclude from scanning |
| `ecosystems.disabled` | string[] | `[]` | Ecosystems to skip |
| `thresholds.active` | int | 7 | Days threshold for "active" status |
| `thresholds.recent` | int | 30 | Days threshold for "recent" status |
| `thresholds.stale` | int | 90 | Days threshold for "stale" status |
| `thresholds.dormant` | int | 90 | Days threshold for "dormant" status |
