---
title: Source reference
updated: 2026-07-13
status: implemented
---

# Source reference

Branchとtagをfull Git refで保持する。

| Kind | Full ref | Pull | Push |
| --- | --- | --- | --- |
| branch | `refs/heads/<name>` | yes | 権限があればyes |
| tag | `refs/tags/<name>` | no | no |

Installは`--branch`または`--tag`を1つだけ受け取る。Commit SHA直接指定は扱わない。

## Resolution

1. GitHub Git refs APIでexact refを取得
2. annotated tagをGit tags APIで最終objectまでpeel
3. 最終objectがcommitでなければ拒否
4. 同じcommitからdiscoveryとsnapshotを取得

保持するSHA:

- `refSHA`: branch/lightweight tagのcommit、またはannotated tag object
- `commitSHA`: peel後のcommit
- `treeSHA`: skill subtree

Tagの`refSHA`が変化した場合は、contentが同じでも`tag_moved`。自動適用しない。

## Operation rules

- install: refを一度解決し、同じcommitから全snapshotを取得
- status: tagのlocal/base/currentとref identityを確認
- pull: tagならremote read前に`fixed_source_ref`
- push: tagならpermission確認前に`source_ref_read_only`
- repin: 同じsourceのtagを`install`で再指定。local clean時だけ更新

Repinは取得と検証を終えてからskillを置換し、manifestをatomic writeする。失敗時は元のskillへ戻す。Process停止による不一致は`status`で検出する。

## Compatibility

Schema v1の`branch`は読取時に`refs/heads/<branch>`へ変換し、`refSHA=commitSHA`とする。Read-only commandではfileを書き換えない。次の成功したmanifest変更でv2を保存する。

対象外:

- `latest`やsemver range
- branch/tagの自動選択と相互切り替え
- default branch fallback
- commit SHA指定
- tag作成、移動、削除、push
