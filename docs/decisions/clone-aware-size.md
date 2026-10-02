# Size is what deleting frees, APFS clones included

## Decision

`size` (JSON) and every displayed size is the space deleting the artifact frees. For artifacts known to be APFS clones on macOS — browser code-sign copies and pnpm-installed `node_modules` — sizing nets out clone sharing:

- A pure clone (APFS reports a clone ID and a clone count) counts once, and only when every clone of it is inside the artifact.
- Any other file that may share blocks has its physical extents read (`fcntl(F_LOG2PHYS_EXT)`); blocks shared inside the artifact count once, and blocks used by the clone source (installed app bundle, pnpm store) are not counted. The source is read only when such a file exists.

The per-file block sum stays available as `allocated_size`, present only when it differs. Other artifacts keep the block count as before.

## Verification

```bash
go test ./internal/scanner/ -run 'Clone|Pnpm' -v
go test ./internal/output/ -run SharedBlocks -v
```

Measured on one machine (2026-10-02): Chrome code-sign copies 11.3 GB → 0.76 GB; a pnpm `node_modules` 448 MB → 3 MB. Full home scan time unchanged (58 s vs 59 s).

## Rationale

- Users pick what to delete by size; a size that is mostly shared blocks (Chrome copies overstated ~15×) leads to deleting for space that never comes back.
- Clone IDs need one `getattrlist` per file and no `open`; reading the pnpm store's extents instead (240k files) added ~15 s per scan.
- `getattrlist` is a direct syscall (no libSystem wrapper in `syscall` or `x/sys`); when it fails, sizing falls back to reading extents — slower, same result.

## Rejected alternatives

- **Keep `size` as blocks, add a reclaim field** — sorting, `--min-size` and totals would keep ranking shared blocks first.
- **`ATTR_CMNEXT_PRIVATESIZE`** (bytes freed if this one file is deleted) — misses blocks shared only among the artifact's own files (Chrome copies: 0 instead of 0.76 GB), and also excludes blocks a snapshot holds, unlike every other size.
- **Extents for every file against the clone source** — correct but loads the whole pnpm store each scan.
- **Clone-aware sizing for every artifact** — one extra syscall per file across the whole scan for artifacts that are rarely clones.

## Limits

- Blocks an APFS snapshot holds are counted as freed, as for every other artifact.
- Linux reflinks (btrfs, XFS) are not read.
