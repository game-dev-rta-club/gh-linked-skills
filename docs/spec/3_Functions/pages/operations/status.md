---
title: Status reference
updated: 2026-07-17
status: implemented
---

# Status

List visible Agent Skills, then calculate synchronization and operation
eligibility for the subset managed by Skill Linker.

## STAT-001 Managed inventory and states

For managed skills, base is manifest `treeSHA`, current is the skill tree SHA at
the source ref, and local is the Git tree SHA calculated from the destination.

Within one status call, batch all refs and push permissions. Read each
project-wide Git inventory once. There is no persistent cache. Local evaluation
does not download the baseline snapshot.

For a tag source, pull reason is `fixed_source_ref` and push reason is
`source_ref_read_only`. When remote `refSHA` differs from the baseline, pull
reason is `tag_moved`. Do not check permission for a tag.

| Local | Remote | State |
| --- | --- | --- |
| same | same | `clean` |
| changed | same | `push` |
| same | changed | `pull` |
| changed | changed | `conflict` |

When local already equals the changed current source tree, report `pull` so the
manifest baseline can advance.

If markers exist, return `conflict` without reading remote content. When current
commit SHA equals the baseline, do not read the repository tree. Otherwise,
read a tree once per repository and commit, then compare the current tree SHA.
Read independent remote requests concurrently with one shared limit of eight.
Do not compare semantic meaning.

## STAT-002 Proposal state

After permission preflight, list open pull requests once per branch repository
unless it is confirmed read-only. A read-only repository cannot push or update
a Skill Linker proposal. Unknown permission retains the lookup so an existing
proposal remains visible. Run independent PR and changed-tree reads
concurrently.

Classify each checked skill as no proposal, `waiting`, `update`,
`source_changed`, `obsolete`, `diverged`, or `ambiguous`. A lookup error
produces `unknown` without discarding file state.

An open proposal makes direct push ineligible with `open_proposal`. A lookup
error changes otherwise eligible direct push to `unknown (proposal_unknown)`.

## STAT-003 Eligibility and reason precedence

An invalid path, file, source, or marker ends evaluation early. Then validate
local frontmatter and Git inventory before calculating state from local/current
tree SHAs.

Validation failures and `source_unavailable` produce state `null`. Markers
produce state `conflict`. A Git or permission failure preserves any state that
was already calculated.

Even with invalid frontmatter or a managed-name mismatch, calculate pull state
and eligibility; only push receives `invalid_local_skill`. A changed current
skill with a mismatched name produces `source_unavailable`. Push reason order is
frontmatter, permission, `remote_changed`, then `ignored_files`. Never overwrite
an earlier non-eligible reason.

`eligible` means only that service preconditions pass. It does not include the
final result of GitHub branch protection or rulesets.

## STAT-004 Visible skill inventory

Run `gh skill list --agent codex --json` to list valid project and user skill
directories. Classify a row with source metadata as `gh-skill`, a `.system/`
row as `codex-system`, and any other direct row as `local`.

Run `codex plugin list --json` when Codex is available. Read only installed and
enabled plugin source paths, then read their current `plugin.json` and declared
skills directory. Do not scan caches or the historical gh skill lock file.
Codex absence is allowed. Other collector failures produce warnings.

These providers report only `present` or `enabled`. Do not query remote update
availability.

## STAT-005 Path identity and merge

Use normalized absolute paths as identity. Render project paths relatively and
include both `path` and exact `absolutePath` in JSON. A direct `local` row confirms a managed path and is
deduplicated. Different non-local provider claims for one path produce
`provider-conflict` and a warning.

Managed rows retain existing JSON fields and operation eligibility. External
rows leave proposal, pull, and push values unavailable.
