---
title: Skill Linker implementation coverage
updated: 2026-07-15
status: implemented
---

# Coverage

This index maps specification IDs to current source, tests, and workflows. It
does not report line coverage.

| ID | Source | Evidence |
| --- | --- | --- |
| CLI-001 | `internal/cli/run.go` | argument tests |
| CLI-002 | `internal/cli/run.go` | exit tests |
| CLI-003 | `cli`, `compat`, `gitcli` | preflight tests |
| CLI-004 | `cli`, `status` | render tests |
| CLI-005 | `internal/cli/run.go` | mutation tests |
| TECH-001 | `go.mod`, adapters | build/CI |
| MAN-001 | `manifest/store.go` | read tests |
| MAN-002 | `manifest/store.go` | round-trip tests |
| MAN-003 | `manifest/store.go` | validation tests |
| MAN-004 | `manifest/store.go` | CAS tests |
| SNAP-001 | `source`, `workspace` | exact tests |
| SNAP-002 | `githubapi`, `workspace`, `skill` | entry tests |
| SNAP-003 | writers | mode inspection |
| PATH-001 | `workspace/path.go` | path tests |
| PATH-002 | `install/service.go` | branch inspection |
| PATH-003 | path/writers | collision inspection |
| TXN-001 | staging implementations | failure inspection |
| INST-001 | `discovery`, `githubapi`, `cli` | matcher/selector/list tests |
| INST-002 | `install`, `workspace` | snapshot install tests |
| INST-003 | `install/service.go` | collision/idempotency tests |
| INST-004 | `install/service.go` | tag re-pin tests |
| INST-005 | `install/service.go` | bulk preflight/partial failure tests |
| STAT-001 | `status`, `syncstate` | state tests |
| STAT-002 | `status/service.go` | reason/frontmatter tests |
| PULL-001 | `pull/service.go` | gate/name tests |
| PULL-002 | `pull/service.go` | no-op tests |
| PULL-003 | `pull`, `workspace` | rollback/conflict path tests |
| PUSH-001 | `push/service.go` | gate/name tests |
| PUSH-002 | `push/service.go` | no-op tests |
| PUSH-003 | `push`, `gitcli` | remote tests |
| MERGE-001 | `merge/threeway.go` | merge tests |
| MERGE-002 | `merge`, `gitcli` | marker/binary/multiple-conflict tests |
| MERGE-003 | `merge/threeway.go` | mode tests |
| MERGE-004 | `syncstate`, services | marker tests |
| GITHUB-001 | `cli`, `command` | redaction tests |
| GITHUB-002 | `githubapi`, `gitcli` | read tests |
| GITHUB-003 | `githubapi`, `gitcli` | permission tests |
| GIT-001 | `gitcli/client.go` | inventory tests |
| DIST-001 | build/release | CI config |
| AUTO-001 | CI/release | workflow config |
| AUTO-002 | live E2E | workflow config |

Every external runtime path and persisted field maps to an ID. Private helpers
share the ID of their owning behavior.
