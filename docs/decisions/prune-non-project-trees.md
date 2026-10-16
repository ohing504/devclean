# Prune trees that are not project output

## Decision

The walk engine skips these trees whole — no artifact matching, no project context, no descent — so nothing inside is ever a deletion target:

- **Installed-package trees**, whichever ecosystems are active. Detected by layout or by a name only package managers create, never by install path:
  - by name, when the directory is not itself a matched artifact: `node_modules`, `site-packages`, `dist-packages`
  - by entries: pnpm store version dir (`v<N>` with `files/` and `index/` or `index.db`), macOS app bundle (`*.app` with `Contents/`), Go module cache (`mod/` with `cache/download/`)
- **An ecosystem's own SDK checkout**, via the table's `PruneRoot` hook. The SDK root is detected by its invariant bootstrap layout, never a hardcoded install path (Flutter: `bin/flutter` + `bin/internal/engine.version`).

When adding an ecosystem or rule, scan that tool's real SDK/toolchain checkout and verify what gets flagged. A scanner's own false positive is fixed in that scanner's PR.

## Verification

```bash
go test ./internal/scanner/ -run 'TestWalkExcludesInstalledPackageTrees|TestFlutterWalk' -v
```

## Rationale

- Each package in an installed-package tree, including the runtime an app ships inside its bundle, carries its own marker (`package.json`, `setup.py`, `go.mod`). Without pruning it roots a project, and its shipped `dist/`, `__pycache__` or `vendor/` matches as `safe` output — deleting the `dist/` of the npm bundled in a GitHub Actions runner breaks the runner. A project's own `node_modules` matches as an artifact before the name check, so it is still reported.
- pnpm v11's `links/` holds unpacked packages; deleting it corrupts the store every project hard-links from. The store itself is reported by the Global Caches scanner.
- A directory name like `build`/`dist` is not always regenerable output. Inside an SDK checkout it can be committed *source*: the Flutter SDK's `engine/src/build` / `engine/src/flutter/build` are GN build-system source trees tracked in git.
- Gitignore-aware protection does not catch this: committed-clean files are not `protected`, so they classify as `safe` and become deletion targets.
- The Android SDK ships no `build.gradle`, so it never establishes a project context and needs no `PruneRoot`.

## Rejected alternatives

- **Detect by install path** — SDKs and stores live wherever the user put them; a hardcoded path misses relocated checkouts.
- **Rely on gitignore-aware protection** — misses committed-clean source (above).
- **Prune every directory a deps rule names** — would also prune `vendor/` and `venv/` outside projects, which are not package-manager trees.
- **Defer a scanner's false positive to a separate change** — the scanner ships able to delete real source in the meantime.
