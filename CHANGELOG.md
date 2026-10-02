# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0](https://github.com/ohing504/devclean/compare/v0.1.0...v0.2.0) (2026-10-02)


### Added

* clean 합계에서 하드링크 공유 블록을 한 번만 세고 pnpm node_modules에 pnpm store 공유 안내 추가 ([#37](https://github.com/ohing504/devclean/issues/37)) ([8d00469](https://github.com/ohing504/devclean/commit/8d00469c10ca741915a5cc8f96c51473a5a7dc1f))
* **cleaner:** per-item delete strategy via DeleteMethod contract ([661aa44](https://github.com/ohing504/devclean/commit/661aa44202a52e60a0c5fa534e7e1efa2eeed408)), closes [#16](https://github.com/ohing504/devclean/issues/16)
* **scanner:** add Android/Gradle ecosystem support ([b510f9c](https://github.com/ohing504/devclean/commit/b510f9cc429ffc5dc20dbb7fa6e618ac77e916f1))
* **scanner:** add Docker disk image scanning (scan only) ([dcc0d1f](https://github.com/ohing504/devclean/commit/dcc0d1fd40ed64583836d683b272349c308f3948))
* **scanner:** add Flutter/Dart ecosystem support ([f1a7154](https://github.com/ohing504/devclean/commit/f1a715457b24a9cb1d718dc09c5bb7f66f7764a8))
* **scanner:** add Global Caches ecosystem ([c1afbe2](https://github.com/ohing504/devclean/commit/c1afbe234bc265025e46e6b24295c7c400f21f76))
* **scanner:** global caches ×56, browser code-sign clones, LLM model stores ([17a9f97](https://github.com/ohing504/devclean/commit/17a9f9796466a92cb158c4ca4f8e42a4e4f8bdc0))
* **scanner:** make the walk's no-follow symlink policy explicit ([297510e](https://github.com/ohing504/devclean/commit/297510e41876560bac2e4bf493457a6703f70e8d))
* **scanner:** register global cache prune commands as vendor cleanups ([6b753db](https://github.com/ohing504/devclean/commit/6b753db5a2aa7da7aad40bfe37ea48a3783ae85e)), closes [#20](https://github.com/ohing504/devclean/issues/20)
* **scanner:** sparse/clone-aware sizing with hard-link-deduped totals ([b39966c](https://github.com/ohing504/devclean/commit/b39966ceca981c3081436b4a1f85351236a53464))
* **scanner:** 설치된 도구가 쓰는 전역 캐시를 caution으로 올리고 Homebrew를 brew cleanup으로 정리 ([#41](https://github.com/ohing504/devclean/issues/41)) ([5bedeaa](https://github.com/ohing504/devclean/commit/5bedeaa0a585e6d30b4b4d5d33739251a2adf854))
* Xcode DerivedData에 원본 workspace 경로와 원본 삭제 안내를 표시하고 scan 표와 clean 화면의 항목 표시를 통일 ([#45](https://github.com/ohing504/devclean/issues/45)) ([619aae6](https://github.com/ohing504/devclean/commit/619aae6b85f8c05d3c00034e85fac3f00fa874c2))


### Changed

* **classifier:** parallelize git classification across a worker pool ([ff145df](https://github.com/ohing504/devclean/commit/ff145dfd78dfcc3edeeea8581b0f8b2ec6b39575))
* **scanner:** parallelize per-artifact sizing across a worker pool ([425091b](https://github.com/ohing504/devclean/commit/425091b226409ac466d40d25ed93dd056901a173))
* **scanner:** read each directory once in the walk engine ([77dc3cb](https://github.com/ohing504/devclean/commit/77dc3cb38619a1f409a4f55cd90868bd0429ea19))
* **scanner:** size stat scanners via the shared parallel worker pool ([2f81ae3](https://github.com/ohing504/devclean/commit/2f81ae35a6bb146d55633b5884c9a65b2c25f245))


### Fixed

* --path가 상대 경로이거나 .으로 시작하는 폴더일 때 스캔 결과가 0건이던 문제 수정 ([#48](https://github.com/ohing504/devclean/issues/48)) ([4e98389](https://github.com/ohing504/devclean/commit/4e983896ab60a395af615129af9a417e00f2847e)), closes [#35](https://github.com/ohing504/devclean/issues/35)
* **cleaner:** fall back to copy+remove when trashing across filesystems ([a876e37](https://github.com/ohing504/devclean/commit/a876e37a8b71130d52476c3e1064e9e97f0e8d53))
* P0 hygiene batch — selector protected leak, vendor-cleanup scope, Ctrl-C ([def2bf0](https://github.com/ohing504/devclean/commit/def2bf0e82bc1c2d60280d430e25bcfee7e3e045))
* pgrep 실행 확인이 실패하면 브라우저 code-sign clone을 safe 대신 caution으로 분류 ([#47](https://github.com/ohing504/devclean/issues/47)) ([f1691ee](https://github.com/ohing504/devclean/commit/f1691ee2d58e3b7ad3fda6172ec7ba65607c0a14)), closes [#39](https://github.com/ohing504/devclean/issues/39)
* **scanner:** never offer irreplaceable data for deletion; gate --yes to safe ([caad061](https://github.com/ohing504/devclean/commit/caad061940657ccd0421773f2fe5b80872f755a0))
* **scanner:** pnpm store와 .app 번들 내부를 삭제 후보에서 제외하고 walk 테스트를 전체 목록 비교로 통일 ([f758209](https://github.com/ohing504/devclean/commit/f758209db2d9f3e04c3970d988b39f965016b4a3))
* **ui:** scroll tree selector viewport instead of dumping every row ([e93aa1d](https://github.com/ohing504/devclean/commit/e93aa1d1d139f9573eec24ae8cbf584542ef3dd1))
* 마운트를 건너뛰도록 후보 찾기, 크기 계산, 삭제가 공통 순회 규칙을 쓰게 변경 ([#43](https://github.com/ohing504/devclean/issues/43)) ([7f5bfd0](https://github.com/ohing504/devclean/commit/7f5bfd07b5cea9f73ca7e8894df01b806eae1800))

## [Unreleased]

### Added

#### Ecosystem scanners
- **Global Caches** scanner (`global`) covering 27 shared caches at fixed home paths: package managers (npm, pnpm store & cache, Yarn, bun, pip, Homebrew, CocoaPods, Gradle caches & wrapper dists, cargo registry, Go build & module caches), dev tools (Playwright, Electron, node-gyp, TypeScript), and Android SDK (AVD, NDK, system images). macOS paths with Linux `~/.cache` fallbacks. Shared caches whose deletion forces re-downloads are marked `caution` with a consequence note.
- **Global Caches** catalog expanded from 27 to 56 entries: uv (XDG `~/.cache/uv` + macOS `~/Library/Caches/uv`), AI tools (Claude Code, Codex, Gemini, Cursor — only Cursor's cache subdirs, never its settings), Puppeteer, Cypress, Deno, Poetry, pipx, Maven repository, rustup toolchains, cargo git cache, nvm/pyenv/rbenv runtimes, RubyGems, CocoaPods spec repos, and Android SDK build-tools. Entries that delete installed runtimes/tools or session history are `caution` with consequence notes.
- **Global Caches**: Browser Temp detection (macOS) — zombie Chromium-family code-sign clones under `/private/var/folders/*/*/X/*.code_sign_clone` left behind by force-killed browsers (headless automation like lighthouse/puppeteer), labeled with browser name and copy count. `safe` when the browser is not running; `caution` while it runs (newest copy may be in use) or for unrecognized bundle IDs.
- **LLM Model Stores** scanner (`llm`) covering local model weights at fixed home paths: LM Studio (`~/.lmstudio/models`, per model) and Hugging Face hub (`~/.cache/huggingface/hub`, per model, `models--org--name` decoded to `org/name`), plus the Ollama (`~/.ollama/models`) and llamafile (`~/.llamafile`) stores as a whole. All `safe` with re-download notes; the Ollama note points to `ollama rm <model>` for removing individual models. Results carry a new `last_used_at` JSON field (model directory mtime; omitted when unknown), shown in the table as a dim "last used …" hint.
- **Node.js**: a pnpm-installed `node_modules` whose pnpm store (the `storeDir` in `node_modules/.modules.yaml`) is on the same volume is annotated that its files are likely shared with the store, so deleting it frees space only after `pnpm store prune`. The note appears in both the scan table and the `clean` selector.

#### Sizing
- Sparse-aware sizing: a sparse artifact shows its real on-disk size next to the size it reports — `8.6 GB (appears as 494.4 GB)` for a `Docker.raw` image. JSON gains an `apparent_size` field alongside the disk-based `size`.

### Changed

- Project scanners (Node, Rust, Ruby, Python, Go) are now declarative rule tables executed by a single shared filesystem walk instead of five independent traversals — one pass over the scan root regardless of how many project ecosystems are active. Stat-based scanners (xcode, global, llm) are unchanged.
- The scan spinner shows a single "Scanning projects..." stage for all project scanners (previously "Scanning node...", "Scanning rust...", … in sequence). Stat scanners still report under their own names.
- Artifact sizing runs in-process (`scanner.Measure`) instead of forking `du` per artifact, collecting disk (allocated blocks) and apparent (logical) size in one pass. Disk stays the primary figure for sorting, filtering and totals.
- Totals count hard-linked blocks shared across artifacts once (e.g. a pnpm store blob linked into `node_modules`), so `total_size` no longer double-counts them. Per-artifact sizes are unchanged.
- `clean` counts hard-linked blocks once in the selector footer, the confirm prompt, and the final summary, matching the scan total. The summary says `moved to Trash` instead of `freed` when items went to the Trash, since that space is reclaimed only once the Trash is emptied.
- The walk engine now reads each directory once and reuses those entries for both project-marker detection and recursion, instead of reading every directory twice (a `filepath.WalkDir` read plus a second `os.ReadDir`). Directory traversal — the dominant cost of scanning a large tree — is roughly 1.8× faster (~29% faster end-to-end on a workspace with tens of thousands of directories); scan results are unchanged.
- Git classification now forks `git` across a bounded worker pool (`min(NumCPU, 8)`) instead of one repo at a time — root resolution, `status`/`log`, and `check-ignore` all run concurrently across repos. On a workspace spanning dozens of repos this cut the classify phase roughly in half (~1.24s → ~0.54s warm cache, ~19% faster end-to-end); protection and activity results are unchanged.
- The status-badge, safety-icon, and relative-time display formatters, previously duplicated between the table output and the interactive tree selector, are now single shared functions in `internal/ui` (`StatusBadge`, `SafetyIcon`, `RelativeTime`). Table and selector rendering are unchanged; the formatters gained direct unit tests.
- Test coverage: the interactive tree selector's toggle, parent/child selection propagation, cursor movement, quick-select keys, and `Update` key routing are now tested (`internal/ui` 35% → 57%); the scan→classify integration workspace gained Python and Ruby project fixtures plus a Global-scanner (fixed-home) pipeline test; and `DirSize` now has a test that pins the sizing arithmetic (the golden test zeroes sizes out for portability).
- Global caches of installed tools are `caution`, so `clean --yes` skips them.
- Homebrew is reclaimed by `brew cleanup` as a scan item sized by its dry-run (was: deleting `~/Library/Caches/Homebrew`; removed from `--vendor-cleanup`).
- The walk engine's no-follow symlink policy is now explicit: it skips any symlink entry (`os.ModeSymlink`) rather than relying on `os.ReadDir`'s incidental `IsDir()==false`, so a future refactor can't silently start following links. Behavior is unchanged — a symlink is never descended into or matched as an artifact (even a symlinked `node_modules` from pnpm/monorepos), which avoids double-counting the target's disk space and never reclaims a shared target; symlink cycles remain unwalkable. Documented in `docs/decisions/symlink-no-follow.md`.

### Fixed

- `--path` with a relative path ending in `.` or `..` (e.g. `--path .`), or naming a directory that starts with `.`, reported "No items found" because the scan root was skipped as a hidden directory. The root is now always scanned, and relative paths are resolved to absolute ones, so results show absolute paths.
- Browser Temp: when the `pgrep` run-state check fails (not installed or erroring), a known browser's code-sign clones are now `caution` instead of `safe`, so `clean --yes` no longer deletes a copy a running browser may be using.
- Directories listed by several ecosystems (e.g. `coverage/` in a project with both `package.json` and `Gemfile`) were reported once per ecosystem, double-counting their size. They are now reported once, attributed to the first ecosystem in scanner order (node → rust → ruby → python → go) among those active in the scan — a full scan attributes a shared `coverage/` to node, while `--eco ruby` attributes it to ruby.
- Scanners no longer walk into another ecosystem's matched artifact: `__pycache__` directories inside `node_modules` were previously reported separately by the Python scanner, and `node_modules` itself was fully traversed by four other scanners.
- Tree selector: `a` (select all) no longer selects artifacts under protected projects, matching the other bulk-select keys and the ✗ rendering (deletion was already blocked by the cleaner guard).
- `clean --vendor-cleanup` without `--eco` ran vendor commands for every registered ecosystem (e.g. `xcrun simctl delete unavailable` when nothing Xcode-related was cleaned); it is now scoped to the targeted ecosystems.
- Ctrl-C / SIGTERM now cancels an in-progress scan instead of being ignored until the walk finishes.
- Config roots and irreplaceable user state are no longer offered for deletion. A home dotfile is now treated as config unless it is unambiguously a package/build cache. Previously listed as `caution`/`safe` and thus removable by `clean --yes`, now excluded: the whole `~/.claude` tree (session transcripts, project memory, agents, skills, plugins), `~/.codex`, `~/.gemini`, Claude Code's `~/Library/Caches/claude-cli-nodejs`, `~/.cursor` (extensions & settings), `~/.gem` (holds the RubyGems credential + installed gems), and `~/.android/avd` (emulator user data). Deleting any of it was unrecoverable data or credential loss, not reclaimed space. Only genuine caches under those trees or dedicated cache dirs remain eligible.
- `clean --yes` now deletes only `safe` items by default; `caution` items are skipped and reported. Previously `--yes` deleted every non-protected item — so a single mis-classified `caution` entry could be removed without a human ever seeing it. Pass `--include-caution` to opt back into deleting `caution` items non-interactively. `protected` is never deleted either way; the interactive selector is unchanged.
- Moving an artifact to the Trash across filesystems (external drive, separate partition) no longer fails. `os.Rename` returns `EXDEV` across volumes, which previously surfaced as an error; the cleaner now falls back to a recursive copy (preserving permissions and symlinks) followed by removing the original — and only removes the original after the copy fully succeeds, so a mid-copy failure leaves the source intact.

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
