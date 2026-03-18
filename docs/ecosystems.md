# Ecosystems

## Supported Ecosystems

| ID | Name | Status |
|----|------|--------|
| `node` | Node.js | implemented |
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
