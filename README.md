# Linked Skills

GitHub CLI extension for installing, publishing, checking, pulling, and pushing project-local agent skills without `gh skill`.

## Requirements

- macOS or Linux
- GitHub CLI
- system Git
- a GitHub.com token from `gh auth login` or a supported environment variable

## Install

```bash
gh extension install game-dev-rta-club/gh-linked-skills
gh linked-skills --help
```

## Commands

```bash
gh linked-skills install <owner>/<repository> --branch <branch>
gh linked-skills install <owner>/<repository> <skill-or-path> --branch <branch>
gh linked-skills install <owner>/<repository> --all --branch <branch>
gh linked-skills install <owner>/<repository> <skill-or-path> --tag <tag>
gh linked-skills publish <owner>/<repository> <skill> --branch <branch>
gh linked-skills status [--json]
gh linked-skills pull <skill>
gh linked-skills push <skill>
```

All commands operate on the current Git project. Install requires an explicit GitHub repository and exactly one branch or tag; local or repository-less installation is not supported. Publish sends an unmanaged `.agents/skills/<name>` to an existing repository. Managed skills are installed at `.agents/skills/<name>`.

Branch-backed skills support pull and push. Tag-backed skills are fixed, read-only snapshots. Re-run the same install command with a different tag to re-pin a clean tag-backed skill. Commit the skill and `.gh-linked-skills.json` after installation.

Run `gh linked-skills <command> --help` for command-specific usage and examples.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Start with [the Obsidian documentation](docs/README.md).
