---
title: Installing from a tag
updated: 2026-07-13
status: implemented
---

# Tag指定のInstall

Release時点のskillを固定してinstallする。

```bash
gh linked-skills install OWNER/REPO --tag TAG
gh linked-skills install OWNER/REPO SKILL --tag TAG
gh linked-skills install OWNER/REPO PATH --tag TAG
gh linked-skills install OWNER/REPO --all --tag TAG
```

`--branch`と`--tag`はどちらか1つだけ必須。完全一致するtag名だけを扱う。

```bash
gh linked-skills install addyosmani/agent-skills \
  skills/code-review-and-quality \
  --tag 0.6.3
```

Tagは固定snapshotとして扱う。

| Operation | Tag指定skill |
| --- | --- |
| install | 指定tagの内容を配置 |
| status | local差分とtag identityを確認 |
| pull | 不可。`fixed_source_ref` |
| push | 不可。`source_ref_read_only` |

Local編集は可能だがpushできない。`status`はlocal変更を警告する。

## Tag変更

同じrepositoryとpathを別tagで再installする。

```bash
gh linked-skills install OWNER/REPO PATH --tag NEW_TAG
```

次の条件を満たす場合だけtagを変更する。

- 既存sourceもtag
- repository、path、destinationが同じ
- localが記録済みbaselineと一致
- 1 skillを明示。`--all`によるtag変更ではない

Mergeとlocal変更の破棄は行わない。同じtagとref SHAならno-op。Branchとの切り替えは拒否する。

Tag名が同じままref SHAが変わった場合は`tag_moved`として拒否する。受け入れる場合だけ明示する。

```bash
gh linked-skills install OWNER/REPO PATH \
  --tag TAG \
  --accept-moved-tag
```

成功時は変更前後のtagとref SHAを表示する。Tag削除や参照失敗は`source_unavailable`。

対象外: `latest`、semver範囲、commit SHA、default branch fallback、tag作成。

内部: [[docs/spec/3_Functions/pages/architecture/source-reference|Source reference]]
