---
title: Pushing a managed skill
updated: 2026-07-15
status: implemented
---

# Push

Return an improved local skill to the source branch selected during install.

```bash
gh skill-linker push <skill-name|project-relative-path>
gh skill-linker push <skill-name|project-relative-path> --pr
```

The repository must be writable, the source skill tree must be unchanged, the
local `SKILL.md` must be valid and free of conflict markers, and every file must
be tracked or untracked but not ignored.

The command creates a normal commit on the same branch and pushes without
force. If the source skill changed, it asks you to pull first. A change to a
different path on the same branch does not block the push.

`--pr` creates one pull request for the skill. Later local changes update the
same pull request. It does not advance the manifest baseline.

If the source branch changes while the pull request is open, run `pull`, resolve
the local files, then rerun `push --pr`. The proposal branch receives a normal
merge commit. It is not rebased or force-pushed.

Direct push is rejected while a managed pull request for the skill is open.

`eligible` from `status` means the preconditions are satisfied. It does not
guarantee that GitHub branch protection or rulesets will accept the push.

Only source-repository skill files are committed. The command does not commit
or push the parent project.

A read-only skill can only be pulled. `status` still reports local changes.

Success output:

- `pushed <path> to <tree-sha>`
- `<path> has no source changes to push`
- `<state> proposal #<number> for <path>: <url>`

Implementation: [Push operation](../../3_Functions/pages/operations/push.md)
