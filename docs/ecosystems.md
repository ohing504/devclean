# Ecosystems

## Supported Ecosystems

| ID | Name | Status |
|----|------|--------|
| `node` | Node.js | implemented |
| `rust` | Rust | implemented |
| `ruby` | Ruby | implemented |
| `xcode` | iOS/Xcode (macOS only) | implemented |
| `python` | Python | implemented |
| `go` | Go | implemented (per-project only) |
| `global` | Global Caches | implemented |
| `llm` | LLM Model Stores | implemented |
| `android` | Android | planned |
| `flutter` | Flutter/Dart | planned |
| `docker` | Docker | planned |

**Dedup attribution**: project ecosystems (node, rust, ruby, python, go) share a single-pass scan — a directory matching artifact rules of several active ecosystems is reported once, attributed to the first in scanner order (node → rust → ruby → python → go), so `--eco` subsets can shift attribution (a shared `coverage/` goes to node in a full scan, to ruby under `--eco ruby`).

## Node.js

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

## Rust

**Detection**: `Cargo.toml` in parent directory

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `target` | build | safe | Rust build artifacts (debug/release binaries, deps) |

**Note**: Tauri projects (Node + Rust hybrid) are detected by both Node.js and Rust scanners, catching both `node_modules` and `target/`.

## Ruby

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

## Python

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

## Go

**Detection**: `go.mod` in parent directory.

**Artifacts**:

| Pattern | Category | Safety | Description |
|---------|----------|--------|-------------|
| `vendor` | deps | **caution** | Vendored modules (regenerate with `go mod vendor`) |

**Notes**:
- `vendor/` is `caution` because `go mod vendor` is an opt-in choice — devs who vendor often do so for offline builds, reproducibility, or supply-chain pinning. Regeneration requires network access plus the original `go.sum`.
- Go's two big disk hogs — `~/.cache/go-build` (build cache) and `~/go/pkg/mod` (module cache) — are global, not per-project. They will be handled by the Global Caches scanner so they aren't double-attributed to every Go project on the machine.

## Xcode (macOS only)

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

`xcrun simctl` is best-effort — if Xcode CLI tools are not installed, simulator devices fall back to UUID display with no label/recommendation, but the rest of the scan still works.

**Notes**:
- `Archives` is `caution` because losing an archive means losing the ability to symbolicate crash reports for that release.
- `CoreSimulator/Devices` is `caution` because it contains app installs, settings, and user data inside simulators currently in use.

## Global Caches

Shared, home-rooted developer caches that are not tied to any single project. Unlike the per-project scanners, paths are fixed (home-relative) and span package managers and dev tools across ecosystems. Paths owned by a dedicated scanner (Xcode's DerivedData, DeviceSupport, Archives, CoreSimulator) are intentionally excluded to avoid double-counting.

Each entry that is `caution` carries a consequence-of-deletion note in `recommendation` (e.g. "every project re-downloads dependencies on next install") so a user — or an AI agent reading `--json` — can decide without external knowledge. Missing paths are skipped, so macOS (`~/Library/Caches/*`, `~/Library/pnpm/store`) and Linux (`~/.cache/*`) variants coexist in the catalog.

| Path | Category | Safety |
|------|----------|--------|
| `~/.npm` | cache | safe |
| `~/.bun/install/cache` | cache | safe |
| `~/.cocoapods` | cache | safe |
| `~/.cache/uv`, `~/.cache/puppeteer` | cache | safe (XDG paths, used on macOS too) |
| `~/Library/Caches/{Yarn,pnpm,pip,Homebrew,CocoaPods,go-build,electron,node-gyp,typescript,uv,Cypress,deno,pypoetry}` | cache | safe |
| `~/.cache/{go-build,pip,node-gyp,yarn,pnpm,electron,Cypress,deno,pypoetry}` | cache | safe |
| `~/Library/pnpm/store` | cache | caution (hard-linked store) |
| `~/.gradle/caches`, `~/.gradle/wrapper/dists` | cache | caution |
| `~/.cargo/registry`, `~/.cargo/git` | cache | caution |
| `~/go/pkg/mod` | deps | caution (read-only files) |
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

**Browser Temp (macOS)**: Chromium-family browsers (Chrome, Brave, Edge, Arc, Vivaldi, …) copy their own bundle to `/private/var/folders/<xx>/<yyy>/X/<bundle-id>.code_sign_clone/` on launch to verify their code signature, removing it on normal exit. Force-killed processes — typically headless automation like lighthouse or puppeteer — leave zombie copies that accumulate (observed: 92 copies / 156 GB).

- **Matching**: a single `*.code_sign_clone` glob, not a per-browser catalog; the label carries the browser name (from the bundle ID) and the copy count.
- **Safety follows run state**: `safe` when the browser is not running (true zombies); `caution` while it runs (checked via `pgrep`, once per browser — the newest copy may be in use) or when the bundle ID is unrecognized (run state unknowable).
- **Scope**: the path lies outside home, so it is reported only when the scan root covers the home directory — a `--path` scan of a home subdirectory never surfaces system temp.
- **Size caveat**: reported size may overstate real usage when the copies are APFS clones of the installed app.

## LLM Model Stores

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

## Categories

| Category | Description |
|----------|-------------|
| `cache` | Global or local caches (npm, Gradle, pip, etc.) |
| `build` | Build artifacts (DerivedData, .next, dist, etc.) |
| `runtime` | Runtimes and simulators (iOS runtimes, Docker images) |
| `deps` | Dependencies (node_modules, venv, etc.) |

## Safety Levels

| Level | Icon | Description |
|-------|------|-------------|
| `safe` | ✔ (green) | Freely deletable, auto-regenerated on next build/install |
| `caution` | ⚠ (yellow) | Deletable but may require rebuild or has shared impact |
| `protected` | ✖ (gray) | Tracked by git with uncommitted changes |

**Gitignore-aware**: artifacts in `.gitignore` (like `node_modules`) are always deletable even in repos with uncommitted changes. Only git-tracked artifacts are protected.
