---
title: Conflict merge reference
updated: 2026-07-13
status: implemented
---

# Conflict merge

Base/local/remoteをfile単位でmergeする。手動手順は[[docs/spec/2_HowToUse/pages/resolve-conflicts|HowTo]]を参照する。

## MERGE-001 Per-file three-way merge

Path unionをsortし、存在/raw byte/実行bitをmergeする。

- 片側追加: 採用
- 両側同byte: 採用しmode merge
- 片側だけbase同一: 反対側を採用
- content変更 + 反対側mode変更: 両方採用
- 両側削除: 削除
- delete + 反対側変更: modify/delete error

`SKILL.md`消失とfile/directory collisionはerror。

## MERGE-002 Text and binary

両側変更でNULありならbinary error。Workspaceは変更しない。textは次を使う。

```bash
git merge-file --diff3 \
  -L gh-linked-skills:local \
  -L gh-linked-skills:base:<base-tree-sha> \
  -L gh-linked-skills:remote:<remote-tree-sha>
```

Overlapは個数にかかわらずmarkerを残す。Frontmatterも同じ。Inputはtemporary file `0600`。encoding/newline変換とapplication独自のsize limitはない。

`git merge-file`の終了値1〜127は競合数として扱う。詳細: [git-merge-file](https://git-scm.com/docs/git-merge-file)

## MERGE-003 Executable mode

- local=base: remote
- remote=base: local
- 両側変更: OR

Mode変更 + deleteはerror。

## MERGE-004 Manual resolution state

次のsubstringでunresolved判定する。

- `<<<<<<< gh-linked-skills:local`
- `||||||| gh-linked-skills:base:`
- `>>>>>>> gh-linked-skills:remote:`

Marker中はstate=`conflict`、pull/push=`unresolved_conflict`。別state fileはない。解消後、remoteと異なれば`push`。
