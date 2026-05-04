# Security Policy

## Supported Versions

devclean is in active development. Security fixes are issued for the latest
released version on the `main` branch. Older releases are not supported.

| Version | Supported |
|---------|-----------|
| Latest tagged release | ✅ |
| Earlier releases      | ❌ |
| Pre-release (untagged main) | best-effort |

## Reporting a Vulnerability

**Do not open a public issue for security problems.**

Use GitHub's private vulnerability reporting:

> <https://github.com/ohing504/devclean/security/advisories/new>

What to include:

- The version or commit hash you tested against
- A description of the vulnerability and its impact (especially around
  unintended file deletion, since `clean` operations are the highest-risk
  surface)
- Steps to reproduce — a minimal failing example helps a lot
- Any suggested fix or mitigation, if you have one

We will:

1. Acknowledge receipt within 5 working days.
2. Investigate and confirm whether the issue is reproducible.
3. Coordinate a fix and disclosure timeline with you, typically aiming for a
   patched release within 30 days for high-severity issues.
4. Credit you in the release notes (unless you prefer to remain anonymous).

## Scope

In scope:

- Unintended file deletion or data loss
- Path traversal or symlink-based attacks against the cleaner
- Trash/force-delete logic that bypasses safety checks
- Dependency vulnerabilities that materially affect runtime behavior

Out of scope:

- Issues that require an attacker who already has write access to the user's
  filesystem
- Reports of `caution` items being deletable (this is the documented behavior
  — see `docs/ecosystems.md`)
- Theoretical race conditions without a reproducible exploit path
