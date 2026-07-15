---
title: Synchronization model
updated: 2026-07-15
status: implemented
---

# Synchronization

A managed skill compares three snapshots: source, local, and baseline.

```text
source current
      ↓
baseline <- manifest
      ↑
local skill
```

- current: snapshot at the repository, path, and source ref
- local: snapshot at `.agents/skills/<name>`
- baseline: commit and tree SHA from the last synchronization

Status calculates a Git tree SHA from the local snapshot and compares it with
the baseline tree SHA. If the source-ref commit SHA equals the baseline, current
is also unchanged. Otherwise, status reads the source skill tree SHA. Local and
current differences produce `clean`, `push`, `pull`, or `conflict`.

Pull request proposal state is separate from these snapshots. GitHub and hidden
PR metadata identify a proposal; the manifest remains the synchronization
baseline only.

A snapshot contains relative paths, raw bytes, and executable state. YAML
meaning and timestamps are not compared.

Pull retrieves the baseline snapshot by tree SHA for a three-way merge. Status
does not retrieve the baseline snapshot.
