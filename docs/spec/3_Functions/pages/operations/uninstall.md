---
title: Uninstall operation
updated: 2026-07-15
status: implemented
---

# Uninstall

Remove a managed project-local skill without reading or changing remote state.

## UNIN-001 Selection

Accept `gh skill-linker uninstall SKILL [--force]`. `SKILL` is a manifest key
or project-relative destination. Reject unmanaged skills.

## UNIN-002 Safety

Confirm that the destination is a regular directory inside the project with no
symlink component.

Without `--force`, delete only when the local tree SHA equals manifest
`treeSHA` and no empty directory was added. Added or removed files, byte or
executable-bit changes, and added empty directories count as local changes.
`--force` skips only the local-equality check, never path safety.

When the destination is absent, remove only the manifest entry because no data
can be lost.

## UNIN-003 Transaction

Move the destination to a temporary directory under the same parent, then
delete the manifest entry through an optimistic comparison with the starting
entry.

If the manifest update fails, restore the destination. After success, delete
the temporary directory. A cleanup failure is an error, but the manifest entry
and original destination remain deleted.

The operation requires no GitHub API, GitHub authentication, or remote source
state.
