---
title: Pull request proposal reference
updated: 2026-07-15
status: implemented
---

# Pull request proposals

`push --pr` and `publish --pr` use one proposal engine.

## PR-001 Identity

Allow one open proposal per repository, base branch, and source path. Use a
generated `skill-linker/<skill>-<path-hash>/...` branch in the same repository.
Recognize it only when the branch namespace and strict hidden PR metadata agree.
GitHub owns open, closed, and merged state. The manifest stores no PR state.

## PR-002 Timeline

- no proposal: create a branch and pull request
- local changed: commit to the same proposal branch
- unchanged: return `waiting`
- base changed: require caller reconciliation, then merge current base into the
  same proposal branch and commit the resolved local skill
- base contains the proposed tree: treat it as applied
- closed unmerged proposal: allocate a new generated branch

Never force-push, rebase, auto-merge, delete branches, or use forks.

## PR-003 Recovery

Branch names include base and proposed tree identities. A rerun can recover a
branch created before PR creation failed. If branch push succeeds before PR
metadata update fails, verify the remote skill tree and repair metadata on the
next rerun.

Reject multiple active proposals, malformed metadata, external head changes,
and unrelated merge conflicts.
