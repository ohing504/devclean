# Ship the agent skill as a Claude Code plugin from this repository

## Decision

The repository root is a Claude Code marketplace (`.claude-plugin/marketplace.json`) listing one plugin, `plugins/devclean/`, which holds one skill. The skill runs the `devclean` binary on the user's PATH; the plugin ships no binary.

- The plugin sets no `version`, so installs track the repository's commits instead of a second version number next to release-please's.
- The skill measures with `scan --json` and deletes with `clean --item … --yes`; size accounting and the safety gates (`protected` never deleted, `caution` only with `--include-caution`) stay in the binary.
- The human interface (table, interactive selector) stays; removing it is a separate decision after the skill has been used.

## Rationale

- Skills under a repository's `.claude/skills/` load only in sessions working in that repository; devclean is run from any directory, and Claude Code's docs direct skills used across repositories to a plugin.
- Keeping the plugin in this repository lets a PR that changes a flag or JSON field change the skill in the same diff.
- An agent rebuilding sizes with `du` or deleting with `rm` gets APFS clones, hard links and sparse files wrong without an error, and skips the safety gates; the binary already handles both.

## Rejected alternatives

- **Skill in `~/.claude/skills/`** — loads everywhere for one user, but changes separately from the CLI it describes, and others cannot install it.
- **Skill only, no CLI** (find/du/rm recipes in the skill) — the size and classification bugs fixed in the scanner would come back as silent errors on each run.
- **Binary bundled in the plugin** — needs per-OS builds inside the plugin and duplicates the GoReleaser release.
- **Plugin `version` bumped by release-please** — would tie skill updates to releases, but adds a second version file to keep in sync for a skill that is plain text.
