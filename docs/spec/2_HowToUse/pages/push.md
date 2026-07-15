---
title: Pushing a managed skill
updated: 2026-07-13
status: implemented
---

# Push

Projectで改善したskillを、install時に指定したsource branchへ戻す。

```bash
gh linked-skills push <skill-name|project-relative-path>
```

条件: repositoryへのpush権限、source skillのtreeが未変更、validな`SKILL.md`、markerなし、全fileがtrackedまたはuntracked non-ignored。

同じbranchへ通常commitをpushする。force pushとPR作成はしない。Source skillのtreeが変わっていればpullを要求する。同じbranchの別pathだけが変わった場合は妨げない。

`status`の`eligible`は事前判定であり、branch protectionやrulesetによる最終拒否までは保証しない。

Source repositoryのskillだけをcommitする。親projectのcommit/pushは行わない。

read-only skillはpullのみ。local変更はstatusで警告する。

成功時の表示:

- `pushed <path> to <tree-sha>`
- `<path> has no source changes to push`

内部: [[docs/spec/3_Functions/pages/operations/push|Push spec]]
