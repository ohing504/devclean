# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Ecosystem scanners
- **Global Caches** scanner (`global`) covering 27 shared caches at fixed home paths: package managers (npm, pnpm store & cache, Yarn, bun, pip, Homebrew, CocoaPods, Gradle caches & wrapper dists, cargo registry, Go build & module caches), dev tools (Playwright, Electron, node-gyp, TypeScript), and Android SDK (AVD, NDK, system images). macOS paths with Linux `~/.cache` fallbacks. Shared caches whose deletion forces re-downloads are marked `caution` with a consequence note.

### Fixed

- Tree selector: `a` (select all) no longer selects artifacts under protected projects, matching the other bulk-select keys and the ✗ rendering (deletion was already blocked by the cleaner guard).
- `clean --vendor-cleanup` without `--eco` ran vendor commands for every registered ecosystem (e.g. `xcrun simctl delete unavailable` when nothing Xcode-related was cleaned); it is now scoped to the targeted ecosystems.
- Ctrl-C / SIGTERM now cancels an in-progress scan instead of being ignored until the walk finishes.

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
