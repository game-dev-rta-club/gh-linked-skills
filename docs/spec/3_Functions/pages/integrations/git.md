---
title: Git integration specification
updated: 2026-07-15
status: implemented
---

# Git

The extension uses system Git to inspect the parent project and to perform
temporary operations on source repositories.

## GIT-001 Project inventory

The root comes from `git rev-parse --show-toplevel`. Running outside a worktree
fails.

- pull: `git ls-files --cached -- .agents/skills`
- push: `git ls-files --cached --others --exclude-standard -- .agents/skills`

Status runs each command once and reuses the results for every managed skill.

Tracked files are eligible for push regardless of ignore rules. Untracked,
non-ignored files are also eligible. Only files that are both untracked and
ignored prevent a push.

The extension does not inspect the parent project's clean, staged, or manifest
state. It does not add, commit, or stash in the parent project.

Other Git operations:

- low-level install/pull/push ref fallback: `git ls-remote --exit-code`
- proposal branch recovery: `git ls-remote` with an exact full ref
- merge: `git merge-file --diff3`
- push permission: only when the repository API reports no push permission,
  use a shallow clone and `git push --dry-run`
- push: shallow clone, scoped add, and normal push
- publish: inspect refs; shallow-clone an existing branch or initialize the
  requested branch in an empty repository, then scoped add and normal push
- proposal: clone base or head, replace only the skill subtree, and
  normal-push a generated branch; when base advanced, create a normal merge
  commit after rejecting conflicts outside that subtree

Status checks refs and permissions only through the GitHub GraphQL API and does
not clone.

Immediately before push or publish, the extension also validates the branch
with [`git check-ref-format --branch`](https://git-scm.com/docs/git-check-ref-format).
