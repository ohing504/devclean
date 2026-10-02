# Contributing to devclean

Thanks for your interest!

## Development

Development docs start at [CLAUDE.md](CLAUDE.md): build, test and lint commands, where each doc lives, and commit/PR conventions. Behavior is specified in [docs/SPEC.md](docs/SPEC.md); adding an ecosystem is in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#adding-an-ecosystem).

## Reporting bugs

File an issue at <https://github.com/ohing504/devclean/issues> with:

- `devclean --version` (once we tag releases) or current commit hash
- OS + Go version (`go version`)
- Exact command that misbehaved
- `--json` output and / or screenshot of the problem
- What you expected vs what happened

For scanner false-positives / false-negatives, including the project layout (`tree -L 2 <project>`) helps a lot.

## Security

Found something that affects user data (e.g. unintended deletion path)? Don't open a public issue. Use GitHub's [private vulnerability reporting](https://github.com/ohing504/devclean/security/advisories/new) instead.

## License

By contributing, you agree your contribution is licensed under the [MIT License](LICENSE).
