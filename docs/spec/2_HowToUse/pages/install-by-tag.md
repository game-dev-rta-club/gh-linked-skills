---
title: Installing from a tag
updated: 2026-07-15
status: implemented
---

# Install from a tag

Use a tag to install a skill as a fixed release snapshot.

```bash
gh skill-linker install OWNER/REPO --tag TAG
gh skill-linker install OWNER/REPO SKILL --tag TAG
gh skill-linker install OWNER/REPO PATH --tag TAG
gh skill-linker install OWNER/REPO --all --tag TAG
```

Exactly one of `--branch` and `--tag` is required. Tag names must match
exactly.

```bash
gh skill-linker install addyosmani/agent-skills \
  skills/code-review-and-quality \
  --tag 0.6.3
```

The extension treats a tag as a fixed snapshot.

| Operation | Tag-backed skill |
| --- | --- |
| install | Place the content from the requested tag |
| status | Check local differences and tag identity |
| pull | Rejected with `fixed_source_ref` |
| push | Rejected with `source_ref_read_only` |

You can edit a local tag-backed skill, but you cannot push it. `status` warns
about local changes.

## Change the tag

Reinstall the same repository and path with a different tag.

```bash
gh skill-linker install OWNER/REPO PATH --tag NEW_TAG
```

The tag changes only when all of these conditions hold:

- the existing source is also a tag
- repository, path, and destination are unchanged
- local content matches the recorded baseline
- one skill is selected explicitly; `--all` cannot change a tag

The command does not merge or discard local changes. Reinstalling the same tag
and ref SHA is a no-op. Switching between a branch and a tag is rejected.

If the tag name is unchanged but its ref SHA moved, the command rejects it as
`tag_moved`. Accept the moved tag only through an explicit override:

```bash
gh skill-linker install OWNER/REPO PATH \
  --tag TAG \
  --accept-moved-tag
```

On success, the command prints the old and new tags and ref SHAs. A deleted or
unresolvable tag produces `source_unavailable`.

The command does not support `latest`, semantic-version ranges, commit SHAs,
default-branch fallback, or tag creation.

Implementation: [Source references](../../3_Functions/pages/architecture/source-reference.md)
