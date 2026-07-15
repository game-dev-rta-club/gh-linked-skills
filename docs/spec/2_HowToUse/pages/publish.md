---
title: Publishing an unmanaged skill
updated: 2026-07-15
status: implemented
---

# Publish

Publish an unmanaged skill created in the current project to an existing
GitHub repository, then begin managing it.

```bash
gh skill-linker publish OWNER/REPO SKILL --branch BRANCH
gh skill-linker publish OWNER/REPO SKILL --branch BRANCH --pr
```

Requirements:

- the local skill is at `.agents/skills/<name>`
- the repository already exists on GitHub
- you have push permission for the repository
- `BRANCH` is explicit
- the skill is not registered in the manifest

The remote path is `skills/<name>`. For an empty repository, the first commit
creates the requested branch. A non-empty repository must already contain the
branch.

If the remote path does not exist, the command commits the skill and performs a
normal push. If remote content is identical, it registers the skill without a
push. It never overwrites different existing content.

`--pr` proposes a missing remote path without registering the skill yet. Rerun
the same command after merge to register the merged revision. If local work
continued during review, registration keeps those newer local changes; send
them later with `push --pr`.

An unrelated, different skill already at the remote path is rejected. Direct
publish is also rejected while a managed proposal is open.

An already managed skill cannot be published. Use `push` to update the same
source. The command does not migrate or copy skills to another repository, and
it does not create repositories.

After success, commit `.gh-skill-linker.json` in the parent project.

Implementation: [Publish operation](../../3_Functions/pages/operations/publish.md)
