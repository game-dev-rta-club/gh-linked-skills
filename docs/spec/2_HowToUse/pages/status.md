---
title: Checking synchronization health
updated: 2026-07-15
status: implemented
---

# Status

`STATE` shows the direction of differences. `PULL` and `PUSH` show whether each
operation is currently available. Only skills registered in the manifest are
shown.

```bash
gh skill-linker status
```

Example:

```text
SKILL          PATH                                STATE     PULL                              PUSH
shared-rules   .agents/skills/shared-rules         clean     eligible                          eligible
review-helper  .agents/skills/review-helper        pull      eligible                          ineligible (remote_changed)
local-notes    .agents/skills/local-notes          push      eligible                          eligible
idea-refine    .agents/skills/idea-refine          conflict  ineligible (unresolved_conflict)  ineligible (unresolved_conflict)
release-skill  .agents/skills/release-skill        clean     ineligible (fixed_source_ref)      ineligible (source_ref_read_only)
```

- `STATE`: direction of differences between local and source
- `PULL` / `PUSH`: whether the command can run
- parenthesized value: reason an operation cannot run

| State | Meaning | Action |
| --- | --- | --- |
| `clean` | No difference | None |
| `pull` | Source changed | Pull |
| `push` | Local content changed | Push |
| `conflict` | Both sides changed or unresolved markers remain | Edit files with markers; otherwise pull |

When the source tree SHA differs from the baseline, `pull` takes precedence
even if file content is identical.

| Reason | Response |
| --- | --- |
| `invalid_local_skill` | Repair `SKILL.md` or its frontmatter |
| `unsafe_local_path` | Repair the path |
| `unsupported_local_file` | Replace it with a regular file |
| `unsupported_host` / `invalid_source_repository` | Check the source |
| `unresolved_conflict` | Resolve conflict markers in the file |
| `source_unavailable` | Check network, authentication, and source availability |
| `git_inventory_unknown` | Check Git state |
| `untracked_files` | Add files to the Git index |
| `ignored_files` | Track ignored untracked files or remove the ignore rule |
| `permission_unknown` | Check network, authentication, and repository access |
| `repository_read_only` | Use the source for pull-only operation |
| `remote_changed` | Pull before pushing |
| `fixed_source_ref` | The tag is fixed; reinstall from a different tag to change it |
| `source_ref_read_only` | The tag is fixed and cannot receive a push |
| `tag_moved` | Reinstall with `--accept-moved-tag` only if you trust the moved tag |

`ineligible` means the operation cannot run. `unknown` means the check failed.

`eligible` confirms preconditions only; it does not guarantee GitHub will
accept a push.

JSON output: `gh skill-linker status --json`

See [Resolving conflicts](resolve-conflicts.md) for marker editing.

Implementation: [Status operation](../../3_Functions/pages/operations/status.md)
