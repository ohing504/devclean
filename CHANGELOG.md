# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Ecosystem scanners
- **Global Caches** scanner (`global`) covering 27 shared caches at fixed home paths: package managers (npm, pnpm store & cache, Yarn, bun, pip, Homebrew, CocoaPods, Gradle caches & wrapper dists, cargo registry, Go build & module caches), dev tools (Playwright, Electron, node-gyp, TypeScript), and Android SDK (AVD, NDK, system images). macOS paths with Linux `~/.cache` fallbacks. Shared caches whose deletion forces re-downloads are marked `caution` with a consequence note.
- **Global Caches** catalog expanded from 27 to 56 entries: uv (XDG `~/.cache/uv` + macOS `~/Library/Caches/uv`), AI tools (Claude Code, Codex, Gemini, Cursor — only Cursor's cache subdirs, never its settings), Puppeteer, Cypress, Deno, Poetry, pipx, Maven repository, rustup toolchains, cargo git cache, nvm/pyenv/rbenv runtimes, RubyGems, CocoaPods spec repos, and Android SDK build-tools. Entries that delete installed runtimes/tools or session history are `caution` with consequence notes.
- **Global Caches**: Browser Temp detection (macOS) — zombie Chromium-family code-sign clones under `/private/var/folders/*/*/X/*.code_sign_clone` left behind by force-killed browsers (headless automation like lighthouse/puppeteer), labeled with browser name and copy count. `safe` when the browser is not running; `caution` while it runs (newest copy may be in use) or for unrecognized bundle IDs.
- **LLM Model Stores** scanner (`llm`) covering local model weights at fixed home paths: LM Studio (`~/.lmstudio/models`, per model) and Hugging Face hub (`~/.cache/huggingface/hub`, per model, `models--org--name` decoded to `org/name`), plus the Ollama (`~/.ollama/models`) and llamafile (`~/.llamafile`) stores as a whole. All `safe` with re-download notes; the Ollama note points to `ollama rm <model>` for removing individual models. Results carry a new `last_used_at` JSON field (model directory mtime; omitted when unknown), shown in the table as a dim "last used …" hint.

### Changed

- Project scanners (Node, Rust, Ruby, Python, Go) are now declarative rule tables executed by a single shared filesystem walk instead of five independent traversals — one pass over the scan root regardless of how many project ecosystems are active. Stat-based scanners (xcode, global, llm) are unchanged.
- The scan spinner shows a single "Scanning projects..." stage for all project scanners (previously "Scanning node...", "Scanning rust...", … in sequence). Stat scanners still report under their own names.
- The project walk now sizes matched artifacts concurrently (bounded worker pool) instead of one `du` call at a time — up to ~4× faster warm-cache sizing on a many-artifact tree, with cold-scan and few-large-artifact gains varying; disk-usage numbers are unchanged.
- The walk engine now reads each directory once and reuses those entries for both project-marker detection and recursion, instead of reading every directory twice (a `filepath.WalkDir` read plus a second `os.ReadDir`). Directory traversal — the dominant cost of scanning a large tree — is roughly 1.8× faster (~29% faster end-to-end on a workspace with tens of thousands of directories); scan results are unchanged.
- Git classification now forks `git` across a bounded worker pool (`min(NumCPU, 8)`) instead of one repo at a time — root resolution, `status`/`log`, and `check-ignore` all run concurrently across repos. On a workspace spanning dozens of repos this cut the classify phase roughly in half (~1.24s → ~0.54s warm cache, ~19% faster end-to-end); protection and activity results are unchanged.

### Fixed

- Directories listed by several ecosystems (e.g. `coverage/` in a project with both `package.json` and `Gemfile`) were reported once per ecosystem, double-counting their size. They are now reported once, attributed to the first ecosystem in scanner order (node → rust → ruby → python → go) among those active in the scan — a full scan attributes a shared `coverage/` to node, while `--eco ruby` attributes it to ruby.
- Scanners no longer walk into another ecosystem's matched artifact: `__pycache__` directories inside `node_modules` were previously reported separately by the Python scanner, and `node_modules` itself was fully traversed by four other scanners.
- Tree selector: `a` (select all) no longer selects artifacts under protected projects, matching the other bulk-select keys and the ✗ rendering (deletion was already blocked by the cleaner guard).
- `clean --vendor-cleanup` without `--eco` ran vendor commands for every registered ecosystem (e.g. `xcrun simctl delete unavailable` when nothing Xcode-related was cleaned); it is now scoped to the targeted ecosystems.
- Ctrl-C / SIGTERM now cancels an in-progress scan instead of being ignored until the walk finishes.
- Irreplaceable data is no longer offered for deletion. AI coding-tool home directories — the whole `~/.claude` tree (session transcripts, project memory, agents, skills, plugins), `~/.codex`, `~/.gemini`, and Claude Code's `~/Library/Caches/claude-cli-nodejs` — plus Android emulator user data (`~/.android/avd`) were previously listed in the Global Caches catalog as `caution`/`safe` entries. They are user state that cannot be regenerated (the "caches" inside only reappear as install-time scaffolding), so deleting them (e.g. via `clean --yes`) was unrecoverable data loss, not reclaimed space. They are now excluded from the catalog entirely; only genuinely regenerable caches/artifacts remain eligible.
- `clean --yes` now deletes only `safe` items by default; `caution` items are skipped and reported. Previously `--yes` deleted every non-protected item — so a single mis-classified `caution` entry could be removed without a human ever seeing it. Pass `--include-caution` to opt back into deleting `caution` items non-interactively. `protected` is never deleted either way; the interactive selector is unchanged.

## [0.1.0] - 2026-05-04

First tagged release. Entries are grouped by capability rather than commit.

### Added

#### Ecosystem scanners
- **Node.js** scanner detecting `node_modules`, `.next`, `.nuxt`, `.output`, `dist`, `.turbo`, `.parcel-cache`, `.svelte-kit`, `coverage` near `package.json`. Monorepo-aware: artifacts in sub-packages group under the git root.
- **React Native / Expo** extension to the Node scanner. When `ios/Podfile` or `metro.config.{js,ts,cjs,mjs}` is present, additionally collects `ios/Pods`, `ios/build`, `ios/DerivedData`, `android/build`, `android/.gradle`, `.expo`, `.metro`.
- **Rust** scanner detecting `target/` near `Cargo.toml`.
- **Ruby** scanner detecting `vendor/bundle`, `.bundle`, `tmp`, `log`, `coverage`, `.ruby-lsp` near `Gemfile`.
- **Python** scanner detecting projects via any of `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt`, `Pipfile`, `uv.lock` and matching `__pycache__`, `.pytest_cache`, `.mypy_cache`, `.ruff_cache`, `.tox`, `.nox`, `.ipynb_checkpoints`, `__pypackages__`, `*.egg-info` at any depth under the deepest containing project root. `.venv` / `venv` are reported as `caution` to preserve hand-curated environments.
- **Go** (per-project) scanner detecting `go.mod` and reporting `vendor/` as `caution`. Global Go caches (`~/.cache/go-build`, `~/go/pkg/mod`) will land in the upcoming Global Caches scanner.
- **Xcode / iOS** scanner (macOS only) covering `DerivedData`, `Archives`, `iOS/watchOS/tvOS DeviceSupport`, `CoreSimulator/Devices`, simulator runtimes, and CoreSimulator caches.

#### CLI commands
- `scan` — discover reclaimable disk space. Filters: `--eco`, `--category`, `--status`, `--min-size`. Sorting: `--sort size|time|name` with `--asc`. `-n / --top` for top-N projects. `--json` for scripting and AI agents. `-v / --verbose` to expand small artifacts.
- `clean` — remove discovered artifacts. Interactive tree selector by default; `--yes` for non-interactive. `--safe` skips caution/protected items. `--dry-run` previews. `--force` permanently deletes (default sends to Trash on macOS/Linux).
- `list` — print supported ecosystems, categories, activity statuses, and safety levels.
- `--version` — print the build version, commit, and date. Populated at build time via goreleaser ldflags; falls back to `dev` for local `go build` invocations.

#### Cross-cutting features
- **Activity classification** — every artifact tagged `active` (<7d), `recent` (7-30d), `stale` (30-90d), or `dormant` (90+d) using the most recent of artifact mtime, last git commit, and project directory mtime.
- **Gitignore-aware protection** — artifacts in repos with uncommitted changes are marked `protected` only if they're git-tracked. Gitignored artifacts (`node_modules`, `.next`, etc.) remain deletable even in dirty repos.
- **Safety levels** — `safe` (auto-regenerated), `caution` (shared impact or hand-curated), `protected` (git-tracked dirty).
- **Interactive tree selector** — bubbletea-based UI for `clean`. Multi-select per project or per artifact. `[a]` all, `[n]` none, `[s]` safe only, `[d]` dormant only, `[space]` toggle, `[enter]` confirm, `[esc]` cancel.
- **Metadata enrichment** — `ScanResult.Label` and `ScanResult.Recommendation` fields populated by scanners after detection. Xcode scanner uses `xcrun simctl list devices --json` to translate simulator UUIDs into human-readable names (e.g. "iPhone 17 Pro · iOS 26.3") and flags removed runtimes as "safe to remove".
- **Vendor cleanup** — `--vendor-cleanup` flag runs ecosystem-native cleanup commands (currently `xcrun simctl delete unavailable` for Xcode) alongside path-based deletion to keep vendor internal state consistent.
- **`--min-size`** filter — both `scan` and `clean` accept human-readable sizes (`1MB`, `500KB`, `2.5GB`, `1KiB`, raw bytes) to skip small artifacts.
- **JSON output** — `--json` on `scan` produces structured output for scripts and AI agents.
- **Trash-by-default cleanup** — uses `trash` on macOS/Linux for recoverable deletion; `--force` skips Trash.
- **Dry-run** — `--dry-run` previews deletions without touching disk.

#### Project tooling
- GitHub Actions CI on Ubuntu and macOS: `go build`, `go test -race -count=1`, `golangci-lint v2.11.4`.
- Pre-commit hooks via lefthook (lint + format on staged Go files).
- `gofumpt` formatting + `golangci-lint v2` config (default + revive, misspell, gocritic).
- Documentation: `docs/architecture.md`, `docs/ecosystems.md`, `docs/commands.md`, `docs/configuration.md`, plus `CONTRIBUTING.md` with a walkthrough for adding new ecosystem scanners.

### Changed

- `model.HumanSize` switched from binary (1024-based) to decimal SI (1000-based) units. Labels remain `KB`/`MB`/`GB` (capitalized) to match macOS Finder convention. This aligns the display with the `--min-size` parser, which uses humanize/SI semantics, so the threshold a user types and the size they see now agree on identical arithmetic. Side effect: a 1 GiB directory now displays as `1.1 GB` rather than `1.0 GB`.

### Security

- No known issues. Report security concerns via [GitHub private vulnerability reporting](https://github.com/ohing504/devclean/security/advisories/new).

[Unreleased]: https://github.com/ohing504/devclean/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ohing504/devclean/releases/tag/v0.1.0
