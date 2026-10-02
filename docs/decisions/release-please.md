# Release with release-please and GoReleaser

## Decision

release-please keeps a release PR open on `main`. Merging it bumps the version, writes the `CHANGELOG.md` section from the squash-merged PR titles, tags `vX.Y.Z` and creates the GitHub Release. In the same workflow run, GoReleaser builds the binaries and uploads them to that Release. Nobody edits `CHANGELOG.md` or pushes release tags by hand.

- Version: Conventional Commits, `feat` bumps minor, `fix`/`perf` bump patch; below 1.0 a breaking change bumps minor (`bump-minor-pre-major`).
- Sections: `feat` → Added, `fix` → Fixed, `perf`/`revert` → Changed; other types are hidden.
- Token: the default `GITHUB_TOKEN`. Its tag does not trigger other workflows, so GoReleaser runs as a job of the release-please workflow. CI on the release PR waits for a manual approval because `GITHUB_TOKEN` opened it.

## Verification

```bash
goreleaser check
```

The first release PR shows the generated version and section; its merge run shows the Release with assets.

## Rationale

- Releases stalled for five months when version, changelog cut and tag were manual steps; an always-open release PR makes a release one merge.
- Squash merge already makes each PR title one Conventional Commit, so the title is the changelog entry and no per-PR changelog edit is needed.
- GoReleaser stays for cross-compiled archives, checksums and a planned Homebrew cask.

## Rejected alternatives

- **git-cliff** — writes Keep a Changelog sections and computes the next version, but does not tag or release, so someone still has to run the release.
- **Hand-written changelog with a version helper (svu)** — keeps the detailed entries but keeps the manual release step that stalled.
- **GitHub App or PAT token** — lets the tag trigger a separate workflow and CI run without approval, at the cost of an app or an expiring secret to manage.
- **Plain Actions matrix instead of GoReleaser** — possible since Go 1.24 stamps the version from the tag, but the Homebrew cask would have to be written by hand.
