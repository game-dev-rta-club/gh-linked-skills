---
title: Publish reference
updated: 2026-07-14
status: implemented
---

# Publish

未管理local skillを既存repositoryへ追加し、remote成功後にmanifestへ同期点を記録する。

## PUB-001 Local preflight

`OWNER/REPO`、local selector、branchは必須。Selectorはnameまたは`.agents/skills/<name>`。

拒否:

- manifest登録済み
- project外path、symlink、非regular file
- nameとdirectory不一致、不正`SKILL.md`、marker、LFS pointer
- ignored local file
- repository read-only

Remote pathは`skills/<name>`。Repository作成、path指定、tag、relink、migration、copyは扱わない。

## PUB-002 Remote mutation

- branchあり、pathなし: subtreeを追加してnormal push
- branchあり、tree SHA一致: commitせず既存subtreeを採用
- branchあり、path不一致: 拒否
- repositoryにrefなし: 指定branchを最初のnormal pushで作成
- repositoryにrefあり、branchなし: 拒否

Bytes、relative path、実行bitをGit treeとして比較する。Ancestorがfile/symlinkの場合も不一致。

Clone後のnormal pushがnon-fast-forwardならremote changedとして拒否する。Unrelated contentを変更しない。

## PUB-003 Manifest registration

Remote成功後、開始時manifestと再読manifestが一致する場合だけentryを追加する。Repository、source path/ref、commit/tree SHA、destinationを1回で記録する。Process lockは持たない。

Manifest更新失敗後はremoteを戻さない。再実行すると一致済みremote subtreeを採用し、manifest登録だけを再試行できる。
