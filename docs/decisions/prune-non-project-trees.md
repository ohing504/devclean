# Prune trees that are not project output

## Decision

The walk engine skips these trees whole — no artifact matching, no project context, no descent — so nothing inside is ever a deletion target:

- **Installed-package trees**, engine-wide regardless of which ecosystems are active: their marker files belong to shipped packages, not projects. Detected by invariant layout or by a directory name only package managers create, never install path.
  - pnpm store version dir (`v<N>` with `files/` and `index/` or `index.db`)
  - macOS app bundle (`*.app` with `Contents/`)
  - `node_modules` reached without matching a project's `node_modules` rule (a global install prefix such as `lib/node_modules`, or a tool's bundled dependencies)
  - Python `site-packages` / `dist-packages`
  - Go module cache (`mod/` with `cache/download/`)
- **An ecosystem's own SDK checkout**, via the table's `PruneRoot` hook. The SDK root is detected by its invariant bootstrap layout, never a hardcoded install path (Flutter: `bin/flutter` + `bin/internal/engine.version`).

When adding an ecosystem or rule, scan that tool's real SDK/toolchain checkout and verify what gets flagged. A scanner's own false positive is fixed in that scanner's PR.

## Verification

```bash
go test ./internal/scanner/ -run 'TestWalkExcludesInstalledPackageTrees|TestFlutterWalk' -v
```

## Rationale

- A directory name like `build`/`dist` is not always regenerable output. Inside an SDK checkout it can be committed *source*: the Flutter SDK's `engine/src/build` / `engine/src/flutter/build` are GN build-system source trees tracked in git.
- Gitignore-aware protection does not catch this: committed-clean files are not `protected`, so they classify as `safe` and become deletion targets.
- pnpm v11's `links/` unpacks packages whose `dist/` would match; deleting it corrupts the store every project hard-links from. The store itself is reported by the Global Caches scanner.
- Apps ship their runtime inside the bundle (e.g. Electron `node_modules` in VS Code update copies under `~/Library/Caches`).
- A `node_modules` outside any project holds installed packages whose own `package.json` would root a project, so their shipped `dist/` matched as `safe` build output (e.g. the npm and corepack bundled in a GitHub Actions runner, a VS Code extension's SDK cache). The same holds for packages in `site-packages` and module sources in the read-only Go module cache. A project's own `node_modules` is matched as an artifact before descent, so it is still reported.
- The Android SDK ships no `build.gradle`, so it never establishes a project context and needs no `PruneRoot`.

## Rejected alternatives

- **Detect by install path** — SDKs and stores live wherever the user put them; a hardcoded path misses relocated checkouts.
- **Rely on gitignore-aware protection** — misses committed-clean source (above).
- **Defer a scanner's false positive to a separate change** — the scanner ships able to delete real source in the meantime.
