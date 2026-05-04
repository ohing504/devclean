# Ecosystems

## Supported Ecosystems

| ID | Name | Status |
|----|------|--------|
| `node` | Node.js | implemented |
| `rust` | Rust | implemented |
| `ruby` | Ruby | implemented |
| `xcode` | iOS/Xcode (macOS only) | implemented |
| `python` | Python | planned |
| `android` | Android | planned |
| `flutter` | Flutter/Dart | planned |
| `docker` | Docker | planned |
| `global` | Global Caches | planned |

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

**Per-version / per-device expansion**: `iOS / watchOS / tvOS DeviceSupport` and `CoreSimulator/Devices` are reported as one result *per child directory* (per iOS version, per simulator device) instead of one big lump. This lets you reclaim a single old iOS runtime (e.g. `iPhone13,3 26.3`) without nuking active versions. Children share the parent path as `ProjectRoot` so they group together in output.

**Notes**:
- `Archives` is `caution` because losing an archive means losing the ability to symbolicate crash reports for that release.
- `CoreSimulator/Devices` is `caution` because it contains app installs, settings, and user data inside simulators currently in use.

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
