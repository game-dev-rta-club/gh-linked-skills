---
title: Resolving conflicts manually
updated: 2026-07-13
status: implemented
---

# Conflict解決

Localとsourceが同じ行を別々に変更した場合、どちらを残すかはtoolでは決められない。

このとき`pull`は自動選択せず、local・前回同期・sourceの内容を実際のskill fileへ書き込む。

```text
CONFLICT (content): Merge conflict in .agents/skills/sample/SKILL.md
Pull completed with conflicts; fix them in the working tree.
After resolving, run:
  gh linked-skills status
If STATE is push, run:
  gh linked-skills push sample
```

これはGitのconflictと同じ考え方である。Exit code `1`はrollbackを意味せず、「working treeへmerge結果を書いたが、手動解決が残っている」ことを表す。

## Fileに書かれる内容

> ```text
> <<<<<<< gh-linked-skills:local
> title: Localで編集した文
> ||||||| gh-linked-skills:base:<tree-sha>
> title: 前回同期した文
> =======
> title: Sourceで更新された文
> >>>>>>> gh-linked-skills:remote:<tree-sha>
> ```

- `local`: project内で編集した内容
- `base`: 前回同期した内容
- `remote`: 現在のsourceにある内容

`<<<<<<<`、`|||||||`、`=======`、`>>>>>>>`で始まる区切り行をconflict markerと呼ぶ。

## 解決する

1. `<<<<<<< gh-linked-skills:local`をproject内で検索する
2. local、前回同期、sourceを比較する
3. 残したい内容へ書き直す
4. 4種類の区切り行と不要な内容を全て削除する
5. `gh linked-skills status`で確認する

たとえば最終的に次だけを残す。

```text
title: Localとsourceを踏まえた最終文
```

Sourceと同じ内容にすると`clean`、localまたは両方を組み合わせた内容を残すと`push`になる。`push`なら表示されたcommandでsourceへ戻す。再pullは不要。

Binary、modify/delete、file/directory conflictではfileを変更せず停止する。

内部: [[docs/spec/3_Functions/pages/operations/conflict-merge|Merge spec]]
