---
title: Pulling a managed skill
updated: 2026-07-13
status: implemented
---

# Pull

Sourceの変更をlocalへ取り込む。Local変更があれば、前回の同期時点をbaseにmergeする。

```bash
gh linked-skills pull <skill-name|project-relative-path>
```

条件: manifest管理済み、local fileがGit indexへ登録済み、未解決conflictなし。

親project全体がcleanである必要はない。

- local変更なし: sourceへ更新
- local変更あり: three-way merge
- 同一snapshot: baselineのみ更新

## Conflictになった場合

Text conflictはfileへ比較内容を残す。解決方法は[[docs/spec/2_HowToUse/pages/resolve-conflicts|Conflict解決]]を参照。

Binary/構造conflictは変更せず停止する。

成功時の表示:

- `pulled <path> to <tree-sha>`
- `<path> is already up to date`

内部: [[docs/spec/3_Functions/pages/operations/pull|Pull spec]]
