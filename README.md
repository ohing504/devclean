# devclean

Developer disk cleanup CLI — scan and clean build artifacts, caches, dependencies, and runtimes across multiple ecosystems.

## Features

- Scan for reclaimable disk space across iOS/Xcode, Android, Flutter, Node.js, Docker, Python, and global caches
- Classify items by activity status and safety level
- Git-aware protection for projects with uncommitted changes
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
devclean scan --eco xcode,node
devclean scan --json

# Clean up
devclean clean --eco node --status dormant
devclean clean --safe
devclean clean --dry-run
devclean clean --force

# Utilities
devclean list
devclean config show
```

## License

MIT
