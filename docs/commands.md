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

### Vendor Cleanups

Some ecosystems ship official cleanup commands that are stricter or safer than
deleting paths directly (they keep the vendor's internal state consistent).
The `--vendor-cleanup` flag runs them in addition to the path-based cleanup.
They are scoped to the ecosystems you target: the `--eco` selection, or — when
`--eco` is omitted — the ecosystems of the artifacts actually being cleaned.

| Ecosystem | Command | What it does |
|-----------|---------|--------------|
| xcode | `xcrun simctl delete unavailable` | Removes simulator devices whose iOS/watchOS/tvOS runtime was uninstalled. |

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
