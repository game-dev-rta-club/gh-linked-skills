---
title: Pulling a managed skill
updated: 2026-07-15
status: implemented
---

# Pull

Bring source changes into the local skill. If local content also changed, the
command merges both sides from the last synchronized baseline.

```bash
gh skill-linker pull <skill-name|project-relative-path>
```

The skill must be managed by the manifest, every local file must be tracked in
the Git index, and no unresolved conflict may remain. The rest of the parent
project does not need to be clean.

- no local changes: update to the source
- local changes: perform a three-way merge
- identical snapshot: update only the baseline

## When a conflict occurs

Text conflicts leave the competing content in the affected file. See
[Resolving conflicts](resolve-conflicts.md). Binary or structural conflicts stop
without changing the affected file.

Success output:

- `pulled <path> to <tree-sha>`
- `<path> is already up to date`

Implementation: [Pull operation](../../3_Functions/pages/operations/pull.md)
