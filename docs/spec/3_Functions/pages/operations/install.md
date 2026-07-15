---
title: Install reference
updated: 2026-07-13
status: implemented
---

# Install

Remote snapshotを初回配置し、manifestへ同期点を記録する。

## INST-001 Source resolution

`OWNER/REPO`形式のrepository、optional selector、source refを受け取る。Repositoryと`--branch` / `--tag`のどちらか1つは必須。Local sourceとrepository未指定installは扱わない。

- selectorなし: discovery結果を表示
- name: discovery結果から一致する1件を選択
- `namespace/name`: namespaceを含めて選択
- exact path: discoveryを省略
- `--all`: discovery結果を全件選択

Exact pathはAgent Skills形式のskill directory、または末尾`/SKILL.md`。File pathは親directoryへ正規化する。Repository rootの`SKILL.md`は対象外。

Source refを一度だけcommit SHAへ解決し、同じSHAのrecursive treeから候補を作る。結果はdisplay name順。Hidden directoryは除外する。

対象pathは[[docs/spec/2_HowToUse/pages/install-skill|SkillのInstall]]に定義する。Simple nameが複数pathに一致する場合はnamespace指定を要求する。

## INST-002 Snapshot install

直下の`SKILL.md`からname/descriptionを検証し、source directory名との一致を確認する。nameから`.agents/skills/<name>`を決める。description長はUnicode code point数で判定する。

Directory全体のraw byte、path、実行bitをstageし、target不存在時だけrenameする。contentへmetadataを追加しない。

## INST-003 Install collision and idempotency

拒否:

- 同名entryのsource/destination不一致
- entry一致、local snapshot不一致
- branch/tagの相互切り替え
- 同じsourceが別名で登録済み
- destination使用済み
- unmanaged destinationあり

Entry、remote、localが完全一致する再実行だけno-op。

Manifest失敗時はtargetを削除する。削除失敗もerrorへ含める。lockはない。

## INST-004 Tag repin

同じrepository、source path、destinationのtag skillは、別tagで再installできる。

1. 新tagを解決し、snapshotを取得・検証
2. 旧baselineとlocalの完全一致を確認
3. 新snapshotへ置換
4. manifestを更新

Local変更があれば拒否する。Mergeは行わない。同名tagの`refSHA`変更は`--accept-moved-tag`が必要。別tagにこのflagを指定した場合は拒否する。

同じtreeを指す別tagでもmanifestを更新する。`--all`では既存skillをrepinしない。

## INST-005 Bulk install

`--all`は1つのdiscovery commitを全snapshotのbaselineにする。

Mutation前に全件を取得し、document、name、source、destination、local snapshot、LFS、path containmentを検証する。Name衝突時は何もinstallしない。

検証後はdisplay name順にskill単位でinstallする。各skillのdirectoryとmanifest更新はINST-003と同じtransaction。途中失敗時に成功済みskillはrollbackしない。Resultは成功済みskillとerrorを返す。
