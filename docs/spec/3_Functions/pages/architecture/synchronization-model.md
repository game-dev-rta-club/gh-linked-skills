---
title: Synchronization model
updated: 2026-07-14
status: implemented
---

# Synchronization

Managed skillはsource、local、baselineの3つを比較する。

```text
source current
      ↓
baseline ← manifest
      ↑
local skill
```

- current: repository/path/source refのsnapshot
- local: `.agents/skills/<name>`のsnapshot
- baseline: 最終同期commit/tree SHA

Statusはlocal snapshotからGit tree SHAを計算し、baseline tree SHAと比較する。Source refのcommit SHAがbaselineと同じならcurrentも同一とする。異なる場合だけsourceのskill tree SHAを読む。Local/current両方の差分から`clean`、`push`、`pull`、`conflict`を算出する。

snapshotはrelative path、raw byte、実行可否の組。YAML意味とtimestampは比較しない。

Pullは3-way mergeのため、baseline snapshotをtree SHAから取得する。Statusはbaseline snapshotを取得しない。
