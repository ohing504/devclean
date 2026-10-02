# CLAUDE.md

devclean is a Go CLI tool that scans developer environments for reclaimable disk space and provides safe cleanup.

## Commands

```bash
go build -o devclean ./cmd/devclean
go test -race -count=1 ./...                     # all tests (same as CI)
go test ./internal/model/ -run TestHumanSize -v  # single test
golangci-lint run ./...                          # lint
golangci-lint fmt                                # format (gofumpt + goimports)
go test ./internal/ -run TestGolden -update      # regenerate golden files after an intended output change
```

- lefthook pre-commit (enable once with `lefthook install`) runs lint + format on staged `.go` files.
- CI (`.github/workflows/ci.yml`) runs build, `go test -race -count=1` and lint on Ubuntu and macOS.
- Tool versions: `.tool-versions` (`mise install`; CI reads the same file). Go: `go.mod`.

## Docs

| File | Holds | Update when |
|---|---|---|
| `docs/SPEC.md` | behavior the user approved: commands, flags, JSON, safety levels, ecosystem catalog, config | behavior changes |
| `docs/ARCHITECTURE.md` | module boundaries, cross-module contracts, adding an ecosystem | a boundary or contract changes |
| `docs/decisions/` | one costly-to-reverse choice per file: decision, verification, rationale, rejected alternatives | such a choice is made or reversed |
| `CHANGELOG.md` | release sections written by release-please from PR titles | never by hand (`docs/decisions/release-please.md`) |

- Update docs in the same commit as the code.
- `docs/plans/` holds temporary design/implementation documents; it is excluded locally via `.git/info/exclude` and never committed.

## Commits and PRs

- Conventional Commits, lowercase prefix (`feat:`, `fix:`, `docs:`, …); one change per PR, squash-merged.
- PR titles are in English and state the user-visible change: release-please copies `feat`, `fix`, `perf` and `revert` titles into `CHANGELOG.md` verbatim.
- Releasing is merging the open release-please PR; do not push tags.
- The user reviews through the PR: its body states what changed and why, and which SPEC lines and decision files changed.
