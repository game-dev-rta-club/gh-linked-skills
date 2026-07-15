---
title: Publishing an unmanaged skill
updated: 2026-07-14
status: implemented
---

# Publish

Projectで作った未管理skillを、既存GitHub repositoryへ初回公開して管理を始める。

```bash
gh linked-skills publish OWNER/REPO SKILL --branch BRANCH
```

前提:

- localは`.agents/skills/<name>`
- repositoryはGitHub上で作成済み
- repositoryへのpush権限がある
- `BRANCH`を明示する
- skillはmanifestに未登録

Remote pathは`skills/<name>`。空repositoryでは指定branchを最初のcommitで作る。非空repositoryではbranchが必要。

Remote pathが存在しなければcommitしてnormal pushする。内容が完全一致すればpushせずmanifestへ登録する。異なる内容は上書きしない。

管理済みskillはpublishできない。同じsourceの更新は`push`を使う。別repositoryへの移行、複製、repository作成は扱わない。

成功後は`.gh-linked-skills.json`も親projectへcommitする。

内部: [[docs/spec/3_Functions/pages/operations/publish|Publish spec]]
