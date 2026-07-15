---
title: Pushing a managed skill
updated: 2026-07-15
status: implemented
---

# Push

Return an improved local skill to the source branch selected during install.

```bash
gh linked-skills push <skill-name|project-relative-path>
```

The repository must be writable, the source skill tree must be unchanged, the
local `SKILL.md` must be valid and free of conflict markers, and every file must
be tracked or untracked but not ignored.

The command creates a normal commit on the same branch and pushes without
force. If the source skill changed, it asks you to pull first. A change to a
different path on the same branch does not block the push.

`eligible` from `status` means the preconditions are satisfied. It does not
guarantee that GitHub branch protection or rulesets will accept the push.

Only source-repository skill files are committed. The command does not commit
or push the parent project.

A read-only skill can only be pulled. `status` still reports local changes.

Success output:

- `pushed <path> to <tree-sha>`
- `<path> has no source changes to push`

Implementation: [Push operation](../../3_Functions/pages/operations/push.md)
