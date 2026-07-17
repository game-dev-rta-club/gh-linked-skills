---
title: Checking synchronization health
updated: 2026-07-17
status: implemented
---

# Status

Status lists Agent Skills visible from project, user, and system scopes.
`PROVIDER` identifies how each skill is supplied. `STATUS` shows synchronization
for managed skills and presence or enablement for other providers.

```bash
gh skill-linker status
```

Example:

```text
SKILL          SCOPE    PROVIDER      SOURCE                   STATUS   PROPOSAL    PULL                              PUSH
shared-rules   project  skill-linker  owner/skills@main        clean    -           eligible                          eligible
review-helper  project  skill-linker  owner/skills@main        pull     #18 waiting eligible                          ineligible (remote_changed)
repo-health    project  gh-skill      owner/community@v1.2.0   present  -           -                                 -
local-notes    project  local         -                        present  -           -                                 -
figma:figma-use user    codex-plugin  figma@marketplace (2.0)  enabled  -           -                                 -
imagegen       system   codex-system  Codex                    present  -           -                                 -
```

- `SCOPE`: `project`, `user`, or `system`
- `PROVIDER`: source classification
- `SOURCE`: linked repository, GitHub metadata, plugin id, or Codex
- `STATUS`: synchronization, presence, or enablement
- `PROPOSAL`: managed pull request number and state
- `PULL` / `PUSH`: whether a managed operation can run; `-` otherwise
- parenthesized value: reason an operation cannot run

Provider values:

| Provider | Evidence |
| --- | --- |
| `skill-linker` | Exact path exists in `.gh-skill-linker.json` |
| `gh-skill` | `gh skill list` reports GitHub source metadata |
| `codex-plugin` | An installed and enabled Codex plugin declares the skill |
| `local` | A project or user skill directory has no external provider metadata |
| `codex-system` | Codex supplies the system skill |

The normal table omits physical paths to stay readable. JSON includes the
display `path` and exact `absolutePath` for every row. Provider conflicts and partial inventory reads produce
warnings. Status does not check non-linker providers for updates.

| Status | Meaning | Action |
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

Proposal lookup is skipped only for confirmed read-only branch repositories.
Those sources remain pullable but show no proposal state.

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

JSON output: `gh skill-linker status --json`. Existing managed fields and
project-relative `path` remain unchanged. `absolutePath`, provider, scope,
source, status, and external rows are additive.

See [Resolving conflicts](resolve-conflicts.md) for marker editing.

Implementation: [Status operation](../../3_Functions/pages/operations/status.md)
