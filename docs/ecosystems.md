# Ecosystems

## Supported Ecosystems

| ID | Name | Status |
|----|------|--------|
| `node` | Node.js | implemented |
| `rust` | Rust | implemented |
| `ruby` | Ruby | implemented |
| `python` | Python | planned |
| `xcode` | iOS/Xcode | planned |
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
