# Prune trees that are not project output

## Decision

The walk engine skips these trees whole — no artifact matching, no project context, no descent — so nothing inside is ever a deletion target:

- **Installed-package trees**, engine-wide regardless of which ecosystems are active: their marker files belong to shipped packages, not projects. Detected by invariant layout, never install path.
  - pnpm store version dir (`v<N>` with `files/` and `index/` or `index.db`)
  - macOS app bundle (`*.app` with `Contents/`)
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
- The Android SDK ships no `build.gradle`, so it never establishes a project context and needs no `PruneRoot`.

## Rejected alternatives

- **Detect by install path** — SDKs and stores live wherever the user put them; a hardcoded path misses relocated checkouts.
- **Rely on gitignore-aware protection** — misses committed-clean source (above).
- **Defer a scanner's false positive to a separate change** — the scanner ships able to delete real source in the meantime.
