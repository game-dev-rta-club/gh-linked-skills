---
name: gh-skill-linker
description: Use when installing, publishing, inspecting, synchronizing, proposing, or removing project-local Agent Skills with the gh-skill-linker GitHub CLI extension; trigger on gh skill-linker, .gh-skill-linker.json, Agent Skill provenance, tag or branch installs, skill pull or push, and skill PR workflows.
---

# gh-skill-linker

Manage Agent Skills as reviewable project files connected to explicit GitHub
sources.

## Workflow

1. Confirm the target Git project and inspect its working tree.
2. Run `gh auth status` before the first remote operation.
3. Run `gh skill-linker status` when managed skills may already exist.
4. Choose the source mode:
   - Use `--tag TAG` to consume a fixed, read-only release.
   - Use `--branch BRANCH` to collaborate with a writable source.
5. Run the narrowest command that completes the request.
6. Review the installed files and the matching `.gh-skill-linker.json` entry
   before committing the parent project.
7. Run `gh skill-linker status` again and report the resulting state.

## Commands

Discover a repository's skills without installing them:

```sh
gh skill-linker install OWNER/REPO --tag TAG
gh skill-linker install OWNER/REPO --branch BRANCH
```

Install a fixed release or a branch-backed skill:

```sh
gh skill-linker install OWNER/REPO SKILL --tag TAG
gh skill-linker install OWNER/REPO SKILL --branch BRANCH
```

Synchronize a branch-backed skill:

```sh
gh skill-linker status
gh skill-linker pull SKILL
gh skill-linker push SKILL
```

Use `gh skill-linker push SKILL --pr` when the source repository expects review
through a pull request. Before direct push, publishing, or pull-request
creation, confirm that the user's request authorizes that remote write and that
the target repository is exact. Use direct push only when its policy permits
it.

Publish a new local skill from `.agents/skills/<name>`:

```sh
gh skill-linker publish OWNER/REPO SKILL --branch BRANCH
gh skill-linker publish OWNER/REPO SKILL --branch BRANCH --pr
```

Remove a managed local copy:

```sh
gh skill-linker uninstall SKILL
```

## Safety

- Treat every skill as trusted instructions: inspect it before installation.
- Preserve local work. If status reports `conflict`, review and resolve the
  markers instead of replacing either side.
- Pull before retrying when push reports that the source changed.
- Keep tag-backed installs fixed. Move to another tag with `install`; do not
  turn a release snapshot into a writable source.
- Use `uninstall --force` only after the user has explicitly chosen to discard
  reviewed local changes.
- Remember that Skill Linker does not commit or push the parent project.

## Review the project change

Read every installed skill file. In `.gh-skill-linker.json`, the skill name is
the entry key; verify its `repository`, `sourcePath`, `sourceRef`, `commitSHA`,
and `destination`. Use the recorded `sourceRef` for the selected
`refs/tags/<tag>` or `refs/heads/<branch>` instead of inventing separate tag or
branch fields.

New files do not appear in an unstaged `git diff`. Inspect them directly, use
`git status --short`, then review the exact staged change with
`git diff --cached` before committing.

## Result

Report:

- the skill and destination;
- the GitHub source and selected tag or branch;
- the final `status` state;
- files the user should review or commit;
- any conflict, permission, or proposal state that still needs action.
