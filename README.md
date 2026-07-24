# gh-skill-linker

[![CI](https://github.com/game-dev-rta-club/gh-skill-linker/actions/workflows/ci.yml/badge.svg)](https://github.com/game-dev-rta-club/gh-skill-linker/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/game-dev-rta-club/gh-skill-linker)](https://github.com/game-dev-rta-club/gh-skill-linker/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Use Agent Skills locally. Return improvements to their GitHub source.**

`gh-skill-linker` is a GitHub CLI extension that copies an Agent Skill—a folder
of instructions and supporting files—from GitHub into `.agents/skills/` in
your project. The files stay visible and reviewable alongside the work.

`.gh-skill-linker.json` remembers where the copy came from. Install from a
branch to improve the skill locally and return changes directly or through a
pull request so another project can reuse them.

![A local project sends an improved SKILL file to its remote Git source and receives the shared version back.](assets/skill-linker-hero.png)

## Quick start

Requirements:

- macOS or Linux
- an authenticated [GitHub CLI](https://cli.github.com/) and system Git
- a Git project where the skill should be installed
- an agent that reads project skills from `.agents/skills/`

> Review an Agent Skill at its source before installing it. Skills are
> instructions to an agent and should be treated as trusted code.

Choose the source mode before installing:

| Source | Choose it when | Behavior |
| --- | --- | --- |
| `--tag TAG` | Consuming a reviewed release | Fixed snapshot; pull and push are disabled |
| `--branch BRANCH` | Authoring or collaborating | Tracks the branch; supports pull, push, and pull requests |

Install the extension:

```sh
gh extension install game-dev-rta-club/gh-skill-linker
```

This example uses this repository's
[companion Agent Skill](skills/gh-skill-linker/), which teaches agents the same
CLI workflow. Review it, then install it from `main` so changes can be returned.
The first argument names the repository; the second selects the skill.

```sh
gh skill-linker install game-dev-rta-club/gh-skill-linker gh-skill-linker --branch main
gh skill-linker status
```

```text
your-project/
├── .agents/skills/gh-skill-linker/   # reviewable instructions the agent reads
└── .gh-skill-linker.json             # GitHub source and synchronized revision
```

Review and commit both together so collaborators receive the same instructions
and provenance:

```sh
git add .agents/skills .gh-skill-linker.json
git diff --cached
git commit -m "chore: install project agent skill"
```

The extension does not commit the parent project for you.

## How synchronization works

For every managed skill, Skill Linker compares three versions:

- the complete copy inside the current project
- the revision recorded at the last successful synchronization
- the current skill at the selected GitHub tag or branch

`status` summarizes that comparison:

| Status | Meaning | Typical next step |
| --- | --- | --- |
| `clean` | Local and source content match | None |
| `pull` | Only the source changed | `pull` |
| `push` | Only the local skill changed | Review, then `push` or `push --pr` |
| `conflict` | Both sides changed or conflict markers remain | Review and resolve |

`pull` brings source changes into the project and uses the recorded revision
for a three-way merge. Returning changes requires write permission to the
source repository:

```sh
# After editing the installed companion skill:
gh skill-linker status
git diff -- .agents/skills/gh-skill-linker
gh skill-linker push gh-skill-linker --pr

# After the pull request is merged:
gh skill-linker pull gh-skill-linker
git add .agents/skills/gh-skill-linker .gh-skill-linker.json
```

Direct `push` creates a normal commit on the tracked source branch.
`push --pr` creates or updates a generated branch and pull request in that same
repository. Skill Linker does not create forks, so a source without write
permission is pull-only. If the source moves first, pushing stops so you can
pull and review the changes. After a pull request is merged, `pull` records the
new synchronization point; when the source already matches the local copy, it
updates only that baseline.

If a pull cannot merge text automatically, it leaves Git-style markers in the
skill files. Resolve them and run `status` again. A resolved result reports
`clean` when it matches the source or `push` when it still needs to be returned;
you do not need to pull again.

## Common workflows

| Goal | Command |
| --- | --- |
| Discover skills | `gh skill-linker install OWNER/REPO --branch BRANCH` |
| Install every discovered skill | `gh skill-linker install OWNER/REPO --all --tag TAG` |
| Check local and source state | `gh skill-linker status` |
| Bring in source changes | `gh skill-linker pull SKILL` |
| Return a local change | `gh skill-linker push SKILL` |
| Propose a local change | `gh skill-linker push SKILL --pr` |
| Publish a new local skill | `gh skill-linker publish OWNER/REPO SKILL --branch BRANCH` |
| Remove a managed local copy | `gh skill-linker uninstall SKILL` |

Run `install OWNER/REPO` with a tag or branch but without `SKILL` to discover
valid skills. Add a displayed name or path to install one, or use `--all`.
Run `gh skill-linker <command> --help` for complete arguments and examples.

## Scope and safety

- Text conflicts remain visible as Git-style markers for manual resolution.
- The extension never force-pushes or rebases proposal branches.
- It manages multiple skills inside the current Git project. It is not a global
  skill manager, package registry, or background service.
- This repository's companion skill and every linked source-skill repository
  are independent projects. Linking does not imply ownership, affiliation, or
  a runtime dependency.

## Documentation

- [User guide](docs/user-guide.md): installation, collaboration, conflicts,
  publishing, and removal
- [Command and behavior documentation](docs/spec/2_HowToUse/index.md)
- [Safety model](docs/spec/3_Functions/pages/architecture/safety-model.md)
- [Verifying release artifacts](docs/release-verification.md): SHA-256
  checksums and signed GitHub build provenance
- [Documentation index](docs/README.md): design specifications and migration
  notes

Report vulnerabilities privately through [SECURITY.md](SECURITY.md).

## Community

Bugs and ideas are welcome in
[GitHub Issues](https://github.com/game-dev-rta-club/gh-skill-linker/issues).
See [CONTRIBUTING.md](CONTRIBUTING.md) before contributing. General contact is
available through the
[Game Dev RTA Club Google Group](https://groups.google.com/g/game-dev-rta-club).

This is a pre-1.0, volunteer-maintained project. Response times, releases,
fixes, and long-term maintenance are not guaranteed.

## License

[MIT](LICENSE) © 2026 Game Dev RTA Club.
