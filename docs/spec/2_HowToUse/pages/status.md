---
title: Checking synchronization health
updated: 2026-07-15
status: implemented
---

# Status

`STATE` shows file synchronization. `PROPOSAL` shows the separate pull request
state. `PULL` and `PUSH` show whether direct operations are available. Only
skills registered in the manifest are shown.

```bash
gh skill-linker status
```

Example:

```text
SKILL          PATH                                STATE     PROPOSAL    PULL                              PUSH
shared-rules   .agents/skills/shared-rules         clean     -           eligible                          eligible
review-helper  .agents/skills/review-helper        pull      #18 waiting eligible                          ineligible (remote_changed)
local-notes    .agents/skills/local-notes          push      #24 update  eligible                          ineligible (open_proposal)
idea-refine    .agents/skills/idea-refine          conflict  -           ineligible (unresolved_conflict)  ineligible (unresolved_conflict)
```

- `STATE`: direction of differences between local and source
- `PROPOSAL`: managed pull request number and state
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

| Proposal | Meaning |
| --- | --- |
| `waiting` | Pull request already contains the local tree |
| `update` | Local changes can update the same pull request |
| `source_changed` | Pull and resolve before updating the pull request |
| `obsolete` | Local now matches the source; close the pull request |
| `diverged` | Proposal branch or metadata changed outside the extension |
| `ambiguous` | More than one managed pull request exists |
| `unknown` | Pull request lookup failed; file state remains valid |

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
| `open_proposal` | Merge or close the proposal, or use `push --pr` |
| `proposal_unknown` | Retry after GitHub pull request lookup recovers |
| `fixed_source_ref` | The tag is fixed; reinstall from a different tag to change it |
| `source_ref_read_only` | The tag is fixed and cannot receive a push |
| `tag_moved` | Reinstall with `--accept-moved-tag` only if you trust the moved tag |

`ineligible` means the operation cannot run. `unknown` means the check failed.

`eligible` confirms preconditions only; it does not guarantee GitHub will
accept a push.

JSON output: `gh skill-linker status --json`

See [Resolving conflicts](resolve-conflicts.md) for marker editing.

Implementation: [Status operation](../../3_Functions/pages/operations/status.md)
