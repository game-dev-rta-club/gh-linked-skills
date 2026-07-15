---
title: Linked Skills 手動競合解決
updated: 2026-07-12
status: implemented
---

# 手動競合解決

> [!SUMMARY]
> 自動で意味を選ばず、pullが書いたGit diff3 markerを利用者が通常fileとして編集する。

## Flow

```text
pull
  → text conflictをfileへ書く
  → manifest baselineをremote headへ進める
  → 利用者がmarkerを解消する
  → statusがpushを示す
  → push
```

markerは次の形式とする。

```text
 <<<<<<< gh-linked-skills:local
 local content
 ||||||| gh-linked-skills:base:<tree-sha>
 base content
 =======
 remote content
 >>>>>>> gh-linked-skills:remote:<tree-sha>
```

利用者はmarker行と不要な内容を削除し、採用する最終内容だけを残す。markerが一つでも残る間、`status`は`conflict`、pull / pushは`unresolved_conflict`で不可とする。専用indexやconflict状態fileは持たない。

binary conflict、modify/delete、file/directory collisionはmarkerへ変換せず、localを変更しない。SKILL frontmatter内のtext conflictも同じmarkerとして書き、解消後のpush時にCodex互換を再検証する。

## Related

- [[gh-linked-skills|概要]]
- [[gh-linked-skills-functions|機能一覧]]
