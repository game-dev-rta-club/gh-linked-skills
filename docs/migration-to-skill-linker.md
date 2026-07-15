# Migrate to Skill Linker

Version 0.6.0 renames the GitHub CLI extension from `gh-linked-skills` to
`gh-skill-linker`. The command changes from `gh linked-skills` to
`gh skill-linker`, and the project management file changes from
`.gh-linked-skills.json` to `.gh-skill-linker.json`.

The manifest schema and installed files under `.agents/skills/` do not change.
The rename is not automatic because GitHub CLI derives an extension command
from its repository name.

## Before migrating

Finish or commit any Skill Linker operation already in progress. From each
managed project, check the old extension state and the Git working tree:

```sh
gh linked-skills status
git status --short
```

Resolve conflicts and preserve any local skill changes before continuing.

## Replace the extension

Remove the former extension and install Skill Linker:

```sh
gh extension remove linked-skills
gh extension install game-dev-rta-club/gh-skill-linker
```

## Rename the management file

Rename the tracked manifest before running another Skill Linker operation:

```sh
git mv -- .gh-linked-skills.json .gh-skill-linker.json
gh skill-linker status
```

If the old manifest was not tracked, use `mv` instead of `git mv`, then add the
new file normally. If both manifest names already exist, stop and reconcile
them manually; do not let one overwrite the other.

Commit the manifest rename with any related project documentation:

```sh
git add -- .gh-skill-linker.json
git commit -m "chore: migrate to gh-skill-linker"
```

All later commands use `gh skill-linker`. Existing skill directories and their
recorded GitHub source information remain unchanged.
