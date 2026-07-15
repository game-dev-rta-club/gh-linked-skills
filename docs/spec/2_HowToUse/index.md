---
title: Skill Linker usage
updated: 2026-07-15
status: implemented
---

# How to use Skill Linker

This section is the source of truth for user-visible operations. Most users do
not need to read beyond this layer.

A `source` is a skill on GitHub. `local` is the copy inside the current Git
project. Every operation targets the root of the current Git worktree.

First install the extension. Then install a skill from a remote source or
publish a local skill.

1. [Install the extension](pages/install-extension.md)
2. [Install a skill](pages/install-skill.md)
3. [Publish a skill](pages/publish.md)
4. [Install from a tag](pages/install-by-tag.md)

Operations:

- [Check status](pages/status.md)
- [Pull changes](pages/pull.md)
- [Push changes](pages/push.md)
- [Uninstall a skill](pages/uninstall.md)
- [Resolve a conflict](pages/resolve-conflicts.md)
- [Command reference](pages/command-reference.md)

For implementation details, see [Functions](../3_Functions/index.md).
