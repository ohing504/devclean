# devclean

Developer disk cleanup CLI — scan and clean build artifacts, caches, dependencies, and runtimes across multiple ecosystems.

## Screenshots

### Scan

![devclean scan](demo/scan.png)

### Clean (Interactive Tree Selector)

![devclean clean](demo/clean.png)

## Features

- Scan for reclaimable disk space across Node.js, Rust, and more
- Monorepo support — artifacts grouped by git root with sub-package breakdown
- Activity classification — active, recent, stale, dormant (based on git + filesystem)
- Gitignore-aware protection — only git-tracked artifacts are protected
- Interactive tree selector for clean — select by project or individual artifact
- Soft delete (Trash) by default, with force delete option
- JSON output for scripting and AI agent integration
- Colored terminal output with ecosystem grouping

## Install

```bash
# Homebrew
brew install devclean

# Go
go install github.com/ohing504/devclean/cmd/devclean@latest
```

## Usage

```bash
# Scan for reclaimable space
devclean scan
devclean scan --path ~/workspace
devclean scan --eco node,rust
devclean scan --status dormant -n 10
devclean scan --json

# Clean up (interactive)
devclean clean --eco node
devclean clean --eco node --status dormant

# Clean up (non-interactive)
devclean clean --eco node --status dormant --yes
devclean clean --safe --dry-run --yes

# Utilities
devclean list
```

## Documentation

- [Architecture](docs/architecture.md) — system design and scanner patterns
- [CLI Commands](docs/commands.md) — full command reference with examples
- [Ecosystems](docs/ecosystems.md) — supported ecosystems, detection patterns, and safety levels
- [Configuration](docs/configuration.md) — settings file spec and options

## License

MIT
