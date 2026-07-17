# Skill Linker — Link Agent Skills to versioned GitHub sources

[![CI](https://github.com/game-dev-rta-club/gh-skill-linker/actions/workflows/ci.yml/badge.svg)](https://github.com/game-dev-rta-club/gh-skill-linker/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/game-dev-rta-club/gh-skill-linker)](https://github.com/game-dev-rta-club/gh-skill-linker/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Link project-local Agent Skills to explicit GitHub sources without hiding their
files or silently overwriting local work.

`gh-skill-linker` is a GitHub CLI extension, not a skill collection. It copies
skill files into the project they affect and records each source repository,
path, branch or tag, and last synchronized revision. Collaborators can review
the files and reproduce the same setup without relying on hidden symlinks.

## Quick start

Requirements: macOS or Linux, an authenticated
[GitHub CLI](https://cli.github.com/), system Git, and a Git project.

```sh
gh extension install game-dev-rta-club/gh-skill-linker
gh skill-linker install game-dev-rta-club/rubber-ducking-skill --all --tag v2.0.0
gh skill-linker status
```

The install prints the destination of each skill. A successful status check
looks like this:

```text
SKILL                 SCOPE    PROVIDER      SOURCE                               STATUS  PROPOSAL  PULL                           PUSH
rubber-duck-caller    project  skill-linker  game-dev-rta-club/rubber-ducking-skill@v2.0.0  clean   -         ineligible (fixed_source_ref)  ineligible (source_ref_read_only)
rubber-duck-partner   project  skill-linker  game-dev-rta-club/rubber-ducking-skill@v2.0.0  clean   -         ineligible (fixed_source_ref)  ineligible (source_ref_read_only)
```

This is expected for a tag-backed install: the files are clean, while pull and
push are disabled to preserve the fixed snapshot.

Status also lists local project skills, skills with `gh skill` metadata,
enabled Codex plugin skills, and Codex system skills. It does not check those
external providers for updates.

Commit the installed files and their source record together:

```sh
git add .agents/skills .gh-skill-linker.json
git commit -m "chore: install agent skills"
```

```text
your-project/
├── .agents/skills/<skill>/...   # files the agent reads
└── .gh-skill-linker.json       # source ref and synchronized revision
```

The extension does not commit the parent project for you.

## Choose a tag or branch

| Source | Best for | Behavior |
| --- | --- | --- |
| `--tag <tag>` | Teams consuming a reviewed release | Fixed, read-only snapshot. `pull` and `push` are disabled. |
| `--branch <branch>` | Skill authors and collaborators | Tracks a writable branch. Supports `status`, `pull`, and `push`. |

Use a tag when you only need to consume a skill. Use a branch when you intend
to exchange changes with the source repository.

```sh
# Install every discovered skill at a fixed release.
gh skill-linker install OWNER/REPO --all --tag TAG

# List available skills on a branch, then install one by name or path.
gh skill-linker install OWNER/REPO --branch BRANCH
gh skill-linker install OWNER/REPO SKILL --branch BRANCH
```

## What it protects

- Local changes are not silently discarded.
- A push stops if the source skill changed since the last synchronization.
- Text conflicts are left as Git-style conflict markers for manual resolution.
- Tag-backed skills stay fixed and cannot push to their source.
- The manifest records where every managed skill came from.

Review any Agent Skill before installing it. Skills are instructions to an
agent and should be treated as trusted code.

## Common workflows

| Goal | Command |
| --- | --- |
| Discover skills | `gh skill-linker install OWNER/REPO --branch BRANCH` |
| Install one skill | `gh skill-linker install OWNER/REPO SKILL --branch BRANCH` |
| Install a fixed release | `gh skill-linker install OWNER/REPO SKILL --tag TAG` |
| Check local and source changes | `gh skill-linker status` |
| Bring source changes into the project | `gh skill-linker pull SKILL` |
| Send a local skill change to its source branch | `gh skill-linker push SKILL` |
| Propose a local skill change for review | `gh skill-linker push SKILL --pr` |
| Publish a new local skill | `gh skill-linker publish OWNER/REPO SKILL --branch BRANCH` |
| Propose a new skill for review | `gh skill-linker publish OWNER/REPO SKILL --branch BRANCH --pr` |
| Stop managing a skill | `gh skill-linker uninstall SKILL` |

Run `gh skill-linker <command> --help` for complete arguments and examples.
For installation, branch collaboration, tag upgrades, conflicts, and removal,
see the [user guide](docs/user-guide.md).

## Security and release integrity

Use v0.6.0 or later. Skill Linker releases are immutable and include SHA-256
checksums and signed GitHub build provenance. Releases before v0.6.0 use the
former extension name and remain available only as historical records.

See [Verifying release artifacts](docs/release-verification.md) for the exact
checksum and attestation commands. Read the
[safety model](docs/spec/3_Functions/pages/architecture/safety-model.md) before
automating write operations. Report vulnerabilities privately as described in
[SECURITY.md](SECURITY.md).

## Requirements and upgrades

- macOS or Linux; Windows is not currently supported
- [GitHub CLI](https://cli.github.com/)
- system Git
- `gh auth login` or a supported token environment variable for remote
  operations

Upgrade an existing installation with:

```sh
gh extension upgrade skill-linker
```

If you installed this extension before v0.6.0, follow the
[rename migration guide](docs/migration-to-skill-linker.md) instead of
upgrading it in place.

## Documentation

Start at the [documentation index](docs/README.md) for links organized by goal.
The detailed design specifications are retained separately for contributors.

## Project status

This project is pre-1.0 and maintained by volunteers. Response times, releases,
fixes, and long-term maintenance are not guaranteed. There is no support SLA.

Use [GitHub Issues](https://github.com/game-dev-rta-club/gh-skill-linker/issues)
for reproducible bugs and proposed improvements. General contact is available
through the [Game Dev RTA Club Google Group](https://groups.google.com/g/game-dev-rta-club).

## Development

Requirements: Go 1.26.5 or later, GitHub CLI, and system Git.

```sh
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

## Contributing

Contributions and forks are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for
the development and pull-request process.

## License

[MIT](LICENSE) © 2026 Game Dev RTA Club.
