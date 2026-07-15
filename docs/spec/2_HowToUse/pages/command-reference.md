---
title: Command reference
updated: 2026-07-15
status: implemented
---

# Commands

This page lists commands and shared CLI rules. Each linked page is the source
of truth for its command syntax, behavior, and output.

| Command | Purpose | Details |
| --- | --- | --- |
| install | Register a skill in the project | [Install a skill](install-skill.md) |
| publish | Publish or propose an unmanaged skill to a source | [Publish](publish.md) |
| status | Inspect synchronization state | [Status](status.md) |
| pull | Bring source changes into the project | [Pull](pull.md) |
| push | Push or propose local changes | [Push](push.md) |
| uninstall | Remove a skill and its management record from the project | [Uninstall](uninstall.md) |

## Help

After installing the extension, the CLI help covers the basic operations.

```bash
gh skill-linker --help
gh skill-linker help install
gh skill-linker install --help
gh skill-linker publish --help
gh skill-linker uninstall --help
```

Root help shows the purpose, command list, and basic examples. Command help
shows the description, syntax, arguments, flags, and examples. Help does not
require GitHub authentication or a Git project.

`-h` is equivalent to `--help`. The CLI does not support other short flags,
`--flag=value`, `--`, or a top-level `--version` flag.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Operation failure |
| `2` | Usage error |

A pull that leaves conflicts also exits with `1`. See
[Resolving conflicts](resolve-conflicts.md) for the output and recovery steps.

Implementation: [CLI runtime](../../3_Functions/pages/cli/runtime.md)
