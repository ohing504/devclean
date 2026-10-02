# Never follow symlinks

## Decision

`fstree` never descends into a symlink and never matches one as an artifact, even when it is named like one (a symlinked `node_modules`, as pnpm and some monorepos produce). Only the scan root is read through, and only for `--path`. Deleting a symlink removes only the link; the cleaner's cross-filesystem trash copy recreates symlinks as links.

## Verification

```bash
go test ./internal/fstree/ -run 'Symlink' -v
go test ./internal/scanner/ -run 'Symlink' -v
go test ./internal/cleaner/ -run 'TestCopyTree_PreservesSymlinkAsLink' -v
```

## Rationale

- A symlink's target is real content that lives on disk elsewhere, so following it would double-count that space and inflate the reported reclaimable total.
- Deleting a symlinked artifact reclaims only the link (bytes) while risking a target shared by other projects.
- No-follow means symlink cycles can never be walked, so no separate cycle guard is needed.
- The reclaimable content behind these links is surfaced instead by the Global Caches scanner (e.g. the pnpm store) and hardlink-aware sizing.

## Rejected alternatives

- **Follow symlinks** — double-counts and puts shared targets at risk (above).
- **Rely on `os.ReadDir` reporting `IsDir()==false` for links** — incidental; the walk skips any `os.ModeSymlink` entry explicitly so a refactor cannot silently start following links.
