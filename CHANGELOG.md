# Changelog

Notable user-facing changes are recorded in this file.

## Unreleased

- Added a companion Agent Skill that teaches agents the safe, project-local
  `gh-skill-linker` workflow.
- Reworked the README around visible skill files, explicit provenance, and the
  improvement loop between a project and its GitHub source.

## 0.6.0 - 2026-07-15

- Renamed the extension and command from `gh-linked-skills` / `gh linked-skills`
  to `gh-skill-linker` / `gh skill-linker`.
- Renamed the project management file from `.gh-linked-skills.json` to
  `.gh-skill-linker.json` without changing its schema.
- Updated release artifacts, conflict markers, documentation, and GitHub
  integration metadata to use the Skill Linker identity.
- Added a [migration guide](docs/migration-to-skill-linker.md) for reinstalling
  the extension and renaming existing project manifests.

## 0.5.3 - 2026-07-15

- Assemble all assets and attestations in a draft before publishing the immutable release.

## 0.5.2 - 2026-07-15

- Published release assets and the associated tag with GitHub release immutability enabled.

## 0.5.1 - 2026-07-15

- Added signed GitHub build provenance attestations and checksums for release assets.

## 0.5.0 - 2026-07-15

- Added install, publish, status, pull, push, and uninstall commands.
- Added branch-backed synchronization and fixed tag-backed snapshots.
- Added conflict detection, remote-change checks, and rollback-oriented writes.
- Added prebuilt binaries for Linux and macOS on AMD64 and ARM64.
