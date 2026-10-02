# Size is what deleting frees, APFS clones included

## Decision

`size` is the space deleting the artifact frees. For browser code-sign copies and pnpm `node_modules` on macOS, sizing nets out APFS clone sharing:

- A pure clone (APFS reports its clone ID and clone count) counts once, only when all its clones are inside the artifact. Groups split across artifacts count once in totals.
- Other files that may share blocks are compared by physical extents (`F_LOG2PHYS_EXT`) against each other and the clone source: the installed browser app. pnpm has no clone source; it installs pure clones.

The per-file block sum is kept as `allocated_size` when larger.

## Verification

```bash
go test ./internal/scanner/ -run 'Clone|Pnpm' -v
go test ./internal/output/ -run SharedBlocks -v
```

Measured 2026-10-02: Chrome copies 11.3 GB → 0.76 GB; a pnpm `node_modules` 448 MB → 3.9 MB; home scan time unchanged (~60 s).

## Rationale

- Sizes that are mostly shared blocks (Chrome copies ~15×) lead users to delete for space that never comes back.
- Clone IDs cost one `getattrlist` per file. `getattrlist` is a direct syscall (no libSystem wrapper in `syscall` or `x/sys`); on failure sizing falls back to extents.

## Rejected alternatives

- **Separate reclaim field, `size` unchanged** — sorting, `--min-size` and totals would still rank shared blocks.
- **`ATTR_CMNEXT_PRIVATESIZE`** — misses blocks shared only among the artifact's files (Chrome copies: 0 instead of 0.76 GB) and excludes snapshot-held blocks.
- **Extents against the pnpm store** — reading 240k store files added ~15 s per scan.
- **Clone-aware sizing everywhere** — a syscall per file for artifacts that are rarely clones.

## Limits

- Snapshot-held blocks count as freed, as for other artifacts.
- A pnpm file partly rewritten after cloning counts in full.
- Partly rewritten copies in different artifacts that share extents are counted in each.
- Linux reflinks are not read.
