# CLI Commands

Run `devclean --help` or `devclean <command> --help` for the most up-to-date flag reference.

## scan

Scan for reclaimable disk space. See `devclean scan --help` for all flags.

```bash
# Scan workspace for Node.js projects
devclean scan --path ~/workspace --eco node

# Top 10 largest dormant projects
devclean scan --status dormant -n 10

# Sort by last activity
devclean scan --sort time

# Skip noise — only artifacts ≥ 100 MB
devclean scan --min-size 100MB

# Verbose: show all sub-packages and artifacts
devclean scan --path ~/workspace --eco node -v

# JSON output for scripting / AI agents
devclean scan --eco node --json
```

### Output

Colored table grouped by ecosystem → project → sub-package → artifacts:

```
● node 3 projects · 5.2 GB
  my-app Active 2.1 GB · 3 days ago
  ~/workspace/my-app
    . (root) (1.8 GB)
      ✔ node_modules (deps)      1.7 GB
      ✔ .turbo (cache)          100.0 MB
    apps/web (300.0 MB)
      ✔ .next (build)           300.0 MB

Total: 5.2 GB (5 items)
Safe to clean: 5.2 GB

Legend: ✔ safe  ⚠ caution  ✖ protected   ● Active  ● Recent  ● Stale  ● Dormant
        Run 'devclean list' for details
```

A sparse artifact is shown as `8.6 GB (appears as 494.4 GB)` — real disk size, then the size it reports. When hard-linked blocks are shared across artifacts, the total counts them once and says so.

### JSON (`--json`)

```json
{
  "total_size": 5583457484,
  "total_count": 5,
  "results": [
    {
      "path": "/Users/you/workspace/my-app/node_modules",
      "ecosystem": "node",
      "category": "deps",
      "size": 1782579200,
      "apparent_size": 1690123456,
      "last_modified": "2026-07-14T09:12:00Z",
      "activity": "active",
      "safety": "safe",
      "protected": false
    }
  ]
}
```

- `size` — disk usage (allocated blocks); sparse-aware, used for sorting and `--min-size`.
- `apparent_size` — logical size; omitted when zero. Much larger than `size` for sparse files.
- `total_size` — sum of `size` with hard-linked blocks counted once.

Also present when known: `reason`, `project_root`, `label`, `recommendation`, `last_used_at`.

## clean

Clean reclaimable disk space. See `devclean clean --help` for all flags.

```bash
# Interactive: select projects, choose trash vs delete
devclean clean --eco node

# Filter dormant only, then select interactively
devclean clean --eco node --status dormant

# Non-interactive: clean all safe dormant items
devclean clean --eco node --status dormant --safe --yes

# Preview without deleting
devclean clean --eco node --dry-run --yes

# Force permanent delete (skip Trash)
devclean clean --eco node --force --yes

# Skip artifacts smaller than 50 MB
devclean clean --eco node --min-size 50MB

# Also run ecosystem-native cleanup commands (e.g. xcrun simctl delete unavailable)
devclean clean --eco xcode --vendor-cleanup --yes
```

### `--min-size`

Both `scan` and `clean` accept `--min-size <size>` to drop artifacts below the
threshold. Useful when scanning a noisy workspace where lots of sub-MB
artifacts crowd out the real targets. Sizes use SI/IEC suffixes:

- Decimal: `KB`, `MB`, `GB` (1 KB = 1000 bytes)
- Binary: `KiB`, `MiB`, `GiB` (1 KiB = 1024 bytes)
- Plain integer = bytes

`devclean scan --min-size 100MB` keeps only artifacts ≥ 100 MB.

### Non-interactive safety (`--yes` / `--include-caution`)

`--yes` skips the interactive selector for scripting and AI-agent use. To keep a
mis-classification from becoming silent data loss, `--yes` deletes only `safe`
(auto-regenerated) items by default. `caution` items — shared impact, or state
that is slow or impossible to regenerate — are skipped and reported:

```
Skipped 3 caution item(s) — pass --include-caution to remove them with --yes.
```

Add `--include-caution` to also delete `caution` items non-interactively. `protected`
items are never deleted either way. The interactive selector (no `--yes`) is
unaffected — you still see and choose caution items yourself.

### Vendor Cleanups

Some ecosystems ship official cleanup commands that are stricter or safer than
deleting paths directly (they keep the vendor's internal state consistent).
The `--vendor-cleanup` flag runs them in addition to the path-based cleanup.
They are scoped to the ecosystems you target: the `--eco` selection, or — when
`--eco` is omitted — the ecosystems of the artifacts actually being cleaned.

| Ecosystem | Command | What it does |
|-----------|---------|--------------|
| xcode | `xcrun simctl delete unavailable` | Removes simulator devices whose iOS/watchOS/tvOS runtime was uninstalled. |
| global | `brew cleanup -s` | Removes stale Homebrew downloads and old versions. |
| global | `npm cache clean --force` | Clears the npm package cache. |
| global | `yarn cache clean` | Clears the Yarn cache. |
| global | `pnpm store prune` | Removes unreferenced packages from the pnpm store. |
| global | `pip cache purge` | Removes all wheels from the pip cache. |
| global | `uv cache prune` | Removes outdated entries from the uv cache. |

Commands for tools not installed on the machine are skipped (detected via PATH
lookup). The `global` ecosystem runs every installed manager's prune together;
individual tools can't be targeted separately since they share one ecosystem.

`--dry-run` prints the commands without executing. `--vendor-cleanup` is
additive — combine with `--safe`, `--status`, `--yes` as usual.

### Interactive Tree Selector

The clean command uses a tree selector matching scan's output style:
- `[↑↓]` move, `[←→]` jump between projects, `[space]` toggle
- `[a]` select all, `[n]` none, `[s]` safe only, `[d]` dormant only
- `[enter]` confirm, `[esc]` cancel
- Projects show `[✔]` selected, `[-]` partial, `[✖]` protected
- Protected projects are hidden with explanation

### Flow

1. Scan and classify (with progress spinner)
2. Tree selector: select projects/artifacts interactively
3. Choose: Move to Trash / Permanently delete / Cancel
4. Execute with per-artifact results

## list

List supported ecosystems, categories, activity statuses, and safety levels.

```bash
devclean list
```
