---
title: Skill Linker functions
updated: 2026-07-15
status: archived
---

# Functions

## Install

```bash
gh skill-linker install OWNER/REPO --path PATH --branch BRANCH
```

Preserve relative paths, content bytes, and `100644` / `100755` modes from the
source subtree when placing it at `.agents/skills/<name>`. Require a
Codex-compatible `name` and `description` in `SKILL.md`. Reject symlinks,
submodules, special modes, and Git LFS pointers.

Install only when the destination is absent. A rerun is a no-op only when
source, baseline, destination, and on-disk snapshot all match. Never overwrite
other content. The command does not run `git add` or commit.

## Status

```bash
gh skill-linker status [--json]
```

Inspect only skills in `.gh-skill-linker.json`. Compare relative paths, raw
bytes, and executable bits without YAML semantic comparison or normalization.

| State | Meaning |
| --- | --- |
| `clean` | local and remote match the baseline |
| `pull` | only remote changed |
| `push` | only local changed |
| `conflict` | both changed or markers remain |

When the remote subtree SHA differs from the baseline, report `pull` even if
content is identical. A repository without push permission remains pullable but
not pushable, and status warns about local changes.

## Pull

```bash
gh skill-linker pull <name|project-relative-path>
```

Run only when every skill file is tracked by the parent project's Git. If local
is clean, atomically replace it with the remote snapshot. When both sides
changed, use system Git for a three-way merge and write text conflicts to files.
Binary, modify/delete, and file/directory conflicts stop without changes.

After apply, advance manifest commit and subtree SHAs. If the manifest update
fails, roll back to the original directory. When remote and local bytes already
match, leave content untouched and advance only the baseline.

## Push

```bash
gh skill-linker push <name|project-relative-path>
```

Push only when:

- the manifest manages the skill
- no generated conflict marker remains
- local `SKILL.md` is Codex-compatible
- local files are not excluded by `.gitignore`
- the source branch is writable
- the remote subtree SHA matches the baseline

Use a temporary shallow clone to replace only the selected subtree, then create
a normal commit and normal push. Reject a remote race as non-fast-forward.
Advance the manifest after success. If only the post-push manifest update fails,
do not push again; use `pull` to restore the baseline.

## Workflow skill

```bash
gh skill-linker skills install --agent <codex|claude-code> --scope project
```

Place the embedded bundle at `.agents/skills/gh-skill-linker` for Codex or
`.claude/skills/gh-skill-linker` for Claude Code. Store version and SHA-256 in
the manifest. A rerun updates only a managed bundle with no local changes.
Never overwrite an unknown destination or local changes.

## Related

- [Overview](gh-skill-linker.md)
- [Manual conflict resolution](gh-skill-linker-conflict-resolution.md)
- [Implementation](gh-skill-linker-implementation.md)
