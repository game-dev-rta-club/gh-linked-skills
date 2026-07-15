---
title: Push reference
updated: 2026-07-15
status: implemented
---

# Push

Reject a tag source with `source_ref_read_only` before local validation,
permission checks, or remote operations.

Return the local snapshot to the same source branch as a normal commit. With
`--pr`, send it through [the proposal engine](proposals.md) instead.

## PUSH-001 Eligibility enforcement

Recheck:

- selector is unique
- path is inside the project and contains no symlink
- local files are regular files
- no conflict markers remain
- `SKILL.md` is valid and its name matches the managed name
- files are tracked or untracked but not ignored
- repository push permission exists
- current source skill tree equals the baseline
- direct mode has no open managed proposal

Tracked and untracked non-ignored files can be pushed. A tracked file remains
eligible even when it matches an ignore rule. A new file does not need to be
added to the Git index first.

The permission check is a repository-level preflight. A branch-protection or
ruleset rejection appears as a PUSH-003 error from the actual push.

## PUSH-002 No-op

When local equals current, do not commit. When the tree is identical but only
the commit SHA advanced, update the manifest.

## PUSH-003 Remote mutation

Clone with `--branch --single-branch --no-tags --depth 1`. Recheck that
`HEAD:<sourcePath>` equals the expected tree SHA, then replace only the selected
subtree.

Write files with `0644/0755` and run `git add -A -- <sourcePath>`. Do not commit
when there is no difference.

- author: `gh-skill-linker <gh-skill-linker@users.noreply.github.com>`
- message: `chore(skill): sync <skill-name>`
- refspec: `HEAD:refs/heads/<branch>`

Never force. Non-fast-forward means `remote changed`. If the manifest fails
after a push, do not roll back the remote; require a pull to reconcile.

## PUSH-004 Pull request mode

Do not advance the manifest baseline. Create or update the proposal using the
current source tree as base and the exact local Git tree as proposed content.
If the source tree differs from the manifest baseline, reject before proposal
mutation and require `pull`.
