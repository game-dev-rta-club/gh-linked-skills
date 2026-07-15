---
title: Uninstall
updated: 2026-07-15
status: implemented
---

# Uninstall

Remove a managed skill from the current project without changing its source
repository.

```bash
gh linked-skills uninstall SKILL
```

`SKILL` may be a skill name or `.agents/skills/<name>`. When local content
matches the last synchronized baseline, the command removes the skill directory
and manifest entry.

To prevent accidental deletion, the command rejects local changes. Use the
force option only when you explicitly want to discard them.

```bash
gh linked-skills uninstall SKILL --force
```

If the skill directory is already missing, the command removes the remaining
manifest entry. Uninstall does not use GitHub authentication or a network
connection.

Implementation: [Uninstall operation](../../3_Functions/pages/operations/uninstall.md)
