---
title: Conflict merge reference
updated: 2026-07-15
status: implemented
---

# Conflict merge

Merge base, local, and remote content one file at a time. See
[Resolving conflicts](../../../2_HowToUse/pages/resolve-conflicts.md) for the
manual workflow.

## MERGE-001 Per-file three-way merge

Sort the union of paths, then merge existence, raw bytes, and executable bits.

- added on one side: accept the addition
- same bytes on both sides: accept content and merge the mode
- one side equals base: accept the other side
- content changed on one side and mode changed on the other: accept both
- deleted on both sides: delete
- deleted on one side and changed on the other: modify/delete error

A missing `SKILL.md` or file/directory collision is an error.

## MERGE-002 Text and binary

When both sides changed and either contains a NUL byte, return a binary error
without changing the workspace. Merge text with:

```bash
git merge-file --diff3 \
  -L gh-skill-linker:local \
  -L gh-skill-linker:base:<base-tree-sha> \
  -L gh-skill-linker:remote:<remote-tree-sha>
```

Any overlap leaves conflict markers, including frontmatter overlaps. Input uses
temporary files with mode `0600`. The application does not transform encoding
or newlines and does not impose its own size limit.

Exit values from 1 through 127 from `git merge-file` are treated as a conflict
count. See [git-merge-file](https://git-scm.com/docs/git-merge-file).

## MERGE-003 Executable mode

- local equals base: remote mode
- remote equals base: local mode
- both changed: logical OR

A mode change combined with deletion is an error.

## MERGE-004 Manual resolution state

These substrings identify an unresolved conflict:

- `<<<<<<< gh-skill-linker:local`
- `||||||| gh-skill-linker:base:`
- `>>>>>>> gh-skill-linker:remote:`

While markers remain, state is `conflict` and pull/push reason is
`unresolved_conflict`. There is no separate state file. After resolution,
content that differs from remote produces `push`.
