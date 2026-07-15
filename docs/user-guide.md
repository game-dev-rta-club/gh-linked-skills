# User guide

`gh-skill-linker` installs Agent Skills from GitHub into `.agents/skills/` and
records their source in `.gh-skill-linker.json`. Run it from the Git project
that should use the skills.

## 1. Install the extension

Requirements:

- macOS or Linux
- [GitHub CLI](https://cli.github.com/) and system Git
- GitHub authentication from `gh auth login` or a supported token environment
  variable

```sh
gh extension install game-dev-rta-club/gh-skill-linker
gh skill-linker --help
```

Upgrade later with:

```sh
gh extension upgrade skill-linker
```

Users of the extension under its former name must follow the
[rename migration guide](migration-to-skill-linker.md) instead.

## 2. Choose a source

Every install requires a GitHub repository and either a tag or a branch.

| Source | Choose it when | Available synchronization |
| --- | --- | --- |
| Tag | You consume a reviewed release | `status`; no `pull` or `push` |
| Branch | You author or collaborate on the source | `status`, `pull`, and `push` |

There is no implicit default branch, local-directory source, or floating
version range. The source is always explicit.

### Install from a tag

List the skills available at a release:

```sh
gh skill-linker install OWNER/REPO --tag TAG
```

Install one skill by the displayed name or path, or install all discovered
skills:

```sh
gh skill-linker install OWNER/REPO SKILL --tag TAG
gh skill-linker install OWNER/REPO --all --tag TAG
```

To move an existing tag-backed skill to a new tag, name that skill or its exact
source path. The command stops if local changes would be overwritten.

```sh
gh skill-linker install OWNER/REPO SKILL --tag NEW_TAG
```

Tag upgrades are intentionally one skill at a time; `--all` does not re-pin
already installed skills.

### Install from a branch

List the available skills, then install one:

```sh
gh skill-linker install OWNER/REPO --branch BRANCH
gh skill-linker install OWNER/REPO SKILL --branch BRANCH
```

Use `--all` to install every discovered skill from the same branch revision:

```sh
gh skill-linker install OWNER/REPO --all --branch BRANCH
```

## 3. Review and commit the result

An install creates or updates:

```text
.agents/skills/<skill>/...
.gh-skill-linker.json
```

Review both, then commit them to the parent project so collaborators receive
the skill content and its source record together.

```sh
git diff -- .agents/skills .gh-skill-linker.json
git add .agents/skills .gh-skill-linker.json
git commit -m "chore: install agent skills"
gh skill-linker status
```

`STATE` describes the direction of the difference:

| State | Meaning | Typical next step |
| --- | --- | --- |
| `clean` | Local and source content match | None |
| `pull` | The source changed | `pull` |
| `push` | The local skill changed | Review, then `push` |
| `conflict` | Both sides changed or markers remain | Resolve the conflict |

`PROPOSAL` reports a managed pull request independently from file state. `PULL`
and `PUSH` show whether each direct operation is eligible and, when it is not,
the reason.

## Collaborate on a branch

Bring source changes into a managed skill:

```sh
gh skill-linker pull SKILL
```

If both sides changed, the command performs a three-way merge. Text conflicts
are written as Git-style markers for you to resolve; binary or structural
conflicts stop without changing the affected file.

After editing and reviewing a branch-backed skill, send it to the source:

```sh
gh skill-linker status
gh skill-linker push SKILL
```

Push uses a normal Git commit, never a force push. It stops if the remote skill
changed since the last synchronization, so you can pull and review first. The
extension does not commit or push the parent project.

To request review instead of writing directly to the source branch:

```sh
gh skill-linker push SKILL --pr
```

Later local changes update the same open pull request. If the source branch
changes, pull and resolve it first, then rerun `push --pr`. The extension never
rebases or force-pushes the proposal branch. Direct push is disabled until the
managed pull request is merged or closed.

## Publish a new local skill

Create `.agents/skills/<name>/SKILL.md`, then publish it to an existing GitHub
repository where you have push access:

```sh
gh skill-linker publish OWNER/REPO SKILL --branch BRANCH
git add .gh-skill-linker.json
git commit -m "chore: track published skill"
```

The remote path is `skills/<name>`. Existing different content is not
overwritten, and the extension does not create the repository for you.

Use `--pr` to propose the new skill without registering it yet:

```sh
gh skill-linker publish OWNER/REPO SKILL --branch BRANCH --pr
```

After merge, rerun the same command to add the source record. Local changes
made during review remain available for a later `push --pr`.

## Uninstall a skill

```sh
gh skill-linker uninstall SKILL
```

Uninstall removes the local skill and its manifest entry without changing the
source repository. It refuses to delete local changes. Use `--force` only after
you have reviewed and intentionally decided to discard those changes.

## Get help

- Run `gh skill-linker <command> --help` for every option and example.
- Use [GitHub Issues](https://github.com/game-dev-rta-club/gh-skill-linker/issues)
  for reproducible bugs or proposed improvements.
- Read the [security policy](../SECURITY.md) before reporting a vulnerability.
