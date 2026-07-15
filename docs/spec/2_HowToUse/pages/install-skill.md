---
title: Installing a managed skill
updated: 2026-07-15
status: implemented
---

# Install a skill

Discover a skill in a repository and register it in the current project. Every
installed skill remains linked to its source repository.

Requirements: [gh-skill-linker installed](install-extension.md), a GitHub
token, and a Git repository.

For GitHub.com, use credentials saved by `gh auth login` or a supported token
environment variable such as `GH_TOKEN`.

Select either a `BRANCH` or a fixed-snapshot `TAG`.

```bash
gh skill-linker install OWNER/REPO --branch BRANCH
gh skill-linker install OWNER/REPO SKILL --branch BRANCH
gh skill-linker install OWNER/REPO PATH --branch BRANCH
gh skill-linker install OWNER/REPO --all --branch BRANCH
gh skill-linker install OWNER/REPO SKILL --tag TAG
```

`OWNER/REPO` and exactly one of `--branch` or `--tag` are required. There is no
local-directory or repository-omitted install path. The command rejects
`./skills`, `../skills`, and `~/skills`. A two-segment value such as
`skills/foo` is interpreted only as GitHub `OWNER/REPO`, never as a local path.
HTTPS URLs, `.git` suffixes, default-branch inference, and commit selection are
not supported.

- `OWNER/REPO`: GitHub repository
- `SKILL`: discovered skill name; `namespace/name` when names collide
- `PATH`: skill directory or its `SKILL.md`, fetched directly
- `--all`: every discovered skill
- `BRANCH`: source used by `pull` and `push`
- `TAG`: fixed snapshot that cannot be pulled or pushed
- destination: `.agents/skills/<name>`

Without a selector, the command lists skill names and paths, then exits.

`PATH` points to a skill directory that follows the
[Agent Skills specification](https://agentskills.io/specification). The
directory must contain `SKILL.md` with `name` and `description` fields.

```bash
gh skill-linker install obra/superpowers skills/brainstorming --branch main
gh skill-linker install obra/superpowers skills/brainstorming/SKILL.md --branch main
```

These commands identify the same source. Files under `scripts/`, `references/`,
`assets/`, and other directories inside the skill are installed unchanged.

Discovery recognizes:

- `skills/<name>/SKILL.md`
- `skills/<namespace>/<name>/SKILL.md`
- `<prefix>/skills/<name>/SKILL.md`
- `<prefix>/skills/<namespace>/<name>/SKILL.md`
- `plugins/<namespace>/skills/<name>/SKILL.md`
- `<name>/SKILL.md`

Directories beginning with `.` and a repository-root `SKILL.md` are excluded
from discovery. An exact path can still select a skill outside these patterns.

`--all` fetches every skill from the same ref commit. If names collide, the
command installs nothing. It validates the complete selection before installing
one skill at a time. If a later skill fails, successful earlier installations
remain and the command reports both successes and failures.

The command does not overwrite an existing destination. Reinstalling the same
source snapshot is a no-op.

The manifest records the repository, path, source ref, and last synchronized
revision. The extension does not commit the parent project, so record the
installation afterward:

```bash
git add -- .agents/skills .gh-skill-linker.json
git commit
gh skill-linker status
```

`clean` confirms that the installation is complete.

Success output:

- `installed <name> at <path>`
- `<name> is already installed at <path>`
- `re-pinned <name> tag: <old> -> <new> (<old-ref-sha> -> <new-ref-sha>)`

Implementation: [Install operation](../../3_Functions/pages/operations/install.md)

See [Installing from a tag](install-by-tag.md) for fixed-release operation and
reinstallation.
