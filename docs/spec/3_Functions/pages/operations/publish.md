---
title: Publish reference
updated: 2026-07-15
status: implemented
---

# Publish

Add an unmanaged local skill to an existing repository, then record the
synchronization point in the manifest after remote success.

## PUB-001 Local preflight

`OWNER/REPO`, a local selector, and a branch are required. The selector is a
name or `.agents/skills/<name>`.

Reject:

- a skill already registered in the manifest
- a path outside the project, symlink, or non-regular file
- a name/directory mismatch, invalid `SKILL.md`, marker, or LFS pointer
- an ignored local file
- a read-only repository

The remote path is `skills/<name>`. Repository creation, arbitrary path
selection, tags, relinking, migration, and copying are not supported.

## PUB-002 Remote mutation

- branch exists, path absent: add the subtree through a normal push
- branch exists, tree SHA matches: adopt the existing subtree without a commit
- branch exists, path differs: reject
- repository has no refs: create the requested branch through the first normal
  push
- repository has refs, branch absent: reject

Compare bytes, relative paths, and executable bits as a Git tree. An ancestor
that is a file or symlink is also a mismatch.

If the normal push after cloning is non-fast-forward, reject it as remote
changed. Do not modify unrelated content.

## PUB-003 Manifest registration

After remote success, add the entry only when the manifest still matches the
version read at the start. Record repository, source path/ref, commit/tree SHA,
and destination in one write. There is no process lock.

Do not roll back the remote after a manifest failure. A rerun can adopt the
identical remote subtree and retry only manifest registration.

## PUB-004 Pull request mode

For a missing path, create or update a proposal and leave the manifest
unchanged. After merge, rerun publish and register the current source revision.
If local content advanced, keep it unchanged while registering the merged
baseline. Reject different existing content unless a matching merged managed
proposal proves ownership of the path.

Common branch and recovery rules: [Pull request proposals](proposals.md).
