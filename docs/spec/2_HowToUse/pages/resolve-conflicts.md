---
title: Resolving conflicts manually
updated: 2026-07-15
status: implemented
---

# Resolve conflicts

When local and source content change the same line differently, the extension
cannot decide which version to keep.

`pull` does not choose automatically. It writes the local, last synchronized,
and current source content into the actual skill file.

```text
CONFLICT (content): Merge conflict in .agents/skills/sample/SKILL.md
Pull completed with conflicts; fix them in the working tree.
After resolving, run:
  gh skill-linker status
If STATE is push, run:
  gh skill-linker push sample
```

This follows Git conflict behavior. Exit code `1` does not mean the operation
rolled back. It means the merge result was written to the working tree and
manual resolution remains.

## Content written to the file

> ```text
> <<<<<<< gh-skill-linker:local
> title: Text edited locally
> ||||||| gh-skill-linker:base:<tree-sha>
> title: Text from the last synchronization
> =======
> title: Text updated at the source
> >>>>>>> gh-skill-linker:remote:<tree-sha>
> ```

- `local`: content edited in the project
- `base`: content from the last synchronization
- `remote`: content currently at the source

Lines beginning with `<<<<<<<`, `|||||||`, `=======`, and `>>>>>>>` are conflict
markers.

## Resolve the conflict

1. Search the project for `<<<<<<< gh-skill-linker:local`.
2. Compare local, last synchronized, and source content.
3. Rewrite the file with the content you want to keep.
4. Remove all four marker types and unwanted content.
5. Run `gh skill-linker status`.

For example, the final file may contain only:

```text
title: Final text combining the useful local and source changes
```

Content identical to the source produces `clean`. Keeping local content or a
combination of both sides produces `push`; use the displayed command to return
it to the source. You do not need to pull again.

Binary, modify/delete, and file/directory conflicts stop without changing the
file.

Implementation: [Conflict merge](../../3_Functions/pages/operations/conflict-merge.md)
