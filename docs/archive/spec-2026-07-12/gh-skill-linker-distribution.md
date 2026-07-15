---
title: Skill Linker distribution and support
updated: 2026-07-15
status: archived
---

# Distribution and support

## Extension

The repository, extension, and project names are all `gh-skill-linker`. Release
it as a precompiled Go GitHub CLI extension so users do not need a Go runtime.

```bash
gh extension install game-dev-rta-club/gh-skill-linker
gh extension upgrade skill-linker
```

Require an update before operation for GitHub CLI versions earlier than 2.96.0.
Do not maintain a compatibility layer for older versions.

## Workflow skill

Embed the workflow skill in the extension binary and install it without an
additional download.

```bash
gh skill-linker skills install --agent codex
gh skill-linker skills install --agent claude-code
```

Provide project scope only. Running the same command after an extension upgrade
updates only managed bundles with no local changes.

## Support

| Target | MVP |
| --- | --- |
| Host | GitHub.com |
| OS | macOS, Linux |
| Git | system Git required |
| Managed skill | Codex-compatible, project scope |
| Workflow adapter | Codex, Claude Code |

GHES, Windows, user scope, and custom destinations are not officially
supported.

## Related

- [Overview](gh-skill-linker.md)
- [Implementation](gh-skill-linker-implementation.md)
