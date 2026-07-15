---
title: Skill Linker implementation plan
updated: 2026-07-15
status: archived
---

# Implementation plan

## Components

| Component | Responsibility |
| --- | --- |
| CLI | arguments, exit codes, presentation |
| manifest | schema validation, atomic JSON write, baseline update |
| GitHub API | read private repository refs, trees, blobs, and permissions |
| workspace | read raw snapshots, stage, and replace with rollback |
| system Git | project inventory, diff3 merge, temporary clone, commit, push |

Production code does not execute a `gh skill` subprocess. Authentication uses
`go-gh` and `gh auth`. Push credentials use a temporary Git extra header and
tokens never appear in logs.

## Manifest

Use `.gh-skill-linker.json` at the project root as the only management record.
Each managed skill stores repository, source path, branch, destination, and the
last synchronized commit and subtree SHAs. A workflow skill stores agent,
destination, bundle version, and SHA-256.

Reject unknown JSON fields and invalid paths, branches, or SHAs. Sync a
temporary file before atomic rename. Skill content and manifest are committed
in the parent project, but the extension never stages or commits implicitly.

## Mutation safety

Install and pull stage the target directory on the same filesystem. Pull moves
the current directory to a rollback location, confirms it still matches the
expected snapshot, then activates the new directory. Restore the original when
the manifest write fails.

Push cannot roll back because remote mutation happens first. Return a dedicated
error for manifest update failure and recommend `pull` for reconciliation
instead of another push. Reject every path that escapes the project or skill
root, and never follow symlinks.

## Tests

- pure tests: state, manifest validation, raw byte/mode comparison
- filesystem tests: install, atomic replacement, rollback, workflow bundle
  ownership
- local Git tests: inventory, diff3, temporary bare-repository push, race
  rejection
- HTTP tests: GitHub tree/blob/ref/permission adapter
- private E2E: install, status, pull, push, conflict markers, manual resolution

Default tests use no network, user credentials, or existing repositories. Run
`go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`
after every change.

## Related

- [Overview](gh-skill-linker.md)
- [Functions](gh-skill-linker-functions.md)
- [Distribution and support](gh-skill-linker-distribution.md)
