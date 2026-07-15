---
title: Manifest specification
updated: 2026-07-15
status: implemented
---

# Manifest

Persist source identity and the last synchronization point at the project root.

## MAN-001 Location

`.gh-skill-linker.json` at the project root is the only registry. When absent,
it represents an empty schema-v2 document. It must be a regular file. Reject
symlinks, unknown fields, multiple JSON values, trailing data, and invalid
schemas.

The manifest stores no pull request or proposal state.

## MAN-002 Schema version 2

```json
{
  "schemaVersion": 2,
  "skills": {
    "example": {
      "repository": "https://github.com/owner/repository.git",
      "sourcePath": "skills/example",
      "sourceRef": "refs/tags/v1.2.0",
      "refSHA": "<tag-object-or-commit-sha>",
      "commitSHA": "<peeled-commit-sha>",
      "treeSHA": "<skill-tree-sha>",
      "destination": ".agents/skills/example"
    }
  }
}
```

Schema v1 is converted to v2 in memory when read. Writes emit only v2.

## MAN-003 Validation

- name: lowercase kebab-case, 1 to 64 characters
- repository: `https://github.com/<owner>/<repo>[.git]`
- path: relative canonical POSIX path
- sourceRef: `refs/heads/` or `refs/tags/` plus a valid ref name
- SHA: 40 or 64 lowercase hexadecimal characters
- destination: `.agents/skills/<key>`

## MAN-004 Write and optimistic comparison

Write two-space JSON plus a newline to a temporary file. Disable HTML escaping,
set mode `0644`, fsync the file, and rename it within the same directory. The
directory is not fsynced.

Update a baseline or delete during uninstall only when the starting entry still
equals the current entry. Before publish adds an entry, compare the starting
document with a reread document. Preserve other entries. A mismatch returns
`management file changed during operation`.

Without a process lock, atomicity is not guaranteed across the comparison and
rename between processes.

Reference model: [Source references](../architecture/source-reference.md)
