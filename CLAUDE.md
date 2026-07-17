# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

devclean is a Go CLI tool that scans developer environments for reclaimable disk space (build artifacts, caches, dependencies, runtimes) and provides safe cleanup. Currently supports Node.js (with React Native / Expo extension), Rust, Ruby, Python, Go (per-project only), and Xcode (macOS) with more ecosystems planned (Android, Flutter, Docker, Global caches).

## Commands

```bash
# Build
go build -o devclean ./cmd/devclean

# Test
go test ./...                          # all tests
go test ./internal/model/ -v           # single package
go test ./internal/model/ -run TestHumanSize -v  # single test

# Lint
golangci-lint run ./...

# Format
gofumpt -w .
```

Pre-commit hooks (lefthook) run lint + format automatically on staged `.go` files.

## Architecture

Pipeline: **Scan → Classify → Filter/Sort → Output/Clean**

- `cmd/devclean/` — entrypoint, wires CLI
- `internal/model/` — core domain types (`Ecosystem`, `Category`, `SafetyLevel`, `ScanResult`, `ArtifactDef`)
- `internal/scanner/` — `Scanner` interface + per-ecosystem implementations with progress callbacks
- `internal/classifier/` — activity status (3-timestamp), gitignore-aware protection
- `internal/cleaner/` — trash (macOS/Linux) + force delete, dry-run, protection enforcement
- `internal/output/` — JSON formatter + lipgloss colored table (ecosystem → project → sub-package)
- `internal/cli/` — cobra commands (scan, clean, list)
- `internal/ui/` — terminal spinner, interactive tree selector (bubbletea), shared styles
- `internal/pathutil/` — shared path helpers (`CachedHomeDir`, etc.)

## Key Design Decisions

- **Safety levels**: `safe` (auto-regenerated), `caution` (shared impact), `protected` (git-tracked with uncommitted changes)
- **Gitignore-aware protection**: gitignored artifacts (node_modules, .next) deletable even in dirty repos; only tracked artifacts are protected
- **Activity detection**: uses most recent of artifact mtime, git commit time, project dir mtime
- **Monorepo support**: artifacts grouped by git root; sub-packages shown with headers
- **Output sorting**: ecosystems grouped by total size desc, projects by size desc, sub-packages by size desc
- **Clean flow**: scan → interactive multiselect (protected hidden) → trash/force choice → per-artifact results
- **AI agent friendly**: all interactions via CLI flags (`--yes`, `--json`), no interactive prompts required
- **Metadata enrichment**: scanners populate `ScanResult.Label` / `Recommendation` so users can decide without decoding paths (UUID → "iPhone 17 Pro · iOS 26.3", "superseded by newer build", "runtime unavailable"). First applied in xcode; reusable for any ecosystem with opaque identifiers
- **Vendor cleanup**: scanners may implement the optional `VendorCleaner` interface to register ecosystem-native cleanup commands (e.g. `xcrun simctl delete unavailable`). `devclean clean --vendor-cleanup` runs them alongside path-based deletion so vendor internal state stays consistent — natural fit for Docker (`system prune`), Homebrew (`cleanup`), Gradle, etc.
- **Deletion strategy**: `ScanResult.Delete` (`model.DeleteMethod`: kind path/command/api + display + Run closure) expresses non-path reclaims per item; nil = path removal. Cleaner applies protected/dry-run gates uniformly, then delegates to the method or falls back to trash/force. `VendorCleanup` embeds the same `DeleteMethod` — bulk (ecosystem-level) vs per-item are two uses of one execution contract

## Documentation Rules

- docs under `docs/` are the SSOT (single source of truth) for the project
- `docs/plans/` is gitignored — temporary design/implementation documents only
- when implementing a feature, update the relevant doc file in the same commit:
  - new ecosystem → update `docs/ecosystems.md`
  - new/changed command or flag → update `docs/commands.md`
  - architecture change → update `docs/architecture.md`
  - config change → update `docs/configuration.md`

## Code Style

- Go formatting via `gofumpt` (stricter than gofmt)
- Linting via `golangci-lint` v2 with `default: standard` + revive, misspell, gocritic
- Test files use `_test.go` suffix with `package_test` external test packages
- Golden file tests for output format stability (`-update` flag to regenerate)
