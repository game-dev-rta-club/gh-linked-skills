---
title: GitHub integration specification
updated: 2026-07-15
status: implemented
---

# GitHub

Source reads, publication, pull requests, and push-permission checks are limited
to GitHub.com. The extension does not create repositories.

## GITHUB-001 Authentication and host

The host is fixed to `github.com`. The extension reads a token through `go-gh`,
including credentials saved by `gh auth login` and compatible environment
variables such as `GH_TOKEN`. API requests include `Cache-Control: no-cache`.

Git push receives the token through a Basic-auth extra header. Tracing is
disabled, and token or Authorization values become `[REDACTED]`. Credentials
are never written to Git configuration.

User environment and global/system Git configuration are inherited. There is
no dedicated home directory, configuration isolation, timeout, or retry.

## GITHUB-002 Ref, discovery, tree, and blob reads

Install, pull, and push resolve source refs through the Git refs API. Annotated
tags are peeled to commits through the Git tags API. Tag object SHA and commit
SHA remain separate, and a ref that does not resolve to a commit is rejected.

Subtrees are read through Trees API `recursive=1` and the Blobs API.

Discovery resolves the source ref to a commit once and reads the repository
tree once. It returns candidate paths, namespaces, and skill tree SHAs. `--all`
and name selection reuse that result.

The extension rejects a missing path or `SKILL.md`, invalid frontmatter,
truncated trees, and unsupported modes, types, or encodings.

Install, pull, and push read snapshots through tree and blob requests. Status
batches source refs and repository permissions through GraphQL, with at most 32
refs per request. Larger inputs are divided across requests. Owner, repository,
and ref values are GraphQL variables.

Status reads trees only for repositories whose source commit SHA differs from
the baseline. It reads and validates a snapshot only when a skill tree SHA
changed.

If part of a GraphQL response fails, only the ref or permission associated with
the error path becomes unknown. An HTTP, decoding, or other whole-request
failure marks every item in that request unknown.

Tree responses are shared only within one status process. A truncated discovery
tree rejects listing, name selection, and `--all`. Status also treats a
truncated tree as unknown. Exact-path install does not use full-repository
discovery.

There is no persistent cache, tree size override, timeout, or retry.

GitHub API limits still apply. Recursive trees are limited to 100,000 entries
or 7 MB, and `truncated=true` is rejected. Each blob is limited to 100 MB. See
the [Trees API](https://docs.github.com/en/rest/git/trees) and
[Blobs API](https://docs.github.com/en/rest/git/blobs).

## GITHUB-003 Push permission

Status reads GraphQL `viewerPermission` once per repository. `ADMIN`,
`MAINTAIN`, and `WRITE` are writable; `READ` and `TRIAGE` are read-only; a read
failure produces unknown permission. Status does not clone.

Before a push, a false API result is rechecked through a shallow clone and
`git push --dry-run`.

A known denial is read-only. A network, clone, or unknown failure produces
unknown permission.

This check cannot fully represent branch protection and rulesets. GitHub may
still reject the final push.

## GITHUB-004 Pull requests

Use REST pull request endpoints to list, create, and update PR bodies. Listing
paginates at 100 items. Status lists open PRs once per repository. Proposal
recovery may list all PRs for one exact generated head branch.

Only same-repository PRs with a generated branch and valid hidden metadata are
managed. The extension does not merge, close, approve, or delete them.
